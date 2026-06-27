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
	"bytes"
	"encoding/json"
	"testing"

	"github.com/blinklabs-io/shai/common"
)

func TestPoolStatePriceXY(t *testing.T) {
	state := &PoolState{
		AssetX: common.AssetAmount{Amount: 1000000},
		AssetY: common.AssetAmount{Amount: 2000000},
	}

	price := state.PriceXY()
	expected := 2.0
	if price != expected {
		t.Errorf("expected price %f, got %f", expected, price)
	}

	// Test with zero X amount
	state.AssetX.Amount = 0
	if state.PriceXY() != 0 {
		t.Error("expected price 0 when X amount is 0")
	}
}

func TestPoolStatePriceYX(t *testing.T) {
	state := &PoolState{
		AssetX: common.AssetAmount{Amount: 1000000},
		AssetY: common.AssetAmount{Amount: 2000000},
	}

	price := state.PriceYX()
	expected := 0.5
	if price != expected {
		t.Errorf("expected price %f, got %f", expected, price)
	}

	// Test with zero Y amount
	state.AssetY.Amount = 0
	if state.PriceYX() != 0 {
		t.Error("expected price 0 when Y amount is 0")
	}
}

func TestPoolStateEffectiveFee(t *testing.T) {
	state := &PoolState{
		FeeNum:   997,
		FeeDenom: 1000,
	}

	fee := state.EffectiveFee()
	expected := 0.003 // 0.3%
	if fee < 0.002999 || fee > 0.003001 {
		t.Errorf("expected fee ~%f, got %f", expected, fee)
	}

	// Test with zero denom
	state.FeeDenom = 0
	if state.EffectiveFee() != 0 {
		t.Error("expected fee 0 when denom is 0")
	}
}

func TestPoolStateKey(t *testing.T) {
	state := &PoolState{
		Network:  "mainnet",
		Protocol: "spectrum",
		PoolId:   "pool123",
	}

	expected := "mainnet:spectrum:pool123"
	if state.Key() != expected {
		t.Errorf("expected key %s, got %s", expected, state.Key())
	}
}

func TestClonePoolStatePreservesEmptyAssetSlices(t *testing.T) {
	state := &PoolState{
		// Empty but non-nil slices (e.g. lovelace via NewAssetClass("", "")).
		AssetX: common.AssetAmount{
			Class: common.AssetClass{PolicyId: []byte{}, Name: []byte{}},
		},
		// Nil slices (e.g. lovelace via the zero-value AssetClass).
		AssetY: common.AssetAmount{
			Class: common.AssetClass{PolicyId: nil, Name: nil},
		},
	}

	clone := ClonePoolState(state)

	if clone.AssetX.Class.PolicyId == nil {
		t.Error("expected empty (non-nil) PolicyId to stay non-nil after clone")
	}
	if clone.AssetX.Class.Name == nil {
		t.Error("expected empty (non-nil) Name to stay non-nil after clone")
	}
	if clone.AssetY.Class.PolicyId != nil {
		t.Error("expected nil PolicyId to stay nil after clone")
	}
	if clone.AssetY.Class.Name != nil {
		t.Error("expected nil Name to stay nil after clone")
	}
}

func TestClonePoolStatePreservesSerializedState(t *testing.T) {
	state := &PoolState{
		PoolId: "pool1",
		AssetX: common.AssetAmount{
			Class:  common.AssetClass{PolicyId: []byte{}, Name: []byte{}},
			Amount: 1000,
		},
		AssetY: common.AssetAmount{
			Class:  common.AssetClass{PolicyId: []byte{}, Name: []byte{}},
			Amount: 2000,
		},
	}

	before, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal original: %v", err)
	}
	after, err := json.Marshal(ClonePoolState(state))
	if err != nil {
		t.Fatalf("marshal clone: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Errorf(
			"clone changed serialized state:\n before: %s\n after:  %s",
			before,
			after,
		)
	}
}

func TestPoolStateMarshalJSON(t *testing.T) {
	state := PoolState{
		PoolId:   "test-pool",
		Protocol: "spectrum",
		AssetX:   common.AssetAmount{Amount: 1000000},
		AssetY:   common.AssetAmount{Amount: 2000000},
		FeeNum:   997,
		FeeDenom: 1000,
	}

	data, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("unexpected error unmarshaling: %v", err)
	}

	// Check computed fields are present
	if _, ok := result["priceXY"]; !ok {
		t.Error("expected priceXY field in JSON output")
	}
	if _, ok := result["priceYX"]; !ok {
		t.Error("expected priceYX field in JSON output")
	}
	if _, ok := result["effectiveFee"]; !ok {
		t.Error("expected effectiveFee field in JSON output")
	}
}

func TestPoolStateQuote(t *testing.T) {
	// Known pool: reserveX=1_000_000, reserveY=2_000_000, 0.3% fee.
	// AssetX is a native token, AssetY is lovelace (empty policy/name).
	tokenPolicy := []byte{0xaa, 0xbb}
	tokenName := []byte{0x01}
	pool := &PoolState{
		PoolId:   "known-pool",
		Protocol: "minswap-v2",
		AssetX: common.AssetAmount{
			Class:  common.AssetClass{PolicyId: tokenPolicy, Name: tokenName},
			Amount: 1_000_000,
		},
		AssetY: common.AssetAmount{
			Class:  common.AssetClass{}, // lovelace
			Amount: 2_000_000,
		},
		FeeNum:   997,
		FeeDenom: 1000,
	}

	t.Run("swap X in (token -> ada)", func(t *testing.T) {
		out, impact, err := pool.Quote(tokenPolicy, tokenName, 1000)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// reserveOut*amountIn*feeNum / (reserveIn*feeDenom + amountIn*feeNum)
		// = 2_000_000*1000*997 / (1_000_000*1000 + 1000*997) = 1992
		if out != 1992 {
			t.Errorf("expected amountOut 1992, got %d", out)
		}
		// spot=2.0, execution=1.992 => impact = 0.4%
		if impact < 0.399 || impact > 0.401 {
			t.Errorf("expected price impact ~0.4%%, got %f", impact)
		}
	})

	t.Run("swap Y in (ada -> token)", func(t *testing.T) {
		out, impact, err := pool.Quote(nil, nil, 2000)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// 1_000_000*2000*997 / (2_000_000*1000 + 2000*997) = 996
		if out != 996 {
			t.Errorf("expected amountOut 996, got %d", out)
		}
		if impact <= 0 {
			t.Errorf("expected positive price impact, got %f", impact)
		}
	})

	t.Run("asset not in pool", func(t *testing.T) {
		if _, _, err := pool.Quote([]byte{0xde, 0xad}, []byte{0x99}, 1000); err == nil {
			t.Error("expected error for asset not in pool")
		}
	})

	t.Run("zero reserves", func(t *testing.T) {
		empty := &PoolState{
			AssetX: common.AssetAmount{
				Class: common.AssetClass{PolicyId: tokenPolicy, Name: tokenName},
			},
			AssetY:   common.AssetAmount{Class: common.AssetClass{}},
			FeeNum:   997,
			FeeDenom: 1000,
		}
		if _, _, err := empty.Quote(tokenPolicy, tokenName, 1000); err == nil {
			t.Error("expected error for zero reserves")
		}
	})

	t.Run("zero fee denominator", func(t *testing.T) {
		bad := &PoolState{
			AssetX: common.AssetAmount{
				Class:  common.AssetClass{PolicyId: tokenPolicy, Name: tokenName},
				Amount: 100,
			},
			AssetY:   common.AssetAmount{Class: common.AssetClass{}, Amount: 100},
			FeeNum:   997,
			FeeDenom: 0,
		}
		if _, _, err := bad.Quote(tokenPolicy, tokenName, 10); err == nil {
			t.Error("expected error for zero fee denominator")
		}
	})

	t.Run("fee numerator exceeds denominator", func(t *testing.T) {
		bad := &PoolState{
			AssetX: common.AssetAmount{
				Class:  common.AssetClass{PolicyId: tokenPolicy, Name: tokenName},
				Amount: 100,
			},
			AssetY:   common.AssetAmount{Class: common.AssetClass{}, Amount: 100},
			FeeNum:   1001,
			FeeDenom: 1000,
		}
		if _, _, err := bad.Quote(tokenPolicy, tokenName, 10); err == nil {
			t.Error("expected error for fee numerator greater than denominator")
		}
	})

	t.Run("zero amount in", func(t *testing.T) {
		out, impact, err := pool.Quote(tokenPolicy, tokenName, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out != 0 || impact != 0 {
			t.Errorf("expected zero out/impact for zero input, got %d/%f", out, impact)
		}
	})

	t.Run("rounded zero output has full price impact", func(t *testing.T) {
		tinyOutputPool := *pool
		tinyOutputPool.AssetY.Amount = 1

		out, impact, err := tinyOutputPool.Quote(tokenPolicy, tokenName, 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out != 0 {
			t.Fatalf("expected rounded zero output, got %d", out)
		}
		if impact < 99.999 || impact > 100.001 {
			t.Errorf("expected price impact ~100%%, got %f", impact)
		}
	})
}

func TestPoolAddressesLocator(t *testing.T) {
	// Each known parser must resolve to at least one mainnet pool address.
	parsers := []PoolParser{
		NewMinswapV1Parser(),
		NewMinswapV2Parser(),
		NewSundaeSwapV1Parser(),
		NewSundaeSwapV3Parser(),
		NewSplashV1Parser(),
		NewWingRidersV2Parser(),
		NewVyFiParser(),
		NewCSwapParser(),
		NewGeniusYieldParser(),
	}
	for _, p := range parsers {
		proto := p.Protocol()
		addrs := PoolAddresses(proto)
		if len(addrs) == 0 {
			t.Errorf("protocol %s has no pool addresses", proto)
		}
		loc, ok := Locator(proto)
		if !ok {
			t.Errorf("protocol %s has no locator", proto)
			continue
		}
		if loc.Network != "mainnet" {
			t.Errorf("protocol %s: expected mainnet, got %s", proto, loc.Network)
		}
	}

	if _, ok := Locator("does-not-exist"); ok {
		t.Error("expected unknown protocol to have no locator")
	}
}
