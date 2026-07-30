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

package dex

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestActivityTrackerSnapshotRestoreAndRollback(t *testing.T) {
	tracker, err := NewActivityTracker(100)
	require.NoError(t, err)

	transition, recorded, err := tracker.ObserveTransition(
		activityPool(9, 1_000, 2_000),
		activityPool(10, 1_100, 1_820),
	)
	require.NoError(t, err)
	require.True(t, recorded)
	require.Equal(t, uint64(100), transition.AmountX)

	_, recorded, err = tracker.ObserveTransition(
		activityPool(9, 1_000, 2_000),
		activityPool(10, 1_100, 1_820),
	)
	require.NoError(t, err)
	require.False(t, recorded, "replayed transition must be idempotent")

	_, recorded, err = tracker.ObserveTransition(
		activityPool(49, 1_100, 1_820),
		activityPool(50, 900, 2_200),
	)
	require.NoError(t, err)
	require.True(t, recorded)

	state := tracker.Snapshot()
	restored, err := NewActivityTracker(100)
	require.NoError(t, err)
	require.NoError(t, restored.Restore(state))

	volume, ok, err := restored.Volume("mainnet", "test", "pool", 50)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, uint64(2), volume.SwapCount)
	require.Equal(t, uint64(300), volume.VolumeX)
	require.Equal(t, uint64(560), volume.VolumeY)

	restored.Rollback(50)
	volume, ok, err = restored.Volume("mainnet", "test", "pool", 49)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, uint64(1), volume.SwapCount)
	require.Equal(t, uint64(10), volume.LastSwapSlot)
}

func TestActivityTrackerRestoreRejectsInvalidState(t *testing.T) {
	tracker, err := NewActivityTracker(100)
	require.NoError(t, err)

	require.ErrorIs(
		t,
		tracker.Restore(ActivityState{WindowSlots: 200}),
		ErrActivityWindowChanged,
	)
	require.ErrorIs(
		t,
		tracker.Restore(ActivityState{
			WindowSlots: 100,
			LatestSlot:  9,
			Swaps: []SwapTransition{{
				PoolID:   "pool",
				Network:  "mainnet",
				Protocol: "test",
				TxHash:   "tx",
				Slot:     10,
				AmountX:  1,
				AmountY:  1,
			}},
		}),
		ErrInvalidActivityState,
	)
	require.Empty(t, tracker.Snapshot().Swaps)
}

func TestActivityTrackerSnapshotIsIndependent(t *testing.T) {
	tracker, err := NewActivityTracker(100)
	require.NoError(t, err)
	_, recorded, err := tracker.ObserveTransition(
		activityPool(9, 1_000, 2_000),
		activityPool(10, 1_100, 1_820),
	)
	require.NoError(t, err)
	require.True(t, recorded)

	state := tracker.Snapshot()
	state.Swaps[0].AssetY.PolicyId[0] = 99

	second := tracker.Snapshot()
	require.Equal(t, byte(1), second.Swaps[0].AssetY.PolicyId[0])
}
