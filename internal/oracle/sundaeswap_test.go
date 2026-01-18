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

func TestNewSundaeSwapV1Parser(t *testing.T) {
	parser := NewSundaeSwapV1Parser()
	if parser == nil {
		t.Fatal("expected non-nil parser")
	}
	if parser.Protocol() != "sundaeswap-v1" {
		t.Errorf("expected protocol 'sundaeswap-v1', got %s", parser.Protocol())
	}
}

func TestNewSundaeSwapV3Parser(t *testing.T) {
	parser := NewSundaeSwapV3Parser()
	if parser == nil {
		t.Fatal("expected non-nil parser")
	}
	if parser.Protocol() != "sundaeswap-v3" {
		t.Errorf("expected protocol 'sundaeswap-v3', got %s", parser.Protocol())
	}
}

func TestSundaeSwapAssetToCommonAssetClass(t *testing.T) {
	asset := SundaeSwapAsset{
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

func TestGenerateSundaeSwapPoolId(t *testing.T) {
	identifier := []byte{
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18,
		0x19, 0x1a, 0x1b, 0x1c,
	}
	poolId := generateSundaeSwapPoolId(identifier)

	expected := "sundaeswap_0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c"
	if poolId != expected {
		t.Errorf("expected pool ID %s, got %s", expected, poolId)
	}
}

func TestSundaeSwapV3DatumUnmarshal(t *testing.T) {
	// Build a test V3 datum
	// Constructor 0 with fields:
	// - identifier: ByteArray (28 bytes)
	// - assets: ((PolicyId, AssetName), (PolicyId, AssetName))
	// - circulatingLp: Int
	// - bidFeesPer10Thousand: Int
	// - askFeesPer10Thousand: Int
	// - feeManager: Optional<MultisigScript> (None = Constructor 1)
	// - marketOpen: Int
	// - protocolFees: Int

	identifier := make([]byte, 28)
	for i := range identifier {
		identifier[i] = byte(i + 1)
	}

	// Asset A (ADA - empty policy and name)
	assetA := cbor.NewConstructor(0, cbor.IndefLengthList{
		[]byte{},
		[]byte{},
	})

	// Asset B (some token)
	assetB := cbor.NewConstructor(0, cbor.IndefLengthList{
		[]byte{0xab, 0xcd, 0xef},
		[]byte("SUNDAE"),
	})

	// Assets pair
	assets := cbor.NewConstructor(0, cbor.IndefLengthList{
		assetA,
		assetB,
	})

	// FeeManager = None (Constructor 1)
	feeManagerNone := cbor.NewConstructor(1, cbor.IndefLengthList{})

	datum := cbor.NewConstructor(0, cbor.IndefLengthList{
		identifier,
		assets,
		uint64(1000000000), // circulatingLp
		uint64(30),         // bidFeesPer10Thousand (0.3%)
		uint64(30),         // askFeesPer10Thousand (0.3%)
		feeManagerNone,
		int64(1704067200000), // marketOpen (Unix ms)
		uint64(50000000),     // protocolFees
	})

	cborData, err := cbor.Encode(&datum)
	if err != nil {
		t.Fatalf("failed to encode test datum: %v", err)
	}

	var poolDatum SundaeSwapV3PoolDatum
	if _, err := cbor.Decode(cborData, &poolDatum); err != nil {
		t.Fatalf("failed to decode datum: %v", err)
	}

	if poolDatum.CirculatingLp != 1000000000 {
		t.Errorf(
			"expected circulatingLp 1000000000, got %d",
			poolDatum.CirculatingLp,
		)
	}
	if poolDatum.BidFeesPer10Thousand != 30 {
		t.Errorf(
			"expected bidFees 30, got %d",
			poolDatum.BidFeesPer10Thousand,
		)
	}
	if poolDatum.AskFeesPer10Thousand != 30 {
		t.Errorf(
			"expected askFees 30, got %d",
			poolDatum.AskFeesPer10Thousand,
		)
	}
	if poolDatum.FeeManager.IsPresent {
		t.Error("expected feeManager to be None")
	}
	if poolDatum.MarketOpen != 1704067200000 {
		t.Errorf(
			"expected marketOpen 1704067200000, got %d",
			poolDatum.MarketOpen,
		)
	}
	if poolDatum.ProtocolFees != 50000000 {
		t.Errorf(
			"expected protocolFees 50000000, got %d",
			poolDatum.ProtocolFees,
		)
	}
}

func TestSundaeSwapV3ParserParsePoolDatum(t *testing.T) {
	// Build test datum
	identifier := make([]byte, 28)
	for i := range identifier {
		identifier[i] = byte(i)
	}

	assetA := cbor.NewConstructor(0, cbor.IndefLengthList{
		[]byte{},
		[]byte{},
	})

	assetB := cbor.NewConstructor(0, cbor.IndefLengthList{
		[]byte{0xab, 0xcd},
		[]byte("TEST"),
	})

	assets := cbor.NewConstructor(0, cbor.IndefLengthList{
		assetA,
		assetB,
	})

	feeManagerNone := cbor.NewConstructor(1, cbor.IndefLengthList{})

	datum := cbor.NewConstructor(0, cbor.IndefLengthList{
		identifier,
		assets,
		uint64(500000000), // circulatingLp
		uint64(25),        // bidFees (0.25%)
		uint64(25),        // askFees (0.25%)
		feeManagerNone,
		int64(1704067200000),
		uint64(10000000),
	})

	cborData, err := cbor.Encode(&datum)
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	parser := NewSundaeSwapV3Parser()
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

	if state.Protocol != "sundaeswap-v3" {
		t.Errorf("expected protocol 'sundaeswap-v3', got %s", state.Protocol)
	}
	if state.Slot != 67890 {
		t.Errorf("expected slot 67890, got %d", state.Slot)
	}
	if state.TxHash != "def456" {
		t.Errorf("expected txHash 'def456', got %s", state.TxHash)
	}

	// Check fee calculation (25 basis points = 0.25%)
	// FeeNum should be 10000 - 25 = 9975
	if state.FeeNum != 9975 {
		t.Errorf("expected feeNum 9975, got %d", state.FeeNum)
	}
	if state.FeeDenom != 10000 {
		t.Errorf("expected feeDenom 10000, got %d", state.FeeDenom)
	}
}

func TestSundaeSwapOptionalMultisig(t *testing.T) {
	// Test None case
	noneConstr := cbor.NewConstructor(1, cbor.IndefLengthList{})
	noneData, _ := cbor.Encode(&noneConstr)

	var optNone SundaeSwapOptionalMultisig
	if _, err := cbor.Decode(noneData, &optNone); err != nil {
		t.Fatalf("failed to decode None: %v", err)
	}
	if optNone.IsPresent {
		t.Error("expected IsPresent to be false for None")
	}

	// Test Some case (we don't parse the content, just detect presence)
	someConstr := cbor.NewConstructor(0, cbor.IndefLengthList{
		[]byte{0x01, 0x02, 0x03}, // Dummy multisig data
	})
	someData, _ := cbor.Encode(&someConstr)

	var optSome SundaeSwapOptionalMultisig
	if _, err := cbor.Decode(someData, &optSome); err != nil {
		t.Fatalf("failed to decode Some: %v", err)
	}
	if !optSome.IsPresent {
		t.Error("expected IsPresent to be true for Some")
	}
}

func TestSundaeSwapAssetPairUnmarshal(t *testing.T) {
	assetA := cbor.NewConstructor(0, cbor.IndefLengthList{
		[]byte{}, // ADA policy
		[]byte{}, // ADA name
	})

	assetB := cbor.NewConstructor(0, cbor.IndefLengthList{
		[]byte{0x12, 0x34, 0x56},
		[]byte("TOKEN"),
	})

	pair := cbor.NewConstructor(0, cbor.IndefLengthList{
		assetA,
		assetB,
	})

	cborData, err := cbor.Encode(&pair)
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	var assetPair SundaeSwapAssetPair
	if _, err := cbor.Decode(cborData, &assetPair); err != nil {
		t.Fatalf("failed to decode asset pair: %v", err)
	}

	// Asset A should be ADA (empty)
	if len(assetPair.AssetA.PolicyId) != 0 {
		t.Error("expected empty policy ID for ADA")
	}

	// Asset B should have the token
	if string(assetPair.AssetB.AssetName) != "TOKEN" {
		t.Errorf(
			"expected asset name 'TOKEN', got %s",
			string(assetPair.AssetB.AssetName),
		)
	}
}

func TestSundaeSwapV1DatumUnmarshal(t *testing.T) {
	// Build a test V1 datum
	// Constructor 0 with fields:
	// - ident: ByteArray (pool identifier)
	// - assetA: (PolicyId, AssetName)
	// - assetB: (PolicyId, AssetName)
	// - circulatingLp: Int
	// - feeNumerator: Int

	ident := make([]byte, 28)
	for i := range ident {
		ident[i] = byte(i + 1)
	}

	// Asset A (ADA - empty policy and name)
	assetA := cbor.NewConstructor(0, cbor.IndefLengthList{
		[]byte{},
		[]byte{},
	})

	// Asset B (some token)
	assetB := cbor.NewConstructor(0, cbor.IndefLengthList{
		[]byte{0xab, 0xcd, 0xef},
		[]byte("SUNDAE"),
	})

	datum := cbor.NewConstructor(0, cbor.IndefLengthList{
		ident,
		assetA,
		assetB,
		uint64(1000000000), // circulatingLp
		uint64(30),         // feeNumerator (0.3%)
	})

	cborData, err := cbor.Encode(&datum)
	if err != nil {
		t.Fatalf("failed to encode test datum: %v", err)
	}

	var poolDatum SundaeSwapV1PoolDatum
	if _, err := cbor.Decode(cborData, &poolDatum); err != nil {
		t.Fatalf("failed to decode datum: %v", err)
	}

	if poolDatum.CirculatingLp != 1000000000 {
		t.Errorf(
			"expected circulatingLp 1000000000, got %d",
			poolDatum.CirculatingLp,
		)
	}
	if poolDatum.FeeNumerator != 30 {
		t.Errorf("expected feeNumerator 30, got %d", poolDatum.FeeNumerator)
	}
	if len(poolDatum.Ident) != 28 {
		t.Errorf("expected ident length 28, got %d", len(poolDatum.Ident))
	}
}

func TestSundaeSwapV1ParserParsePoolDatum(t *testing.T) {
	// Build test datum
	ident := make([]byte, 28)
	for i := range ident {
		ident[i] = byte(i)
	}

	assetA := cbor.NewConstructor(0, cbor.IndefLengthList{
		[]byte{},
		[]byte{},
	})

	assetB := cbor.NewConstructor(0, cbor.IndefLengthList{
		[]byte{0xab, 0xcd},
		[]byte("TEST"),
	})

	datum := cbor.NewConstructor(0, cbor.IndefLengthList{
		ident,
		assetA,
		assetB,
		uint64(500000000), // circulatingLp
		uint64(30),        // feeNumerator (0.3%)
	})

	cborData, err := cbor.Encode(&datum)
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	parser := NewSundaeSwapV1Parser()
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

	if state.Protocol != "sundaeswap-v1" {
		t.Errorf("expected protocol 'sundaeswap-v1', got %s", state.Protocol)
	}
	if state.Slot != 12345 {
		t.Errorf("expected slot 12345, got %d", state.Slot)
	}
	if state.TxHash != "abc123" {
		t.Errorf("expected txHash 'abc123', got %s", state.TxHash)
	}

	// Check fee calculation (30 basis points = 0.3%)
	// FeeNum should be 10000 - 30 = 9970
	if state.FeeNum != 9970 {
		t.Errorf("expected feeNum 9970, got %d", state.FeeNum)
	}
	if state.FeeDenom != 10000 {
		t.Errorf("expected feeDenom 10000, got %d", state.FeeDenom)
	}
}

func TestSundaeSwapV1PoolDatumGetPoolIdent(t *testing.T) {
	datum := &SundaeSwapV1PoolDatum{
		Ident: []byte{0xab, 0xcd, 0xef},
	}

	ident := datum.GetPoolIdent()
	expected := "abcdef"
	if ident != expected {
		t.Errorf("expected ident %s, got %s", expected, ident)
	}
}
