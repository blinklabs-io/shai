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
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/shai/internal/common"
	"github.com/blinklabs-io/shai/internal/oracle/vyfi"
)

func TestNewVyFiParser(t *testing.T) {
	parser := NewVyFiParser()
	if parser == nil {
		t.Fatal("expected non-nil parser")
	}
	if parser.Protocol() != "vyfi" {
		t.Errorf("expected protocol 'vyfi', got %s", parser.Protocol())
	}
}

func TestVyFiGeneratePoolId(t *testing.T) {
	// Test with NFT
	poolId := generateVyFiPoolId(
		"abc123",
		common.AssetClass{},
		common.AssetClass{},
	)
	expected := "vyfi_abc123"
	if poolId != expected {
		t.Errorf("expected pool ID %s, got %s", expected, poolId)
	}

	// Test without NFT (fallback to asset pair)
	poolId = generateVyFiPoolId(
		"",
		common.AssetClass{PolicyId: []byte{0xab, 0xcd}, Name: []byte("TokenA")},
		common.AssetClass{PolicyId: []byte{0x12, 0x34}, Name: []byte("TokenB")},
	)
	expected = "vyfi_abcd.546f6b656e41_1234.546f6b656e42"
	if poolId != expected {
		t.Errorf("expected pool ID %s, got %s", expected, poolId)
	}
}

func TestVyFiPoolDatumUnmarshal(t *testing.T) {
	// Build a pool datum: Pool = #6.121([treasuryA, treasuryB, issuedShares])
	datum := cbor.NewConstructor(0, cbor.IndefLengthList{
		int64(1000000000), // treasuryA (1000 ADA worth)
		int64(500000000),  // treasuryB
		int64(750000000),  // issuedShares
	})

	cborData, err := cbor.Encode(&datum)
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	var poolDatum vyfi.PoolDatum
	if _, err := cbor.Decode(cborData, &poolDatum); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	if poolDatum.TreasuryA != 1000000000 {
		t.Errorf("expected treasuryA 1000000000, got %d", poolDatum.TreasuryA)
	}
	if poolDatum.TreasuryB != 500000000 {
		t.Errorf("expected treasuryB 500000000, got %d", poolDatum.TreasuryB)
	}
	if poolDatum.IssuedShares != 750000000 {
		t.Errorf(
			"expected issuedShares 750000000, got %d",
			poolDatum.IssuedShares,
		)
	}
}

func TestVyFiOrderTypeAddLiquidity(t *testing.T) {
	// AddLiquidity = #6.121([minWantedShares])
	orderDetails := cbor.NewConstructor(0, cbor.IndefLengthList{
		int64(100000),
	})

	cborData, err := cbor.Encode(&orderDetails)
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	var details vyfi.OrderDetails
	if _, err := cbor.Decode(cborData, &details); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	if details.Type != vyfi.OrderTypeAddLiquidity {
		t.Errorf("expected AddLiquidity type, got %d", details.Type)
	}
	if details.MinWantedShares != 100000 {
		t.Errorf(
			"expected minWantedShares 100000, got %d",
			details.MinWantedShares,
		)
	}
	if !details.IsLiquidity() {
		t.Error("expected IsLiquidity to be true")
	}
}

func TestVyFiOrderTypeTradeAToB(t *testing.T) {
	// TradeAToB = #6.124([minWantedTokens])
	// Constructor 3 in our mapping
	orderDetails := cbor.NewConstructor(3, cbor.IndefLengthList{
		int64(50000),
	})

	cborData, err := cbor.Encode(&orderDetails)
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	var details vyfi.OrderDetails
	if _, err := cbor.Decode(cborData, &details); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	if details.Type != vyfi.OrderTypeTradeAToB {
		t.Errorf("expected TradeAToB type, got %d", details.Type)
	}
	if details.MinWantedTokens != 50000 {
		t.Errorf(
			"expected minWantedTokens 50000, got %d",
			details.MinWantedTokens,
		)
	}
	if !details.IsSwap() {
		t.Error("expected IsSwap to be true")
	}
}

func TestVyFiOrderTypeTradeBToA(t *testing.T) {
	// TradeBToA = #6.125([minWantedTokens])
	// Constructor 4 in our mapping
	orderDetails := cbor.NewConstructor(4, cbor.IndefLengthList{
		int64(75000),
	})

	cborData, err := cbor.Encode(&orderDetails)
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	var details vyfi.OrderDetails
	if _, err := cbor.Decode(cborData, &details); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	if details.Type != vyfi.OrderTypeTradeBToA {
		t.Errorf("expected TradeBToA type, got %d", details.Type)
	}
	if details.MinWantedTokens != 75000 {
		t.Errorf(
			"expected minWantedTokens 75000, got %d",
			details.MinWantedTokens,
		)
	}
	if !details.IsSwap() {
		t.Error("expected IsSwap to be true")
	}
}

func TestVyFiOrderTypeServe(t *testing.T) {
	// Serve = #6.123([])
	// Constructor 2 with no fields
	orderDetails := cbor.NewConstructor(2, cbor.IndefLengthList{})

	cborData, err := cbor.Encode(&orderDetails)
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	var details vyfi.OrderDetails
	if _, err := cbor.Decode(cborData, &details); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	if details.Type != vyfi.OrderTypeServe {
		t.Errorf("expected Serve type, got %d", details.Type)
	}
	if details.IsSwap() {
		t.Error("expected IsSwap to be false for Serve")
	}
	if details.IsLiquidity() {
		t.Error("expected IsLiquidity to be false for Serve")
	}
}

func TestVyFiOrderTypeZapInA(t *testing.T) {
	// ZapInA = #6.126([minWantedShares])
	// Constructor 5
	orderDetails := cbor.NewConstructor(5, cbor.IndefLengthList{
		int64(200000),
	})

	cborData, err := cbor.Encode(&orderDetails)
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	var details vyfi.OrderDetails
	if _, err := cbor.Decode(cborData, &details); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	if details.Type != vyfi.OrderTypeZapInA {
		t.Errorf("expected ZapInA type, got %d", details.Type)
	}
	if details.MinWantedShares != 200000 {
		t.Errorf(
			"expected minWantedShares 200000, got %d",
			details.MinWantedShares,
		)
	}
	if !details.IsLiquidity() {
		t.Error("expected IsLiquidity to be true for ZapInA")
	}
}

func TestVyFiParserParsePoolDatum(t *testing.T) {
	// Build a pool datum
	datum := cbor.NewConstructor(0, cbor.IndefLengthList{
		int64(2000000000), // treasuryA
		int64(1000000000), // treasuryB
		int64(1500000000), // issuedShares
	})

	cborData, err := cbor.Encode(&datum)
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	parser := NewVyFiParser()

	assetA := common.AssetClass{
		PolicyId: []byte{},
		Name:     []byte{},
	}
	assetB := common.AssetClass{
		PolicyId: []byte{0xab, 0xcd},
		Name:     []byte("TEST"),
	}

	state, err := parser.ParsePoolDatum(
		cborData,
		"abc123def456",
		0,
		12345,
		time.Now(),
		assetA,
		assetB,
		"pool_nft_123",
	)
	if err != nil {
		t.Fatalf("failed to parse datum: %v", err)
	}

	if state.Protocol != "vyfi" {
		t.Errorf("expected protocol 'vyfi', got %s", state.Protocol)
	}
	if state.PoolId != "vyfi_pool_nft_123" {
		t.Errorf("expected pool ID 'vyfi_pool_nft_123', got %s", state.PoolId)
	}
	if state.Slot != 12345 {
		t.Errorf("expected slot 12345, got %d", state.Slot)
	}
	if state.AssetX.Amount != 2000000000 {
		t.Errorf(
			"expected assetX amount 2000000000, got %d",
			state.AssetX.Amount,
		)
	}
	if state.AssetY.Amount != 1000000000 {
		t.Errorf(
			"expected assetY amount 1000000000, got %d",
			state.AssetY.Amount,
		)
	}

	// Check fee (0.3% = 997/1000)
	if state.FeeNum != 997 {
		t.Errorf("expected feeNum 997, got %d", state.FeeNum)
	}
	if state.FeeDenom != 1000 {
		t.Errorf("expected feeDenom 1000, got %d", state.FeeDenom)
	}
}

func TestVyFiParserParsePoolDatumSimple(t *testing.T) {
	// Build a pool datum
	datum := cbor.NewConstructor(0, cbor.IndefLengthList{
		int64(500000000),
		int64(250000000),
		int64(375000000),
	})

	cborData, err := cbor.Encode(&datum)
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	parser := NewVyFiParser()
	poolDatum, err := parser.ParsePoolDatumSimple(cborData)
	if err != nil {
		t.Fatalf("failed to parse datum: %v", err)
	}

	if poolDatum.TreasuryA != 500000000 {
		t.Errorf("expected treasuryA 500000000, got %d", poolDatum.TreasuryA)
	}
	if poolDatum.TreasuryB != 250000000 {
		t.Errorf("expected treasuryB 250000000, got %d", poolDatum.TreasuryB)
	}
	if poolDatum.IssuedShares != 375000000 {
		t.Errorf(
			"expected issuedShares 375000000, got %d",
			poolDatum.IssuedShares,
		)
	}
}
