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
	"bytes"
	"errors"
	"sort"
)

var (
	ErrInvalidActivityState  = errors.New("dex: invalid persisted activity state")
	ErrActivityWindowChanged = errors.New(
		"dex: persisted activity window does not match tracker",
	)
)

// ActivityState is the durable representation of a rolling activity tracker.
type ActivityState struct {
	WindowSlots uint64           `json:"windowSlots"`
	LatestSlot  uint64           `json:"latestSlot"`
	Swaps       []SwapTransition `json:"swaps"`
}

// ObserveTransition records a confirmed state and returns the inferred swap
// when one was added. It provides callers the exact value to persist.
func (t *ActivityTracker) ObserveTransition(
	previous,
	current *PoolState,
) (SwapTransition, bool, error) {
	if current == nil || current.FromMempool {
		return SwapTransition{}, false, nil
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	if current.Slot < t.latestSlot {
		return SwapTransition{}, false, ErrOutOfOrderActivity
	}

	// Every rejection is checked before the tracker is mutated. A caller that
	// gets an error abandons the state, so advancing the latest slot or pruning
	// first would move the window for a state that was never accepted.
	transition, ok, err := InferSwapTransition(previous, current)
	if err != nil {
		return SwapTransition{}, false, err
	}
	key := current.Key()
	var duplicate bool
	if ok {
		for _, existing := range t.swaps[key] {
			if existing.Slot != transition.Slot ||
				!sameSwapIdentity(existing, transition) {
				continue
			}
			if !equalSwapTransition(existing, transition) {
				return SwapTransition{}, false, ErrInvalidActivityState
			}
			duplicate = true
			break
		}
	}

	t.latestSlot = current.Slot
	t.prune(current.Slot)
	if !ok {
		return SwapTransition{}, false, nil
	}
	if duplicate {
		return cloneSwapTransition(transition), false, nil
	}
	// The retained slice is re-read because pruning may have replaced it.
	t.swaps[key] = append(t.swaps[key], cloneSwapTransition(transition))
	return cloneSwapTransition(transition), true, nil
}

// WindowSlots returns the tracker's configured rolling window.
func (t *ActivityTracker) WindowSlots() uint64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.windowSlots
}

// Snapshot returns a deep copy suitable for durable storage.
func (t *ActivityTracker) Snapshot() ActivityState {
	t.mu.Lock()
	defer t.mu.Unlock()

	state := ActivityState{
		WindowSlots: t.windowSlots,
		LatestSlot:  t.latestSlot,
	}
	for _, swaps := range t.swaps {
		for _, swap := range swaps {
			state.Swaps = append(state.Swaps, cloneSwapTransition(swap))
		}
	}
	sortSwapTransitions(state.Swaps)
	return state
}

// Restore replaces the tracker contents with validated durable state. The
// configured window must match so a restart cannot silently change retention
// and therefore the reported volume.
func (t *ActivityTracker) Restore(state ActivityState) error {
	if state.WindowSlots == 0 {
		return ErrInvalidActivityState
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	if state.WindowSlots != t.windowSlots {
		return ErrActivityWindowChanged
	}

	swaps := make([]SwapTransition, len(state.Swaps))
	for i, swap := range state.Swaps {
		if err := validateSwapTransition(swap, state.LatestSlot); err != nil {
			return err
		}
		swaps[i] = cloneSwapTransition(swap)
	}
	sortSwapTransitions(swaps)

	restored := make(map[string][]SwapTransition)
	for _, swap := range swaps {
		pool := PoolState{
			PoolId:   swap.PoolID,
			Network:  swap.Network,
			Protocol: swap.Protocol,
		}
		key := pool.Key()
		poolSwaps := restored[key]
		if len(poolSwaps) > 0 &&
			sameSwapIdentity(poolSwaps[len(poolSwaps)-1], swap) {
			return ErrInvalidActivityState
		}
		restored[key] = append(poolSwaps, swap)
	}

	t.latestSlot = state.LatestSlot
	t.swaps = restored
	t.prune(state.LatestSlot)
	return nil
}

func validateSwapTransition(swap SwapTransition, latestSlot uint64) error {
	if swap.PoolID == "" ||
		swap.Network == "" ||
		swap.Protocol == "" ||
		swap.AmountX == 0 ||
		swap.AmountY == 0 ||
		swap.Slot > latestSlot ||
		(bytes.Equal(swap.AssetX.PolicyId, swap.AssetY.PolicyId) &&
			bytes.Equal(swap.AssetX.Name, swap.AssetY.Name)) {
		return ErrInvalidActivityState
	}
	return nil
}

func cloneSwapTransition(swap SwapTransition) SwapTransition {
	swap.AssetX.PolicyId = append([]byte(nil), swap.AssetX.PolicyId...)
	swap.AssetX.Name = append([]byte(nil), swap.AssetX.Name...)
	swap.AssetY.PolicyId = append([]byte(nil), swap.AssetY.PolicyId...)
	swap.AssetY.Name = append([]byte(nil), swap.AssetY.Name...)
	return swap
}

func sortSwapTransitions(swaps []SwapTransition) {
	sort.Slice(swaps, func(i, j int) bool {
		if swaps[i].Slot != swaps[j].Slot {
			return swaps[i].Slot < swaps[j].Slot
		}
		if swaps[i].TxHash != swaps[j].TxHash {
			return swaps[i].TxHash < swaps[j].TxHash
		}
		if swaps[i].TxIndex != swaps[j].TxIndex {
			return swaps[i].TxIndex < swaps[j].TxIndex
		}
		if swaps[i].Network != swaps[j].Network {
			return swaps[i].Network < swaps[j].Network
		}
		if swaps[i].Protocol != swaps[j].Protocol {
			return swaps[i].Protocol < swaps[j].Protocol
		}
		return swaps[i].PoolID < swaps[j].PoolID
	})
}

func sameSwapIdentity(a, b SwapTransition) bool {
	return a.Slot == b.Slot &&
		a.TxHash == b.TxHash &&
		a.TxIndex == b.TxIndex &&
		a.Network == b.Network &&
		a.Protocol == b.Protocol &&
		a.PoolID == b.PoolID
}

func equalSwapTransition(a, b SwapTransition) bool {
	return sameSwapIdentity(a, b) &&
		a.AmountX == b.AmountX &&
		a.AmountY == b.AmountY &&
		a.InputIsX == b.InputIsX &&
		a.BlockHash == b.BlockHash &&
		bytes.Equal(a.AssetX.PolicyId, b.AssetX.PolicyId) &&
		bytes.Equal(a.AssetX.Name, b.AssetX.Name) &&
		bytes.Equal(a.AssetY.PolicyId, b.AssetY.PolicyId) &&
		bytes.Equal(a.AssetY.Name, b.AssetY.Name)
}
