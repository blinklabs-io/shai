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
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/blinklabs-io/adder/event"
	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/shai/dex/liqwid"
	"github.com/blinklabs-io/shai/internal/config"
	"github.com/blinklabs-io/shai/internal/indexer"
	"github.com/dgraph-io/badger/v4"
)

func TestLendingOracleHandleTransactionUsesOutputsWhenTransactionNil(
	t *testing.T,
) {
	parser := testLendingParser{protocol: "test-lending"}
	o := &LendingOracle{
		parser:    parser,
		states:    make(map[string]*LendingState),
		stopChan:  make(chan struct{}),
		addresses: []string{liqwid.Charli3OracleAddress},
	}

	txHash := strings.Repeat("ab", 32)
	err := o.HandleChainsyncEvent(event.Event{
		Context: event.TransactionContext{
			TransactionHash: txHash,
			SlotNumber:      42,
		},
		Payload: event.TransactionEvent{
			Outputs: []ledger.TransactionOutput{
				testLendingOutput(t, liqwid.Charli3OracleAddress),
			},
		},
	})
	if err != nil {
		t.Fatalf("HandleChainsyncEvent returned error: %v", err)
	}

	state, ok := o.GetState(
		fmt.Sprintf("%s:%s:state-0", config.GetConfig().Network, parser.Protocol()),
	)
	if !ok {
		t.Fatal("expected output-only transaction event to update lending state")
	}
	if state.TxHash != txHash || state.TxIndex != 0 || state.Slot != 42 {
		t.Fatalf(
			"unexpected transaction metadata: txHash=%s txIndex=%d slot=%d",
			state.TxHash,
			state.TxIndex,
			state.Slot,
		)
	}
}

func TestLendingOracleStoppedIgnoresChainsyncEvents(t *testing.T) {
	o := &LendingOracle{
		parser:    testLendingParser{protocol: "test-lending"},
		states:    make(map[string]*LendingState),
		stopChan:  make(chan struct{}),
		addresses: []string{liqwid.Charli3OracleAddress},
	}
	o.Stop()

	err := o.HandleChainsyncEvent(event.Event{
		Context: event.TransactionContext{
			TransactionHash: strings.Repeat("cd", 32),
			SlotNumber:      42,
		},
		Payload: event.TransactionEvent{
			Outputs: []ledger.TransactionOutput{
				testLendingOutput(t, liqwid.Charli3OracleAddress),
			},
		},
	})
	if err != nil {
		t.Fatalf("HandleChainsyncEvent returned error: %v", err)
	}
	if got := o.StateCount(); got != 0 {
		t.Fatalf("expected stopped oracle to ignore event, got %d states", got)
	}
}

func TestLendingOracleRollbackDeletesRolledBackStates(t *testing.T) {
	storage := newTestLendingStorage(t)
	states := []*LendingState{
		{
			StateId:   "before",
			StateType: LendingStateTypeMarket,
			Network:   "mainnet",
			Protocol:  "liqwid",
			Slot:      99,
		},
		{
			StateId:   "at",
			StateType: LendingStateTypeMarket,
			Network:   "mainnet",
			Protocol:  "liqwid",
			Slot:      100,
		},
		{
			StateId:   "after",
			StateType: LendingStateTypeMarket,
			Network:   "mainnet",
			Protocol:  "liqwid",
			Slot:      101,
		},
	}
	stateMap := make(map[string]*LendingState, len(states))
	for _, state := range states {
		stateMap[state.Key()] = state
		if err := storage.SaveLendingState(state); err != nil {
			t.Fatalf("failed to save lending state %s: %v", state.StateId, err)
		}
	}
	o := &LendingOracle{
		states:   stateMap,
		storage:  storage,
		stopChan: make(chan struct{}),
	}

	if err := o.handleRollback(event.RollbackEvent{SlotNumber: 100}); err != nil {
		t.Fatalf("handleRollback returned error: %v", err)
	}

	if _, ok := o.GetState("mainnet:liqwid:before"); !ok {
		t.Fatal("expected pre-rollback state to remain in memory")
	}
	if _, ok := o.GetState("mainnet:liqwid:at"); ok {
		t.Fatal("expected state at rollback slot to be removed from memory")
	}
	if _, ok := o.GetState("mainnet:liqwid:after"); ok {
		t.Fatal("expected state after rollback slot to be removed from memory")
	}

	persisted, err := storage.LoadAllLendingStates()
	if err != nil {
		t.Fatalf("failed to load persisted lending states: %v", err)
	}
	if len(persisted) != 1 || persisted[0].StateId != "before" {
		t.Fatalf("expected only pre-rollback state in storage, got %#v", persisted)
	}
}

func TestLendingOracleLoadPersistedStatesFiltersByProtocol(t *testing.T) {
	storage := newTestLendingStorage(t)
	for _, state := range []*LendingState{
		{
			StateId:   "liqwid-market",
			StateType: LendingStateTypeMarket,
			Network:   config.GetConfig().Network,
			Protocol:  "liqwid",
		},
		{
			StateId:   "other-market",
			StateType: LendingStateTypeMarket,
			Network:   config.GetConfig().Network,
			Protocol:  "other-lending",
		},
	} {
		if err := storage.SaveLendingState(state); err != nil {
			t.Fatalf("failed to save lending state %s: %v", state.StateId, err)
		}
	}

	o := &LendingOracle{
		parser:   testLendingParser{protocol: "liqwid"},
		states:   make(map[string]*LendingState),
		storage:  storage,
		stopChan: make(chan struct{}),
	}
	if err := o.loadPersistedStates(); err != nil {
		t.Fatalf("loadPersistedStates returned error: %v", err)
	}

	if got := o.StateCount(); got != 1 {
		t.Fatalf("expected one protocol-scoped state, got %d", got)
	}
	if _, ok := o.GetState("liqwid-market"); !ok {
		t.Fatal("expected Liqwid state to be loaded")
	}
	if _, ok := o.GetState("other-market"); ok {
		t.Fatal("expected other protocol state to be filtered out")
	}
}

func TestLendingOracleStartRequiresInjectedStorage(t *testing.T) {
	o := NewLendingOracle(
		indexer.New(),
		&config.Profile{
			Name: "lending",
			Type: config.ProfileTypeLending,
			Config: config.LendingProfileConfig{
				Protocol: "liqwid",
			},
		},
		testLendingParser{protocol: "liqwid"},
		nil,
	)
	if err := o.Start(); err == nil {
		t.Fatal("expected Start to require caller-owned lending storage")
	}
}

func newTestLendingStorage(t *testing.T) *LendingStorage {
	t.Helper()

	dir := filepath.Join(t.TempDir(), "lending")
	opts := badger.DefaultOptions(dir).WithLoggingLevel(badger.WARNING)
	db, err := badger.Open(opts)
	if err != nil {
		t.Fatalf("failed to open test badger DB: %v", err)
	}

	storage := &LendingStorage{db: db}
	t.Cleanup(func() {
		if err := storage.Close(); err != nil {
			t.Fatalf("failed to close test badger DB: %v", err)
		}
	})
	return storage
}

func testLendingOutput(
	t *testing.T,
	addressString string,
) ledger.TransactionOutput {
	t.Helper()
	address, err := lcommon.NewAddress(addressString)
	if err != nil {
		t.Fatalf("failed to parse lending test address: %v", err)
	}
	datum, err := cbor.Encode(uint64(1))
	if err != nil {
		t.Fatalf("failed to encode lending test datum: %v", err)
	}
	outputCbor, err := cbor.Encode(&map[uint64]any{
		0: address,
		1: uint64(2_000_000),
		2: []any{
			uint64(1),
			cbor.Tag{
				Number:  24,
				Content: datum,
			},
		},
	})
	if err != nil {
		t.Fatalf("failed to encode lending test output: %v", err)
	}
	output, err := ledger.NewTransactionOutputFromCbor(outputCbor)
	if err != nil {
		t.Fatalf("failed to decode lending test output: %v", err)
	}
	if output.Datum() == nil {
		t.Fatal("expected lending test output to have inline datum")
	}
	return output
}

type testLendingParser struct {
	protocol string
}

func (p testLendingParser) Protocol() string {
	if p.protocol == "" {
		return "test-lending"
	}
	return p.protocol
}

func (p testLendingParser) ParseDatum(
	_ []byte,
	txHash string,
	txIndex uint32,
	slot uint64,
	timestamp time.Time,
) (*LendingState, error) {
	return &LendingState{
		StateId:   fmt.Sprintf("state-%d", txIndex),
		StateType: LendingStateTypeMarket,
		Protocol:  p.Protocol(),
		Slot:      slot,
		TxHash:    txHash,
		TxIndex:   txIndex,
		Timestamp: timestamp,
	}, nil
}

func (p testLendingParser) GetAddresses() []string {
	return []string{liqwid.Charli3OracleAddress}
}
