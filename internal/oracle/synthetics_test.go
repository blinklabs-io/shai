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
	"testing"

	"github.com/blinklabs-io/adder/event"
	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/blinklabs-io/shai/internal/config"
)

func TestSyntheticsOracleProcessesButaneDatum(t *testing.T) {
	const addr = "addr1w9qx9rs39dztl3ugtq2s588f2jw25jluq95hvfqzqp84wxgytkmex"
	profile := &config.Profile{
		Name: "butane",
		Config: config.SyntheticsProfileConfig{
			Protocol: "butane",
			CDPAddresses: []config.ProfileConfigAddress{
				{Address: addr},
			},
		},
	}
	o := NewSynthetics(nil, profile, NewButaneParser())
	updates := o.Subscribe()
	defer o.Unsubscribe(updates)

	pubKeyHash := make([]byte, 28)
	owner := cbor.NewConstructorEncoder(0, cbor.IndefLengthList{
		pubKeyHash,
	})
	synthetic := cbor.NewConstructorEncoder(0, cbor.IndefLengthList{
		[]byte{0x12, 0x34},
		[]byte("bBTC"),
	})
	datum := cbor.NewConstructorEncoder(1, cbor.IndefLengthList{
		owner,
		synthetic,
		uint64(50000000),
		int64(1704067200123),
	})
	datumCbor, err := cbor.Encode(&datum)
	if err != nil {
		t.Fatalf("failed to encode datum: %v", err)
	}
	txHash := "abc123def456789012345678901234567890"
	txIndex := uint32(3)

	err = o.handleTransaction(
		event.Event{
			Context: event.TransactionContext{
				TransactionHash: txHash,
				SlotNumber:      12345,
			},
		},
		event.TransactionEvent{
			BlockHash: "block123",
			Transaction: &testSyntheticsTransaction{
				produced: []common.Utxo{
					{
						Id: shelley.NewShelleyTransactionInput(
							"0000000000000000000000000000000000000000000000000000000000000000",
							int(txIndex),
						),
						Output: newTestSyntheticsOutput(t, addr, datumCbor),
					},
				},
			},
		},
	)
	if err != nil {
		t.Fatalf("handleTransaction returned error: %v", err)
	}

	key := "butane:butane_cdp_abc123def456789012345678901234567890#3"
	if _, ok := o.GetState(key); !ok {
		t.Fatalf("expected Butane state with key %s", key)
	}

	select {
	case update := <-updates:
		if update.StateKey != key {
			t.Fatalf("expected update key %s, got %s", key, update.StateKey)
		}
	default:
		t.Fatal("expected synthetics update")
	}
}

func newTestSyntheticsOutput(
	t *testing.T,
	addr string,
	datumCbor []byte,
) common.TransactionOutput {
	t.Helper()
	outputAddr, err := common.NewAddress(addr)
	if err != nil {
		t.Fatalf("failed to parse address: %v", err)
	}
	addrBytes, err := outputAddr.Bytes()
	if err != nil {
		t.Fatalf("failed to encode address: %v", err)
	}
	outputCbor, err := cbor.Encode(map[uint64]any{
		0: addrBytes,
		1: uint64(1000000),
		2: []any{
			uint64(1),
			cbor.Tag{Number: 24, Content: datumCbor},
		},
	})
	if err != nil {
		t.Fatalf("failed to encode output: %v", err)
	}
	output, err := babbage.NewBabbageTransactionOutputFromCbor(outputCbor)
	if err != nil {
		t.Fatalf("failed to decode output: %v", err)
	}
	return output
}

type testSyntheticsTransaction struct {
	common.TransactionBodyBase
	produced []common.Utxo
}

func (t testSyntheticsTransaction) Type() int                    { return 0 }
func (t testSyntheticsTransaction) Hash() common.Blake2b256      { return common.Blake2b256{} }
func (t testSyntheticsTransaction) LeiosHash() common.Blake2b256 { return common.Blake2b256{} }
func (t testSyntheticsTransaction) Metadata() common.TransactionMetadatum {
	return nil
}
func (t testSyntheticsTransaction) AuxiliaryData() common.AuxiliaryData {
	return nil
}
func (t testSyntheticsTransaction) IsValid() bool { return true }
func (t testSyntheticsTransaction) Consumed() []common.TransactionInput {
	return nil
}
func (t testSyntheticsTransaction) Produced() []common.Utxo {
	return t.produced
}
func (t testSyntheticsTransaction) Witnesses() common.TransactionWitnessSet {
	return nil
}
func (t testSyntheticsTransaction) ProtocolParameterUpdates() (
	uint64,
	map[common.Blake2b224]common.ProtocolParameterUpdate,
) {
	return 0, nil
}
