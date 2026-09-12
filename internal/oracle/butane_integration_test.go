// Copyright 2026 Blink Labs Software
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package oracle

import (
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/blinklabs-io/adder/event"
	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/blinklabs-io/shai/dex/butane"
	"github.com/blinklabs-io/shai/internal/config"
)

const (
	liveMainnetButaneCDPAddress = "addr1z94xwev8steqxcxvfn044qy2hymrh04wktu8w0f85263f6ulzvhpxn9c4g2fyxe5rlmn6z5qmm3dtjqfjn2vvy58l88s4tzemw"
	liveMainnetButaneCDPDatum   = "d87a9fd8799f581c3258f32901c7ac8acfb0815ac78515d7e27f949e7ec71f23ee1aa7bc40ff454d494441531b00000003383ef9821b0000019c07cd50b0ff"
)

func TestMainnetButaneProfileTracksPersistsAndSpendsCDP(t *testing.T) {
	profile := config.Profiles["mainnet"]["butane"]
	o := New(nil, &profile, NewButaneParser())
	o.storage = newTestOracleStorage(t)

	oldHash := strings.Repeat("a", 64)
	oldID := butane.GenerateCDPId(oldHash, 0)
	err := o.handleTransaction(
		event.Event{Context: event.TransactionContext{
			TransactionHash: oldHash,
			SlotNumber:      profile.InterceptSlot,
		}},
		event.TransactionEvent{
			BlockHash: "butane-block",
			Outputs: []ledger.TransactionOutput{
				newTestButaneCDPOutput(t, liveMainnetButaneCDPAddress),
			},
		},
	)
	if err != nil {
		t.Fatalf("handleTransaction returned error: %v", err)
	}

	state, ok := o.GetCDPState(oldID)
	if !ok || state == nil {
		t.Fatalf("expected Butane CDP %s to be tracked", oldID)
	}
	if state.Protocol != "butane" {
		t.Fatalf("expected Butane protocol, got %q", state.Protocol)
	}
	if state.MintedAmount != 50_000_000 {
		t.Fatalf("expected minted amount 50000000, got %d", state.MintedAmount)
	}
	if !state.HasOwner || len(state.Owner) != 56 {
		t.Fatalf("expected a 28-byte owner hash, got %q", state.Owner)
	}
	if state.IAsset != "00000000000410c2d9e01e8ec78ab1dc6bbc383fae76cbe2689beb02.62425443" {
		t.Fatalf("expected bBTC asset fingerprint, got %q", state.IAsset)
	}
	if _, err := o.storage.LoadCDPState("mainnet", "butane", oldID); err != nil {
		t.Fatalf("expected Butane CDP in storage: %v", err)
	}

	err = o.handleTransaction(
		event.Event{Context: event.TransactionContext{
			TransactionHash: strings.Repeat("b", 64),
			SlotNumber:      profile.InterceptSlot + 1,
		}},
		event.TransactionEvent{Inputs: []ledger.TransactionInput{
			shelley.NewShelleyTransactionInput(oldHash, 0),
		}},
	)
	if err != nil {
		t.Fatalf("handleTransaction for spend returned error: %v", err)
	}
	if _, ok := o.GetCDPState(oldID); ok {
		t.Fatalf("expected spent Butane CDP %s to be removed", oldID)
	}
	if _, err := o.storage.LoadCDPState("mainnet", "butane", oldID); err == nil {
		t.Fatalf("expected spent Butane CDP %s to be deleted from storage", oldID)
	}
}

func TestMainnetButaneProfileMatchesLiveStakedCDPAddress(t *testing.T) {
	profile := config.Profiles["mainnet"]["butane"]
	o := New(nil, &profile, NewButaneParser())
	o.storage = newTestOracleStorage(t)
	if o.isPoolAddress("addr1w9qx9rs39dztl3ugtq2s588f2jw25jluq95hvfqzqp84wxgytkmex") {
		t.Fatal("expected synthetics.validate reward script hash not to match CDPs")
	}

	datum, err := hex.DecodeString(liveMainnetButaneCDPDatum)
	if err != nil {
		t.Fatalf("failed to decode live datum fixture: %v", err)
	}
	txHash := "89fe03b8fc7cc9e4fae4de3fc7ee3b3be7326d0092b51d1b43902cf8ea479a5e"
	err = o.handleTransaction(
		event.Event{Context: event.TransactionContext{
			TransactionHash: txHash,
			SlotNumber:      196_882_173,
		}},
		event.TransactionEvent{
			BlockHash: "f75583ac139cb87d7ed157f6e547f85e3a758a2baa86ea72eda89624900d4e49",
			Outputs: []ledger.TransactionOutput{
				newTestButaneOutput(t, liveMainnetButaneCDPAddress, datum),
			},
		},
	)
	if err != nil {
		t.Fatalf("handleTransaction returned error: %v", err)
	}
	state, ok := o.GetCDPState(butane.GenerateCDPId(txHash, 0))
	if !ok || state == nil {
		t.Fatal("expected live staked Butane CDP address to be monitored")
	}
	if state.MintedAmount != 13_828_553_090 {
		t.Fatalf("unexpected minted amount: %d", state.MintedAmount)
	}
}

func TestButaneRollbackRestoresSpentCDPAcrossRestart(t *testing.T) {
	profile := config.Profiles["mainnet"]["butane"]
	storage := newTestOracleStorage(t)
	o := New(nil, &profile, NewButaneParser())
	o.storage = storage

	oldHash := strings.Repeat("d", 64)
	oldID := butane.GenerateCDPId(oldHash, 0)
	oldState := &CDPState{
		CDPId:        oldID,
		Network:      "mainnet",
		Protocol:     "butane",
		Owner:        strings.Repeat("1", 56),
		HasOwner:     true,
		IAsset:       "00000000000410c2d9e01e8ec78ab1dc6bbc383fae76cbe2689beb02.4d49444153",
		MintedAmount: 5_000_000,
		Slot:         profile.InterceptSlot,
		BlockHash:    profile.InterceptHash,
		TxHash:       oldHash,
		TxIndex:      0,
	}
	o.cdps[oldID] = oldState
	if err := storage.SaveCDPState(oldState); err != nil {
		t.Fatalf("failed to seed old CDP: %v", err)
	}

	spendSlot := profile.InterceptSlot + 1
	newHash := strings.Repeat("e", 64)
	spendEvent := event.Event{Context: event.TransactionContext{
		TransactionHash: newHash,
		SlotNumber:      spendSlot,
	}}
	spendTransaction := event.TransactionEvent{
		BlockHash: strings.Repeat("f", 64),
		Inputs: []ledger.TransactionInput{
			shelley.NewShelleyTransactionInput(oldHash, 0),
		},
		Outputs: []ledger.TransactionOutput{
			newTestButaneCDPOutput(t, liveMainnetButaneCDPAddress),
		},
	}
	if err := o.handleTransaction(
		spendEvent,
		spendTransaction,
	); err != nil {
		t.Fatalf("failed to spend old CDP: %v", err)
	}
	// A duplicate chain event must not overwrite the predecessor snapshot in
	// the durable undo record.
	if err := o.handleTransaction(spendEvent, spendTransaction); err != nil {
		t.Fatalf("failed to process duplicate CDP transition: %v", err)
	}
	if _, ok := o.GetCDPState(oldID); ok {
		t.Fatal("expected spent CDP to be absent before rollback")
	}
	newID := butane.GenerateCDPId(newHash, 0)
	if _, ok := o.GetCDPState(newID); !ok {
		t.Fatal("expected replacement CDP before rollback")
	}

	restarted := New(nil, &profile, NewButaneParser())
	restarted.storage = storage
	if err := restarted.loadPersistedStates(); err != nil {
		t.Fatalf("failed to load state after spend: %v", err)
	}
	if _, ok := restarted.GetCDPState(newID); !ok {
		t.Fatal("expected replacement CDP after restart")
	}
	if err := restarted.handleRollback(event.RollbackEvent{
		SlotNumber: profile.InterceptSlot,
		BlockHash:  profile.InterceptHash,
	}); err != nil {
		t.Fatalf("failed to roll back spend: %v", err)
	}

	restored, ok := restarted.GetCDPState(oldID)
	if !ok || restored == nil {
		t.Fatal("expected rollback to restore the spent CDP")
	}
	if restored.MintedAmount != oldState.MintedAmount {
		t.Fatalf(
			"unexpected restored minted amount: got %d want %d",
			restored.MintedAmount,
			oldState.MintedAmount,
		)
	}
	if _, err := storage.LoadCDPState("mainnet", "butane", oldID); err != nil {
		t.Fatalf("expected restored CDP to be durable: %v", err)
	}
	if _, ok := restarted.GetCDPState(newID); ok {
		t.Fatal("expected rollback to remove the replacement CDP")
	}
	if _, err := storage.LoadCDPState("mainnet", "butane", newID); err == nil {
		t.Fatal("expected rolled-back replacement CDP to be absent from storage")
	}
}

func TestButaneRollbackReversesSameSlotTransitionsInBlockOrder(t *testing.T) {
	profile := config.Profiles["mainnet"]["butane"]
	storage := newTestOracleStorage(t)
	o := New(nil, &profile, NewButaneParser())
	o.storage = storage

	seedHash := strings.Repeat("a", 64)
	seedID := butane.GenerateCDPId(seedHash, 0)
	seedState := &CDPState{
		CDPId:     seedID,
		Network:   "mainnet",
		Protocol:  "butane",
		Slot:      profile.InterceptSlot,
		TxHash:    seedHash,
		BlockHash: profile.InterceptHash,
	}
	o.cdps[seedID] = seedState
	if err := storage.SaveCDPState(seedState); err != nil {
		t.Fatalf("failed to seed CDP: %v", err)
	}

	transitionSlot := profile.InterceptSlot + 1
	// Deliberately order the hashes opposite their transaction indexes. A
	// rollback ordered by hash would leave the intermediate state restored.
	firstHash := strings.Repeat("f", 64)
	secondHash := strings.Repeat("e", 64)
	transitions := []struct {
		hash      string
		index     uint32
		spentHash string
	}{
		{hash: firstHash, index: 0, spentHash: seedHash},
		{hash: secondHash, index: 1, spentHash: firstHash},
	}
	for _, transition := range transitions {
		txEvent := event.Event{Context: event.TransactionContext{
			TransactionHash: transition.hash,
			SlotNumber:      transitionSlot,
			TransactionIdx:  transition.index,
		}}
		tx := event.TransactionEvent{
			BlockHash: strings.Repeat("b", 64),
			Inputs: []ledger.TransactionInput{
				shelley.NewShelleyTransactionInput(transition.spentHash, 0),
			},
			Outputs: []ledger.TransactionOutput{
				newTestButaneCDPOutput(t, liveMainnetButaneCDPAddress),
			},
		}
		err := o.handleTransaction(txEvent, tx)
		if err != nil {
			t.Fatalf("failed to apply transaction %d: %v", transition.index, err)
		}
		if err := o.handleTransaction(txEvent, tx); err != nil {
			t.Fatalf(
				"failed to apply duplicate transaction %d: %v",
				transition.index,
				err,
			)
		}
	}

	restarted := New(nil, &profile, NewButaneParser())
	restarted.storage = storage
	if err := restarted.loadPersistedStates(); err != nil {
		t.Fatalf("failed to load dependent transition after restart: %v", err)
	}
	if err := restarted.handleRollback(event.RollbackEvent{
		SlotNumber: profile.InterceptSlot,
		BlockHash:  profile.InterceptHash,
	}); err != nil {
		t.Fatalf("failed to roll back same-slot transitions: %v", err)
	}
	if _, ok := restarted.GetCDPState(seedID); !ok {
		t.Fatal("expected rollback to restore the seed CDP")
	}
	for _, hash := range []string{firstHash, secondHash} {
		cdpID := butane.GenerateCDPId(hash, 0)
		if _, ok := restarted.GetCDPState(cdpID); ok {
			t.Fatalf("expected rollback to remove transition state %s", hash)
		}
		if _, err := storage.LoadCDPState("mainnet", "butane", cdpID); err == nil {
			t.Fatalf("expected durable transition state %s to be removed", hash)
		}
	}
}

func TestButaneParserRejectsMintedAmountOverflow(t *testing.T) {
	parser := NewButaneParser()
	_, err := parser.ParseCDPDatum(
		testButaneCDPDatum(t, ^uint64(0)),
		strings.Repeat("c", 64),
		0,
		1,
		time.Unix(0, 0),
	)
	if err == nil {
		t.Fatal("expected out-of-range minted amount to fail")
	}
}

func TestButaneOracleLoadsOnlyButaneCDPs(t *testing.T) {
	storage := newTestOracleStorage(t)
	if err := storage.SavePoolState(&PoolState{
		PoolId:   "minswap-pool",
		Network:  "mainnet",
		Protocol: "minswap-v2",
	}); err != nil {
		t.Fatalf("failed to seed Minswap pool state: %v", err)
	}
	for _, state := range []*CDPState{
		{
			CDPId:    "butane-cdp",
			Network:  "mainnet",
			Protocol: "butane",
		},
		{
			CDPId:    "indigo-cdp",
			Network:  "mainnet",
			Protocol: "indigo",
		},
	} {
		if err := storage.SaveCDPState(state); err != nil {
			t.Fatalf("failed to seed %s state: %v", state.Protocol, err)
		}
	}

	profile := config.Profiles["mainnet"]["butane"]
	o := New(nil, &profile, NewButaneParser())
	o.storage = storage
	if err := o.loadPersistedStates(); err != nil {
		t.Fatalf("failed to load persisted states: %v", err)
	}

	if o.CDPCount() != 1 {
		t.Fatalf("expected one Butane CDP, got %d", o.CDPCount())
	}
	if _, ok := o.GetCDPState("butane-cdp"); !ok {
		t.Fatal("expected persisted Butane CDP")
	}
	if _, ok := o.GetCDPState("indigo-cdp"); ok {
		t.Fatal("expected persisted Indigo CDP to be excluded")
	}
	if o.PoolCount() != 0 {
		t.Fatalf("expected no AMM pools in Butane oracle, got %d", o.PoolCount())
	}
}

func newTestButaneCDPOutput(t *testing.T, addr string) ledger.TransactionOutput {
	t.Helper()
	return newTestButaneOutput(t, addr, testButaneCDPDatum(t, 50_000_000))
}

func newTestButaneOutput(
	t *testing.T,
	addr string,
	datumCbor []byte,
) ledger.TransactionOutput {
	t.Helper()
	address, err := common.NewAddress(addr)
	if err != nil {
		t.Fatalf("failed to parse Butane CDP address: %v", err)
	}
	outputCbor, err := cbor.Encode(&map[uint64]any{
		0: address,
		1: uint64(2_000_000),
		2: []any{
			uint64(1),
			cbor.Tag{Number: 24, Content: datumCbor},
		},
	})
	if err != nil {
		t.Fatalf("failed to encode Butane CDP output: %v", err)
	}
	output, err := ledger.NewTransactionOutputFromCbor(outputCbor)
	if err != nil {
		t.Fatalf("failed to decode Butane CDP output: %v", err)
	}
	return output
}

func testButaneCDPDatum(t *testing.T, minted uint64) []byte {
	t.Helper()
	owner := cbor.NewConstructorEncoder(0, cbor.IndefLengthList{
		make([]byte, 28),
		[]byte{},
	})
	synthetic := []byte("bBTC")
	datum := cbor.NewConstructorEncoder(1, cbor.IndefLengthList{
		owner,
		synthetic,
		minted,
		int64(1_704_067_200_123),
	})
	datumCbor, err := cbor.Encode(&datum)
	if err != nil {
		t.Fatalf("failed to encode Butane datum: %v", err)
	}
	return datumCbor
}
