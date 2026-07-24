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
	"sync"
	"time"
)

var ErrNoCurrentObservation = errors.New(
	"djed: no authenticated unspent oracle observation",
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
	t.observations[ref] = trackedObservation{observation: observation}
	return observation, nil
}

// ConsumeAt marks an oracle UTxO spent at the supplied chain slot.
func (t *Tracker) ConsumeAt(ref OutputRef, slot uint64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	tracked, ok := t.observations[ref]
	if !ok {
		return
	}
	spentAt := slot
	tracked.spentAt = &spentAt
	t.observations[ref] = tracked
}

// Rollback removes observations produced at or after the rollback slot and
// restores observations whose spends were rolled back.
func (t *Tracker) Rollback(slot uint64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for ref, tracked := range t.observations {
		if tracked.observation.Slot >= slot {
			delete(t.observations, ref)
			continue
		}
		if tracked.spentAt != nil && *tracked.spentAt >= slot {
			tracked.spentAt = nil
			t.observations[ref] = tracked
		}
	}
}

// Current returns the newest authenticated, unspent observation and checks its
// on-chain validity interval at the supplied time.
func (t *Tracker) Current(now time.Time) (Observation, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	var current Observation
	found := false
	for _, tracked := range t.observations {
		if tracked.spentAt != nil {
			continue
		}
		candidate := tracked.observation
		if !found || observationAfter(candidate, current) {
			current = candidate
			found = true
		}
	}
	if !found {
		return Observation{}, ErrNoCurrentObservation
	}
	if err := current.ValidateAt(now); err != nil {
		return current, err
	}
	return current, nil
}

func observationAfter(candidate, current Observation) bool {
	if candidate.Slot != current.Slot {
		return candidate.Slot > current.Slot
	}
	if candidate.TxIndex != current.TxIndex {
		return candidate.TxIndex > current.TxIndex
	}
	return candidate.TxHash > current.TxHash
}
