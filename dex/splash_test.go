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

package dex

import (
	"testing"
	"time"

	"github.com/blinklabs-io/shai/dex/splash"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSplashV1Parser(t *testing.T) {
	parser := NewSplashV1Parser()
	require.NotNil(t, parser, "NewSplashV1Parser returned nil")
}

func TestSplashParser_Protocol(t *testing.T) {
	parser := NewSplashV1Parser()
	assert.Equal(t, "splash-v1", parser.Protocol())
}

func TestSplashParser_ImplementsPoolParser(t *testing.T) {
	parser := NewSplashV1Parser()
	var _ PoolParser = parser // Compile-time check that SplashParser implements PoolParser
}

// TestSplashParser_FeeMatchesOnChainContract verifies that a Splash pool's
// on-chain feeNum is interpreted against the correct denominator. The Splash
// (Spectrum) AMM validator hard-codes feeDen = 1000 and feeNum is the post-fee
// multiplier (e.g. 997 => 0.3% fee). If the parser pairs feeNum=997 with a
// denominator of 10000, the pool is reported as charging ~90% fees and Quote
// returns swap outputs that are an order of magnitude too small.
func TestSplashParser_FeeMatchesOnChainContract(t *testing.T) {
	tokenPolicy := make([]byte, 28)
	for i := range tokenPolicy {
		tokenPolicy[i] = 0xab
	}
	tokenName := []byte("TEST")

	nftPolicy := make([]byte, 28)
	for i := range nftPolicy {
		nftPolicy[i] = 0x01
	}
	lqPolicy := make([]byte, 28)
	for i := range lqPolicy {
		lqPolicy[i] = 0x02
	}

	// feeNum = 997 is the canonical Splash/Spectrum 0.3% fee pool value.
	poolDatum := splash.PoolDatum{
		Nft:    splash.AssetClass{PolicyId: nftPolicy, Name: []byte("NFT")},
		X:      splash.AssetClass{PolicyId: []byte{}, Name: []byte{}}, // ADA
		Y:      splash.AssetClass{PolicyId: tokenPolicy, Name: tokenName},
		Lq:     splash.AssetClass{PolicyId: lqPolicy, Name: []byte("LQ")},
		FeeNum: 997,
	}
	datum, err := poolDatum.MarshalCBOR()
	require.NoError(t, err, "failed to encode Splash pool datum")

	// Pool holds 1000 ADA and 2000 TEST.
	utxoValue, err := buildMaryOutputCbor(
		1_000_000_000,
		tokenPolicy,
		tokenName,
		2_000_000_000,
	)
	require.NoError(t, err, "failed to build UTxO output")

	parser := NewSplashV1Parser()
	state, err := parser.ParsePoolDatum(
		datum,
		utxoValue,
		"abc123",
		0,
		12345,
		time.Now(),
	)
	require.NoError(t, err, "failed to parse Splash pool datum")

	assert.Equal(t, uint64(997), state.FeeNum, "feeNum should pass through unchanged")
	assert.Equal(t, uint64(1000), state.FeeDenom, "Splash feeDen is 1000 on-chain")

	// feeNum/feeDenom must yield a 0.3% effective fee, not ~90%.
	assert.InDelta(t, 0.003, state.EffectiveFee(), 1e-9,
		"Splash 997/1000 pool should charge 0.3%")

	amountOut, impact, err := state.Quote([]byte{}, []byte{}, 1_000_000)
	require.NoError(t, err, "failed to quote Splash pool")
	assert.Equal(t, uint64(1_992_013), amountOut)
	assert.InDelta(t, 0.3993, impact, 0.0001)
}
