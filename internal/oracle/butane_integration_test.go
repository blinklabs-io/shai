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

func TestMainnetButaneProfileTracksPersistsAndSpendsCDP(t *testing.T) {
	profile := config.Profiles["mainnet"]["butane"]
	synthCfg := profile.Config.(config.SyntheticsProfileConfig)
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
				newTestButaneCDPOutput(t, synthCfg.CDPAddresses[0].Address),
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
	if state.IAsset != "1234.62425443" {
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
	address, err := common.NewAddress(addr)
	if err != nil {
		t.Fatalf("failed to parse Butane CDP address: %v", err)
	}
	datumCbor := testButaneCDPDatum(t, 50_000_000)
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
	})
	synthetic := cbor.NewConstructorEncoder(0, cbor.IndefLengthList{
		[]byte{0x12, 0x34},
		[]byte("bBTC"),
	})
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
