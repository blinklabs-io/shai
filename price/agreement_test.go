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

	"github.com/blinklabs-io/shai/price/djed"
	"github.com/stretchr/testify/require"
)

func TestCrossValidateADAUSDAgreesExactly(t *testing.T) {
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	dexResult := qualifiedResult(big.NewRat(1, 6))
	djedObservation := djedResult(now, 17, 100)

	result, err := CrossValidateADAUSD(
		dexResult,
		djedObservation,
		DefaultCrossSourceConfig(),
		now,
	)

	require.NoError(t, err)
	require.Equal(t, ValidationQualified, result.Validation)
	require.Equal(t, SourceLocalCrossCheck, result.Source)
	require.Equal(t, MethodExactMidpoint, result.Method)
	require.Equal(t, big.NewRat(101, 600), result.Rat())
	require.Equal(t, "101", result.PriceNum)
	require.Equal(t, "600", result.PriceDen)
	require.Equal(t, big.NewRat(1, 50), mustResultDivergence(t, result))
	require.Equal(t, dexResult, result.DEX)
	require.Equal(t, djedObservation, result.Djed)
}

func TestCrossValidateADAUSDAllowsExactLimit(t *testing.T) {
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	config := CrossSourceConfig{
		MaxDivergenceNumerator:   1,
		MaxDivergenceDenominator: 20,
	}

	result, err := CrossValidateADAUSD(
		qualifiedResult(big.NewRat(1, 5)),
		djedResult(now, 21, 100),
		config,
		now,
	)

	require.NoError(t, err)
	require.Equal(t, big.NewRat(41, 200), result.Rat())
	require.Equal(t, big.NewRat(1, 20), mustResultDivergence(t, result))
}

func TestCrossValidateADAUSDTripsDivergenceCircuit(t *testing.T) {
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)

	result, err := CrossValidateADAUSD(
		qualifiedResult(big.NewRat(1, 5)),
		djedResult(now, 3, 10),
		DefaultCrossSourceConfig(),
		now,
	)

	require.ErrorIs(t, err, ErrCrossSourceDivergence)
	require.Equal(t, ValidationDivergent, result.Validation)
	require.Equal(t, big.NewRat(1, 2), mustResultDivergence(t, result))
	require.Zero(t, result.Rat().Sign())
	require.Empty(t, result.PriceNum)
}

func TestCrossValidateADAUSDRejectsUnavailableDEX(t *testing.T) {
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	tests := map[string]Result{
		"unavailable": {
			Pair:       "ADA/USD",
			Source:     SourceLocalDEXStablecoins,
			Validation: ValidationUnavailable,
			price:      big.NewRat(1, 5),
		},
		"wrong pair": {
			Pair:       "ADA/EUR",
			Source:     SourceLocalDEXStablecoins,
			Validation: ValidationQualified,
			price:      big.NewRat(1, 5),
		},
		"wrong source": {
			Pair:       "ADA/USD",
			Source:     "remote",
			Validation: ValidationQualified,
			price:      big.NewRat(1, 5),
		},
		"zero rate": {
			Pair:       "ADA/USD",
			Source:     SourceLocalDEXStablecoins,
			Validation: ValidationQualified,
		},
	}
	for name, dexResult := range tests {
		t.Run(name, func(t *testing.T) {
			result, err := CrossValidateADAUSD(
				dexResult,
				djedResult(now, 1, 5),
				DefaultCrossSourceConfig(),
				now,
			)
			require.ErrorIs(t, err, ErrUnqualifiedDEX)
			require.Equal(t, ValidationUnavailable, result.Validation)
		})
	}
}

func TestCrossValidateADAUSDEnforcesDjedValidity(t *testing.T) {
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	dexResult := qualifiedResult(big.NewRat(1, 5))

	notYetValid := djedResult(now, 1, 5)
	notYetValid.ValidFrom = now.Add(time.Second)
	_, err := CrossValidateADAUSD(
		dexResult,
		notYetValid,
		DefaultCrossSourceConfig(),
		now,
	)
	require.ErrorIs(t, err, djed.ErrNotYetValid)

	expired := djedResult(now, 1, 5)
	expired.ValidUntil = now.Add(-time.Second)
	_, err = CrossValidateADAUSD(
		dexResult,
		expired,
		DefaultCrossSourceConfig(),
		now,
	)
	require.ErrorIs(t, err, djed.ErrExpired)

	exclusiveStart := djedResult(now, 1, 5)
	exclusiveStart.ValidFrom = now
	exclusiveStart.ValidFromInclusive = false
	_, err = CrossValidateADAUSD(
		dexResult,
		exclusiveStart,
		DefaultCrossSourceConfig(),
		now,
	)
	require.ErrorIs(t, err, djed.ErrNotYetValid)

	exclusiveEnd := djedResult(now, 1, 5)
	exclusiveEnd.ValidUntil = now
	exclusiveEnd.ValidUntilInclusive = false
	_, err = CrossValidateADAUSD(
		dexResult,
		exclusiveEnd,
		DefaultCrossSourceConfig(),
		now,
	)
	require.ErrorIs(t, err, djed.ErrExpired)
}

func TestCrossValidateADAUSDRejectsInvalidInput(t *testing.T) {
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	dexResult := qualifiedResult(big.NewRat(1, 5))

	_, err := CrossValidateADAUSD(
		dexResult,
		djedResult(now, 1, 5),
		CrossSourceConfig{MaxDivergenceNumerator: 1},
		now,
	)
	require.ErrorIs(t, err, ErrInvalidCrossSourceConfig)

	tests := map[string]func(*djed.Observation){
		"wrong pair": func(value *djed.Observation) {
			value.Pair = "ADA/EUR"
		},
		"wrong source": func(value *djed.Observation) {
			value.Source = "remote"
		},
		"zero numerator": func(value *djed.Observation) {
			value.PriceNumerator = 0
		},
		"zero denominator": func(value *djed.Observation) {
			value.PriceDenominator = 0
		},
		"reversed interval": func(value *djed.Observation) {
			value.ValidFrom = value.ValidUntil.Add(time.Second)
		},
		"empty exclusive interval": func(value *djed.Observation) {
			value.ValidFrom = value.ValidUntil
			value.ValidFromInclusive = false
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			observation := djedResult(now, 1, 5)
			mutate(&observation)
			_, err := CrossValidateADAUSD(
				dexResult,
				observation,
				DefaultCrossSourceConfig(),
				now,
			)
			require.ErrorIs(t, err, ErrInvalidDjedObservation)
		})
	}
}

func qualifiedResult(rate *big.Rat) Result {
	value, _ := rate.Float64()
	return Result{
		Pair:       "ADA/USD",
		Source:     SourceLocalDEXStablecoins,
		Method:     "test",
		Validation: ValidationQualified,
		PriceNum:   rate.Num().String(),
		PriceDen:   rate.Denom().String(),
		Price:      value,
		price:      new(big.Rat).Set(rate),
	}
}

func djedResult(
	now time.Time,
	numerator uint64,
	denominator uint64,
) djed.Observation {
	return djed.Observation{
		Pair:                "ADA/USD",
		Source:              "djed",
		PriceNumerator:      numerator,
		PriceDenominator:    denominator,
		ValidFrom:           now.Add(-time.Minute),
		ValidFromInclusive:  true,
		ValidUntil:          now.Add(time.Minute),
		ValidUntilInclusive: true,
		TxHash:              "djed-tx",
		TxIndex:             1,
	}
}

func mustResultDivergence(
	t *testing.T,
	result CrossSourceResult,
) *big.Rat {
	t.Helper()
	value, ok := new(big.Rat).SetString(
		result.DivergenceNum + "/" + result.DivergenceDen,
	)
	require.True(t, ok)
	return value
}
