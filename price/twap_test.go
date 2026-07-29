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
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTWAPExactFullWindow(t *testing.T) {
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	engine := newTestTWAP(t, TWAPConfig{
		Window:          10 * time.Minute,
		MinCoverage:     10 * time.Minute,
		MaxStaleness:    10 * time.Minute,
		MaxObservations: 10,
	})
	require.NoError(t, engine.Observe(
		1,
		now.Add(-15*time.Minute),
		big.NewRat(2, 1),
	))
	require.NoError(t, engine.Observe(
		2,
		now.Add(-5*time.Minute),
		big.NewRat(4, 1),
	))

	result, err := engine.TWAPAt(now)
	require.NoError(t, err)
	require.Equal(t, big.NewRat(3, 1), result.Rat())
	require.Equal(t, "3", result.PriceNum)
	require.Equal(t, "1", result.PriceDen)
	require.Equal(t, 3.0, result.Price)
	require.Equal(t, now.Add(-10*time.Minute), result.WindowStart)
	require.Equal(t, now, result.WindowEnd)
	require.Equal(t, 10*time.Minute, result.Coverage)
	require.Equal(t, 2, result.ObservationCount)
	require.Equal(t, uint64(2), result.LastSlot)
	require.Equal(t, now.Add(-5*time.Minute), result.LastObservedAt)
}

func TestTWAPUsesExactRationalArithmetic(t *testing.T) {
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	engine := newTestTWAP(t, TWAPConfig{
		Window:          3 * time.Second,
		MinCoverage:     3 * time.Second,
		MaxStaleness:    3 * time.Second,
		MaxObservations: 10,
	})
	require.NoError(t, engine.Observe(
		1,
		now.Add(-3*time.Second),
		big.NewRat(1, 3),
	))
	require.NoError(t, engine.Observe(
		2,
		now.Add(-2*time.Second),
		big.NewRat(2, 3),
	))

	result, err := engine.TWAPAt(now)
	require.NoError(t, err)
	require.Equal(t, big.NewRat(5, 9), result.Rat())
	require.Equal(t, "5", result.PriceNum)
	require.Equal(t, "9", result.PriceDen)
}

func TestTWAPPartialCoverageQualification(t *testing.T) {
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	config := TWAPConfig{
		Window:          10 * time.Minute,
		MinCoverage:     5 * time.Minute,
		MaxStaleness:    5 * time.Minute,
		MaxObservations: 10,
	}
	engine := newTestTWAP(t, config)
	require.NoError(t, engine.Observe(
		1,
		now.Add(-6*time.Minute),
		big.NewRat(2, 1),
	))
	require.NoError(t, engine.Observe(
		2,
		now.Add(-3*time.Minute),
		big.NewRat(4, 1),
	))

	result, err := engine.TWAPAt(now)
	require.NoError(t, err)
	require.Equal(t, 6*time.Minute, result.Coverage)
	require.Equal(t, big.NewRat(3, 1), result.Rat())
}

func TestTWAPRejectsInsufficientHistory(t *testing.T) {
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	engine := newTestTWAP(t, TWAPConfig{
		Window:          10 * time.Minute,
		MinCoverage:     5 * time.Minute,
		MaxStaleness:    10 * time.Minute,
		MaxObservations: 10,
	})

	_, err := engine.TWAPAt(now)
	require.ErrorIs(t, err, ErrInsufficientTWAPHistory)
	require.NoError(t, engine.Observe(
		1,
		now.Add(-4*time.Minute),
		big.NewRat(1, 1),
	))
	_, err = engine.TWAPAt(now)
	require.ErrorIs(t, err, ErrInsufficientTWAPHistory)
}

func TestTWAPRejectsStaleLatestObservation(t *testing.T) {
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	engine := newTestTWAP(t, TWAPConfig{
		Window:          10 * time.Minute,
		MinCoverage:     5 * time.Minute,
		MaxStaleness:    time.Minute,
		MaxObservations: 10,
	})
	require.NoError(t, engine.Observe(
		1,
		now.Add(-5*time.Minute),
		big.NewRat(1, 1),
	))

	_, err := engine.TWAPAt(now)
	require.ErrorIs(t, err, ErrStaleTWAP)
}

func TestTWAPAcceptsStalenessBoundary(t *testing.T) {
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	engine := newTestTWAP(t, TWAPConfig{
		Window:          10 * time.Minute,
		MinCoverage:     5 * time.Minute,
		MaxStaleness:    5 * time.Minute,
		MaxObservations: 10,
	})
	require.NoError(t, engine.Observe(
		1,
		now.Add(-5*time.Minute),
		big.NewRat(1, 1),
	))

	_, err := engine.TWAPAt(now)
	require.NoError(t, err)
}

func TestTWAPCopiesPrices(t *testing.T) {
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	engine := newTestTWAP(t, TWAPConfig{
		Window:          time.Minute,
		MinCoverage:     time.Minute,
		MaxStaleness:    time.Minute,
		MaxObservations: 2,
	})
	input := big.NewRat(1, 2)
	require.NoError(t, engine.Observe(1, now.Add(-time.Minute), input))
	input.SetInt64(100)

	result, err := engine.TWAPAt(now)
	require.NoError(t, err)
	require.Equal(t, big.NewRat(1, 2), result.Rat())
	copyOfResult := result.Rat()
	copyOfResult.SetInt64(200)
	require.Equal(t, big.NewRat(1, 2), result.Rat())
}

func TestTWAPRejectsInvalidAndOutOfOrderObservations(t *testing.T) {
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	engine := newTestTWAP(t, validTWAPConfig())
	require.ErrorIs(
		t,
		engine.Observe(1, time.Time{}, big.NewRat(1, 1)),
		ErrInvalidTWAPObservation,
	)
	require.ErrorIs(
		t,
		engine.Observe(1, now, nil),
		ErrInvalidTWAPObservation,
	)
	require.ErrorIs(
		t,
		engine.Observe(1, now, big.NewRat(0, 1)),
		ErrInvalidTWAPObservation,
	)
	require.NoError(t, engine.Observe(2, now, big.NewRat(1, 1)))
	require.ErrorIs(
		t,
		engine.Observe(2, now.Add(time.Second), big.NewRat(1, 1)),
		ErrOutOfOrderTWAP,
	)
	require.ErrorIs(
		t,
		engine.Observe(3, now, big.NewRat(1, 1)),
		ErrOutOfOrderTWAP,
	)
}

func TestTWAPBoundedHistoryAndRollback(t *testing.T) {
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	config := validTWAPConfig()
	config.MaxObservations = 3
	engine := newTestTWAP(t, config)
	slots := []uint64{1, 100, 101, 102}
	for i, slot := range slots {
		require.NoError(t, engine.Observe(
			slot,
			now.Add(time.Duration(i+1)*time.Second),
			big.NewRat(int64(i+1), 1),
		))
	}
	require.Equal(t, 3, engine.Len())
	require.ErrorIs(t, engine.Rollback(50), ErrTWAPRollbackUnavailable)
	require.Equal(t, 3, engine.Len())
	require.NoError(t, engine.Rollback(100))
	require.Equal(t, 1, engine.Len())

	require.NoError(t, engine.Observe(
		101,
		now.Add(5*time.Second),
		big.NewRat(30, 1),
	))
	require.Equal(t, 2, engine.Len())
}

func TestTWAPTimePruningPreservesWindowAnchor(t *testing.T) {
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	engine := newTestTWAP(t, TWAPConfig{
		Window:          10 * time.Minute,
		MinCoverage:     10 * time.Minute,
		MaxStaleness:    time.Minute,
		MaxObservations: 10,
	})
	require.NoError(t, engine.Observe(
		1,
		now.Add(-30*time.Minute),
		big.NewRat(1, 1),
	))
	require.NoError(t, engine.Observe(
		2,
		now.Add(-20*time.Minute),
		big.NewRat(2, 1),
	))
	require.NoError(t, engine.Observe(
		3,
		now.Add(-5*time.Minute),
		big.NewRat(3, 1),
	))
	require.NoError(t, engine.Observe(4, now, big.NewRat(4, 1)))
	require.Equal(t, 3, engine.Len())

	result, err := engine.TWAPAt(now)
	require.NoError(t, err)
	require.Equal(t, big.NewRat(5, 2), result.Rat())
	require.Equal(t, 3, result.ObservationCount)
}

func TestTWAPHistoricalEvaluationIgnoresFutureObservations(t *testing.T) {
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	engine := newTestTWAP(t, validTWAPConfig())
	require.NoError(t, engine.Observe(
		1,
		now.Add(-time.Second),
		big.NewRat(1, 1),
	))
	require.NoError(t, engine.Observe(
		2,
		now.Add(time.Second),
		big.NewRat(100, 1),
	))

	result, err := engine.TWAPAt(now)
	require.NoError(t, err)
	require.Equal(t, big.NewRat(1, 1), result.Rat())
	require.Equal(t, uint64(1), result.LastSlot)
}

func TestNewTWAPEngineRejectsInvalidConfig(t *testing.T) {
	tests := []TWAPConfig{
		{},
		{
			Window:          time.Second,
			MinCoverage:     2 * time.Second,
			MaxObservations: 1,
		},
		{
			Window:          time.Second,
			MinCoverage:     time.Second,
			MaxStaleness:    -1,
			MaxObservations: 1,
		},
		{
			Window:      time.Second,
			MinCoverage: time.Second,
		},
	}
	for _, config := range tests {
		_, err := NewTWAPEngine(config)
		require.ErrorIs(t, err, ErrInvalidTWAPConfig)
	}
}

func newTestTWAP(t *testing.T, config TWAPConfig) *TWAPEngine {
	t.Helper()
	engine, err := NewTWAPEngine(config)
	require.NoError(t, err)
	return engine
}

func validTWAPConfig() TWAPConfig {
	return TWAPConfig{
		Window:          10 * time.Second,
		MinCoverage:     time.Second,
		MaxStaleness:    10 * time.Second,
		MaxObservations: 10,
	}
}
