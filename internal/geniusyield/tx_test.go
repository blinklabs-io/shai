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

package geniusyield

import (
	"encoding/hex"
	"testing"
	"time"

	"github.com/Salvionied/apollo/serialization/TransactionInput"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/shai/internal/common"
)

func TestBuildRedeemerPlutusData_CompleteFill(t *testing.T) {
	output := orderFillOutput{
		orderId:      "test-order-1",
		isComplete:   true,
		inputAmount:  1000000,
		outputAmount: 500000,
	}

	redeemerData, err := buildRedeemerPlutusData(output)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check that Value is a Constructor with index 1 (CompleteFill)
	constr, ok := redeemerData.Value.(cbor.Constructor)
	if !ok {
		t.Fatalf("expected Constructor, got %T", redeemerData.Value)
	}

	if constr.Constructor() != 1 {
		t.Errorf(
			"expected constructor 1 for CompleteFill, got %d",
			constr.Constructor(),
		)
	}
}

func TestBuildRedeemerPlutusData_PartialFill(t *testing.T) {
	output := orderFillOutput{
		orderId:      "test-order-2",
		isComplete:   false,
		inputAmount:  500000,
		outputAmount: 250000,
	}

	redeemerData, err := buildRedeemerPlutusData(output)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check that Value is a Constructor with index 0 (PartialFill)
	constr, ok := redeemerData.Value.(cbor.Constructor)
	if !ok {
		t.Fatalf("expected Constructor, got %T", redeemerData.Value)
	}

	if constr.Constructor() != 0 {
		t.Errorf(
			"expected constructor 0 for PartialFill, got %d",
			constr.Constructor(),
		)
	}
}

func TestBuildOrderRedeemer_CompleteFill(t *testing.T) {
	output := orderFillOutput{
		orderId:      "test-order-1",
		isComplete:   true,
		inputAmount:  1000000,
		outputAmount: 500000,
	}

	redeemerBytes, err := buildOrderRedeemer(output)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(redeemerBytes) == 0 {
		t.Error("expected non-empty redeemer bytes")
	}

	// Decode and verify it's a CompleteFillRedeemer (constructor 1)
	var constr cbor.Constructor
	if _, err := cbor.Decode(redeemerBytes, &constr); err != nil {
		t.Fatalf("failed to decode redeemer: %v", err)
	}

	if constr.Constructor() != 1 {
		t.Errorf("expected constructor 1, got %d", constr.Constructor())
	}
}

func TestBuildOrderRedeemer_PartialFill(t *testing.T) {
	output := orderFillOutput{
		orderId:      "test-order-2",
		isComplete:   false,
		inputAmount:  750000,
		outputAmount: 375000,
	}

	redeemerBytes, err := buildOrderRedeemer(output)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(redeemerBytes) == 0 {
		t.Error("expected non-empty redeemer bytes")
	}

	// Decode and verify it's a PartialFillRedeemer (constructor 0)
	var constr cbor.Constructor
	if _, err := cbor.Decode(redeemerBytes, &constr); err != nil {
		t.Fatalf("failed to decode redeemer: %v", err)
	}

	if constr.Constructor() != 0 {
		t.Errorf("expected constructor 0, got %d", constr.Constructor())
	}
}

func TestBuildRationalDatum(t *testing.T) {
	tests := []struct {
		name  string
		num   int64
		denom int64
	}{
		{"positive rational", 3, 4},
		{"whole number", 5, 1},
		{"zero numerator", 0, 1},
		{"large values", 1000000000, 1000000},
		{"negative numerator", -3, 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildRationalDatum(tt.num, tt.denom)

			if result.Constructor() != 0 {
				t.Errorf("expected constructor 0, got %d", result.Constructor())
			}

			// Encode and decode to verify structure
			encoded, err := cbor.Encode(&result)
			if err != nil {
				t.Fatalf("failed to encode: %v", err)
			}

			if len(encoded) == 0 {
				t.Error("expected non-empty encoded bytes")
			}
		})
	}
}

func TestBuildOptionalPOSIX_Present(t *testing.T) {
	now := time.Now()
	result := buildOptionalPOSIX(&now)

	// Should be constructor 0 (Some)
	if result.Constructor() != 0 {
		t.Errorf(
			"expected constructor 0 for present value, got %d",
			result.Constructor(),
		)
	}

	// Encode and verify
	encoded, err := cbor.Encode(&result)
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	if len(encoded) == 0 {
		t.Error("expected non-empty encoded bytes")
	}
}

func TestBuildOptionalPOSIX_None(t *testing.T) {
	result := buildOptionalPOSIX(nil)

	// Should be constructor 1 (None)
	if result.Constructor() != 1 {
		t.Errorf(
			"expected constructor 1 for None, got %d",
			result.Constructor(),
		)
	}
}

func TestBuildContainedFeeDatum(t *testing.T) {
	result := buildContainedFeeDatum()

	if result.Constructor() != 0 {
		t.Errorf("expected constructor 0, got %d", result.Constructor())
	}

	// Encode and verify
	encoded, err := cbor.Encode(&result)
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	if len(encoded) == 0 {
		t.Error("expected non-empty encoded bytes")
	}
}

func TestBuildAddressDatum(t *testing.T) {
	// Valid 28-byte pubkey hash (56 hex chars)
	ownerPkh := "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4"
	ownerBytes, err := hex.DecodeString(ownerPkh)
	if err != nil {
		t.Fatalf("failed to decode hex: %v", err)
	}

	result := buildAddressDatum(ownerBytes)

	if result.Constructor() != 0 {
		t.Errorf("expected constructor 0, got %d", result.Constructor())
	}

	// Encode and verify
	encoded, err := cbor.Encode(&result)
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	if len(encoded) == 0 {
		t.Error("expected non-empty encoded bytes")
	}
}

func TestBuildAddressDatum_EmptyBytes(t *testing.T) {
	// Empty bytes should still produce a valid constructor
	result := buildAddressDatum([]byte{})

	// Should still return a constructor
	if result.Constructor() != 0 {
		t.Errorf("expected constructor 0, got %d", result.Constructor())
	}
}

func TestBuildAssetDatum(t *testing.T) {
	// Create a test asset that implements IsLovelace
	asset := common.AssetClass{
		PolicyId: []byte{0x01, 0x02, 0x03},
		Name:     []byte("TEST"),
	}

	result := buildAssetDatum(asset)

	if result.Constructor() != 0 {
		t.Errorf("expected constructor 0, got %d", result.Constructor())
	}
}

func TestBuildAssetDatum_Lovelace(t *testing.T) {
	// Lovelace has empty policy and name
	asset := common.AssetClass{
		PolicyId: []byte{},
		Name:     []byte{},
	}

	result := buildAssetDatum(asset)

	if result.Constructor() != 0 {
		t.Errorf("expected constructor 0, got %d", result.Constructor())
	}
}

func TestCalculateOwnerPayment_TakerLovelace(t *testing.T) {
	fill := orderFillOutput{
		orderId:      "test-order",
		isComplete:   true,
		inputAmount:  1000000,
		outputAmount: 5000000,
	}

	route := &Route{
		InputAsset: common.AssetClass{
			PolicyId: []byte{0x01, 0x02, 0x03},
			Name:     []byte("TOKEN"),
		},
		OutputAsset: common.AssetClass{
			PolicyId: []byte{},
			Name:     []byte{},
		}, // Lovelace
		TotalInput:  1000000,
		TotalOutput: 5000000,
	}

	lovelace, units := calculateOwnerPayment(fill, route, true)

	// Taker receives lovelace (output asset is lovelace)
	if lovelace != 5000000 {
		t.Errorf("expected lovelace 5000000, got %d", lovelace)
	}

	if len(units) != 0 {
		t.Errorf("expected no units for lovelace output, got %d", len(units))
	}
}

func TestCalculateOwnerPayment_TakerToken(t *testing.T) {
	fill := orderFillOutput{
		orderId:      "test-order",
		isComplete:   true,
		inputAmount:  5000000,
		outputAmount: 1000000,
	}

	route := &Route{
		InputAsset: common.AssetClass{
			PolicyId: []byte{},
			Name:     []byte{},
		}, // Lovelace
		OutputAsset: common.AssetClass{
			PolicyId: []byte{0x01, 0x02, 0x03},
			Name:     []byte("TOKEN"),
		},
		TotalInput:  5000000,
		TotalOutput: 1000000,
	}

	lovelace, units := calculateOwnerPayment(fill, route, true)

	// Taker receives minimum lovelace plus tokens
	if lovelace != minUtxoLovelace {
		t.Errorf("expected lovelace %d, got %d", minUtxoLovelace, lovelace)
	}

	if len(units) != 1 {
		t.Errorf("expected 1 unit, got %d", len(units))
	}
}

func TestCalculateOwnerPayment_MakerLovelace(t *testing.T) {
	fill := orderFillOutput{
		orderId:      "test-order",
		isComplete:   true,
		inputAmount:  1000000,
		outputAmount: 500000,
	}

	route := &Route{
		InputAsset: common.AssetClass{
			PolicyId: []byte{},
			Name:     []byte{},
		}, // Lovelace
		OutputAsset: common.AssetClass{
			PolicyId: []byte{0x01, 0x02, 0x03},
			Name:     []byte("TOKEN"),
		},
		TotalInput:  5000000,
		TotalOutput: 1000000,
	}

	lovelace, units := calculateOwnerPayment(fill, route, false)

	// Maker receives lovelace (input asset is lovelace)
	if lovelace != 1000000 {
		t.Errorf("expected lovelace 1000000, got %d", lovelace)
	}

	if len(units) != 0 {
		t.Errorf("expected no units, got %d", len(units))
	}
}

func TestCalculateOwnerPayment_MakerToken(t *testing.T) {
	fill := orderFillOutput{
		orderId:      "test-order",
		isComplete:   true,
		inputAmount:  500000,
		outputAmount: 2500000,
	}

	route := &Route{
		InputAsset: common.AssetClass{
			PolicyId: []byte{0x01, 0x02, 0x03},
			Name:     []byte("TOKEN"),
		},
		OutputAsset: common.AssetClass{
			PolicyId: []byte{},
			Name:     []byte{},
		}, // Lovelace
		TotalInput:  1000000,
		TotalOutput: 5000000,
	}

	lovelace, units := calculateOwnerPayment(fill, route, false)

	// Maker receives minimum lovelace plus tokens
	if lovelace != minUtxoLovelace {
		t.Errorf("expected lovelace %d, got %d", minUtxoLovelace, lovelace)
	}

	if len(units) != 1 {
		t.Errorf("expected 1 unit, got %d", len(units))
	}
}

func TestUtxoKey(t *testing.T) {
	txId := []byte{
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18,
		0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f, 0x20,
	}

	input := TransactionInput.TransactionInput{
		TransactionId: txId,
		Index:         5,
	}

	key := utxoKey(input)
	expected := hex.EncodeToString(txId) + "#5"

	if key != expected {
		t.Errorf("expected %s, got %s", expected, key)
	}
}

func TestUtxoKey_IndexZero(t *testing.T) {
	txId := []byte{
		0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff, 0x00, 0x11,
		0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99,
		0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff, 0x00, 0x11,
		0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99,
	}

	input := TransactionInput.TransactionInput{
		TransactionId: txId,
		Index:         0,
	}

	key := utxoKey(input)
	expected := hex.EncodeToString(txId) + "#0"

	if key != expected {
		t.Errorf("expected %s, got %s", expected, key)
	}
}

func TestEstimateFee(t *testing.T) {
	tests := []struct {
		name       string
		numInputs  int
		numOutputs int
		minFee     uint64
	}{
		{"single input/output", 1, 1, 200000},
		{"two inputs/outputs", 2, 2, 200000},
		{"many inputs", 5, 2, 200000},
		{"many outputs", 2, 5, 200000},
		{"zero inputs", 0, 1, 200000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fee := estimateFee(tt.numInputs, tt.numOutputs)

			if fee < tt.minFee {
				t.Errorf("expected fee >= %d, got %d", tt.minFee, fee)
			}

			// Fee should increase with more inputs/outputs
			if tt.numInputs > 0 || tt.numOutputs > 0 {
				baseFee := estimateFee(0, 0)
				if fee <= baseFee && (tt.numInputs > 0 || tt.numOutputs > 0) {
					t.Errorf(
						"fee should increase with inputs/outputs, base=%d, actual=%d",
						baseFee,
						fee,
					)
				}
			}
		})
	}
}

func TestEstimateFee_Scaling(t *testing.T) {
	// Fee should scale linearly with inputs
	fee1 := estimateFee(1, 1)
	fee2 := estimateFee(2, 1)
	fee3 := estimateFee(3, 1)

	// Check that adding inputs increases fee
	if fee2 <= fee1 {
		t.Errorf(
			"fee should increase with inputs: fee1=%d, fee2=%d",
			fee1,
			fee2,
		)
	}
	if fee3 <= fee2 {
		t.Errorf(
			"fee should increase with inputs: fee2=%d, fee3=%d",
			fee2,
			fee3,
		)
	}

	// Check that the increase is roughly linear
	diff1 := fee2 - fee1
	diff2 := fee3 - fee2
	if diff1 != diff2 {
		t.Errorf(
			"fee increase should be linear: diff1=%d, diff2=%d",
			diff1,
			diff2,
		)
	}
}

func TestBuildUpdatedOrderDatum(t *testing.T) {
	now := time.Now()
	order := &OrderState{
		OrderId: "test-order-123",
		Owner:   "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4",
		OfferedAsset: common.AssetClass{
			PolicyId: []byte{0x01},
			Name:     []byte("TKN"),
		},
		OfferedAmount:  1000000,
		OriginalAmount: 2000000,
		AskedAsset:     common.AssetClass{PolicyId: []byte{}, Name: []byte{}},
		Price:          2.0,
		PriceNum:       2,
		PriceDenom:     1,
		PartialFills:   3,
		StartTime:      &now,
		EndTime:        nil,
	}

	newAmount := uint64(500000)
	datum, err := buildUpdatedOrderDatum(order, newAmount)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if datum == nil {
		t.Fatal("expected non-nil datum")
	}

	// Verify it's a constructor
	constr, ok := datum.Value.(cbor.Constructor)
	if !ok {
		t.Fatalf("expected Constructor, got %T", datum.Value)
	}

	if constr.Constructor() != 0 {
		t.Errorf("expected constructor 0, got %d", constr.Constructor())
	}

	// Encode to verify it's valid CBOR
	encoded, err := cbor.Encode(datum.Value)
	if err != nil {
		t.Fatalf("failed to encode datum: %v", err)
	}

	if len(encoded) == 0 {
		t.Error("expected non-empty encoded datum")
	}
}

func TestBuildUpdatedOrderDatum_InvalidHex(t *testing.T) {
	order := &OrderState{
		OrderId: "test-order",
		Owner:   "not-valid-hex", // Invalid hex string
	}

	_, err := buildUpdatedOrderDatum(order, 100)
	if err == nil {
		t.Error("expected error for invalid hex owner")
	}
}

func TestOrderFillOutput_Fields(t *testing.T) {
	output := orderFillOutput{
		orderId:      "order-abc-123",
		isComplete:   true,
		inputAmount:  1500000,
		outputAmount: 750000,
	}

	if output.orderId != "order-abc-123" {
		t.Errorf("unexpected orderId: %s", output.orderId)
	}
	if !output.isComplete {
		t.Error("expected isComplete to be true")
	}
	if output.inputAmount != 1500000 {
		t.Errorf("unexpected inputAmount: %d", output.inputAmount)
	}
	if output.outputAmount != 750000 {
		t.Errorf("unexpected outputAmount: %d", output.outputAmount)
	}
}

func TestCalculateFillOutputs(t *testing.T) {
	gy := &GeniusYield{}

	newOrder := &OrderState{
		OrderId:       "taker-order",
		OfferedAmount: 1000000,
	}

	route := &Route{
		TotalInput:  1000000,
		TotalOutput: 500000,
		Legs: []RouteLeg{
			{
				Order: &OrderState{
					OrderId:       "maker-order-1",
					OfferedAmount: 300000,
				},
				InputAmount:  200000,
				OutputAmount: 300000,
			},
			{
				Order: &OrderState{
					OrderId:       "maker-order-2",
					OfferedAmount: 500000,
				},
				InputAmount:  300000,
				OutputAmount: 500000,
			},
		},
	}

	outputs, err := gy.calculateFillOutputs(route, newOrder)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have 3 outputs: taker + 2 makers
	if len(outputs) != 3 {
		t.Fatalf("expected 3 outputs, got %d", len(outputs))
	}

	// First output is for taker (new order)
	if outputs[0].orderId != "taker-order" {
		t.Errorf("expected taker order first, got %s", outputs[0].orderId)
	}
	if !outputs[0].isComplete {
		t.Error("taker order should be complete (totalInput >= offeredAmount)")
	}

	// Second output is maker-order-1
	if outputs[1].orderId != "maker-order-1" {
		t.Errorf("expected maker-order-1, got %s", outputs[1].orderId)
	}
	if !outputs[1].isComplete {
		t.Error(
			"maker-order-1 should be complete (outputAmount >= offeredAmount)",
		)
	}

	// Third output is maker-order-2
	if outputs[2].orderId != "maker-order-2" {
		t.Errorf("expected maker-order-2, got %s", outputs[2].orderId)
	}
	if !outputs[2].isComplete {
		t.Error(
			"maker-order-2 should be complete (outputAmount >= offeredAmount)",
		)
	}
}

func TestCalculateFillOutputs_PartialFills(t *testing.T) {
	gy := &GeniusYield{}

	newOrder := &OrderState{
		OrderId:       "taker-order",
		OfferedAmount: 1000000,
	}

	route := &Route{
		TotalInput:  500000, // Less than offered amount - partial fill
		TotalOutput: 250000,
		Legs: []RouteLeg{
			{
				Order: &OrderState{
					OrderId:       "maker-order-1",
					OfferedAmount: 1000000,
				},
				InputAmount:  250000,
				OutputAmount: 500000, // Less than offered - partial
			},
		},
	}

	outputs, err := gy.calculateFillOutputs(route, newOrder)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(outputs) != 2 {
		t.Fatalf("expected 2 outputs, got %d", len(outputs))
	}

	// Taker should be partial fill
	if outputs[0].isComplete {
		t.Error("taker order should be partial fill")
	}

	// Maker should be partial fill
	if outputs[1].isComplete {
		t.Error("maker order should be partial fill")
	}
}

func TestBuildMatchTxOpts_Fields(t *testing.T) {
	// Test that the struct can be created with expected fields
	opts := buildMatchTxOpts{
		route: &Route{
			TotalInput:  1000000,
			TotalOutput: 500000,
		},
		newOrder: &OrderState{
			OrderId: "test-order",
		},
		newOrderOutput: nil, // Would be a ledger.TransactionOutput in real use
	}

	if opts.route.TotalInput != 1000000 {
		t.Errorf("unexpected TotalInput: %d", opts.route.TotalInput)
	}
	if opts.newOrder.OrderId != "test-order" {
		t.Errorf("unexpected OrderId: %s", opts.newOrder.OrderId)
	}
}

func TestConstants(t *testing.T) {
	// Verify constants have expected values
	if matchTxTtlSlots != 60 {
		t.Errorf("expected matchTxTtlSlots=60, got %d", matchTxTtlSlots)
	}

	if matchTxFee != 500_000 {
		t.Errorf("expected matchTxFee=500000, got %d", matchTxFee)
	}

	if minUtxoLovelace != 2_000_000 {
		t.Errorf("expected minUtxoLovelace=2000000, got %d", minUtxoLovelace)
	}

	if defaultMatcherReward != 1_500_000 {
		t.Errorf(
			"expected defaultMatcherReward=1500000, got %d",
			defaultMatcherReward,
		)
	}
}

func TestBuildSortedInputIndexMap_Empty(t *testing.T) {
	// Empty slice should return empty map
	result := buildSortedInputIndexMap(nil)

	if len(result) != 0 {
		t.Errorf("expected empty map, got %d entries", len(result))
	}
}

func TestFeeConfig_CalculateMakerFee(t *testing.T) {
	tests := []struct {
		name     string
		config   FeeConfig
		amount   uint64
		expected uint64
	}{
		{
			name: "flat fee only",
			config: FeeConfig{
				MakerFeeFlat:    1000000,
				MakerFeePercent: 0,
			},
			amount:   10000000,
			expected: 1000000,
		},
		{
			name: "percent fee only",
			config: FeeConfig{
				MakerFeeFlat:    0,
				MakerFeePercent: 0.01, // 1%
			},
			amount:   10000000,
			expected: 100000, // 1% of 10 ADA
		},
		{
			name: "flat plus percent",
			config: FeeConfig{
				MakerFeeFlat:    500000,
				MakerFeePercent: 0.005, // 0.5%
			},
			amount:   10000000,
			expected: 550000, // 500000 + 50000
		},
		{
			name: "percent capped by max",
			config: FeeConfig{
				MakerFeeFlat:       0,
				MakerFeePercent:    0.1, // 10%
				MakerFeePercentMax: 1000000,
			},
			amount:   100000000, // 100 ADA
			expected: 1000000,   // Capped at 1 ADA
		},
		{
			name: "zero amount",
			config: FeeConfig{
				MakerFeeFlat:    1000000,
				MakerFeePercent: 0.01,
			},
			amount:   0,
			expected: 1000000, // Just flat fee
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.CalculateMakerFee(tt.amount)
			if result != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, result)
			}
		})
	}
}

func TestFeeConfig_CalculateTakerFee(t *testing.T) {
	config := FeeConfig{
		TakerFee: 500000,
	}

	// Taker fee is fixed regardless of amount
	fee1 := config.CalculateTakerFee(1000000)
	fee2 := config.CalculateTakerFee(100000000)

	if fee1 != 500000 {
		t.Errorf("expected 500000, got %d", fee1)
	}
	if fee2 != 500000 {
		t.Errorf("expected 500000, got %d", fee2)
	}
}

func TestFeeConfigDefaults(t *testing.T) {
	// Test that default constants are set correctly
	if defaultMakerFeeFlat != 1_000_000 {
		t.Errorf(
			"expected defaultMakerFeeFlat=1000000, got %d",
			defaultMakerFeeFlat,
		)
	}

	if defaultMakerFeePercent != 0.003 {
		t.Errorf(
			"expected defaultMakerFeePercent=0.003, got %f",
			defaultMakerFeePercent,
		)
	}

	if defaultTakerFee != 500_000 {
		t.Errorf("expected defaultTakerFee=500000, got %d", defaultTakerFee)
	}

	if defaultMatcherReward != 1_500_000 {
		t.Errorf(
			"expected defaultMatcherReward=1500000, got %d",
			defaultMatcherReward,
		)
	}
}
