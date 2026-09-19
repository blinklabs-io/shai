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
	"time"

	"github.com/blinklabs-io/shai/price/djed"
)

const (
	SourceLocalCrossCheck = "local-dex-djed"
	MethodExactMidpoint   = "exact-two-source-midpoint"
	ValidationDivergent   = "divergent"
)

var (
	ErrInvalidCrossSourceConfig = errors.New(
		"price: invalid cross-source configuration",
	)
	ErrUnqualifiedDEX = errors.New(
		"price: DEX price is unavailable or unqualified",
	)
	ErrInvalidDjedObservation = errors.New(
		"price: invalid Djed observation",
	)
	ErrCrossSourceDivergence = errors.New(
		"price: DEX and Djed prices diverge",
	)
)

// CrossSourceConfig controls agreement between independently derived local
// prices. The maximum divergence is an exact non-negative ratio.
type CrossSourceConfig struct {
	MaxDivergenceNumerator   uint64
	MaxDivergenceDenominator uint64
}

// DefaultCrossSourceConfig permits at most five percent divergence.
func DefaultCrossSourceConfig() CrossSourceConfig {
	return CrossSourceConfig{
		MaxDivergenceNumerator:   5,
		MaxDivergenceDenominator: 100,
	}
}

// CrossSourceResult records the decision and both complete local provenance
// inputs. With two equally independent sources, the deterministic combined
// price is their exact arithmetic midpoint; neither source receives an
// unsupported confidence weight.
type CrossSourceResult struct {
	Pair          string           `json:"pair"`
	Source        string           `json:"source"`
	Method        string           `json:"method"`
	Validation    string           `json:"validation"`
	PriceNum      string           `json:"priceNumerator"`
	PriceDen      string           `json:"priceDenominator"`
	Price         float64          `json:"price"`
	DivergenceNum string           `json:"divergenceNumerator"`
	DivergenceDen string           `json:"divergenceDenominator"`
	Divergence    float64          `json:"divergence"`
	DEX           Result           `json:"dex"`
	Djed          djed.Observation `json:"djed"`

	price *big.Rat
}

// Rat returns a copy of the exact agreed price.
func (r CrossSourceResult) Rat() *big.Rat {
	if r.price == nil {
		return new(big.Rat)
	}
	return new(big.Rat).Set(r.price)
}

// CrossValidateADAUSD checks a qualified local DEX result against a currently
// valid Djed observation. Divergence is measured conservatively as the
// difference divided by the lower price.
func CrossValidateADAUSD(
	dexResult Result,
	djedObservation djed.Observation,
	config CrossSourceConfig,
	now time.Time,
) (CrossSourceResult, error) {
	result := CrossSourceResult{
		Pair:       "ADA/USD",
		Source:     SourceLocalCrossCheck,
		Method:     MethodExactMidpoint,
		Validation: ValidationUnavailable,
		DEX:        dexResult,
		Djed:       djedObservation,
	}
	maxDivergence, err := crossSourceLimit(config)
	if err != nil {
		return result, err
	}
	dexPrice, err := qualifiedDEXPrice(dexResult)
	if err != nil {
		return result, err
	}
	djedPrice, err := validDjedPrice(djedObservation, now)
	if err != nil {
		return result, err
	}

	low := dexPrice
	high := djedPrice
	if low.Cmp(high) > 0 {
		low, high = high, low
	}
	divergence := new(big.Rat).Quo(
		new(big.Rat).Sub(high, low),
		low,
	)
	result.DivergenceNum = divergence.Num().String()
	result.DivergenceDen = divergence.Denom().String()
	result.Divergence, _ = divergence.Float64()
	if divergence.Cmp(maxDivergence) > 0 {
		result.Validation = ValidationDivergent
		return result, ErrCrossSourceDivergence
	}

	midpoint := new(big.Rat).Quo(
		new(big.Rat).Add(dexPrice, djedPrice),
		big.NewRat(2, 1),
	)
	result.price = midpoint
	result.PriceNum = midpoint.Num().String()
	result.PriceDen = midpoint.Denom().String()
	result.Price, _ = midpoint.Float64()
	result.Validation = ValidationQualified
	return result, nil
}

func crossSourceLimit(config CrossSourceConfig) (*big.Rat, error) {
	if config.MaxDivergenceDenominator == 0 {
		return nil, fmt.Errorf(
			"%w: maximum divergence denominator must be positive",
			ErrInvalidCrossSourceConfig,
		)
	}
	return new(big.Rat).SetFrac(
		new(big.Int).SetUint64(config.MaxDivergenceNumerator),
		new(big.Int).SetUint64(config.MaxDivergenceDenominator),
	), nil
}

func qualifiedDEXPrice(result Result) (*big.Rat, error) {
	price := result.Rat()
	if result.Pair != "ADA/USD" ||
		result.Source != SourceLocalDEXStablecoins ||
		result.Validation != ValidationQualified ||
		price.Sign() <= 0 {
		return nil, ErrUnqualifiedDEX
	}
	return price, nil
}

func validDjedPrice(
	observation djed.Observation,
	now time.Time,
) (*big.Rat, error) {
	if observation.Pair != "ADA/USD" ||
		observation.Source != "djed" ||
		observation.PriceNumerator == 0 ||
		observation.PriceDenominator == 0 ||
		observation.ValidFrom.After(observation.ValidUntil) ||
		(observation.ValidFrom.Equal(observation.ValidUntil) &&
			(!observation.ValidFromInclusive ||
				!observation.ValidUntilInclusive)) {
		return nil, ErrInvalidDjedObservation
	}
	now = now.UTC()
	if now.Before(observation.ValidFrom) ||
		(now.Equal(observation.ValidFrom) &&
			!observation.ValidFromInclusive) {
		return nil, djed.ErrNotYetValid
	}
	if now.After(observation.ValidUntil) ||
		(now.Equal(observation.ValidUntil) &&
			!observation.ValidUntilInclusive) {
		return nil, djed.ErrExpired
	}
	return new(big.Rat).SetFrac(
		new(big.Int).SetUint64(observation.PriceNumerator),
		new(big.Int).SetUint64(observation.PriceDenominator),
	), nil
}
