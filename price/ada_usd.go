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

// Package price derives prices solely from locally supplied Cardano state.
package price

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"sort"
	"strconv"

	"github.com/blinklabs-io/shai/common"
	"github.com/blinklabs-io/shai/dex"
)

var (
	ErrInsufficientObservations = errors.New("price: insufficient qualified ADA/stablecoin pools")
	ErrInsufficientDiversity    = errors.New("price: insufficient stablecoin diversity")
	ErrConcentratedLiquidity    = errors.New("price: one pool dominates qualified liquidity")
	ErrDivergentPrices          = errors.New("price: qualified pool prices diverge")
)

const usdMicrosDecimals = 6

// Config controls qualification and agreement for ADA/USD pool observations.
type Config struct {
	Stablecoins     []Stablecoin
	MinADAReserve   uint64
	MinStableUSD    uint64
	MinObservations int
	MinStablecoins  int
	MaxPoolShare    float64
	MaxDivergence   float64
	IncludeMempool  bool
}

// DefaultConfig is conservative enough to reject dust pools while accepting
// the independently pegged mainnet USDM and USDCx CSWAP pools observed during
// implementation.
func DefaultConfig() Config {
	return Config{
		Stablecoins:     MainnetStablecoins(),
		MinADAReserve:   1_000_000_000,
		MinStableUSD:    100_000_000,
		MinObservations: 2,
		MinStablecoins:  2,
		MaxPoolShare:    0.75,
		MaxDivergence:   0.05,
	}
}

// PoolObservation is one qualified ADA/stablecoin spot price.
type PoolObservation struct {
	PoolID        string  `json:"poolId"`
	Protocol      string  `json:"protocol"`
	Stablecoin    string  `json:"stablecoin"`
	ADAReserve    uint64  `json:"adaReserve"`
	StableReserve uint64  `json:"stableReserve"`
	StableMicros  uint64  `json:"stableMicros"`
	PriceNum      string  `json:"priceNumerator"`
	PriceDen      string  `json:"priceDenominator"`
	Price         float64 `json:"price"`
	Slot          uint64  `json:"slot"`
	TxHash        string  `json:"txHash"`
	TxIndex       uint32  `json:"txIndex"`

	price *big.Rat
}

// Result is a liquidity-weighted local ADA/USD estimate.
type Result struct {
	Pair         string            `json:"pair"`
	Method       string            `json:"method"`
	PriceNum     string            `json:"priceNumerator"`
	PriceDen     string            `json:"priceDenominator"`
	Price        float64           `json:"price"`
	Spread       float64           `json:"spread"`
	Observations []PoolObservation `json:"observations"`

	price *big.Rat
}

// Rat returns a copy of the exact aggregate price.
func (r Result) Rat() *big.Rat {
	if r.price == nil {
		return new(big.Rat)
	}
	return new(big.Rat).Set(r.price)
}

// AggregateADAUSD qualifies ADA/stablecoin pools, enforces diversity and
// agreement, then computes a stablecoin-liquidity-weighted mean.
func AggregateADAUSD(
	pools []*dex.PoolState,
	config Config,
) (Result, error) {
	if err := validateConfig(config); err != nil {
		return Result{}, err
	}
	observations := observationsFromPools(pools, config)
	result := Result{
		Pair:         "ADA/USD",
		Method:       "local-dex-stablecoin-weighted",
		Observations: observations,
	}
	if len(observations) < config.MinObservations {
		return result, ErrInsufficientObservations
	}

	symbols := make(map[string]struct{}, len(observations))
	var totalWeight uint64
	for _, observation := range observations {
		symbols[observation.Stablecoin] = struct{}{}
		if ^uint64(0)-totalWeight < observation.StableMicros {
			return result, fmt.Errorf("price: aggregate liquidity overflows uint64")
		}
		totalWeight += observation.StableMicros
	}
	if len(symbols) < config.MinStablecoins {
		return result, ErrInsufficientDiversity
	}
	if totalWeight == 0 {
		return result, ErrInsufficientObservations
	}
	maxPoolShare := configRatio(config.MaxPoolShare)
	for _, observation := range observations {
		share := new(big.Rat).SetFrac(
			new(big.Int).SetUint64(observation.StableMicros),
			new(big.Int).SetUint64(totalWeight),
		)
		if share.Cmp(maxPoolShare) > 0 {
			return result, ErrConcentratedLiquidity
		}
	}

	minPrice := new(big.Rat).Set(observations[0].price)
	maxPrice := new(big.Rat).Set(observations[0].price)
	weightedPrice := new(big.Rat)
	for _, observation := range observations {
		if observation.price.Cmp(minPrice) < 0 {
			minPrice.Set(observation.price)
		}
		if observation.price.Cmp(maxPrice) > 0 {
			maxPrice.Set(observation.price)
		}
		term := new(big.Rat).Mul(
			observation.price,
			new(big.Rat).SetInt(
				new(big.Int).SetUint64(observation.StableMicros),
			),
		)
		weightedPrice.Add(weightedPrice, term)
	}
	spread := new(big.Rat).Quo(
		new(big.Rat).Sub(maxPrice, minPrice),
		minPrice,
	)
	result.Spread, _ = spread.Float64()
	if spread.Cmp(configRatio(config.MaxDivergence)) > 0 {
		return result, ErrDivergentPrices
	}
	weightedPrice.Quo(
		weightedPrice,
		new(big.Rat).SetInt(new(big.Int).SetUint64(totalWeight)),
	)
	result.price = weightedPrice
	result.PriceNum = weightedPrice.Num().String()
	result.PriceDen = weightedPrice.Denom().String()
	result.Price, _ = weightedPrice.Float64()
	return result, nil
}

func observationsFromPools(
	pools []*dex.PoolState,
	config Config,
) []PoolObservation {
	latest := make(map[string]*dex.PoolState)
	for _, pool := range pools {
		if pool == nil || (!config.IncludeMempool && pool.FromMempool) {
			continue
		}
		key := pool.Protocol + ":" + pool.PoolId
		current, ok := latest[key]
		if !ok ||
			pool.Slot > current.Slot ||
			(pool.Slot == current.Slot &&
				pool.TxIndex > current.TxIndex) ||
			(pool.Slot == current.Slot &&
				pool.TxIndex == current.TxIndex &&
				pool.TxHash > current.TxHash) {
			latest[key] = pool
		}
	}

	var observations []PoolObservation
	for _, pool := range latest {
		observation, ok := observationFromPool(pool, config)
		if ok {
			observations = append(observations, observation)
		}
	}
	sort.Slice(observations, func(i, j int) bool {
		if observations[i].Stablecoin != observations[j].Stablecoin {
			return observations[i].Stablecoin < observations[j].Stablecoin
		}
		if observations[i].Protocol != observations[j].Protocol {
			return observations[i].Protocol < observations[j].Protocol
		}
		return observations[i].PoolID < observations[j].PoolID
	})
	return observations
}

func observationFromPool(
	pool *dex.PoolState,
	config Config,
) (PoolObservation, bool) {
	var ada common.AssetAmount
	var stable common.AssetAmount
	var stablecoin Stablecoin
	switch {
	case pool.AssetX.IsLovelace():
		ada = pool.AssetX
		stable = pool.AssetY
	case pool.AssetY.IsLovelace():
		ada = pool.AssetY
		stable = pool.AssetX
	default:
		return PoolObservation{}, false
	}
	for _, candidate := range config.Stablecoins {
		if stable.IsAsset(candidate.Asset) {
			stablecoin = candidate
			break
		}
	}
	if stablecoin.Symbol == "" ||
		ada.Amount == 0 ||
		stable.Amount == 0 ||
		ada.Amount < config.MinADAReserve {
		return PoolObservation{}, false
	}
	stableMicros, ok := normalizeToMicros(stable.Amount, stablecoin.Decimals)
	if !ok || stableMicros < config.MinStableUSD {
		return PoolObservation{}, false
	}
	price := new(big.Rat).SetFrac(
		new(big.Int).Mul(
			new(big.Int).SetUint64(stable.Amount),
			pow10Big(usdMicrosDecimals),
		),
		new(big.Int).Mul(
			new(big.Int).SetUint64(ada.Amount),
			pow10Big(stablecoin.Decimals),
		),
	)
	priceFloat, _ := price.Float64()
	return PoolObservation{
		PoolID:        pool.PoolId,
		Protocol:      pool.Protocol,
		Stablecoin:    stablecoin.Symbol,
		ADAReserve:    ada.Amount,
		StableReserve: stable.Amount,
		StableMicros:  stableMicros,
		PriceNum:      price.Num().String(),
		PriceDen:      price.Denom().String(),
		Price:         priceFloat,
		Slot:          pool.Slot,
		TxHash:        pool.TxHash,
		TxIndex:       pool.TxIndex,
		price:         price,
	}, true
}

func normalizeToMicros(
	amount uint64,
	decimals uint8,
) (uint64, bool) {
	switch {
	case decimals == usdMicrosDecimals:
		return amount, true
	case decimals < usdMicrosDecimals:
		multiplier := pow10Uint(usdMicrosDecimals - decimals)
		if multiplier == 0 || amount > ^uint64(0)/multiplier {
			return 0, false
		}
		return amount * multiplier, true
	default:
		divisor := pow10Uint(decimals - usdMicrosDecimals)
		if divisor == 0 {
			return 0, false
		}
		return amount / divisor, true
	}
}

func validateConfig(config Config) error {
	if len(config.Stablecoins) == 0 {
		return errors.New("price: no stablecoins configured")
	}
	if config.MinObservations < 1 || config.MinStablecoins < 1 {
		return errors.New("price: minimum counts must be positive")
	}
	if math.IsNaN(config.MaxPoolShare) ||
		math.IsInf(config.MaxPoolShare, 0) ||
		config.MaxPoolShare <= 0 ||
		config.MaxPoolShare > 1 {
		return errors.New("price: max pool share must be in (0,1]")
	}
	if math.IsNaN(config.MaxDivergence) ||
		math.IsInf(config.MaxDivergence, 0) ||
		config.MaxDivergence < 0 {
		return errors.New("price: max divergence must be non-negative")
	}
	return nil
}

func configRatio(value float64) *big.Rat {
	ratio, ok := new(big.Rat).SetString(
		strconv.FormatFloat(value, 'g', -1, 64),
	)
	if !ok {
		panic("validated floating-point configuration is not rational")
	}
	return ratio
}

func pow10Uint(power uint8) uint64 {
	var value uint64 = 1
	for range power {
		if value > ^uint64(0)/10 {
			return 0
		}
		value *= 10
	}
	return value
}

func pow10Big(power uint8) *big.Int {
	return new(big.Int).Exp(
		big.NewInt(10),
		new(big.Int).SetUint64(uint64(power)),
		nil,
	)
}
