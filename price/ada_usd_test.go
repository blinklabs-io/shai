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
	"math"
	"math/big"
	"testing"
	"time"

	"github.com/blinklabs-io/shai/common"
	"github.com/blinklabs-io/shai/dex"
	"github.com/stretchr/testify/require"
)

func TestAggregateADAUSDCurrentCSwapFixtures(t *testing.T) {
	// Current unspent mainnet CSWAP pools checked on 2026-07-23.
	pools := []*dex.PoolState{
		poolFixture(
			t,
			"usdcx",
			USDCxPolicyID,
			USDCxAssetName,
			8_547_275_688,
			1_439_463_431,
			"a24fd5df3faebb06ba0aa815890d5c7e3907c27f428c906b792d81321254a8d1",
			1,
		),
		poolFixture(
			t,
			"usdm",
			USDMPolicyID,
			USDMAssetName,
			4_579_285_253,
			774_654_393,
			"1237c072b8d283e3ccc3b9956502825f11533973b5402ed1ef1df459ffca8bfc",
			1,
		),
	}

	result, err := AggregateADAUSD(pools, DefaultConfig())
	require.NoError(t, err)
	require.Equal(t, "ADA/USD", result.Pair)
	require.Equal(t, "local-dex-liquidity-weighted-median", result.Method)
	require.Len(t, result.Observations, 2)
	require.Less(t, result.Spread, 0.01)
	require.Equal(t, "205637633/1221039384", result.Rat().String())
	require.InDelta(t, 0.168412, result.Price, 0.000001)
	require.Equal(t, "205637633", result.PriceNum)
	require.Equal(t, "1221039384", result.PriceDen)
}

func TestAggregateADAUSDUsesLiquidityWeightedMedian(t *testing.T) {
	config := DefaultConfig()
	config.MinADAReserve = 0
	config.MinStableUSD = 0
	config.MinObservations = 3
	config.MaxPoolShare = 1
	config.MaxDivergence = 10
	pools := []*dex.PoolState{
		poolFixture(
			t,
			"low",
			USDCxPolicyID,
			USDCxAssetName,
			200,
			20,
			"low",
			1,
		),
		poolFixture(
			t,
			"median",
			USDMPolicyID,
			USDMAssetName,
			250,
			50,
			"median",
			1,
		),
		poolFixture(
			t,
			"high",
			USDCxPolicyID,
			USDCxAssetName,
			100,
			30,
			"high",
			1,
		),
	}

	result, err := AggregateADAUSD(pools, config)
	require.NoError(t, err)
	require.Equal(t, "1/5", result.Rat().String())
	require.Equal(t, 0.2, result.Price)
}

func TestAggregateADAUSDMidpointsExactWeightSplit(t *testing.T) {
	config := DefaultConfig()
	config.MinADAReserve = 0
	config.MinStableUSD = 0
	config.MaxPoolShare = 0.5
	config.MaxDivergence = 10
	pools := []*dex.PoolState{
		poolFixture(
			t,
			"low",
			USDCxPolicyID,
			USDCxAssetName,
			600,
			60,
			"low",
			1,
		),
		poolFixture(
			t,
			"high",
			USDMPolicyID,
			USDMAssetName,
			200,
			60,
			"high",
			1,
		),
	}

	result, err := AggregateADAUSD(pools, config)
	require.NoError(t, err)
	require.Equal(t, "1/5", result.Rat().String())
	require.Equal(t, 0.2, result.Price)
}

func TestAggregateADAUSDSelectsPriceAboveHalfWeight(t *testing.T) {
	config := DefaultConfig()
	config.MinADAReserve = 0
	config.MinStableUSD = 0
	config.MaxPoolShare = 1
	config.MaxDivergence = 10
	pools := []*dex.PoolState{
		poolFixture(
			t,
			"low",
			USDCxPolicyID,
			USDCxAssetName,
			510,
			51,
			"low",
			1,
		),
		poolFixture(
			t,
			"high",
			USDMPolicyID,
			USDMAssetName,
			245,
			49,
			"high",
			1,
		),
	}

	result, err := AggregateADAUSD(pools, config)
	require.NoError(t, err)
	require.Equal(t, "1/10", result.Rat().String())
}

func TestAggregateADAUSDDefaultAllowsDominantQualifiedLiquidity(
	t *testing.T,
) {
	config := DefaultConfig()
	config.MinADAReserve = 0
	config.MinStableUSD = 0
	pools := []*dex.PoolState{
		poolFixture(
			t,
			"dominant",
			USDCxPolicyID,
			USDCxAssetName,
			10_000,
			2_100,
			"dominant",
			1,
		),
		poolFixture(
			t,
			"minority",
			USDMPolicyID,
			USDMAssetName,
			3_500,
			700,
			"minority",
			1,
		),
	}

	result, err := AggregateADAUSD(pools, config)
	require.NoError(t, err)
	require.Equal(t, "21/100", result.Rat().String())
	require.Equal(t, 0.05, result.Spread)
}

func TestAggregateADAUSDCanUseOneQualifiedPool(t *testing.T) {
	config := DefaultConfig()
	config.MinADAReserve = 0
	config.MinStableUSD = 0
	config.MinObservations = 1
	config.MinStablecoins = 1
	config.MaxPoolShare = 1
	pool := poolFixture(
		t,
		"single",
		USDCxPolicyID,
		USDCxAssetName,
		1_000,
		200,
		"single",
		1,
	)

	result, err := AggregateADAUSD([]*dex.PoolState{pool}, config)
	require.NoError(t, err)
	require.Equal(t, "1/5", result.Rat().String())
}

func TestAggregateADAUSDWithActivityCanUseOneLiquidActivePool(t *testing.T) {
	config := DefaultConfig()
	config.MinObservations = 1
	config.MinStablecoins = 1
	config.MaxPoolShare = 1
	pool := poolFixture(
		t,
		"single",
		USDCxPolicyID,
		USDCxAssetName,
		5_000_000_000,
		1_000_000_000,
		"single",
		1,
	)
	pool.Network = "mainnet"
	volume := poolVolume(pool, 3, 250_000_000)

	result, err := AggregateADAUSDWithActivity(
		[]*dex.PoolState{pool},
		[]dex.PoolVolume{volume},
		config,
		DefaultActivityConfig(),
	)
	require.NoError(t, err)
	require.Equal(t, "1/5", result.Rat().String())
	require.Equal(
		t,
		uint64(250_000_000),
		result.Observations[0].StableVolumeMicros,
	)
	require.Equal(t, uint64(3), result.Observations[0].SwapCount)
}

func TestAggregateADAUSDWithActivityRejectsInactivePool(t *testing.T) {
	config := DefaultConfig()
	config.MinObservations = 1
	config.MinStablecoins = 1
	config.MaxPoolShare = 1
	pool := poolFixture(
		t,
		"inactive",
		USDCxPolicyID,
		USDCxAssetName,
		5_000_000_000,
		1_000_000_000,
		"inactive",
		1,
	)
	pool.Network = "mainnet"
	volume := poolVolume(pool, 1, 99_999_999)

	result, err := AggregateADAUSDWithActivity(
		[]*dex.PoolState{pool},
		[]dex.PoolVolume{volume},
		config,
		DefaultActivityConfig(),
	)
	require.ErrorIs(t, err, ErrInsufficientObservations)
	require.Empty(t, result.Observations)
}

func TestAggregateADAUSDWithActivityRejectsInvalidVolume(t *testing.T) {
	pool := poolFixture(
		t,
		"pool",
		USDCxPolicyID,
		USDCxAssetName,
		5_000_000_000,
		1_000_000_000,
		"pool",
		1,
	)
	pool.Network = "mainnet"
	volume := poolVolume(pool, 1, 100_000_000)
	volume.AssetY = common.Lovelace()

	_, err := AggregateADAUSDWithActivity(
		[]*dex.PoolState{pool},
		[]dex.PoolVolume{volume},
		DefaultConfig(),
		DefaultActivityConfig(),
	)
	require.ErrorIs(t, err, ErrInvalidPoolActivity)
}

func TestLiquidityWeightedMedianRejectsInvalidWeights(t *testing.T) {
	_, _, err := liquidityWeightedMedian(nil)
	require.ErrorIs(t, err, ErrInsufficientObservations)

	_, _, err = liquidityWeightedMedian([]PoolObservation{{
		StableMicros: 1,
	}})
	require.ErrorContains(t, err, "invalid pool observation price")

	observations := []PoolObservation{
		{StableMicros: ^uint64(0), price: big.NewRat(1, 5)},
		{StableMicros: 1, price: big.NewRat(1, 5)},
	}
	_, _, err = liquidityWeightedMedian(observations)
	require.ErrorContains(t, err, "overflows")
}

func TestFiniteFloat64RejectsOverflow(t *testing.T) {
	hugeInteger := new(big.Int).Exp(
		big.NewInt(10),
		big.NewInt(400),
		nil,
	)
	_, err := finiteFloat64(new(big.Rat).SetInt(hugeInteger))
	require.ErrorIs(t, err, ErrNonFinitePrice)
}

func TestAggregateADAUSDReportsLocalProvenance(t *testing.T) {
	now := time.Date(2026, 7, 23, 20, 0, 0, 0, time.UTC)
	usdcx := poolFixture(
		t,
		"usdcx",
		USDCxPolicyID,
		USDCxAssetName,
		8_547_275_688,
		1_439_463_431,
		"usdcx-tx",
		1,
	)
	usdcx.BlockHash = "usdcx-block"
	usdcx.Timestamp = now.Add(-2 * time.Minute)
	usdm := poolFixture(
		t,
		"usdm",
		USDMPolicyID,
		USDMAssetName,
		4_579_285_253,
		774_654_393,
		"usdm-tx",
		1,
	)
	usdm.BlockHash = "usdm-block"
	usdm.Timestamp = now.Add(-5 * time.Minute)

	result, err := AggregateADAUSDAt(
		[]*dex.PoolState{usdcx, usdm},
		DefaultConfig(),
		now,
	)
	require.NoError(t, err)
	require.Equal(t, SourceLocalDEXStablecoins, result.Source)
	require.Equal(t, ValidationQualified, result.Validation)
	require.Equal(t, usdm.Timestamp, result.ObservedAt)
	require.NotNil(t, result.AgeSeconds)
	require.Equal(t, int64(300), *result.AgeSeconds)
	require.Equal(t, "usdcx-block", result.Observations[0].BlockHash)
	require.Equal(
		t,
		ValidationQualified,
		result.Observations[0].Validation,
	)
	require.NotNil(t, result.Observations[0].AgeSeconds)
	require.Equal(t, int64(120), *result.Observations[0].AgeSeconds)
}

func TestAggregateADAUSDRequiresStablecoinDiversity(t *testing.T) {
	pool := poolFixture(
		t,
		"usdcx",
		USDCxPolicyID,
		USDCxAssetName,
		8_547_275_688,
		1_439_463_431,
		"one",
		1,
	)
	duplicate := *pool
	duplicate.PoolId = "usdcx-2"
	duplicate.TxHash = "two"

	_, err := AggregateADAUSD(
		[]*dex.PoolState{pool, &duplicate},
		DefaultConfig(),
	)
	require.ErrorIs(t, err, ErrInsufficientDiversity)
}

func TestAggregateADAUSDRejectsDivergenceAndMempool(t *testing.T) {
	usdcx := poolFixture(
		t,
		"usdcx",
		USDCxPolicyID,
		USDCxAssetName,
		5_000_000_000,
		1_000_000_000,
		"one",
		1,
	)
	usdm := poolFixture(
		t,
		"usdm",
		USDMPolicyID,
		USDMAssetName,
		5_000_000_000,
		500_000_000,
		"two",
		1,
	)
	_, err := AggregateADAUSD(
		[]*dex.PoolState{usdcx, usdm},
		DefaultConfig(),
	)
	require.ErrorIs(t, err, ErrDivergentPrices)

	usdm.FromMempool = true
	_, err = AggregateADAUSD(
		[]*dex.PoolState{usdcx, usdm},
		DefaultConfig(),
	)
	require.True(t, errors.Is(err, ErrInsufficientObservations))
}

func TestAggregateADAUSDRejectsThinAndConcentratedPools(t *testing.T) {
	thin := poolFixture(
		t,
		"thin",
		USDCxPolicyID,
		USDCxAssetName,
		999_999_999,
		200_000_000,
		"thin",
		1,
	)
	_, err := AggregateADAUSD([]*dex.PoolState{thin}, DefaultConfig())
	require.ErrorIs(t, err, ErrInsufficientObservations)

	usdcx := poolFixture(
		t,
		"usdcx",
		USDCxPolicyID,
		USDCxAssetName,
		20_000_000_000,
		3_000_000_000,
		"one",
		1,
	)
	usdm := poolFixture(
		t,
		"usdm",
		USDMPolicyID,
		USDMAssetName,
		1_000_000_000,
		150_000_000,
		"two",
		1,
	)
	_, err = AggregateADAUSD(
		[]*dex.PoolState{usdcx, usdm},
		DefaultConfig(),
	)
	require.ErrorIs(t, err, ErrConcentratedLiquidity)
}

func TestAggregateADAUSDAllowsExactDivergenceLimit(t *testing.T) {
	config := DefaultConfig()
	config.MinADAReserve = 0
	config.MinStableUSD = 0
	config.MaxDivergence = 0.05
	pools := []*dex.PoolState{
		poolFixture(
			t,
			"usdcx",
			USDCxPolicyID,
			USDCxAssetName,
			10_000_000_000,
			1_000_000_000,
			"one",
			1,
		),
		poolFixture(
			t,
			"usdm",
			USDMPolicyID,
			USDMAssetName,
			10_000_000_000,
			1_050_000_000,
			"two",
			1,
		),
	}

	result, err := AggregateADAUSD(pools, config)
	require.NoError(t, err)
	require.Equal(t, 0.05, result.Spread)
}

func TestAggregateADAUSDRejectsZeroReservesWithZeroThresholds(t *testing.T) {
	config := DefaultConfig()
	config.MinADAReserve = 0
	config.MinStableUSD = 0
	config.MinObservations = 1
	config.MinStablecoins = 1
	config.MaxPoolShare = 1

	for _, amounts := range []struct {
		name   string
		ada    uint64
		stable uint64
	}{
		{name: "ADA", stable: 1_000_000},
		{name: "stablecoin", ada: 1_000_000},
	} {
		t.Run(amounts.name, func(t *testing.T) {
			pool := poolFixture(
				t,
				"zero",
				USDCxPolicyID,
				USDCxAssetName,
				amounts.ada,
				amounts.stable,
				"zero",
				1,
			)
			_, err := AggregateADAUSD([]*dex.PoolState{pool}, config)
			require.ErrorIs(t, err, ErrInsufficientObservations)
		})
	}
}

func TestObservationsFromPoolsUsesLatestSameSlotOutput(t *testing.T) {
	older := poolFixture(
		t,
		"usdcx",
		USDCxPolicyID,
		USDCxAssetName,
		8_000_000_000,
		1_000_000_000,
		"older",
		1,
	)
	newer := poolFixture(
		t,
		"usdcx",
		USDCxPolicyID,
		USDCxAssetName,
		8_000_000_000,
		1_250_000_000,
		"newer",
		2,
	)

	observations, err := observationsFromPools(
		[]*dex.PoolState{newer, older},
		DefaultConfig(),
		time.Now(),
	)
	require.NoError(t, err)
	require.Len(t, observations, 1)
	require.Equal(t, uint32(2), observations[0].TxIndex)
	require.Equal(t, uint64(1_250_000_000), observations[0].StableReserve)
}

func TestAggregateADAUSDRejectsNonFiniteConfig(t *testing.T) {
	pool := poolFixture(
		t,
		"usdcx",
		USDCxPolicyID,
		USDCxAssetName,
		8_000_000_000,
		1_000_000_000,
		"one",
		1,
	)
	for _, value := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		config := DefaultConfig()
		config.MaxPoolShare = value
		_, err := AggregateADAUSD([]*dex.PoolState{pool}, config)
		require.Error(t, err)

		config = DefaultConfig()
		config.MaxDivergence = value
		_, err = AggregateADAUSD([]*dex.PoolState{pool}, config)
		require.Error(t, err)
	}
}

func TestMainnetStablecoinRegistry(t *testing.T) {
	stablecoins := MainnetStablecoins()
	require.Len(t, stablecoins, 2)
	require.Equal(t, "USDM", stablecoins[0].Symbol)
	require.Equal(t, USDMPolicyID, stablecoins[0].Asset.PolicyIdHex())
	require.Equal(t, USDMAssetName, stablecoins[0].Asset.NameHex())
	require.Equal(t, uint8(6), stablecoins[0].Decimals)
	require.Equal(t, "USDCx", stablecoins[1].Symbol)
	require.Equal(t, USDCxPolicyID, stablecoins[1].Asset.PolicyIdHex())
	require.Equal(t, USDCxAssetName, stablecoins[1].Asset.NameHex())
	require.Equal(t, uint8(6), stablecoins[1].Decimals)
}

func poolFixture(
	t *testing.T,
	id,
	policyID,
	assetName string,
	ada,
	stable uint64,
	txHash string,
	txIndex uint32,
) *dex.PoolState {
	t.Helper()
	asset, err := common.NewAssetClass(policyID, assetName)
	require.NoError(t, err)
	return &dex.PoolState{
		PoolId:   id,
		Protocol: "cswap",
		AssetX: common.AssetAmount{
			Class:  common.Lovelace(),
			Amount: ada,
		},
		AssetY: common.AssetAmount{
			Class:  asset,
			Amount: stable,
		},
		Slot:    193_000_000,
		TxHash:  txHash,
		TxIndex: txIndex,
	}
}

func poolVolume(
	pool *dex.PoolState,
	swaps uint64,
	stableVolume uint64,
) dex.PoolVolume {
	return dex.PoolVolume{
		PoolID:      pool.PoolId,
		Network:     pool.Network,
		Protocol:    pool.Protocol,
		AssetX:      pool.AssetX.Class,
		AssetY:      pool.AssetY.Class,
		VolumeY:     stableVolume,
		SwapCount:   swaps,
		WindowSlots: 86_400,
		WindowEnd:   pool.Slot,
	}
}
