// Copyright 2026 Blink Labs Software
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package djed

import (
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

var ErrNoCurrentObservation = errors.New(
	"djed: no authenticated unspent oracle observation",
)

var ErrConflictingObservation = errors.New(
	"djed: output reference has conflicting oracle observations",
)

var ErrInvalidTrackerState = errors.New(
	"djed: invalid persisted tracker state",
)

// OutputRef identifies an on-chain transaction output.
type OutputRef struct {
	TxHash  string
	TxIndex uint32
}

type trackedObservation struct {
	observation Observation
	spentAt     *uint64
}

// TrackedObservation is the persistence-safe state for one authenticated
// oracle output.
type TrackedObservation struct {
	Observation Observation `json:"observation"`
	SpentAt     *uint64     `json:"spentAtSlot,omitempty"`
}

// TrackerState is a complete snapshot of retained rollback history.
type TrackerState struct {
	Observations []TrackedObservation `json:"observations"`
}

// Tracker maintains Djed observations from a caller's local chain-sync stream.
// It keeps spent entries so a rollback can restore the preceding oracle UTxO.
type Tracker struct {
	mu           sync.RWMutex
	observations map[OutputRef]trackedObservation
}

// NewTracker creates an empty Djed oracle tracker.
func NewTracker() *Tracker {
	return &Tracker{
		observations: make(map[OutputRef]trackedObservation),
	}
}

// NewTrackerFromState restores a previously persisted tracker snapshot.
func NewTrackerFromState(state TrackerState) (*Tracker, error) {
	tracker := NewTracker()
	for _, persisted := range state.Observations {
		observation := persisted.Observation
		if err := validatePersistedObservation(observation); err != nil {
			return nil, err
		}
		if persisted.SpentAt != nil && *persisted.SpentAt < observation.Slot {
			return nil, fmt.Errorf(
				"%w: spend predates observation",
				ErrInvalidTrackerState,
			)
		}
		ref := OutputRef{
			TxHash:  observation.TxHash,
			TxIndex: observation.TxIndex,
		}
		if _, exists := tracker.observations[ref]; exists {
			return nil, fmt.Errorf(
				"%w: duplicate output reference",
				ErrInvalidTrackerState,
			)
		}
		var spentAt *uint64
		if persisted.SpentAt != nil {
			value := *persisted.SpentAt
			spentAt = &value
		}
		tracker.observations[ref] = trackedObservation{
			observation: observation,
			spentAt:     spentAt,
		}
	}
	return tracker, nil
}

// Apply validates and records a produced Djed oracle UTxO.
func (t *Tracker) Apply(
	data []byte,
	utxo OracleUTxO,
	now time.Time,
) (Observation, error) {
	observation, err := ParseMainnetObservation(data, utxo, now)
	if err != nil {
		return Observation{}, err
	}
	ref := OutputRef{TxHash: utxo.TxHash, TxIndex: utxo.TxIndex}
	t.mu.Lock()
	defer t.mu.Unlock()
	if existing, ok := t.observations[ref]; ok {
		if existing.observation != observation {
			return Observation{}, ErrConflictingObservation
		}
		// Chain replay may deliver the same output again. Preserve its spend
		// state rather than resurrecting an already-consumed UTxO.
		return observation, nil
	}
	t.observations[ref] = trackedObservation{observation: observation}
	return observation, nil
}

// ConsumeAt marks an oracle UTxO spent at the supplied chain slot and reports
// whether retained state changed.
func (t *Tracker) ConsumeAt(ref OutputRef, slot uint64) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	tracked, ok := t.observations[ref]
	if !ok {
		// A tracker started mid-chain may see a spend whose output predates
		// its local history. There is no state to update in that case.
		return false
	}
	if tracked.spentAt != nil && *tracked.spentAt == slot {
		return false
	}
	spentAt := slot
	tracked.spentAt = &spentAt
	t.observations[ref] = tracked
	return true
}

// Contains reports whether the output reference is retained by the tracker.
func (t *Tracker) Contains(ref OutputRef) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	_, ok := t.observations[ref]
	return ok
}

// Prune removes spent entries older than beforeSlot. Callers should advance
// beforeSlot only beyond their immutable rollback horizon. Entries spent at the
// boundary are retained.
func (t *Tracker) Prune(beforeSlot uint64) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	removed := 0
	for ref, tracked := range t.observations {
		if tracked.spentAt != nil && *tracked.spentAt < beforeSlot {
			delete(t.observations, ref)
			removed++
		}
	}
	return removed
}

// Rollback removes observations produced after the rollback point and restores
// observations whose spends occurred after it. State in the point's block is
// retained.
func (t *Tracker) Rollback(slot uint64) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	changed := false
	for ref, tracked := range t.observations {
		if tracked.observation.Slot > slot {
			delete(t.observations, ref)
			changed = true
			continue
		}
		if tracked.spentAt != nil && *tracked.spentAt > slot {
			tracked.spentAt = nil
			t.observations[ref] = tracked
			changed = true
		}
	}
	return changed
}

// Current returns the newest authenticated, unspent observation and checks its
// on-chain validity interval at the supplied time.
func (t *Tracker) Current(now time.Time) (Observation, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	var current Observation
	found := false
	var newestInvalid Observation
	var newestInvalidErr error
	sawUnspent := false
	for _, tracked := range t.observations {
		if tracked.spentAt != nil {
			continue
		}
		sawUnspent = true
		candidate := tracked.observation
		if err := candidate.ValidateAt(now); err != nil {
			if newestInvalidErr == nil ||
				observationAfter(candidate, newestInvalid) {
				newestInvalid = candidate
				newestInvalidErr = err
			}
			continue
		}
		if !found || observationAfter(candidate, current) {
			current = candidate
			found = true
		}
	}
	if found {
		return current, nil
	}
	if !sawUnspent {
		return Observation{}, ErrNoCurrentObservation
	}
	return Observation{}, newestInvalidErr
}

// Snapshot returns a deep persistence-safe copy of retained tracker state.
func (t *Tracker) Snapshot() TrackerState {
	t.mu.RLock()
	defer t.mu.RUnlock()

	state := TrackerState{
		Observations: make(
			[]TrackedObservation,
			0,
			len(t.observations),
		),
	}
	for _, tracked := range t.observations {
		var spentAt *uint64
		if tracked.spentAt != nil {
			value := *tracked.spentAt
			spentAt = &value
		}
		state.Observations = append(state.Observations, TrackedObservation{
			Observation: tracked.observation,
			SpentAt:     spentAt,
		})
	}
	sort.Slice(state.Observations, func(i, j int) bool {
		left := state.Observations[i].Observation
		right := state.Observations[j].Observation
		return observationAfter(right, left)
	})
	return state
}

func validatePersistedObservation(observation Observation) error {
	if observation.Pair != "ADA/USD" ||
		observation.Source != "djed" ||
		observation.PriceNumerator == 0 ||
		observation.PriceDenominator == 0 ||
		observation.TxHash == "" ||
		observation.ValidFrom.After(observation.ValidUntil) {
		return ErrInvalidTrackerState
	}
	return nil
}

func observationAfter(candidate, current Observation) bool {
	if candidate.Slot != current.Slot {
		return candidate.Slot > current.Slot
	}
	if candidate.TransactionIndex != current.TransactionIndex {
		return candidate.TransactionIndex > current.TransactionIndex
	}
	if candidate.TxIndex != current.TxIndex {
		return candidate.TxIndex > current.TxIndex
	}
	// With complete provenance, block transaction index and output index are
	// unique. The hash fallback is deterministic for incomplete caller data,
	// but does not claim to reconstruct chain order.
	return candidate.TxHash > current.TxHash
}
