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
	"testing"

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
	require.Equal(t, "local-dex-stablecoin-weighted", result.Method)
	require.Len(t, result.Observations, 2)
	require.Less(t, result.Spread, 0.01)
	require.InDelta(t, 0.16868, result.Price, 0.0001)
	require.NotZero(t, result.PriceNum)
	require.NotZero(t, result.PriceDen)
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
