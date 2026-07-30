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

package price

import (
	"errors"
	"fmt"
	"math/big"
	"sync"
	"time"
)

var (
	ErrInvalidTWAPConfig      = errors.New("price: invalid TWAP configuration")
	ErrInvalidTWAPObservation = errors.New(
		"price: invalid TWAP observation",
	)
	ErrOutOfOrderTWAP          = errors.New("price: TWAP observation is out of order")
	ErrInsufficientTWAPHistory = errors.New(
		"price: insufficient TWAP history",
	)
	ErrStaleTWAP               = errors.New("price: TWAP observation is stale")
	ErrTWAPRollbackUnavailable = errors.New(
		"price: TWAP rollback history is unavailable",
	)
	ErrInvalidTWAPState = errors.New("price: invalid persisted TWAP state")
)

// TWAPConfig controls the time-weighted average and retained history.
type TWAPConfig struct {
	Window          time.Duration
	MinCoverage     time.Duration
	MaxStaleness    time.Duration
	MaxObservations int
}

// TWAPResult is an exact time-weighted average over the reported coverage.
type TWAPResult struct {
	PriceNum         string        `json:"priceNumerator"`
	PriceDen         string        `json:"priceDenominator"`
	Price            float64       `json:"price"`
	WindowStart      time.Time     `json:"windowStart"`
	WindowEnd        time.Time     `json:"windowEnd"`
	Coverage         time.Duration `json:"coverage"`
	ObservationCount int           `json:"observationCount"`
	LastSlot         uint64        `json:"lastSlot"`
	LastObservedAt   time.Time     `json:"lastObservedAt"`

	price *big.Rat
}

// TWAPObservation is the persistence-safe representation of one exact price
// observation.
type TWAPObservation struct {
	Slot     uint64    `json:"slot"`
	At       time.Time `json:"observedAt"`
	PriceNum string    `json:"priceNumerator"`
	PriceDen string    `json:"priceDenominator"`
}

// TWAPState is a complete bounded snapshot that can restore an engine after a
// restart without weakening rollback detection.
type TWAPState struct {
	Config        TWAPConfig        `json:"config"`
	Observations  []TWAPObservation `json:"observations"`
	HistoryPruned bool              `json:"historyPruned"`
}

// Rat returns a copy of the exact time-weighted average.
func (r TWAPResult) Rat() *big.Rat {
	if r.price == nil {
		return new(big.Rat)
	}
	return new(big.Rat).Set(r.price)
}

type twapObservation struct {
	slot  uint64
	at    time.Time
	price *big.Rat
}

// TWAPEngine retains a bounded, rollback-aware sequence of locally supplied
// exact price observations. Prices use step-function weighting: an observation
// remains in effect until the next observation.
type TWAPEngine struct {
	mu            sync.RWMutex
	config        TWAPConfig
	observations  []twapObservation
	historyPruned bool
}

// NewTWAPEngine creates an empty local TWAP engine.
func NewTWAPEngine(config TWAPConfig) (*TWAPEngine, error) {
	if config.Window <= 0 ||
		config.MinCoverage <= 0 ||
		config.MinCoverage > config.Window ||
		config.MaxStaleness < 0 ||
		config.MaxObservations < 1 {
		return nil, ErrInvalidTWAPConfig
	}
	return &TWAPEngine{config: config}, nil
}

// NewTWAPEngineFromState restores a previously persisted engine snapshot.
func NewTWAPEngineFromState(state TWAPState) (*TWAPEngine, error) {
	engine, err := NewTWAPEngine(state.Config)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidTWAPState, err)
	}
	engine.historyPruned = state.HistoryPruned
	for _, persisted := range state.Observations {
		numerator, ok := new(big.Int).SetString(persisted.PriceNum, 10)
		if !ok {
			return nil, fmt.Errorf(
				"%w: invalid price numerator",
				ErrInvalidTWAPState,
			)
		}
		denominator, ok := new(big.Int).SetString(persisted.PriceDen, 10)
		if !ok || denominator.Sign() <= 0 {
			return nil, fmt.Errorf(
				"%w: invalid price denominator",
				ErrInvalidTWAPState,
			)
		}
		value := new(big.Rat).SetFrac(numerator, denominator)
		if err := engine.Observe(persisted.Slot, persisted.At, value); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidTWAPState, err)
		}
	}
	// Observe may prune while validating a snapshot. A valid persisted state is
	// already bounded, so any additional pruning means it was inconsistent
	// with its own configuration.
	if len(engine.observations) != len(state.Observations) {
		return nil, fmt.Errorf(
			"%w: observations exceed configured retention",
			ErrInvalidTWAPState,
		)
	}
	engine.historyPruned = state.HistoryPruned
	return engine, nil
}

// Observe adds an exact confirmed observation. Slots and timestamps must both
// increase, making replay and rollback behavior deterministic.
func (e *TWAPEngine) Observe(
	slot uint64,
	observedAt time.Time,
	value *big.Rat,
) error {
	if observedAt.IsZero() || value == nil || value.Sign() <= 0 {
		return ErrInvalidTWAPObservation
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.observations) > 0 {
		latest := e.observations[len(e.observations)-1]
		if slot <= latest.slot || !observedAt.After(latest.at) {
			return ErrOutOfOrderTWAP
		}
	}
	e.observations = append(e.observations, twapObservation{
		slot:  slot,
		at:    observedAt,
		price: new(big.Rat).Set(value),
	})
	e.prune(observedAt)
	return nil
}

// TWAPAt calculates the average at an explicit time. Supplying the evaluation
// time keeps historical replay and tests independent of the wall clock.
func (e *TWAPEngine) TWAPAt(now time.Time) (TWAPResult, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	end := len(e.observations)
	for end > 0 && e.observations[end-1].at.After(now) {
		end--
	}
	if end == 0 {
		return TWAPResult{}, ErrInsufficientTWAPHistory
	}
	observations := e.observations[:end]
	latest := observations[len(observations)-1]
	if now.Sub(latest.at) > e.config.MaxStaleness {
		return TWAPResult{}, ErrStaleTWAP
	}

	windowStart := now.Add(-e.config.Window)
	start := 0
	for start < len(observations) &&
		observations[start].at.Before(windowStart) {
		start++
	}
	if start > 0 {
		// Retain the newest observation before the window as its price anchor.
		start--
	}
	observations = observations[start:]

	coverageStart := windowStart
	if observations[0].at.After(windowStart) {
		coverageStart = observations[0].at
	}
	coverage := now.Sub(coverageStart)
	if coverage < e.config.MinCoverage {
		return TWAPResult{}, ErrInsufficientTWAPHistory
	}

	current := observations[0]
	intervalStart := coverageStart
	if current.at.After(intervalStart) {
		intervalStart = current.at
	}
	weighted := new(big.Rat)
	count := 1
	for _, observation := range observations[1:] {
		if !observation.at.After(intervalStart) {
			current = observation
			continue
		}
		addWeightedPrice(weighted, current.price, observation.at.Sub(intervalStart))
		current = observation
		intervalStart = observation.at
		count++
	}
	addWeightedPrice(weighted, current.price, now.Sub(intervalStart))
	weighted.Quo(
		weighted,
		new(big.Rat).SetInt64(int64(coverage)),
	)
	value, err := finiteFloat64(weighted)
	if err != nil {
		return TWAPResult{}, err
	}
	return TWAPResult{
		PriceNum:         weighted.Num().String(),
		PriceDen:         weighted.Denom().String(),
		Price:            value,
		WindowStart:      windowStart,
		WindowEnd:        now,
		Coverage:         coverage,
		ObservationCount: count,
		LastSlot:         latest.slot,
		LastObservedAt:   latest.at,
		price:            weighted,
	}, nil
}

// Rollback removes observations after the supplied chain slot. It returns an
// error when bounded-history pruning has removed state needed for the rollback.
func (e *TWAPEngine) Rollback(slot uint64) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.observations) > 0 &&
		slot >= e.observations[len(e.observations)-1].slot {
		return nil
	}
	if e.historyPruned &&
		(len(e.observations) == 0 ||
			slot < e.observations[0].slot) {
		return ErrTWAPRollbackUnavailable
	}
	end := len(e.observations)
	for end > 0 && e.observations[end-1].slot > slot {
		end--
	}
	e.observations = e.observations[:end]
	return nil
}

// Len returns the number of retained observations.
func (e *TWAPEngine) Len() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.observations)
}

// Snapshot returns a deep persistence-safe copy of the bounded engine state.
func (e *TWAPEngine) Snapshot() TWAPState {
	e.mu.RLock()
	defer e.mu.RUnlock()

	state := TWAPState{
		Config:        e.config,
		Observations:  make([]TWAPObservation, 0, len(e.observations)),
		HistoryPruned: e.historyPruned,
	}
	for _, observation := range e.observations {
		state.Observations = append(state.Observations, TWAPObservation{
			Slot:     observation.slot,
			At:       observation.at,
			PriceNum: observation.price.Num().String(),
			PriceDen: observation.price.Denom().String(),
		})
	}
	return state
}

func (e *TWAPEngine) prune(latest time.Time) {
	cutoff := latest.Add(-e.config.Window)
	firstAfter := 0
	for firstAfter < len(e.observations) &&
		!e.observations[firstAfter].at.After(cutoff) {
		firstAfter++
	}
	keepFrom := 0
	if firstAfter > 0 {
		keepFrom = firstAfter - 1
	}
	if remaining := len(e.observations) - keepFrom; remaining > e.config.MaxObservations {
		keepFrom += remaining - e.config.MaxObservations
	}
	if keepFrom == 0 {
		return
	}
	e.notePruned()
	copy(e.observations, e.observations[keepFrom:])
	e.observations = e.observations[:len(e.observations)-keepFrom]
}

func (e *TWAPEngine) notePruned() {
	e.historyPruned = true
}

func addWeightedPrice(total, value *big.Rat, duration time.Duration) {
	if duration <= 0 {
		return
	}
	term := new(big.Rat).Mul(
		value,
		new(big.Rat).SetInt64(int64(duration)),
	)
	total.Add(total, term)
}
