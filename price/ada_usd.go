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
	"slices"
	"sort"
	"strconv"
	"time"

	"github.com/blinklabs-io/shai/common"
	"github.com/blinklabs-io/shai/dex"
)

var (
	ErrInsufficientObservations = errors.New("price: insufficient qualified ADA/stablecoin pools")
	ErrInsufficientDiversity    = errors.New("price: insufficient stablecoin diversity")
	ErrConcentratedLiquidity    = errors.New("price: one pool dominates qualified liquidity")
	ErrDivergentPrices          = errors.New("price: qualified pool prices diverge")
	ErrNonFinitePrice           = errors.New("price: non-finite floating-point price")
	ErrInvalidActivityConfig    = errors.New("price: invalid pool activity configuration")
	ErrInvalidPoolActivity      = errors.New("price: invalid pool activity")
)

const (
	usdMicrosDecimals = 6

	SourceLocalDEXStablecoins = "local-dex-stablecoins"
	ValidationQualified       = "qualified"
	ValidationUnavailable     = "unavailable"
)

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

// ActivityConfig controls confirmed-volume qualification. Stablecoin volume is
// normalized to six decimal places, matching StableMicros.
type ActivityConfig struct {
	MinSwapCount    uint64
	MinStableVolume uint64
}

// DefaultActivityConfig requires both confirmed turnover and at least $100 of
// stablecoin-side volume over the tracker's configured rolling window.
func DefaultActivityConfig() ActivityConfig {
	return ActivityConfig{
		MinSwapCount:    1,
		MinStableVolume: 100_000_000,
	}
}

// DefaultConfig rejects dust pools while accepting the independently pegged
// mainnet USDM and USDCx CSWAP pools observed during implementation. A pool
// holding up to 75% of qualified liquidity may determine the weighted median;
// the remaining pools must still agree within MaxDivergence. Operators may
// explicitly lower the minimum counts and raise MaxPoolShare when only one
// sufficiently liquid pool is available.
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
	PoolID             string    `json:"poolId"`
	Protocol           string    `json:"protocol"`
	Stablecoin         string    `json:"stablecoin"`
	ADAReserve         uint64    `json:"adaReserve"`
	StableReserve      uint64    `json:"stableReserve"`
	StableMicros       uint64    `json:"stableMicros"`
	PriceNum           string    `json:"priceNumerator"`
	PriceDen           string    `json:"priceDenominator"`
	Price              float64   `json:"price"`
	Slot               uint64    `json:"slot"`
	BlockHash          string    `json:"blockHash"`
	TxHash             string    `json:"txHash"`
	TxIndex            uint32    `json:"txIndex"`
	ObservedAt         time.Time `json:"observedAt"`
	AgeSeconds         *int64    `json:"ageSeconds"`
	Validation         string    `json:"validation"`
	StableVolumeMicros uint64    `json:"stableVolumeMicros,omitempty"`
	SwapCount          uint64    `json:"swapCount,omitempty"`
	ActivitySlots      uint64    `json:"activityWindowSlots,omitempty"`

	price *big.Rat
}

// Result is a liquidity-weighted local ADA/USD estimate.
type Result struct {
	Pair         string            `json:"pair"`
	Source       string            `json:"source"`
	Method       string            `json:"method"`
	Validation   string            `json:"validation"`
	ObservedAt   time.Time         `json:"observedAt"`
	AgeSeconds   *int64            `json:"ageSeconds"`
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
// agreement, then computes a stablecoin-liquidity-weighted median.
func AggregateADAUSD(
	pools []*dex.PoolState,
	config Config,
) (Result, error) {
	return AggregateADAUSDAt(pools, config, time.Now())
}

// AggregateADAUSDAt is AggregateADAUSD with an explicit evaluation time for
// deterministic provenance and age reporting.
func AggregateADAUSDAt(
	pools []*dex.PoolState,
	config Config,
	now time.Time,
) (Result, error) {
	if err := validateConfig(config); err != nil {
		return Result{}, err
	}
	observations, err := observationsFromPools(pools, config, now)
	if err != nil {
		return Result{}, err
	}
	result := Result{
		Pair:         "ADA/USD",
		Source:       SourceLocalDEXStablecoins,
		Method:       "local-dex-liquidity-weighted-median",
		Validation:   ValidationUnavailable,
		Observations: observations,
	}
	if len(observations) < config.MinObservations {
		return result, ErrInsufficientObservations
	}

	symbols := make(map[string]struct{}, len(observations))
	for _, observation := range observations {
		symbols[observation.Stablecoin] = struct{}{}
	}
	if len(symbols) < config.MinStablecoins {
		return result, ErrInsufficientDiversity
	}
	medianPrice, totalWeight, err := liquidityWeightedMedian(observations)
	if err != nil {
		return result, err
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
	for _, observation := range observations {
		if observation.price.Cmp(minPrice) < 0 {
			minPrice.Set(observation.price)
		}
		if observation.price.Cmp(maxPrice) > 0 {
			maxPrice.Set(observation.price)
		}
	}
	spread := new(big.Rat).Quo(
		new(big.Rat).Sub(maxPrice, minPrice),
		minPrice,
	)
	result.Spread, err = finiteFloat64(spread)
	if err != nil {
		return result, err
	}
	if spread.Cmp(configRatio(config.MaxDivergence)) > 0 {
		return result, ErrDivergentPrices
	}
	result.price = medianPrice
	result.PriceNum = medianPrice.Num().String()
	result.PriceDen = medianPrice.Denom().String()
	result.Price, err = finiteFloat64(medianPrice)
	if err != nil {
		return result, err
	}
	result.Validation = ValidationQualified
	for _, observation := range observations {
		if observation.ObservedAt.IsZero() {
			continue
		}
		if result.ObservedAt.IsZero() ||
			observation.ObservedAt.Before(result.ObservedAt) {
			result.ObservedAt = observation.ObservedAt
			result.AgeSeconds = observation.AgeSeconds
		}
	}
	return result, nil
}

// AggregateADAUSDWithActivity qualifies pools by locally inferred confirmed
// swap volume before applying the normal liquidity and agreement checks.
func AggregateADAUSDWithActivity(
	pools []*dex.PoolState,
	volumes []dex.PoolVolume,
	config Config,
	activityConfig ActivityConfig,
) (Result, error) {
	return AggregateADAUSDWithActivityAt(
		pools,
		volumes,
		config,
		activityConfig,
		time.Now(),
	)
}

// AggregateADAUSDWithActivityAt is AggregateADAUSDWithActivity with an
// explicit evaluation time.
func AggregateADAUSDWithActivityAt(
	pools []*dex.PoolState,
	volumes []dex.PoolVolume,
	config Config,
	activityConfig ActivityConfig,
	now time.Time,
) (Result, error) {
	if activityConfig.MinSwapCount == 0 ||
		activityConfig.MinStableVolume == 0 {
		return Result{}, ErrInvalidActivityConfig
	}
	activityByPool := make(map[string]dex.PoolVolume, len(volumes))
	for _, volume := range volumes {
		key := volume.Key()
		if volume.PoolID == "" ||
			volume.Network == "" ||
			volume.Protocol == "" {
			return Result{}, ErrInvalidPoolActivity
		}
		if _, exists := activityByPool[key]; exists {
			return Result{}, fmt.Errorf(
				"%w: duplicate volume for %s",
				ErrInvalidPoolActivity,
				key,
			)
		}
		activityByPool[key] = volume
	}

	qualified := make([]*dex.PoolState, 0, len(pools))
	qualifiedActivity := make(map[string]dex.PoolVolume)
	for _, pool := range pools {
		if pool == nil {
			continue
		}
		volume, ok := activityByPool[pool.Key()]
		if !ok {
			continue
		}
		stableVolume, err := stableVolumeMicros(pool, volume, config)
		if err != nil {
			return Result{}, err
		}
		if stableVolume < activityConfig.MinStableVolume ||
			volume.SwapCount < activityConfig.MinSwapCount {
			continue
		}
		qualified = append(qualified, pool)
		qualifiedActivity[pool.Key()] = volume
	}

	result, err := AggregateADAUSDAt(qualified, config, now)
	for i := range result.Observations {
		observation := &result.Observations[i]
		for _, pool := range qualified {
			if pool.PoolId != observation.PoolID ||
				pool.Protocol != observation.Protocol {
				continue
			}
			volume := qualifiedActivity[pool.Key()]
			stableVolume, volumeErr := stableVolumeMicros(
				pool,
				volume,
				config,
			)
			if volumeErr != nil {
				return Result{}, volumeErr
			}
			observation.StableVolumeMicros = stableVolume
			observation.SwapCount = volume.SwapCount
			observation.ActivitySlots = volume.WindowSlots
			break
		}
	}
	return result, err
}

func stableVolumeMicros(
	pool *dex.PoolState,
	volume dex.PoolVolume,
	config Config,
) (uint64, error) {
	if pool == nil ||
		volume.Key() != pool.Key() ||
		volume.WindowSlots == 0 ||
		volume.WindowEnd < pool.Slot ||
		!pool.AssetX.IsAsset(volume.AssetX) ||
		!pool.AssetY.IsAsset(volume.AssetY) {
		return 0, ErrInvalidPoolActivity
	}
	var stable common.AssetAmount
	var amount uint64
	switch {
	case pool.AssetX.IsLovelace():
		stable = pool.AssetY
		amount = volume.VolumeY
	case pool.AssetY.IsLovelace():
		stable = pool.AssetX
		amount = volume.VolumeX
	default:
		return 0, ErrInvalidPoolActivity
	}
	for _, candidate := range config.Stablecoins {
		if stable.IsAsset(candidate.Asset) {
			normalized, ok := normalizeToMicros(amount, candidate.Decimals)
			if !ok {
				return 0, ErrInvalidPoolActivity
			}
			return normalized, nil
		}
	}
	return 0, ErrInvalidPoolActivity
}

func liquidityWeightedMedian(
	observations []PoolObservation,
) (*big.Rat, uint64, error) {
	ordered := append([]PoolObservation(nil), observations...)
	for _, observation := range ordered {
		if observation.price == nil || observation.price.Sign() <= 0 {
			return nil, 0, fmt.Errorf(
				"price: invalid pool observation price",
			)
		}
	}
	slices.SortStableFunc(ordered, func(a, b PoolObservation) int {
		return a.price.Cmp(b.price)
	})

	var totalWeight uint64
	for _, observation := range ordered {
		if ^uint64(0)-totalWeight < observation.StableMicros {
			return nil, 0, fmt.Errorf(
				"price: aggregate liquidity overflows uint64",
			)
		}
		totalWeight += observation.StableMicros
	}
	if totalWeight == 0 {
		return nil, 0, ErrInsufficientObservations
	}

	var cumulative uint64
	for i, observation := range ordered {
		if observation.StableMicros == 0 {
			continue
		}
		cumulative += observation.StableMicros
		remaining := totalWeight - cumulative
		switch {
		case cumulative > remaining:
			return new(big.Rat).Set(observation.price), totalWeight, nil
		case cumulative == remaining:
			for _, next := range ordered[i+1:] {
				if next.StableMicros == 0 {
					continue
				}
				median := new(big.Rat).Quo(
					new(big.Rat).Add(observation.price, next.price),
					big.NewRat(2, 1),
				)
				return median, totalWeight, nil
			}
		}
	}
	return nil, 0, ErrInsufficientObservations
}

func observationsFromPools(
	pools []*dex.PoolState,
	config Config,
	now time.Time,
) ([]PoolObservation, error) {
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
		observation, ok, err := observationFromPool(pool, config, now)
		if err != nil {
			return nil, err
		}
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
	return observations, nil
}

func observationFromPool(
	pool *dex.PoolState,
	config Config,
	now time.Time,
) (PoolObservation, bool, error) {
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
		return PoolObservation{}, false, nil
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
		return PoolObservation{}, false, nil
	}
	stableMicros, ok := normalizeToMicros(stable.Amount, stablecoin.Decimals)
	if !ok || stableMicros < config.MinStableUSD {
		return PoolObservation{}, false, nil
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
	priceFloat, err := finiteFloat64(price)
	if err != nil {
		return PoolObservation{}, false, err
	}
	var ageSeconds *int64
	if !pool.Timestamp.IsZero() {
		age := int64(now.Sub(pool.Timestamp).Seconds())
		ageSeconds = &age
	}
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
		BlockHash:     pool.BlockHash,
		TxHash:        pool.TxHash,
		TxIndex:       pool.TxIndex,
		ObservedAt:    pool.Timestamp,
		AgeSeconds:    ageSeconds,
		Validation:    ValidationQualified,
		price:         price,
	}, true, nil
}

func finiteFloat64(value *big.Rat) (float64, error) {
	approximation, exact := value.Float64()
	if math.IsInf(approximation, 0) || math.IsNaN(approximation) {
		return 0, ErrNonFinitePrice
	}
	if exact {
		return approximation, nil
	}
	// JSON exposes a float approximation alongside the exact rational fields.
	return approximation, nil
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
