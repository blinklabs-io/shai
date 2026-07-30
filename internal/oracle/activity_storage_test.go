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
