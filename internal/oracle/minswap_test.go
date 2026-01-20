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
	"github.com/blinklabs-io/shai/internal/oracle/minswap"
)

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
	asset := minswap.Asset{
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
	poolId := minswap.GeneratePoolId(
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

	var poolDatum minswap.V2PoolDatum
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
		[]byte{}, // dummy utxo value
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

func TestMinswapOptionalUint64(t *testing.T) {
	// Test None case
	noneConstr := cbor.NewConstructor(1, cbor.IndefLengthList{})
	noneData, err := cbor.Encode(&noneConstr)
	if err != nil {
		t.Fatalf("failed to encode None: %v", err)
	}

	var optNone minswap.OptionalUint64
	if _, err := cbor.Decode(noneData, &optNone); err != nil {
		t.Fatalf("failed to decode None: %v", err)
	}
	if optNone.IsPresent {
		t.Error("expected IsPresent to be false for None")
	}

	// Test Some case
	someConstr := cbor.NewConstructor(0, cbor.IndefLengthList{uint64(42)})
	someData, err := cbor.Encode(&someConstr)
	if err != nil {
		t.Fatalf("failed to encode Some: %v", err)
	}

	var optSome minswap.OptionalUint64
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
