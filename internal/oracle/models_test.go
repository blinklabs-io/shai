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

package oracle

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/blinklabs-io/shai/internal/common"
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

	var result map[string]interface{}
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

func TestNewPriceUpdate(t *testing.T) {
	state := &PoolState{
		PoolId:   "test-pool",
		Protocol: "spectrum",
		AssetX: common.AssetAmount{
			Class:  common.Lovelace(),
			Amount: 1000000,
		},
		AssetY: common.AssetAmount{
			Class: common.AssetClass{
				PolicyId: []byte{0x01},
				Name:     []byte("T"),
			},
			Amount: 2000000,
		},
		Slot:      12345,
		Timestamp: time.Now(),
	}

	// Test with no previous price
	update := NewPriceUpdate(state, 0)
	if update.PoolId != state.PoolId {
		t.Errorf("expected pool ID %s, got %s", state.PoolId, update.PoolId)
	}
	if update.PriceChangeX != 0 {
		t.Errorf("expected no price change, got %f", update.PriceChangeX)
	}

	// Test with previous price
	prevPrice := 1.5
	update = NewPriceUpdate(state, prevPrice)
	if update.PrevPriceXY != prevPrice {
		t.Errorf(
			"expected prev price %f, got %f",
			prevPrice,
			update.PrevPriceXY,
		)
	}
	// Current price is 2.0, prev was 1.5, so change should be ~33.33%
	expectedChange := (2.0 - 1.5) / 1.5 * 100
	if update.PriceChangeX < expectedChange-0.1 ||
		update.PriceChangeX > expectedChange+0.1 {
		t.Errorf(
			"expected price change ~%f, got %f",
			expectedChange,
			update.PriceChangeX,
		)
	}
}
