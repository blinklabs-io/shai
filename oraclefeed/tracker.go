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

package oraclefeed

import (
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

var (
	ErrNoFreshObservation    = errors.New("oraclefeed: no fresh authenticated observation")
	ErrDivergentObservations = errors.New("oraclefeed: fresh observations diverge")
)

const DefaultMaxDivergence = 0.05

// OutputRef identifies an on-chain transaction output.
type OutputRef struct {
	TxHash  string
	TxIndex uint32
}

// SourceStatus explains whether the latest authenticated observation from a
// source is currently usable.
type SourceStatus struct {
	Source      string      `json:"source"`
	Pair        string      `json:"pair"`
	Observation Observation `json:"observation,omitempty"`
	Price       float64     `json:"price,omitempty"`
	Fresh       bool        `json:"fresh"`
	Error       string      `json:"error,omitempty"`
}

type configuredParser struct {
	parser Parser
	maxAge time.Duration
}

type trackedObservation struct {
	ref         OutputRef
	observation Observation
}

// Tracker maintains the unspent, authenticated observations seen through a
// caller's local chain-sync stream. It retains enough history to restore the
// preceding observation after a spend or rollback.
type Tracker struct {
	mu            sync.RWMutex
	parsers       []configuredParser
	byAddr        map[string]configuredParser
	utxos         map[string]map[OutputRef]Observation
	spent         map[string]map[OutputRef]uint64
	maxDivergence float64
}

// NewTracker returns a tracker for the supported mainnet ADA/USD deployments.
func NewTracker() *Tracker {
	return NewTrackerWithParsers(
		map[Parser]time.Duration{
			OrcfaxADAUSDParser{}:  2 * time.Hour,
			Charli3ADAUSDParser{}: 7 * time.Hour,
		},
	)
}

// NewTrackerWithParsers creates a tracker with caller-supplied freshness
// limits. It is primarily useful to add deployments without changing tracker
// mechanics and to construct deterministic tests.
func NewTrackerWithParsers(config map[Parser]time.Duration) *Tracker {
	ret := &Tracker{
		byAddr:        make(map[string]configuredParser, len(config)),
		utxos:         make(map[string]map[OutputRef]Observation, len(config)),
		spent:         make(map[string]map[OutputRef]uint64, len(config)),
		maxDivergence: DefaultMaxDivergence,
	}
	for parser, maxAge := range config {
		item := configuredParser{parser: parser, maxAge: maxAge}
		ret.parsers = append(ret.parsers, item)
		ret.byAddr[parser.Address()] = item
		ret.utxos[parser.Source()] = make(map[OutputRef]Observation)
		ret.spent[parser.Source()] = make(map[OutputRef]uint64)
	}
	sort.Slice(ret.parsers, func(i, j int) bool {
		return ret.parsers[i].parser.Source() < ret.parsers[j].parser.Source()
	})
	return ret
}

// Addresses returns the script addresses the tracker recognizes.
func (t *Tracker) Addresses() []string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	ret := make([]string, 0, len(t.byAddr))
	for address := range t.byAddr {
		ret = append(ret, address)
	}
	sort.Strings(ret)
	return ret
}

// TracksAddress reports whether an address belongs to a configured parser.
func (t *Tracker) TracksAddress(address string) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	_, ok := t.byAddr[address]
	return ok
}

// Apply authenticates a produced UTxO and adds its observation to the unspent
// set. The boolean is false for addresses not owned by a configured parser.
func (t *Tracker) Apply(utxo UTxO) (Observation, bool, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	configured, ok := t.byAddr[utxo.Address]
	if !ok {
		return Observation{}, false, nil
	}
	observation, err := configured.parser.Parse(utxo)
	if err != nil {
		return Observation{}, true, err
	}
	ref := OutputRef{TxHash: utxo.TxHash, TxIndex: utxo.TxIndex}
	t.utxos[configured.parser.Source()][ref] = observation
	delete(t.spent[configured.parser.Source()], ref)
	return observation, true, nil
}

// Consume removes a spent oracle UTxO.
func (t *Tracker) Consume(ref OutputRef) {
	t.ConsumeAt(ref, 0)
}

// ConsumeAt marks an oracle UTxO spent at a chain slot. Supplying the slot lets
// Rollback restore the UTxO if the spending block is rolled back.
func (t *Tracker) ConsumeAt(ref OutputRef, slot uint64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for source, observations := range t.utxos {
		if _, ok := observations[ref]; ok {
			t.spent[source][ref] = slot
		}
	}
}

// Rollback removes observations produced at or after the rollback slot.
func (t *Tracker) Rollback(slot uint64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for source, observations := range t.utxos {
		for ref, observation := range observations {
			if observation.Slot >= slot {
				delete(observations, ref)
				delete(t.spent[source], ref)
			}
		}
		for ref, spentAt := range t.spent[source] {
			if spentAt >= slot {
				delete(t.spent[source], ref)
			}
		}
	}
}

// Sources returns the latest authenticated observation and freshness state for
// every configured source.
func (t *Tracker) Sources(now time.Time) []SourceStatus {
	t.mu.RLock()
	defer t.mu.RUnlock()
	statuses := make([]SourceStatus, 0, len(t.parsers))
	for _, configured := range t.parsers {
		status := SourceStatus{
			Source: configured.parser.Source(),
			Pair:   configured.parser.Pair(),
		}
		latest, ok := latestObservation(
			t.utxos[configured.parser.Source()],
			t.spent[configured.parser.Source()],
		)
		if !ok {
			status.Error = "no authenticated unspent observation"
		} else {
			status.Observation = latest.observation
			status.Price = latest.observation.Float64()
			status.Fresh = latest.observation.FreshAt(now, configured.maxAge)
			if !status.Fresh {
				status.Error = "authenticated observation is stale"
			}
		}
		statuses = append(statuses, status)
	}
	return statuses
}

// ADAUSD returns the newest fresh authenticated ADA/USD observation.
func (t *Tracker) ADAUSD(now time.Time) (Observation, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	var candidates []Observation
	for _, configured := range t.parsers {
		if configured.parser.Pair() != PairADAUSD {
			continue
		}
		latest, ok := latestObservation(
			t.utxos[configured.parser.Source()],
			t.spent[configured.parser.Source()],
		)
		if !ok || !latest.observation.FreshAt(now, configured.maxAge) {
			continue
		}
		candidates = append(candidates, latest.observation)
	}
	if len(candidates) == 0 {
		return Observation{}, ErrNoFreshObservation
	}
	if observationsDiverge(candidates, t.maxDivergence) {
		return Observation{}, ErrDivergentObservations
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].ObservedAt.After(candidates[j].ObservedAt)
	})
	return candidates[0], nil
}

func observationsDiverge(observations []Observation, maximum float64) bool {
	if len(observations) < 2 {
		return false
	}
	minimum := observations[0].Float64()
	maximumValue := minimum
	for _, observation := range observations[1:] {
		value := observation.Float64()
		if value < minimum {
			minimum = value
		}
		if value > maximumValue {
			maximumValue = value
		}
	}
	return minimum <= 0 || (maximumValue-minimum)/minimum > maximum
}

func latestObservation(
	observations map[OutputRef]Observation,
	spent map[OutputRef]uint64,
) (trackedObservation, bool) {
	var latest trackedObservation
	found := false
	for ref, observation := range observations {
		if _, isSpent := spent[ref]; isSpent {
			continue
		}
		if !found || observation.ObservedAt.After(latest.observation.ObservedAt) {
			latest = trackedObservation{ref: ref, observation: observation}
			found = true
		}
	}
	return latest, found
}

// ValidateConfiguration reports ambiguous parser addresses and invalid
// freshness limits.
func (t *Tracker) ValidateConfiguration() error {
	t.mu.RLock()
	defer t.mu.RUnlock()
	seen := make(map[string]string, len(t.parsers))
	for _, configured := range t.parsers {
		if configured.maxAge <= 0 {
			return fmt.Errorf("%s: max age must be positive", configured.parser.Source())
		}
		if source, ok := seen[configured.parser.Address()]; ok {
			return fmt.Errorf(
				"address %s is shared by %s and %s",
				configured.parser.Address(),
				source,
				configured.parser.Source(),
			)
		}
		seen[configured.parser.Address()] = configured.parser.Source()
	}
	return nil
}
