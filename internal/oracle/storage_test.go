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
	"strings"
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

func newClosedTestOracleStorage(t *testing.T) *OracleStorage {
	t.Helper()
	db, err := badger.Open(
		badger.DefaultOptions("").WithInMemory(true).WithLogger(nil),
	)
	if err != nil {
		t.Fatalf("failed to open test badger DB: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("failed to close test badger DB: %v", err)
	}
	return &OracleStorage{db: db}
}

func TestOrderUndoHistoryPrunesFinalizedTransactions(t *testing.T) {
	storage := newTestOracleStorage(t)
	oldRecord := orderUndoRecord{
		Network:         "mainnet",
		Protocol:        "geniusyield",
		TransactionHash: strings.Repeat("a", 64),
		Slot:            1,
	}
	oldState := &OrderState{
		Network:  oldRecord.Network,
		Protocol: oldRecord.Protocol,
		OrderId:  "old-order",
		Slot:     oldRecord.Slot,
	}
	if _, err := storage.ApplyOrderTransaction(
		oldRecord,
		[]*OrderState{oldState},
	); err != nil {
		t.Fatalf("failed to apply old order transaction: %v", err)
	}
	boundaryRecord := orderUndoRecord{
		Network:         oldRecord.Network,
		Protocol:        oldRecord.Protocol,
		TransactionHash: strings.Repeat("c", 64),
		Slot:            2,
	}
	if err := storage.db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(orderUndoKey(boundaryRecord)), []byte("{"))
	}); err != nil {
		t.Fatalf("failed to seed retained boundary record: %v", err)
	}

	newRecord := orderUndoRecord{
		Network:         oldRecord.Network,
		Protocol:        oldRecord.Protocol,
		TransactionHash: strings.Repeat("b", 64),
		Slot:            orderRollbackRetentionSlots + 2,
	}
	newState := &OrderState{
		Network:  newRecord.Network,
		Protocol: newRecord.Protocol,
		OrderId:  "new-order",
		Slot:     newRecord.Slot,
	}
	if _, err := storage.ApplyOrderTransaction(
		newRecord,
		[]*OrderState{newState},
	); err != nil {
		t.Fatalf("failed to apply new order transaction: %v", err)
	}

	err := storage.db.View(func(txn *badger.Txn) error {
		if _, err := txn.Get([]byte(orderUndoKey(oldRecord))); err != badger.ErrKeyNotFound {
			return fmt.Errorf("old undo record lookup returned %v", err)
		}
		if _, err := txn.Get([]byte(orderUndoKey(newRecord))); err != nil {
			return fmt.Errorf("new undo record lookup returned %v", err)
		}
		if _, err := txn.Get([]byte(orderUndoKey(boundaryRecord))); err != nil {
			return fmt.Errorf("boundary undo record lookup returned %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestOracleStorageOrderUndoPruneStaysWithinTransactionLimit(t *testing.T) {
	const retained = 2_000
	storage := openSmallBatchActivityStorage(t)
	batch := storage.db.NewWriteBatch()
	defer batch.Cancel()
	for i := 1; i <= retained; i++ {
		record := orderUndoRecord{
			Network:         "mainnet",
			Protocol:        "geniusyield",
			TransactionHash: fmt.Sprintf("%064x", i),
			Slot:            uint64(i),
		}
		data, err := json.Marshal(record)
		if err != nil {
			t.Fatalf("failed to marshal seeded undo record: %v", err)
		}
		if err := batch.Set([]byte(orderUndoKey(record)), data); err != nil {
			t.Fatalf("failed to seed undo record: %v", err)
		}
	}
	if err := batch.Flush(); err != nil {
		t.Fatalf("failed to flush seeded undo records: %v", err)
	}

	cutoff := uint64(retained + 1)
	currentSlot := orderRollbackRetentionSlots + cutoff
	remaining := retained
	for attempt := range 20 {
		record := orderUndoRecord{
			Network:         "mainnet",
			Protocol:        "geniusyield",
			TransactionHash: fmt.Sprintf("new-%d", attempt),
			Slot:            currentSlot,
		}
		state := &OrderState{
			Network:  record.Network,
			Protocol: record.Protocol,
			OrderId:  fmt.Sprintf("new-order-%d", attempt),
			Slot:     currentSlot,
		}
		applied, err := storage.ApplyOrderTransaction(
			record,
			[]*OrderState{state},
		)
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
			if _, err := storage.LoadOrderState(
				record.Network, record.Protocol, state.OrderId,
			); err != nil {
				t.Fatalf("new order state was not durable: %v", err)
			}
		}
		next := countOrderUndoRecordsBeforeSlot(
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

func countOrderUndoRecordsBeforeSlot(
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
			orderUndoKeyPrefix + network + ":" + protocol + ":",
		)
		it := txn.NewIterator(opts)
		defer it.Close()
		for it.Rewind(); it.Valid(); it.Next() {
			var record orderUndoRecord
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

func TestOrderRollbackReadsOnlyInvalidUndoSuffix(t *testing.T) {
	storage := newTestOracleStorage(t)
	retained := orderUndoRecord{
		Network:         "mainnet",
		Protocol:        "geniusyield",
		TransactionHash: "retained",
		Slot:            9,
	}
	if err := storage.db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(orderUndoKey(retained)), []byte("{"))
	}); err != nil {
		t.Fatalf("failed to seed retained undo record: %v", err)
	}

	first := orderUndoRecord{
		Network:         retained.Network,
		Protocol:        retained.Protocol,
		TransactionHash: "z",
		Slot:            10,
	}
	firstState := &OrderState{
		Network:  first.Network,
		Protocol: first.Protocol,
		OrderId:  "first",
		Slot:     first.Slot,
	}
	if _, err := storage.ApplyOrderTransaction(
		first,
		[]*OrderState{firstState},
	); err != nil {
		t.Fatalf("failed to apply first order transition: %v", err)
	}
	second := orderUndoRecord{
		Network:         first.Network,
		Protocol:        first.Protocol,
		TransactionHash: "a",
		TransactionIdx:  1,
		Slot:            first.Slot,
		Spent:           []*OrderState{firstState},
	}
	secondState := &OrderState{
		Network:  second.Network,
		Protocol: second.Protocol,
		OrderId:  "second",
		Slot:     second.Slot,
	}
	if _, err := storage.ApplyOrderTransaction(
		second,
		[]*OrderState{secondState},
	); err != nil {
		t.Fatalf("failed to apply second order transition: %v", err)
	}

	changes, err := storage.RollbackOrderStates(
		first.Network,
		first.Protocol,
		first.Slot,
	)
	if err != nil {
		t.Fatalf("failed to roll back order transitions: %v", err)
	}
	if len(changes) != 2 || changes[firstState.OrderId] != nil ||
		changes[secondState.OrderId] != nil {
		t.Fatalf("unexpected order rollback changes: %+v", changes)
	}
	if err := storage.db.View(func(txn *badger.Txn) error {
		if _, err := txn.Get([]byte(orderUndoKey(retained))); err != nil {
			return fmt.Errorf("retained undo record lookup returned %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestOrderRollbackPersistsProgressBeforeCorruptRecord(t *testing.T) {
	storage := newTestOracleStorage(t)
	oldest := orderUndoRecord{
		Network:         "mainnet",
		Protocol:        "geniusyield",
		TransactionHash: "oldest",
		Slot:            10,
	}
	oldestState := &OrderState{
		Network:  oldest.Network,
		Protocol: oldest.Protocol,
		OrderId:  "oldest",
		Slot:     oldest.Slot,
	}
	if _, err := storage.ApplyOrderTransaction(
		oldest,
		[]*OrderState{oldestState},
	); err != nil {
		t.Fatalf("failed to apply oldest order transition: %v", err)
	}
	newest := orderUndoRecord{
		Network:         oldest.Network,
		Protocol:        oldest.Protocol,
		TransactionHash: "newest",
		Slot:            12,
	}
	newestState := &OrderState{
		Network:  newest.Network,
		Protocol: newest.Protocol,
		OrderId:  "newest",
		Slot:     newest.Slot,
	}
	if _, err := storage.ApplyOrderTransaction(
		newest,
		[]*OrderState{newestState},
	); err != nil {
		t.Fatalf("failed to apply newest order transition: %v", err)
	}
	corrupt := orderUndoRecord{
		Network:         oldest.Network,
		Protocol:        oldest.Protocol,
		TransactionHash: "corrupt",
		Slot:            11,
	}
	if err := storage.db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(orderUndoKey(corrupt)), []byte("{"))
	}); err != nil {
		t.Fatalf("failed to seed corrupt undo record: %v", err)
	}

	changes, err := storage.RollbackOrderStates(
		oldest.Network,
		oldest.Protocol,
		oldest.Slot,
	)
	if err == nil {
		t.Fatal("expected corrupt undo record to stop rollback")
	}
	if len(changes) != 1 || changes[newestState.OrderId] != nil {
		t.Fatalf("unexpected persisted rollback changes: %+v", changes)
	}
	if _, err := storage.LoadOrderState(
		newest.Network,
		newest.Protocol,
		newestState.OrderId,
	); err == nil {
		t.Fatal("newest order state remained after its undo record was processed")
	}
	if _, err := storage.LoadOrderState(
		oldest.Network,
		oldest.Protocol,
		oldestState.OrderId,
	); err != nil {
		t.Fatalf("oldest order state was changed before its undo record: %v", err)
	}

	if err := storage.db.Update(func(txn *badger.Txn) error {
		return txn.Delete([]byte(orderUndoKey(corrupt)))
	}); err != nil {
		t.Fatalf("failed to remove corrupt undo record: %v", err)
	}
	changes, err = storage.RollbackOrderStates(
		oldest.Network,
		oldest.Protocol,
		oldest.Slot,
	)
	if err != nil {
		t.Fatalf("failed to resume order rollback: %v", err)
	}
	if len(changes) != 1 || changes[oldestState.OrderId] != nil {
		t.Fatalf("unexpected resumed rollback changes: %+v", changes)
	}
}

func TestOrderRollbackReturnsOnlyChangedStates(t *testing.T) {
	const retainedCount = 2_000
	storage := newTestOracleStorage(t)
	batch := storage.db.NewWriteBatch()
	defer batch.Cancel()
	for i := range retainedCount {
		state := &OrderState{
			Network:  "mainnet",
			Protocol: "geniusyield",
			OrderId:  fmt.Sprintf("retained-%04d", i),
			Slot:     9,
		}
		data, err := json.Marshal(state)
		if err != nil {
			t.Fatalf("failed to marshal retained order: %v", err)
		}
		if err := batch.Set(
			[]byte(orderStateKey(state.Network, state.Protocol, state.OrderId)),
			data,
		); err != nil {
			t.Fatalf("failed to seed retained order: %v", err)
		}
	}
	if err := batch.Flush(); err != nil {
		t.Fatalf("failed to flush retained orders: %v", err)
	}

	rollbackRecord := orderUndoRecord{
		Network:         "mainnet",
		Protocol:        "geniusyield",
		TransactionHash: "rollback",
		Slot:            10,
	}
	invalid := &OrderState{
		Network:  rollbackRecord.Network,
		Protocol: rollbackRecord.Protocol,
		OrderId:  "invalid",
		Slot:     rollbackRecord.Slot,
	}
	if _, err := storage.ApplyOrderTransaction(
		rollbackRecord,
		[]*OrderState{invalid},
	); err != nil {
		t.Fatalf("failed to apply rolled-back order: %v", err)
	}

	changes, err := storage.RollbackOrderStates("mainnet", "geniusyield", 10)
	if err != nil {
		t.Fatalf("failed to roll back order: %v", err)
	}
	if len(changes) != 1 || changes[invalid.OrderId] != nil {
		t.Fatalf(
			"rollback returned %d changes for one invalid order",
			len(changes),
		)
	}
	if _, err := storage.LoadOrderState(
		"mainnet",
		"geniusyield",
		"retained-1999",
	); err != nil {
		t.Fatalf("retained order was not preserved: %v", err)
	}
}

func TestOrderRollbackIndexesLegacyStates(t *testing.T) {
	storage := newTestOracleStorage(t)
	retained := &OrderState{
		Network:  "mainnet",
		Protocol: "geniusyield",
		OrderId:  "retained",
		Slot:     9,
	}
	invalid := &OrderState{
		Network:  retained.Network,
		Protocol: retained.Protocol,
		OrderId:  "invalid",
		Slot:     10,
	}
	for _, state := range []*OrderState{retained, invalid} {
		data, err := json.Marshal(state)
		if err != nil {
			t.Fatalf("failed to marshal legacy order: %v", err)
		}
		if err := storage.db.Update(func(txn *badger.Txn) error {
			return txn.Set(
				[]byte(orderStateKey(state.Network, state.Protocol, state.OrderId)),
				data,
			)
		}); err != nil {
			t.Fatalf("failed to seed legacy order: %v", err)
		}
	}
	states, err := storage.LoadAllOrderStates()
	if err != nil {
		t.Fatalf("failed to load legacy orders: %v", err)
	}
	if err := storage.ensureOrderStateIndexes(states); err != nil {
		t.Fatalf("failed to index legacy orders: %v", err)
	}

	changes, err := storage.RollbackOrderStates("mainnet", "geniusyield", 10)
	if err != nil {
		t.Fatalf("failed to roll back legacy order: %v", err)
	}
	if len(changes) != 1 || changes[invalid.OrderId] != nil {
		t.Fatalf("unexpected legacy rollback changes: %+v", changes)
	}
	if _, err := storage.LoadOrderState(
		retained.Network,
		retained.Protocol,
		retained.OrderId,
	); err != nil {
		t.Fatalf("retained legacy order was not preserved: %v", err)
	}
	if _, err := storage.LoadOrderState(
		invalid.Network,
		invalid.Protocol,
		invalid.OrderId,
	); err == nil {
		t.Fatal("invalid legacy order remained after rollback")
	}
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
