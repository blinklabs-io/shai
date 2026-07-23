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
	"fmt"
	"sync"

	"github.com/blinklabs-io/shai/common"
)

var (
	ErrInvalidActivityWindow = errors.New(
		"dex: activity window must be positive",
	)
	ErrMismatchedPoolTransition = errors.New(
		"dex: pool transition identity or assets changed",
	)
	ErrOutOfOrderActivity = errors.New(
		"dex: pool activity arrived before the latest observed slot",
	)
	ErrVolumeOverflow = errors.New("dex: pool volume overflows uint64")
)

// SwapTransition is a confirmed reserve transition that has the shape of a
// swap: one reserve increased while the other decreased. Amounts are kept in
// each asset's native on-chain units.
type SwapTransition struct {
	PoolID    string            `json:"poolId"`
	Network   string            `json:"network"`
	Protocol  string            `json:"protocol"`
	AssetX    common.AssetClass `json:"assetX"`
	AssetY    common.AssetClass `json:"assetY"`
	AmountX   uint64            `json:"amountX"`
	AmountY   uint64            `json:"amountY"`
	InputIsX  bool              `json:"inputIsX"`
	Slot      uint64            `json:"slot"`
	BlockHash string            `json:"blockHash"`
	TxHash    string            `json:"txHash"`
	TxIndex   uint32            `json:"txIndex"`
}

// PoolVolume is the exact net reserve turnover inferred from swap-shaped pool
// transitions over a slot window. It does not claim to measure gross volume
// when multiple actions are batched into one pool-state transition.
type PoolVolume struct {
	PoolID        string            `json:"poolId"`
	Network       string            `json:"network"`
	Protocol      string            `json:"protocol"`
	AssetX        common.AssetClass `json:"assetX"`
	AssetY        common.AssetClass `json:"assetY"`
	VolumeX       uint64            `json:"volumeX"`
	VolumeY       uint64            `json:"volumeY"`
	SwapCount     uint64            `json:"swapCount"`
	WindowSlots   uint64            `json:"windowSlots"`
	WindowEnd     uint64            `json:"windowEndSlot"`
	FirstSwapSlot uint64            `json:"firstSwapSlot,omitempty"`
	LastSwapSlot  uint64            `json:"lastSwapSlot,omitempty"`
}

// ActivityTracker retains confirmed swap-shaped pool transitions over a
// bounded slot window. Slot windows make historical chain sync deterministic
// and avoid mistaking sync wall-clock time for trading time.
type ActivityTracker struct {
	mu          sync.Mutex
	windowSlots uint64
	latestSlot  uint64
	swaps       map[string][]SwapTransition
}

// NewActivityTracker creates a rolling pool-activity tracker.
func NewActivityTracker(windowSlots uint64) (*ActivityTracker, error) {
	if windowSlots == 0 {
		return nil, ErrInvalidActivityWindow
	}
	return &ActivityTracker{
		windowSlots: windowSlots,
		swaps:       make(map[string][]SwapTransition),
	}, nil
}

// InferSwapTransition classifies a pair of confirmed pool states. Deposits,
// withdrawals, and fee-only/no-op changes are excluded because their reserves
// move in the same direction or do not move.
func InferSwapTransition(
	previous,
	current *PoolState,
) (SwapTransition, bool, error) {
	if previous == nil || current == nil {
		return SwapTransition{}, false, nil
	}
	if previous.FromMempool || current.FromMempool {
		return SwapTransition{}, false, nil
	}
	if previous.Key() != current.Key() ||
		!previous.AssetX.IsAsset(current.AssetX.Class) ||
		!previous.AssetY.IsAsset(current.AssetY.Class) {
		return SwapTransition{}, false, ErrMismatchedPoolTransition
	}

	xIn := current.AssetX.Amount > previous.AssetX.Amount
	xOut := current.AssetX.Amount < previous.AssetX.Amount
	yIn := current.AssetY.Amount > previous.AssetY.Amount
	yOut := current.AssetY.Amount < previous.AssetY.Amount
	isSwap := (xIn && yOut) || (xOut && yIn)
	if !isSwap {
		return SwapTransition{}, false, nil
	}

	transition := SwapTransition{
		PoolID:    current.PoolId,
		Network:   current.Network,
		Protocol:  current.Protocol,
		AssetX:    current.AssetX.Class,
		AssetY:    current.AssetY.Class,
		InputIsX:  xIn,
		Slot:      current.Slot,
		BlockHash: current.BlockHash,
		TxHash:    current.TxHash,
		TxIndex:   current.TxIndex,
	}
	if xIn {
		transition.AmountX = current.AssetX.Amount - previous.AssetX.Amount
		transition.AmountY = previous.AssetY.Amount - current.AssetY.Amount
	} else {
		transition.AmountX = previous.AssetX.Amount - current.AssetX.Amount
		transition.AmountY = current.AssetY.Amount - previous.AssetY.Amount
	}
	return transition, true, nil
}

// Observe records a confirmed swap-shaped transition and advances the rolling
// window even when the transition is a liquidity event rather than a swap.
func (t *ActivityTracker) Observe(
	previous,
	current *PoolState,
) (bool, error) {
	if current == nil || current.FromMempool {
		return false, nil
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	if current.Slot < t.latestSlot {
		return false, ErrOutOfOrderActivity
	}
	t.latestSlot = current.Slot
	t.prune(current.Slot)

	transition, ok, err := InferSwapTransition(previous, current)
	if err != nil || !ok {
		return false, err
	}
	key := current.Key()
	t.swaps[key] = append(t.swaps[key], transition)
	return true, nil
}

// Volume returns net reserve turnover for a pool at the requested slot.
func (t *ActivityTracker) Volume(
	network,
	protocol,
	poolID string,
	atSlot uint64,
) (PoolVolume, bool, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if atSlot < t.latestSlot {
		return PoolVolume{}, false, ErrOutOfOrderActivity
	}

	key := fmt.Sprintf("%s:%s:%s", network, protocol, poolID)
	swaps := t.swaps[key]
	var cutoff uint64
	if atSlot > t.windowSlots {
		cutoff = atSlot - t.windowSlots
	}
	volume := PoolVolume{
		PoolID:      poolID,
		Network:     network,
		Protocol:    protocol,
		WindowSlots: t.windowSlots,
		WindowEnd:   atSlot,
	}
	for _, swap := range swaps {
		if swap.Slot < cutoff {
			continue
		}
		if ^uint64(0)-volume.VolumeX < swap.AmountX ||
			^uint64(0)-volume.VolumeY < swap.AmountY ||
			volume.SwapCount == ^uint64(0) {
			return PoolVolume{}, false, ErrVolumeOverflow
		}
		if volume.SwapCount == 0 {
			volume.AssetX = swap.AssetX
			volume.AssetY = swap.AssetY
			volume.FirstSwapSlot = swap.Slot
		}
		volume.VolumeX += swap.AmountX
		volume.VolumeY += swap.AmountY
		volume.SwapCount++
		volume.LastSwapSlot = swap.Slot
	}
	if volume.SwapCount == 0 {
		return PoolVolume{}, false, nil
	}
	return volume, true, nil
}

// Rollback removes activity at or after the rollback slot.
func (t *ActivityTracker) Rollback(slot uint64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for key, swaps := range t.swaps {
		keep := len(swaps)
		for keep > 0 && swaps[keep-1].Slot >= slot {
			keep--
		}
		if keep == 0 {
			delete(t.swaps, key)
			continue
		}
		t.swaps[key] = swaps[:keep]
	}
	t.latestSlot = 0
	if slot > 0 {
		t.latestSlot = slot - 1
	}
	t.prune(t.latestSlot)
}

func (t *ActivityTracker) prune(atSlot uint64) {
	var cutoff uint64
	if atSlot > t.windowSlots {
		cutoff = atSlot - t.windowSlots
	}
	for key, swaps := range t.swaps {
		first := 0
		for first < len(swaps) && swaps[first].Slot < cutoff {
			first++
		}
		if first == len(swaps) {
			delete(t.swaps, key)
			continue
		}
		if first > 0 {
			t.swaps[key] = append(
				[]SwapTransition(nil),
				swaps[first:]...,
			)
		}
	}
}
