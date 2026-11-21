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
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/blinklabs-io/shai/common"
	"github.com/dgraph-io/badger/v4"
)

func newTestOracleStorage(t *testing.T) *OracleStorage {
	t.Helper()

	dir := filepath.Join(t.TempDir(), "oracle")
	opts := badger.DefaultOptions(dir).WithLoggingLevel(badger.WARNING)
	db, err := badger.Open(opts)
	if err != nil {
		t.Fatalf("failed to open test badger DB: %v", err)
	}

	storage := &OracleStorage{db: db}
	t.Cleanup(func() {
		if err := storage.Close(); err != nil {
			t.Fatalf("failed to close test badger DB: %v", err)
		}
	})
	return storage
}

func TestOracleStorageRollbackDeletesInvalidStatesInBatches(t *testing.T) {
	storage := newTestOracleStorage(t)
	const stateCount = cdpRollbackDeleteBatchSize + 1
	requireStates := make([]*CDPState, 0, stateCount)
	for i := 0; i < stateCount; i++ {
		requireStates = append(requireStates, &CDPState{
			CDPId:        fmt.Sprintf("cdp-%03d", i),
			Network:      "mainnet",
			Protocol:     "butane",
			Slot:         100,
			MintedAmount: 1,
		})
	}
	if err := storage.db.Update(func(txn *badger.Txn) error {
		for _, state := range requireStates {
			data, err := json.Marshal(state)
			if err != nil {
				return err
			}
			if err := txn.Set(
				[]byte(cdpStateKey(state.Network, state.Protocol, state.CDPId)),
				data,
			); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("failed to seed CDP states: %v", err)
	}

	states, err := storage.RollbackCDPStates("mainnet", "butane", 100)
	if err != nil {
		t.Fatalf("failed to roll back CDP states: %v", err)
	}
	if len(states) != 0 {
		t.Fatalf("expected no CDP states after rollback, got %d", len(states))
	}
	states, err = storage.LoadAllCDPStates()
	if err != nil {
		t.Fatalf("failed to load CDP states after rollback: %v", err)
	}
	if len(states) != 0 {
		t.Fatalf("expected storage to be empty after rollback, got %d", len(states))
	}
}

func TestOracleStorageRollbackCDPStatesSkipsRetainedUndo(t *testing.T) {
	storage := newTestOracleStorage(t)
	retainedRecord := cdpUndoRecord{
		Network:         "mainnet",
		Protocol:        "butane",
		TransactionHash: "retained",
		Slot:            99,
	}
	retainedState := &CDPState{
		Network:  retainedRecord.Network,
		Protocol: retainedRecord.Protocol,
		CDPId:    "retained-cdp",
		Slot:     retainedRecord.Slot,
	}
	if _, err := storage.ApplyCDPTransaction(
		retainedRecord,
		[]*CDPState{retainedState},
	); err != nil {
		t.Fatalf("failed to apply retained CDP transaction: %v", err)
	}

	rollbackRecord := cdpUndoRecord{
		Network:         retainedRecord.Network,
		Protocol:        retainedRecord.Protocol,
		TransactionHash: "rollback",
		TransactionIdx:  1,
		Slot:            100,
		Spent:           []*CDPState{retainedState},
	}
	replacement := &CDPState{
		Network:  rollbackRecord.Network,
		Protocol: rollbackRecord.Protocol,
		CDPId:    "replacement-cdp",
		Slot:     rollbackRecord.Slot,
	}
	if _, err := storage.ApplyCDPTransaction(
		rollbackRecord,
		[]*CDPState{replacement},
	); err != nil {
		t.Fatalf("failed to apply rollback CDP transaction: %v", err)
	}
	if err := storage.db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(cdpUndoStateKey(retainedRecord)), []byte("{"))
	}); err != nil {
		t.Fatalf("failed to corrupt retained undo record: %v", err)
	}

	states, err := storage.RollbackCDPStates("mainnet", "butane", 100)
	if err != nil {
		t.Fatalf("failed to roll back CDP states: %v", err)
	}
	if len(states) != 1 || states[0].CDPId != retainedState.CDPId {
		t.Fatalf("expected restored state %q, got %+v", retainedState.CDPId, states)
	}
	if _, err := storage.LoadCDPState(
		replacement.Network,
		replacement.Protocol,
		replacement.CDPId,
	); err == nil {
		t.Fatal("replacement CDP state remained after rollback")
	}
}

func TestCDPUndoHistoryPrunesFinalizedTransactions(t *testing.T) {
	storage := newTestOracleStorage(t)
	oldRecord := cdpUndoRecord{
		Network:         "mainnet",
		Protocol:        "butane",
		TransactionHash: "old",
		Slot:            1,
	}
	oldState := &CDPState{
		Network:  oldRecord.Network,
		Protocol: oldRecord.Protocol,
		CDPId:    "old-cdp",
		Slot:     oldRecord.Slot,
	}
	if _, err := storage.ApplyCDPTransaction(
		oldRecord,
		[]*CDPState{oldState},
	); err != nil {
		t.Fatalf("failed to apply old CDP transaction: %v", err)
	}
	boundaryRecord := cdpUndoRecord{
		Network:         oldRecord.Network,
		Protocol:        oldRecord.Protocol,
		TransactionHash: "boundary",
		Slot:            2,
	}
	if err := storage.db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(cdpUndoStateKey(boundaryRecord)), []byte("{"))
	}); err != nil {
		t.Fatalf("failed to seed retained boundary record: %v", err)
	}

	newRecord := cdpUndoRecord{
		Network:         oldRecord.Network,
		Protocol:        oldRecord.Protocol,
		TransactionHash: "new",
		Slot:            cdpRollbackRetentionSlots + 2,
	}
	newState := &CDPState{
		Network:  newRecord.Network,
		Protocol: newRecord.Protocol,
		CDPId:    "new-cdp",
		Slot:     newRecord.Slot,
	}
	if _, err := storage.ApplyCDPTransaction(
		newRecord,
		[]*CDPState{newState},
	); err != nil {
		t.Fatalf("failed to apply new CDP transaction: %v", err)
	}

	err := storage.db.View(func(txn *badger.Txn) error {
		if _, err := txn.Get(
			[]byte(cdpUndoStateKey(oldRecord)),
		); err != badger.ErrKeyNotFound {
			return fmt.Errorf("old undo record lookup returned %v", err)
		}
		if _, err := txn.Get([]byte(cdpUndoStateKey(newRecord))); err != nil {
			return fmt.Errorf("new undo record lookup returned %v", err)
		}
		if _, err := txn.Get([]byte(cdpUndoStateKey(boundaryRecord))); err != nil {
			return fmt.Errorf("boundary undo record lookup returned %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestOracleStorageCDPUndoPruneStaysWithinTransactionLimit(t *testing.T) {
	const retained = 2_000
	storage := openSmallBatchActivityStorage(t)
	batch := storage.db.NewWriteBatch()
	defer batch.Cancel()
	for i := 1; i <= retained; i++ {
		record := cdpUndoRecord{
			Network:         "mainnet",
			Protocol:        "butane",
			TransactionHash: fmt.Sprintf("%064x", i),
			Slot:            uint64(i),
		}
		data, err := json.Marshal(record)
		if err != nil {
			t.Fatalf("failed to marshal seeded undo record: %v", err)
		}
		if err := batch.Set([]byte(cdpUndoStateKey(record)), data); err != nil {
			t.Fatalf("failed to seed undo record: %v", err)
		}
	}
	if err := batch.Flush(); err != nil {
		t.Fatalf("failed to flush seeded undo records: %v", err)
	}

	cutoff := uint64(retained + 1)
	currentSlot := cdpRollbackRetentionSlots + cutoff
	remaining := retained
	for attempt := range 20 {
		record := cdpUndoRecord{
			Network:         "mainnet",
			Protocol:        "butane",
			TransactionHash: fmt.Sprintf("new-%d", attempt),
			Slot:            currentSlot,
		}
		state := &CDPState{
			Network:  record.Network,
			Protocol: record.Protocol,
			CDPId:    fmt.Sprintf("new-cdp-%d", attempt),
			Slot:     currentSlot,
		}
		applied, err := storage.ApplyCDPTransaction(record, []*CDPState{state})
		if err != nil {
			t.Fatalf(
				"failed to apply transition while pruning backlog: %v",
				err,
			)
		}
		if !applied {
			t.Fatal("expected unique transition to be applied")
		}
		if attempt == 0 {
			if _, err := storage.LoadCDPState(
				record.Network, record.Protocol, state.CDPId,
			); err != nil {
				t.Fatalf("new CDP state was not durable: %v", err)
			}
		}
		next := countCDPUndoRecordsBeforeSlot(
			t, storage, record.Network, record.Protocol, cutoff,
		)
		if next >= remaining {
			t.Fatalf("prune made no progress: %d records remain", next)
		}
		if attempt == 0 && next == 0 {
			t.Fatal("prune exceeded its transaction budget")
		}
		remaining = next
		if remaining == 0 {
			return
		}
	}
	t.Fatalf("pruning did not drain %d expired undo records", remaining)
}

func countCDPUndoRecordsBeforeSlot(
	t *testing.T,
	storage *OracleStorage,
	network,
	protocol string,
	cutoff uint64,
) int {
	t.Helper()
	count := 0
	err := storage.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte(
			cdpUndoStateKeyPrefix + network + ":" + protocol + ":",
		)
		it := txn.NewIterator(opts)
		defer it.Close()
		for it.Rewind(); it.Valid(); it.Next() {
			var record cdpUndoRecord
			if err := it.Item().Value(func(value []byte) error {
				return json.Unmarshal(value, &record)
			}); err != nil {
				return err
			}
			if record.Slot < cutoff {
				count++
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("failed to count expired undo records: %v", err)
	}
	return count
}

func TestOracleStorageLoadPoolState(t *testing.T) {
	storage := newTestOracleStorage(t)

	state := &PoolState{
		Network:  "mainnet",
		Protocol: "minswap-v2",
		PoolId:   "pool1",
		AssetX:   common.AssetAmount{Amount: 1000},
		AssetY:   common.AssetAmount{Amount: 2000},
	}
	if err := storage.SavePoolState(state); err != nil {
		t.Fatalf("failed to save pool state: %v", err)
	}

	loaded, err := storage.LoadPoolState("mainnet", "minswap-v2", "pool1")
	if err != nil {
		t.Fatalf("failed to load pool state: %v", err)
	}
	if loaded == nil {
		t.Fatalf(
			"expected loaded pool state for mainnet/minswap-v2/pool1, got nil",
		)
	}
	if loaded.PoolId != state.PoolId {
		t.Fatalf("expected pool ID %s, got %s", state.PoolId, loaded.PoolId)
	}
	if loaded.AssetX.Amount != state.AssetX.Amount {
		t.Fatalf(
			"expected assetX amount %d, got %d",
			state.AssetX.Amount,
			loaded.AssetX.Amount,
		)
	}
}

func TestOracleStorageLoadPoolStatesByProtocol(t *testing.T) {
	storage := newTestOracleStorage(t)

	states := []*PoolState{
		{
			Network:  "mainnet",
			Protocol: "minswap-v2",
			PoolId:   "pool1",
		},
		{
			Network:  "mainnet",
			Protocol: "splash-v1",
			PoolId:   "pool2",
		},
		{
			Network:  "mainnet",
			Protocol: "minswap-v2",
			PoolId:   "pool3",
		},
	}

	for _, state := range states {
		if err := storage.SavePoolState(state); err != nil {
			t.Fatalf("failed to save state %s: %v", state.PoolId, err)
		}
	}

	filtered, err := storage.LoadPoolStatesByProtocol("minswap-v2")
	if err != nil {
		t.Fatalf("failed to load filtered states: %v", err)
	}
	if len(filtered) != 2 {
		t.Fatalf("expected 2 filtered states, got %d", len(filtered))
	}
	for _, state := range filtered {
		if state.Protocol != "minswap-v2" {
			t.Fatalf("expected protocol minswap-v2, got %s", state.Protocol)
		}
	}
}

func TestParsePoolStateKey(t *testing.T) {
	network, protocol, poolId, err := ParsePoolStateKey(
		"oracle_pool_mainnet:minswap-v2:pool1",
	)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if network != "mainnet" {
		t.Fatalf("expected network mainnet, got %s", network)
	}
	if protocol != "minswap-v2" {
		t.Fatalf("expected protocol minswap-v2, got %s", protocol)
	}
	if poolId != "pool1" {
		t.Fatalf("expected pool ID pool1, got %s", poolId)
	}

	_, _, _, err = ParsePoolStateKey("bad-key")
	if err == nil {
		t.Fatal("expected parse error for invalid key")
	}

	_, _, _, err = ParsePoolStateKey("foo:bar:baz")
	if err == nil {
		t.Fatal("expected parse error for key missing oracle prefix")
	}

	_, _, _, err = ParsePoolStateKey("oracle_pool_mainnet::pool1")
	if err == nil {
		t.Fatal("expected parse error for key with empty protocol")
	}
}

func TestOracleStorageLoadPoolStatesByProtocolSkipsMalformedEntries(t *testing.T) {
	storage := newTestOracleStorage(t)

	goodState := &PoolState{
		Network:  "mainnet",
		Protocol: "minswap-v2",
		PoolId:   "pool1",
	}
	if err := storage.SavePoolState(goodState); err != nil {
		t.Fatalf("failed to save good pool state: %v", err)
	}

	err := storage.db.Update(func(txn *badger.Txn) error {
		// Invalid key: empty protocol segment
		if err := txn.Set(
			[]byte("oracle_pool_mainnet::pool-bad-key"),
			[]byte(`{"poolId":"pool-bad-key","protocol":"minswap-v2"}`),
		); err != nil {
			return err
		}
		// Invalid value for target protocol
		return txn.Set(
			[]byte("oracle_pool_mainnet:minswap-v2:pool-bad-value"),
			[]byte("not-json"),
		)
	})
	if err != nil {
		t.Fatalf("failed to seed malformed rows: %v", err)
	}

	filtered, err := storage.LoadPoolStatesByProtocol("minswap-v2")
	if err != nil {
		t.Fatalf("expected malformed rows to be skipped, got error: %v", err)
	}
	if len(filtered) != 1 {
		t.Fatalf("expected 1 valid state, got %d", len(filtered))
	}
	if filtered[0].PoolId != "pool1" {
		t.Fatalf("expected pool1, got %s", filtered[0].PoolId)
	}
}
