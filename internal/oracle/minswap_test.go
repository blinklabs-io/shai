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
)

func TestNewMinswapV1Parser(t *testing.T) {
	parser := NewMinswapV1Parser()
	if parser == nil {
		t.Fatal("expected non-nil parser")
	}
	if parser.Protocol() != "minswap-v1" {
		t.Errorf("expected protocol 'minswap-v1', got %s", parser.Protocol())
	}
}

func TestNewMinswapV2Parser(t *testing.T) {
	parser := NewMinswapV2Parser()
	if parser == nil {
		t.Fatal("expected non-nil parser")
	}
	if parser.Protocol() != "minswap-v2" {
		t.Errorf("expected protocol 'minswap-v2', got %s", parser.Protocol())
	}
}

func TestMinswapAssetToCommonAssetClass(t *testing.T) {
	asset := MinswapAsset{
		PolicyId:  []byte{0x01, 0x02, 0x03},
		AssetName: []byte("TEST"),
	}

	common := asset.ToCommonAssetClass()
	if string(common.PolicyId) != string(asset.PolicyId) {
		t.Error("policy ID mismatch")
	}
	if string(common.Name) != string(asset.AssetName) {
		t.Error("asset name mismatch")
	}
}

func TestGenerateMinswapPoolId(t *testing.T) {
	poolId := generateMinswapPoolId(
		[]byte{0xab, 0xcd},
		[]byte("TokenA"),
		[]byte{0x12, 0x34},
		[]byte("TokenB"),
	)

	expected := "minswap_abcd.546f6b656e41_1234.546f6b656e42"
	if poolId != expected {
		t.Errorf("expected pool ID %s, got %s", expected, poolId)
	}
}

func TestMinswapV2DatumUnmarshal(t *testing.T) {
	// Build a test V2 datum
	// Constructor 0 with fields:
	// - StakeCredential (Constructor 0 with hash)
	// - AssetA (Constructor 0 with policy, name)
	// - AssetB (Constructor 0 with policy, name)
	// - TotalLiquidity
	// - ReserveA
	// - ReserveB
	// - BaseFee (Constructor 0 with feeA, feeB)
	// - FeeSharingNumerator (Constructor 1 for None)
	// - AllowDynamicFee (Constructor 1 for false)

	stakeCredential := cbor.NewConstructor(0, cbor.IndefLengthList{
		[]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
			0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
			0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18,
			0x19, 0x1a, 0x1b, 0x1c},
	})

	assetA := cbor.NewConstructor(0, cbor.IndefLengthList{
		[]byte{}, // Empty policy = ADA
		[]byte{},
	})

	assetB := cbor.NewConstructor(0, cbor.IndefLengthList{
		[]byte{0xab, 0xcd, 0xef},
		[]byte("MIN"),
	})

	baseFee := cbor.NewConstructor(0, cbor.IndefLengthList{
		uint64(30), // 0.3% fee
		uint64(30),
	})

	feeSharingNone := cbor.NewConstructor(1, cbor.IndefLengthList{})
	allowDynamicFalse := cbor.NewConstructor(1, cbor.IndefLengthList{})

	datum := cbor.NewConstructor(0, cbor.IndefLengthList{
		stakeCredential,
		assetA,
		assetB,
		uint64(1000000000), // totalLiquidity
		uint64(500000000),  // reserveA
		uint64(750000000),  // reserveB
		baseFee,
		feeSharingNone,
		allowDynamicFalse,
	})

	cborData, err := cbor.Encode(&datum)
	if err != nil {
		t.Fatalf("failed to encode test datum: %v", err)
	}

	var poolDatum MinswapV2PoolDatum
	if _, err := cbor.Decode(cborData, &poolDatum); err != nil {
		t.Fatalf("failed to decode datum: %v", err)
	}

	if poolDatum.ReserveA != 500000000 {
		t.Errorf("expected reserveA 500000000, got %d", poolDatum.ReserveA)
	}
	if poolDatum.ReserveB != 750000000 {
		t.Errorf("expected reserveB 750000000, got %d", poolDatum.ReserveB)
	}
	if poolDatum.BaseFee.FeeANumerator != 30 {
		t.Errorf("expected feeA 30, got %d", poolDatum.BaseFee.FeeANumerator)
	}
}

func TestMinswapV2ParserParsePoolDatum(t *testing.T) {
	// Build test datum
	stakeCredential := cbor.NewConstructor(0, cbor.IndefLengthList{
		make([]byte, 28),
	})

	assetA := cbor.NewConstructor(0, cbor.IndefLengthList{
		[]byte{},
		[]byte{},
	})

	assetB := cbor.NewConstructor(0, cbor.IndefLengthList{
		[]byte{0xab, 0xcd},
		[]byte("TEST"),
	})

	baseFee := cbor.NewConstructor(0, cbor.IndefLengthList{
		uint64(30),
		uint64(30),
	})

	feeSharingNone := cbor.NewConstructor(1, cbor.IndefLengthList{})
	allowDynamicFalse := cbor.NewConstructor(1, cbor.IndefLengthList{})

	datum := cbor.NewConstructor(0, cbor.IndefLengthList{
		stakeCredential,
		assetA,
		assetB,
		uint64(1000000000),
		uint64(100000000), // 100 ADA
		uint64(200000000), // 200 TEST
		baseFee,
		feeSharingNone,
		allowDynamicFalse,
	})

	cborData, err := cbor.Encode(&datum)
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	parser := NewMinswapV2Parser()
	state, err := parser.ParsePoolDatum(
		cborData,
		"abc123",
		0,
		12345,
		time.Now(),
	)
	if err != nil {
		t.Fatalf("failed to parse datum: %v", err)
	}

	if state.Protocol != "minswap-v2" {
		t.Errorf("expected protocol 'minswap-v2', got %s", state.Protocol)
	}
	if state.AssetX.Amount != 100000000 {
		t.Errorf(
			"expected assetX amount 100000000, got %d",
			state.AssetX.Amount,
		)
	}
	if state.AssetY.Amount != 200000000 {
		t.Errorf(
			"expected assetY amount 200000000, got %d",
			state.AssetY.Amount,
		)
	}
	if state.Slot != 12345 {
		t.Errorf("expected slot 12345, got %d", state.Slot)
	}

	// Check price calculation
	expectedPrice := 2.0 // 200/100
	if state.PriceXY() != expectedPrice {
		t.Errorf("expected price %f, got %f", expectedPrice, state.PriceXY())
	}
}

func TestMinswapV1DatumUnmarshal(t *testing.T) {
	// Build a test V1 datum
	// Constructor 0 with fields: assetA, assetB, totalLiquidity, rootKLast, feeSharing

	assetA := cbor.NewConstructor(0, cbor.IndefLengthList{
		[]byte{}, // Empty policy = ADA
		[]byte{},
	})

	assetB := cbor.NewConstructor(0, cbor.IndefLengthList{
		[]byte{0xab, 0xcd, 0xef},
		[]byte("MIN"),
	})

	feeSharingNone := cbor.NewConstructor(1, cbor.IndefLengthList{})

	datum := cbor.NewConstructor(0, cbor.IndefLengthList{
		assetA,
		assetB,
		uint64(1000000000), // totalLiquidity
		uint64(12345678),   // rootKLast
		feeSharingNone,
	})

	cborData, err := cbor.Encode(&datum)
	if err != nil {
		t.Fatalf("failed to encode test datum: %v", err)
	}

	var poolDatum MinswapV1PoolDatum
	if _, err := cbor.Decode(cborData, &poolDatum); err != nil {
		t.Fatalf("failed to decode datum: %v", err)
	}

	if poolDatum.TotalLiquidity != 1000000000 {
		t.Errorf(
			"expected totalLiquidity 1000000000, got %d",
			poolDatum.TotalLiquidity,
		)
	}
	if poolDatum.RootKLast != 12345678 {
		t.Errorf("expected rootKLast 12345678, got %d", poolDatum.RootKLast)
	}
	if poolDatum.FeeSharing.IsPresent {
		t.Error("expected FeeSharing.IsPresent to be false")
	}
	// Verify asset fields
	if len(poolDatum.AssetA.PolicyId) != 0 {
		t.Error("expected AssetA to be ADA (empty policy)")
	}
	if string(poolDatum.AssetB.AssetName) != "MIN" {
		t.Errorf(
			"expected AssetB name 'MIN', got '%s'",
			string(poolDatum.AssetB.AssetName),
		)
	}
}

func TestMinswapV1ParserParsePoolDatum(t *testing.T) {
	// Build test V1 datum
	assetA := cbor.NewConstructor(0, cbor.IndefLengthList{
		[]byte{},
		[]byte{},
	})

	assetB := cbor.NewConstructor(0, cbor.IndefLengthList{
		[]byte{0xab, 0xcd},
		[]byte("TEST"),
	})

	feeSharingNone := cbor.NewConstructor(1, cbor.IndefLengthList{})

	datum := cbor.NewConstructor(0, cbor.IndefLengthList{
		assetA,
		assetB,
		uint64(1000000000), // totalLiquidity
		uint64(12345678),   // rootKLast
		feeSharingNone,
	})

	cborData, err := cbor.Encode(&datum)
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	parser := NewMinswapV1Parser()
	state, err := parser.ParsePoolDatum(
		cborData,
		"def456",
		1,
		67890,
		time.Now(),
	)
	if err != nil {
		t.Fatalf("failed to parse datum: %v", err)
	}

	if state.Protocol != "minswap-v1" {
		t.Errorf("expected protocol 'minswap-v1', got %s", state.Protocol)
	}
	if state.Slot != 67890 {
		t.Errorf("expected slot 67890, got %d", state.Slot)
	}
	if state.TxHash != "def456" {
		t.Errorf("expected txHash 'def456', got %s", state.TxHash)
	}
	if state.TxIndex != 1 {
		t.Errorf("expected txIndex 1, got %d", state.TxIndex)
	}
	// V1 doesn't store reserves in datum, so amounts should be 0
	if state.AssetX.Amount != 0 {
		t.Errorf("expected assetX amount 0 (V1), got %d", state.AssetX.Amount)
	}
	if state.AssetY.Amount != 0 {
		t.Errorf("expected assetY amount 0 (V1), got %d", state.AssetY.Amount)
	}
	// Verify default fee
	if state.FeeNum != 9970 {
		t.Errorf("expected feeNum 9970, got %d", state.FeeNum)
	}
	if state.FeeDenom != 10000 {
		t.Errorf("expected feeDenom 10000, got %d", state.FeeDenom)
	}
}

func TestMinswapV1FeeSharingPresent(t *testing.T) {
	// Test V1 datum with fee sharing present
	assetA := cbor.NewConstructor(0, cbor.IndefLengthList{
		[]byte{},
		[]byte{},
	})

	assetB := cbor.NewConstructor(0, cbor.IndefLengthList{
		[]byte{0x12, 0x34},
		[]byte("TOKEN"),
	})

	// Fee sharing present (Constructor 0 = Some)
	feeSharingPresent := cbor.NewConstructor(0, cbor.IndefLengthList{
		[]byte{0xaa, 0xbb, 0xcc}, // feeTo
		[]byte{0xdd, 0xee, 0xff}, // feeToDatumHash
	})

	datum := cbor.NewConstructor(0, cbor.IndefLengthList{
		assetA,
		assetB,
		uint64(500000000),
		uint64(9999999),
		feeSharingPresent,
	})

	cborData, err := cbor.Encode(&datum)
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	var poolDatum MinswapV1PoolDatum
	if _, err := cbor.Decode(cborData, &poolDatum); err != nil {
		t.Fatalf("failed to decode datum: %v", err)
	}

	if !poolDatum.FeeSharing.IsPresent {
		t.Error("expected FeeSharing.IsPresent to be true")
	}
}

func TestMinswapOptionalUint64(t *testing.T) {
	// Test None case
	noneConstr := cbor.NewConstructor(1, cbor.IndefLengthList{})
	noneData, _ := cbor.Encode(&noneConstr)

	var optNone MinswapOptionalUint64
	if _, err := cbor.Decode(noneData, &optNone); err != nil {
		t.Fatalf("failed to decode None: %v", err)
	}
	if optNone.IsPresent {
		t.Error("expected IsPresent to be false for None")
	}

	// Test Some case
	someConstr := cbor.NewConstructor(0, cbor.IndefLengthList{uint64(42)})
	someData, _ := cbor.Encode(&someConstr)

	var optSome MinswapOptionalUint64
	if _, err := cbor.Decode(someData, &optSome); err != nil {
		t.Fatalf("failed to decode Some: %v", err)
	}
	if !optSome.IsPresent {
		t.Error("expected IsPresent to be true for Some")
	}
	if optSome.Value != 42 {
		t.Errorf("expected value 42, got %d", optSome.Value)
	}
}
