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

func TestInferSwapTransitionCurrentMainnetCSWAP(t *testing.T) {
	usdm, err := common.NewAssetClass(
		"c48cbb3d5e57ed56e276bc45f99ab39abe94e6cd7ac39fb402da47ad",
		"0014df105553444d",
	)
	require.NoError(t, err)
	usdcx, err := common.NewAssetClass(
		"1f3aec8bfe7ea4fe14c5f121e2a92e301afe414147860d557cac7e34",
		"5553444378",
	)
	require.NoError(t, err)

	t.Run("USDM swap", func(t *testing.T) {
		// Pool input 53c6a52f...#1 was consumed by
		// 1237c072... and replaced by output #1 at slot 193255027.
		previous := cswapActivityPool(
			usdm,
			4_517_941_431,
			785_087_730,
			193_253_135,
		)
		previous.TxHash = "53c6a52f402b9fd2d30ae76962010fe57a2a1cc30d6d286db815280c2563f5e6"
		previous.TxIndex = 1
		current := cswapActivityPool(
			usdm,
			4_579_285_253,
			774_654_393,
			193_255_027,
		)
		current.TxHash = "1237c072b8d283e3ccc3b9956502825f11533973b5402ed1ef1df459ffca8bfc"
		current.TxIndex = 1

		swap, ok, err := InferSwapTransition(previous, current)
		require.NoError(t, err)
		require.True(t, ok)
		require.True(t, swap.InputIsX)
		require.Equal(t, uint64(61_343_822), swap.AmountX)
		require.Equal(t, uint64(10_433_337), swap.AmountY)
	})

	t.Run("USDCx liquidity addition", func(t *testing.T) {
		// Pool input 74aa2e44...#1 was consumed by a24fd5df...; both
		// reserves increased, so it must not be counted as swap volume.
		previous := cswapActivityPool(
			usdcx,
			8_369_182_751,
			1_409_463_431,
			193_253_159,
		)
		previous.TxHash = "74aa2e4478ca051a43fc5be6c705271080120ab89b73d451e181ce56d556dea6"
		previous.TxIndex = 1
		current := cswapActivityPool(
			usdcx,
			8_547_275_688,
			1_439_463_431,
			193_253_908,
		)
		current.TxHash = "a24fd5df3faebb06ba0aa815890d5c7e3907c27f428c906b792d81321254a8d1"
		current.TxIndex = 1

		_, ok, err := InferSwapTransition(previous, current)
		require.NoError(t, err)
		require.False(t, ok)
	})
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

func cswapActivityPool(
	stablecoin common.AssetClass,
	adaReserve,
	stableReserve,
	slot uint64,
) *PoolState {
	return &PoolState{
		PoolId:   stablecoin.Fingerprint(),
		Network:  "mainnet",
		Protocol: "cswap",
		AssetX: common.AssetAmount{
			Class:  common.Lovelace(),
			Amount: adaReserve,
		},
		AssetY: common.AssetAmount{
			Class:  stablecoin,
			Amount: stableReserve,
		},
		Slot: slot,
	}
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
