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
	"errors"
	"testing"

	"github.com/blinklabs-io/shai/common"
	"github.com/stretchr/testify/require"
)

func TestInferSwapTransition(t *testing.T) {
	previous := activityPool(100, 1_000, 2_000)
	current := activityPool(101, 1_100, 1_820)
	current.BlockHash = "block"
	current.TxHash = "tx"
	current.TxIndex = 2

	swap, ok, err := InferSwapTransition(previous, current)
	require.NoError(t, err)
	require.True(t, ok)
	require.True(t, swap.InputIsX)
	require.Equal(t, uint64(100), swap.AmountX)
	require.Equal(t, uint64(180), swap.AmountY)
	require.Equal(t, uint64(101), swap.Slot)
	require.Equal(t, "block", swap.BlockHash)
	require.Equal(t, "tx", swap.TxHash)
	require.Equal(t, uint32(2), swap.TxIndex)

	reverse := activityPool(102, 900, 2_200)
	swap, ok, err = InferSwapTransition(current, reverse)
	require.NoError(t, err)
	require.True(t, ok)
	require.False(t, swap.InputIsX)
	require.Equal(t, uint64(200), swap.AmountX)
	require.Equal(t, uint64(380), swap.AmountY)
}

func TestInferSwapTransitionExcludesLiquidityChanges(t *testing.T) {
	previous := activityPool(100, 1_000, 2_000)
	for _, current := range []*PoolState{
		activityPool(101, 1_100, 2_200),
		activityPool(101, 900, 1_800),
		activityPool(101, 1_000, 2_000),
	} {
		_, ok, err := InferSwapTransition(previous, current)
		require.NoError(t, err)
		require.False(t, ok)
	}

	mempool := activityPool(101, 1_100, 1_800)
	mempool.FromMempool = true
	_, ok, err := InferSwapTransition(previous, mempool)
	require.NoError(t, err)
	require.False(t, ok)
}

func TestInferSwapTransitionRejectsIdentityChanges(t *testing.T) {
	previous := activityPool(100, 1_000, 2_000)
	current := activityPool(101, 1_100, 1_800)
	current.PoolId = "other"
	_, _, err := InferSwapTransition(previous, current)
	require.ErrorIs(t, err, ErrMismatchedPoolTransition)

	current = activityPool(101, 1_100, 1_800)
	current.AssetY.Class = common.Lovelace()
	_, _, err = InferSwapTransition(previous, current)
	require.ErrorIs(t, err, ErrMismatchedPoolTransition)
}

func TestActivityTrackerVolumeWindowAndRollback(t *testing.T) {
	tracker, err := NewActivityTracker(100)
	require.NoError(t, err)

	firstBefore := activityPool(9, 1_000, 2_000)
	firstAfter := activityPool(10, 1_100, 1_820)
	recorded, err := tracker.Observe(firstBefore, firstAfter)
	require.NoError(t, err)
	require.True(t, recorded)

	secondBefore := activityPool(49, 1_100, 1_820)
	secondAfter := activityPool(50, 900, 2_200)
	recorded, err = tracker.Observe(secondBefore, secondAfter)
	require.NoError(t, err)
	require.True(t, recorded)

	volume, ok, err := tracker.Volume("mainnet", "test", "pool", 50)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, uint64(300), volume.VolumeX)
	require.Equal(t, uint64(560), volume.VolumeY)
	require.Equal(t, uint64(2), volume.SwapCount)
	require.Equal(t, uint64(10), volume.FirstSwapSlot)
	require.Equal(t, uint64(50), volume.LastSwapSlot)

	// Advancing beyond the first swap's window removes it.
	volume, ok, err = tracker.Volume("mainnet", "test", "pool", 111)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, uint64(200), volume.VolumeX)
	require.Equal(t, uint64(380), volume.VolumeY)
	require.Equal(t, uint64(1), volume.SwapCount)

	tracker.Rollback(50)
	volume, ok, err = tracker.Volume("mainnet", "test", "pool", 49)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, uint64(100), volume.VolumeX)
	require.Equal(t, uint64(180), volume.VolumeY)
	require.Equal(t, uint64(1), volume.SwapCount)
}

func TestActivityTrackerRejectsOutOfOrderAndOverflow(t *testing.T) {
	tracker, err := NewActivityTracker(100)
	require.NoError(t, err)
	_, err = tracker.Observe(
		activityPool(99, 0, ^uint64(0)),
		activityPool(100, ^uint64(0), 0),
	)
	require.NoError(t, err)
	_, err = tracker.Observe(
		activityPool(100, ^uint64(0), 0),
		activityPool(101, 0, ^uint64(0)),
	)
	require.NoError(t, err)

	_, _, err = tracker.Volume("mainnet", "test", "pool", 101)
	require.ErrorIs(t, err, ErrVolumeOverflow)

	_, err = tracker.Observe(
		activityPool(89, 1_000, 2_000),
		activityPool(90, 1_100, 1_800),
	)
	require.True(t, errors.Is(err, ErrOutOfOrderActivity))
}

func TestNewActivityTrackerRejectsZeroWindow(t *testing.T) {
	_, err := NewActivityTracker(0)
	require.ErrorIs(t, err, ErrInvalidActivityWindow)
}

func activityPool(slot, reserveX, reserveY uint64) *PoolState {
	return &PoolState{
		PoolId:   "pool",
		Network:  "mainnet",
		Protocol: "test",
		AssetX: common.AssetAmount{
			Class:  common.Lovelace(),
			Amount: reserveX,
		},
		AssetY: common.AssetAmount{
			Class: common.AssetClass{
				PolicyId: []byte{1},
				Name:     []byte{2},
			},
			Amount: reserveY,
		},
		Slot: slot,
	}
}
