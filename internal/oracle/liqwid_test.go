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
	"github.com/blinklabs-io/shai/internal/oracle/liqwid"
)

func TestNewLiqwidParser(t *testing.T) {
	parser := NewLiqwidParser()
	if parser == nil {
		t.Fatal("expected non-nil parser")
	}
	if parser.Protocol() != "liqwid" {
		t.Errorf("expected protocol 'liqwid', got %s", parser.Protocol())
	}
}

func TestLiqwidGenerateMarketId(t *testing.T) {
	marketId := generateLiqwidMarketId(
		[]byte{0xab, 0xcd, 0xef},
		[]byte("ADAMarket"),
	)
	expected := "liqwid_abcdef.4144414d61726b6574"
	if marketId != expected {
		t.Errorf("expected market ID %s, got %s", expected, marketId)
	}
}

func TestLiqwidGeneratePositionId(t *testing.T) {
	positionId := generateLiqwidPositionId(
		"abc123def456789012345678901234567890",
		2,
	)
	expected := "liqwid_pos_abc123def4567890#2"
	if positionId != expected {
		t.Errorf("expected position ID %s, got %s", expected, positionId)
	}
}

func TestLiqwidAssetUnmarshal(t *testing.T) {
	// Build an asset: Constructor 0 with [policyId, assetName]
	asset := cbor.NewConstructor(0, cbor.IndefLengthList{
		[]byte{0x12, 0x34, 0x56},
		[]byte("qADA"),
	})

	cborData, err := cbor.Encode(&asset)
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	var liqwidAsset liqwid.Asset
	if _, err := cbor.Decode(cborData, &liqwidAsset); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	if string(liqwidAsset.PolicyId) != string([]byte{0x12, 0x34, 0x56}) {
		t.Error("policy ID mismatch")
	}
	if string(liqwidAsset.AssetName) != "qADA" {
		t.Errorf(
			"expected asset name 'qADA', got '%s'",
			string(liqwidAsset.AssetName),
		)
	}
}

func TestLiqwidAssetIsLovelace(t *testing.T) {
	// Test ADA (empty policy)
	adaAsset := liqwid.Asset{
		PolicyId:  []byte{},
		AssetName: []byte{},
	}
	if !adaAsset.IsLovelace() {
		t.Error("expected ADA to be lovelace")
	}

	// Test non-ADA asset
	tokenAsset := liqwid.Asset{
		PolicyId:  []byte{0x12, 0x34},
		AssetName: []byte("TOKEN"),
	}
	if tokenAsset.IsLovelace() {
		t.Error("expected token not to be lovelace")
	}
}

func TestLiqwidCredentialUnmarshal(t *testing.T) {
	// PubKeyHash credential (Constructor 0)
	pubKeyCredential := cbor.NewConstructor(0, cbor.IndefLengthList{
		[]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
			0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
			0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18,
			0x19, 0x1a, 0x1b, 0x1c},
	})

	cborData, err := cbor.Encode(&pubKeyCredential)
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	var cred liqwid.Credential
	if _, err := cbor.Decode(cborData, &cred); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	if !cred.IsPubKey() {
		t.Error("expected PubKey credential")
	}
	if cred.IsScript() {
		t.Error("expected not a script credential")
	}
	if len(cred.Hash) != 28 {
		t.Errorf("expected hash length 28, got %d", len(cred.Hash))
	}
}

func TestLiqwidMarketDatumUnmarshal(t *testing.T) {
	// Build market NFT asset
	marketNft := cbor.NewConstructor(0, cbor.IndefLengthList{
		[]byte{0xaa, 0xbb, 0xcc},
		[]byte("ADAMarketNFT"),
	})

	// Build underlying asset (ADA)
	underlyingAsset := cbor.NewConstructor(0, cbor.IndefLengthList{
		[]byte{},
		[]byte{},
	})

	// Build qToken asset
	qTokenAsset := cbor.NewConstructor(0, cbor.IndefLengthList{
		[]byte{0x11, 0x22, 0x33},
		[]byte("qADA"),
	})

	// Build market datum
	datum := cbor.NewConstructor(0, cbor.IndefLengthList{
		marketNft,
		underlyingAsset,
		qTokenAsset,
		uint64(1000000000000), // totalSupply (1M ADA)
		uint64(500000000000),  // totalBorrows (500K ADA)
		uint64(10000000000),   // reserveAmount (10K ADA)
		uint64(500),           // interestRate (5% = 500 basis points)
		uint64(7500),          // collateralFactor (75% = 7500 basis points)
	})

	cborData, err := cbor.Encode(&datum)
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	var marketDatum liqwid.MarketDatum
	if _, err := cbor.Decode(cborData, &marketDatum); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	if marketDatum.TotalSupply != 1000000000000 {
		t.Errorf(
			"expected totalSupply 1000000000000, got %d",
			marketDatum.TotalSupply,
		)
	}
	if marketDatum.TotalBorrows != 500000000000 {
		t.Errorf(
			"expected totalBorrows 500000000000, got %d",
			marketDatum.TotalBorrows,
		)
	}
	if marketDatum.ReserveAmount != 10000000000 {
		t.Errorf(
			"expected reserveAmount 10000000000, got %d",
			marketDatum.ReserveAmount,
		)
	}
	if marketDatum.InterestRate != 500 {
		t.Errorf("expected interestRate 500, got %d", marketDatum.InterestRate)
	}
	if marketDatum.CollateralFactor != 7500 {
		t.Errorf(
			"expected collateralFactor 7500, got %d",
			marketDatum.CollateralFactor,
		)
	}

	// Test utilization rate (50%)
	expectedUtilization := 0.5
	if marketDatum.UtilizationRate() != expectedUtilization {
		t.Errorf(
			"expected utilization rate %f, got %f",
			expectedUtilization,
			marketDatum.UtilizationRate(),
		)
	}

	// Test available liquidity
	expectedLiquidity := uint64(500000000000)
	if marketDatum.AvailableLiquidity() != expectedLiquidity {
		t.Errorf(
			"expected available liquidity %d, got %d",
			expectedLiquidity,
			marketDatum.AvailableLiquidity(),
		)
	}

	// Test collateral factor float
	expectedCF := 0.75
	if marketDatum.CollateralFactorFloat() != expectedCF {
		t.Errorf(
			"expected collateral factor %f, got %f",
			expectedCF,
			marketDatum.CollateralFactorFloat(),
		)
	}

	// Test interest rate float
	expectedIR := 0.05
	if marketDatum.InterestRateFloat() != expectedIR {
		t.Errorf(
			"expected interest rate %f, got %f",
			expectedIR,
			marketDatum.InterestRateFloat(),
		)
	}
}

func TestLiqwidParserParseMarketDatum(t *testing.T) {
	// Build test market datum
	marketNft := cbor.NewConstructor(0, cbor.IndefLengthList{
		[]byte{0xde, 0xad, 0xbe, 0xef},
		[]byte("Market1"),
	})

	underlyingAsset := cbor.NewConstructor(0, cbor.IndefLengthList{
		[]byte{},
		[]byte{},
	})

	qTokenAsset := cbor.NewConstructor(0, cbor.IndefLengthList{
		[]byte{0x99, 0x88, 0x77},
		[]byte("qADA"),
	})

	datum := cbor.NewConstructor(0, cbor.IndefLengthList{
		marketNft,
		underlyingAsset,
		qTokenAsset,
		uint64(2000000000000), // 2M ADA supply
		uint64(800000000000),  // 800K ADA borrowed
		uint64(20000000000),   // 20K ADA reserves
		uint64(350),           // 3.5% interest
		uint64(8000),          // 80% collateral factor
	})

	cborData, err := cbor.Encode(&datum)
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	parser := NewLiqwidParser()
	state, err := parser.ParseMarketDatum(
		cborData,
		"abc123def456",
		0,
		12345,
		time.Now(),
	)
	if err != nil {
		t.Fatalf("failed to parse datum: %v", err)
	}

	if state.Protocol != "liqwid" {
		t.Errorf("expected protocol 'liqwid', got %s", state.Protocol)
	}
	if state.MarketId != "liqwid_deadbeef.4d61726b657431" {
		t.Errorf("unexpected market ID: %s", state.MarketId)
	}
	if state.TotalSupply != 2000000000000 {
		t.Errorf(
			"expected totalSupply 2000000000000, got %d",
			state.TotalSupply,
		)
	}
	if state.TotalBorrows != 800000000000 {
		t.Errorf(
			"expected totalBorrows 800000000000, got %d",
			state.TotalBorrows,
		)
	}
	if state.Slot != 12345 {
		t.Errorf("expected slot 12345, got %d", state.Slot)
	}

	// Check utilization rate (40%)
	expectedUtilization := 0.4
	if state.UtilizationRate != expectedUtilization {
		t.Errorf(
			"expected utilization %f, got %f",
			expectedUtilization,
			state.UtilizationRate,
		)
	}

	// Check available liquidity
	expectedLiquidity := uint64(1200000000000)
	if state.AvailableLiquidity() != expectedLiquidity {
		t.Errorf(
			"expected available liquidity %d, got %d",
			expectedLiquidity,
			state.AvailableLiquidity(),
		)
	}
}

func TestLiqwidSupplyPositionDatumUnmarshal(t *testing.T) {
	// Build owner credential
	owner := cbor.NewConstructor(0, cbor.IndefLengthList{
		make([]byte, 28),
	})

	// Build market NFT
	marketNft := cbor.NewConstructor(0, cbor.IndefLengthList{
		[]byte{0xaa, 0xbb},
		[]byte("Market"),
	})

	// Build supply position datum
	datum := cbor.NewConstructor(0, cbor.IndefLengthList{
		owner,
		marketNft,
		uint64(1000000000), // 1000 qTokens
		uint64(50000000),   // deposit slot
	})

	cborData, err := cbor.Encode(&datum)
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	var supplyDatum liqwid.SupplyPositionDatum
	if _, err := cbor.Decode(cborData, &supplyDatum); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	if supplyDatum.QTokenAmount != 1000000000 {
		t.Errorf(
			"expected qTokenAmount 1000000000, got %d",
			supplyDatum.QTokenAmount,
		)
	}
	if supplyDatum.DepositSlot != 50000000 {
		t.Errorf(
			"expected depositSlot 50000000, got %d",
			supplyDatum.DepositSlot,
		)
	}
}

func TestLiqwidBorrowPositionDatumUnmarshal(t *testing.T) {
	// Build owner credential
	owner := cbor.NewConstructor(0, cbor.IndefLengthList{
		make([]byte, 28),
	})

	// Build market NFT
	marketNft := cbor.NewConstructor(0, cbor.IndefLengthList{
		[]byte{0xcc, 0xdd},
		[]byte("BorrowMarket"),
	})

	// Build borrow position datum
	datum := cbor.NewConstructor(0, cbor.IndefLengthList{
		owner,
		marketNft,
		uint64(500000000),           // borrow amount
		uint64(1000000000000000000), // borrow index (1e18)
		uint64(60000000),            // borrow slot
	})

	cborData, err := cbor.Encode(&datum)
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	var borrowDatum liqwid.BorrowPositionDatum
	if _, err := cbor.Decode(cborData, &borrowDatum); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	if borrowDatum.BorrowAmount != 500000000 {
		t.Errorf(
			"expected borrowAmount 500000000, got %d",
			borrowDatum.BorrowAmount,
		)
	}
	if borrowDatum.BorrowIndex != 1000000000000000000 {
		t.Errorf(
			"expected borrowIndex 1000000000000000000, got %d",
			borrowDatum.BorrowIndex,
		)
	}
	if borrowDatum.BorrowSlot != 60000000 {
		t.Errorf("expected borrowSlot 60000000, got %d", borrowDatum.BorrowSlot)
	}
}

func TestLiqwidOracleDatumUnmarshal(t *testing.T) {
	// Build asset
	asset := cbor.NewConstructor(0, cbor.IndefLengthList{
		[]byte{},
		[]byte{},
	})

	// Build oracle datum
	datum := cbor.NewConstructor(0, cbor.IndefLengthList{
		asset,
		uint64(350000),       // price ($0.35 if denom is 1M)
		uint64(1000000),      // denominator
		int64(1700000000000), // validFrom (POSIX ms)
		int64(1700003600000), // validTo (1 hour later)
	})

	cborData, err := cbor.Encode(&datum)
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	var oracleDatum liqwid.OracleDatum
	if _, err := cbor.Decode(cborData, &oracleDatum); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	if oracleDatum.Price != 350000 {
		t.Errorf("expected price 350000, got %d", oracleDatum.Price)
	}
	if oracleDatum.Denominator != 1000000 {
		t.Errorf(
			"expected denominator 1000000, got %d",
			oracleDatum.Denominator,
		)
	}

	// Test price float
	expectedPrice := 0.35
	if oracleDatum.PriceFloat() != expectedPrice {
		t.Errorf(
			"expected price %f, got %f",
			expectedPrice,
			oracleDatum.PriceFloat(),
		)
	}
}

func TestLiqwidInterestRateModelDatumUnmarshal(t *testing.T) {
	// Build interest rate model datum
	datum := cbor.NewConstructor(0, cbor.IndefLengthList{
		uint64(100),  // baseRatePerSlot
		uint64(200),  // multiplierPerSlot
		uint64(1000), // jumpMultiplierPerSlot
		uint64(8000), // kink (80%)
	})

	cborData, err := cbor.Encode(&datum)
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	var modelDatum liqwid.InterestRateModelDatum
	if _, err := cbor.Decode(cborData, &modelDatum); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	if modelDatum.BaseRatePerSlot != 100 {
		t.Errorf(
			"expected baseRatePerSlot 100, got %d",
			modelDatum.BaseRatePerSlot,
		)
	}
	if modelDatum.Kink != 8000 {
		t.Errorf("expected kink 8000, got %d", modelDatum.Kink)
	}

	// Test kink float
	expectedKink := 0.8
	if modelDatum.KinkFloat() != expectedKink {
		t.Errorf(
			"expected kink %f, got %f",
			expectedKink,
			modelDatum.KinkFloat(),
		)
	}
}

func TestLiqwidMarketStateKey(t *testing.T) {
	state := &LiqwidMarketState{
		MarketId: "test_market_123",
	}
	expected := "liqwid:test_market_123"
	if state.Key() != expected {
		t.Errorf("expected key %s, got %s", expected, state.Key())
	}
}

func TestLiqwidSupplyStateKey(t *testing.T) {
	state := &LiqwidSupplyState{
		PositionId: "supply_pos_123",
	}
	expected := "liqwid:supply:supply_pos_123"
	if state.Key() != expected {
		t.Errorf("expected key %s, got %s", expected, state.Key())
	}
}

func TestLiqwidBorrowStateKey(t *testing.T) {
	state := &LiqwidBorrowState{
		PositionId: "borrow_pos_456",
	}
	expected := "liqwid:borrow:borrow_pos_456"
	if state.Key() != expected {
		t.Errorf("expected key %s, got %s", expected, state.Key())
	}
}

func TestLiqwidUtilizationRateZeroSupply(t *testing.T) {
	marketDatum := liqwid.MarketDatum{
		TotalSupply:  0,
		TotalBorrows: 0,
	}
	if marketDatum.UtilizationRate() != 0 {
		t.Error("expected utilization rate 0 for zero supply")
	}
}

func TestLiqwidAvailableLiquidityFullyBorrowed(t *testing.T) {
	marketDatum := liqwid.MarketDatum{
		TotalSupply:  1000000,
		TotalBorrows: 1000000,
	}
	if marketDatum.AvailableLiquidity() != 0 {
		t.Error("expected 0 available liquidity when fully borrowed")
	}

	// Test overborrowed edge case (shouldn't happen but handle gracefully)
	marketDatum.TotalBorrows = 1500000
	if marketDatum.AvailableLiquidity() != 0 {
		t.Error("expected 0 available liquidity when overborrowed")
	}
}

func TestLiqwidParserParseMarketDatumSimple(t *testing.T) {
	// Build minimal market datum
	marketNft := cbor.NewConstructor(0, cbor.IndefLengthList{
		[]byte{0x01},
		[]byte("M"),
	})
	underlyingAsset := cbor.NewConstructor(0, cbor.IndefLengthList{
		[]byte{},
		[]byte{},
	})
	qTokenAsset := cbor.NewConstructor(0, cbor.IndefLengthList{
		[]byte{0x02},
		[]byte("q"),
	})

	datum := cbor.NewConstructor(0, cbor.IndefLengthList{
		marketNft,
		underlyingAsset,
		qTokenAsset,
		uint64(100),
		uint64(50),
		uint64(5),
		uint64(300),
		uint64(7000),
	})

	cborData, err := cbor.Encode(&datum)
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	parser := NewLiqwidParser()
	marketDatum, err := parser.ParseMarketDatumSimple(cborData)
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	if marketDatum.TotalSupply != 100 {
		t.Errorf("expected totalSupply 100, got %d", marketDatum.TotalSupply)
	}
}

func TestLiqwidOracleStatePriceFloat(t *testing.T) {
	state := &LiqwidOracleState{
		Price:       0,
		Denominator: 0,
	}
	if state.PriceFloat() != 0 {
		t.Error("expected 0 price for zero denominator")
	}

	state.Price = 500
	state.Denominator = 1000
	if state.PriceFloat() != 0.5 {
		t.Errorf("expected price 0.5, got %f", state.PriceFloat())
	}
}
