// Copyright 2026 Blink Labs Software
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
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
	"github.com/stretchr/testify/require"
)

func TestOracleStorageActivitySurvivesRestartAndRollback(t *testing.T) {
	dir := t.TempDir()
	storage := openActivityStorage(t, dir)

	first := activityStorageSwap(10, "tx10", 100, 180)
	second := activityStorageSwap(50, "tx50", 200, 380)
	require.NoError(t, storage.SavePoolStateAndActivity(
		activityStoragePool(10),
		&first,
		100,
	))
	require.NoError(t, storage.SavePoolStateAndActivity(
		activityStoragePool(50),
		&second,
		100,
	))
	require.NoError(t, storage.Close())

	storage = openActivityStorage(t, dir)
	state, err := storage.LoadActivityState()
	require.NoError(t, err)
	require.Equal(t, uint64(50), state.LatestSlot)
	require.Len(t, state.Swaps, 2)

	tracker, err := NewActivityTracker(100)
	require.NoError(t, err)
	require.NoError(t, tracker.Restore(state))
	volume, ok, err := tracker.Volume("mainnet", "test", "pool", 50)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, uint64(2), volume.SwapCount)
	require.Equal(t, uint64(300), volume.VolumeX)

	require.NoError(t, storage.RollbackActivity(50))
	require.NoError(t, storage.Close())

	storage = openActivityStorage(t, dir)
	t.Cleanup(func() {
		require.NoError(t, storage.Close())
	})
	state, err = storage.LoadActivityState()
	require.NoError(t, err)
	require.Equal(t, uint64(49), state.LatestSlot)
	require.Len(t, state.Swaps, 1)
	require.Equal(t, uint64(10), state.Swaps[0].Slot)

	restored, err := NewActivityTracker(100)
	require.NoError(t, err)
	require.NoError(t, restored.Restore(state))
	volume, ok, err = restored.Volume("mainnet", "test", "pool", 49)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, uint64(1), volume.SwapCount)
}

func TestOracleStorageActivityPrunesRollingWindow(t *testing.T) {
	storage := newTestOracleStorage(t)
	old := activityStorageSwap(10, "old", 100, 180)
	require.NoError(t, storage.SavePoolStateAndActivity(
		activityStoragePool(10),
		&old,
		100,
	))
	require.NoError(t, storage.SavePoolStateAndActivity(
		activityStoragePool(111),
		nil,
		100,
	))
	olderOtherPool := activityStoragePool(50)
	olderOtherPool.PoolId = "other"
	require.NoError(t, storage.SavePoolStateAndActivity(
		olderOtherPool,
		nil,
		100,
	))

	state, err := storage.LoadActivityState()
	require.NoError(t, err)
	require.Equal(t, uint64(111), state.LatestSlot)
	require.Empty(t, state.Swaps)
}

func TestOracleStorageActivityReturnsCorruption(t *testing.T) {
	t.Run("malformed metadata", func(t *testing.T) {
		storage := newTestOracleStorage(t)
		require.NoError(t, storage.db.Update(func(txn *badger.Txn) error {
			return txn.Set([]byte(activityMetaKey), []byte("{"))
		}))

		_, err := storage.LoadActivityState()
		require.Error(t, err)
	})
	t.Run("orphaned swap", func(t *testing.T) {
		storage := newTestOracleStorage(t)
		swap := activityStorageSwap(10, "orphaned", 1, 1)
		data, err := json.Marshal(swap)
		require.NoError(t, err)
		require.NoError(t, storage.db.Update(func(txn *badger.Txn) error {
			return txn.Set([]byte(poolActivityKey(swap)), data)
		}))

		_, err = storage.LoadActivityState()
		require.Error(t, err)
	})
}

func openActivityStorage(t *testing.T, dir string) *OracleStorage {
	t.Helper()
	opts := badger.DefaultOptions(dir).WithLoggingLevel(badger.WARNING)
	db, err := badger.Open(opts)
	require.NoError(t, err)
	return &OracleStorage{db: db}
}

func activityStoragePool(slot uint64) *PoolState {
	return &PoolState{
		PoolId:   "pool",
		Network:  "mainnet",
		Protocol: "test",
		Slot:     slot,
		TxHash:   "pool-state",
		AssetX: common.AssetAmount{
			Class:  common.Lovelace(),
			Amount: 1_000,
		},
		AssetY: common.AssetAmount{
			Class: common.AssetClass{
				PolicyId: []byte{1},
				Name:     []byte{2},
			},
			Amount: 2_000,
		},
	}
}

func activityStorageSwap(
	slot uint64,
	txHash string,
	amountX,
	amountY uint64,
) SwapTransition {
	return SwapTransition{
		PoolID:   "pool",
		Network:  "mainnet",
		Protocol: "test",
		AssetX:   common.Lovelace(),
		AssetY: common.AssetClass{
			PolicyId: []byte{1},
			Name:     []byte{2},
		},
		AmountX: amountX,
		AmountY: amountY,
		Slot:    slot,
		TxHash:  txHash,
	}
}

// TestOracleStorageActivityPruneStaysWithinTransactionLimit covers the pruning
// gap a restart after a long outage produces: the persisted latest slot jumps
// past the whole retained window, so one write would otherwise queue every
// retained key for deletion. Badger rejects a transaction that large, which
// would fail the pool-state write itself and, because a persist failure exits
// the process, leave the oracle unable to make progress at all.
func TestOracleStorageActivityPruneStaysWithinTransactionLimit(t *testing.T) {
	const (
		retained = 2_000
		window   = 100
	)
	storage := openSmallBatchActivityStorage(t)
	seedActivityHistory(t, storage, retained, window, retained)

	// The next pool state is a full window past every retained swap, so the
	// cutoff moves beyond all of them at once.
	state := activityStoragePool(retained + window + 1)
	for range 20 {
		require.NoError(t, storage.SavePoolStateAndActivity(state, nil, window))
		loaded, err := storage.LoadActivityState()
		require.NoError(t, err)
		if len(loaded.Swaps) == 0 {
			return
		}
	}
	t.Fatal("pruning did not drain the retained activity history")
}

// TestOracleStorageRollbackActivityBatchesLargeReorg covers the same Badger
// transaction limit on the rollback path, where the deletions cannot be
// deferred: every swap at or after the rollback slot has to go.
func TestOracleStorageRollbackActivityBatchesLargeReorg(t *testing.T) {
	const (
		retained = 2_000
		window   = 100_000
	)
	storage := openSmallBatchActivityStorage(t)
	seedActivityHistory(t, storage, retained, window, retained)

	require.NoError(t, storage.RollbackActivity(500))

	loaded, err := storage.LoadActivityState()
	require.NoError(t, err)
	require.Len(t, loaded.Swaps, 499)
	require.Equal(t, uint64(1), loaded.Swaps[0].Slot)
	require.Equal(t, uint64(499), loaded.Swaps[len(loaded.Swaps)-1].Slot)
	require.Equal(t, uint64(499), loaded.LatestSlot)
}

// TestOracleStorageRollbackActivityBatchKeepsRecordedTipAhead asserts the
// invariant every committed rollback batch has to keep: no retained swap sits
// above the persisted latest slot. A batch that deletes newer swaps without
// rewinding the tip in the same transaction would let an interrupted rollback
// restore reorged swaps and count their volume again.
func TestOracleStorageRollbackActivityBatchKeepsRecordedTipAhead(t *testing.T) {
	storage := newTestOracleStorage(t)
	seedActivityHistory(t, storage, 10, 100, 10)

	for batch := range 10 {
		// A budget of one byte forces a single slot per batch.
		exhausted, err := storage.rollbackActivityBatch(3, 1, 1)
		require.NoError(t, err)
		loaded, err := storage.LoadActivityState()
		require.NoError(t, err)
		for _, swap := range loaded.Swaps {
			require.LessOrEqual(
				t,
				swap.Slot,
				loaded.LatestSlot,
				"batch %d left swap slot %d above the recorded tip",
				batch,
				swap.Slot,
			)
		}
		if exhausted {
			require.Len(t, loaded.Swaps, 2)
			require.Equal(t, uint64(2), loaded.LatestSlot)
			return
		}
	}
	t.Fatal("batched rollback did not complete")
}

// TestOracleStorageRollbackActivityKeepsLatestSlot covers a rollback point
// ahead of the persisted latest slot, which is the normal case whenever the
// chain-sync cursor moved through blocks that carried no pool update. The
// recorded tip states what activity was observed, so it must not be advanced.
func TestOracleStorageRollbackActivityKeepsLatestSlot(t *testing.T) {
	storage := newTestOracleStorage(t)
	swap := activityStorageSwap(10, "tx10", 100, 180)
	require.NoError(t, storage.SavePoolStateAndActivity(
		activityStoragePool(10),
		&swap,
		100,
	))

	require.NoError(t, storage.RollbackActivity(5_000))

	loaded, err := storage.LoadActivityState()
	require.NoError(t, err)
	require.Equal(t, uint64(10), loaded.LatestSlot)
	require.Len(t, loaded.Swaps, 1)
}

// TestOracleStorageActivityRejectsZeroWindowMetadata covers metadata whose
// window is missing or zero. Startup only restores activity when the loaded
// window is non-zero, so returning that state instead of an error would drop
// every persisted swap from the tracker without saying so.
func TestOracleStorageActivityRejectsZeroWindowMetadata(t *testing.T) {
	storage := newTestOracleStorage(t)
	swap := activityStorageSwap(10, "tx10", 100, 180)
	data, err := json.Marshal(swap)
	require.NoError(t, err)
	require.NoError(t, storage.db.Update(func(txn *badger.Txn) error {
		if err := txn.Set([]byte(poolActivityKey(swap)), data); err != nil {
			return err
		}
		return txn.Set([]byte(activityMetaKey), []byte(`{"latestSlot":10}`))
	}))

	_, err = storage.LoadActivityState()
	require.Error(t, err)
}

// openSmallBatchActivityStorage opens storage whose Badger transaction limits
// are small enough that a few hundred queued deletions reach ErrTxnTooBig.
func openSmallBatchActivityStorage(t *testing.T) *OracleStorage {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "oracle")
	opts := badger.DefaultOptions(dir).
		WithLoggingLevel(badger.WARNING).
		WithMemTableSize(1 << 20).
		WithValueThreshold(64)
	db, err := badger.Open(opts)
	require.NoError(t, err)
	storage := &OracleStorage{db: db}
	t.Cleanup(func() {
		require.NoError(t, storage.Close())
	})
	return storage
}

// seedActivityHistory writes count swaps at slots 1..count plus the matching
// metadata, bypassing the pruning that a normal save would apply.
func seedActivityHistory(
	t *testing.T,
	storage *OracleStorage,
	count int,
	window,
	latestSlot uint64,
) {
	t.Helper()
	batch := storage.db.NewWriteBatch()
	defer batch.Cancel()
	for i := 1; i <= count; i++ {
		swap := activityStorageSwap(
			uint64(i),
			fmt.Sprintf("tx%d", i),
			100,
			180,
		)
		data, err := json.Marshal(swap)
		require.NoError(t, err)
		require.NoError(t, batch.Set([]byte(poolActivityKey(swap)), data))
	}
	meta, err := json.Marshal(activityMetadata{
		WindowSlots: window,
		LatestSlot:  latestSlot,
	})
	require.NoError(t, err)
	require.NoError(t, batch.Set([]byte(activityMetaKey), meta))
	require.NoError(t, batch.Flush())
}
