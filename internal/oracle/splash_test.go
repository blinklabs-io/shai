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

func TestNewSplashV1Parser(t *testing.T) {
	parser := NewSplashV1Parser()
	if parser == nil {
		t.Fatal("expected non-nil parser")
	}
	if parser.Protocol() != "splash-v1" {
		t.Errorf("expected protocol 'splash-v1', got %s", parser.Protocol())
	}
}

func TestSplashAssetToCommonAssetClass(t *testing.T) {
	asset := SplashAsset{
		PolicyId:  []byte{0x01, 0x02, 0x03},
		TokenName: []byte("TEST"),
	}

	common := asset.ToCommonAssetClass()
	if string(common.PolicyId) != string(asset.PolicyId) {
		t.Error("policy ID mismatch")
	}
	if string(common.Name) != string(asset.TokenName) {
		t.Error("token name mismatch")
	}
}

func TestGenerateSplashPoolId(t *testing.T) {
	policyId := []byte{
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18,
		0x19, 0x1a, 0x1b, 0x1c,
	}
	tokenName := []byte("POOL")
	poolId := generateSplashPoolId(policyId, tokenName)

	expected := "splash_0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c504f4f4c"
	if poolId != expected {
		t.Errorf("expected pool ID %s, got %s", expected, poolId)
	}
}

func TestSplashV1DatumUnmarshal(t *testing.T) {
	// Build a test V1 datum
	// Constructor 0 with fields:
	// - poolNft: Asset (policyId, tokenName)
	// - poolX: Asset
	// - poolY: Asset
	// - poolLq: Asset
	// - poolFeeNum: int
	// - unspecifiedField: BytesSingletonArray
	// - nonce: int

	// Pool NFT asset
	poolNft := cbor.NewConstructor(0, cbor.IndefLengthList{
		[]byte{0xab, 0xcd, 0xef},
		[]byte("NFT"),
	})

	// Pool X asset (ADA)
	poolX := cbor.NewConstructor(0, cbor.IndefLengthList{
		[]byte{},
		[]byte{},
	})

	// Pool Y asset (some token)
	poolY := cbor.NewConstructor(0, cbor.IndefLengthList{
		[]byte{0x12, 0x34, 0x56},
		[]byte("TOKEN"),
	})

	// Pool LQ asset (LP token)
	poolLq := cbor.NewConstructor(0, cbor.IndefLengthList{
		[]byte{0xab, 0xcd, 0xef},
		[]byte("LP"),
	})

	// BytesSingletonArray - single element array of bytes
	unspecified := [][]byte{{0x00}}

	datum := cbor.NewConstructor(0, cbor.IndefLengthList{
		poolNft,
		poolX,
		poolY,
		poolLq,
		uint64(30),      // poolFeeNum (30 basis points = 0.3%)
		unspecified,     // unspecifiedField
		int64(12345678), // nonce
	})

	cborData, err := cbor.Encode(&datum)
	if err != nil {
		t.Fatalf("failed to encode test datum: %v", err)
	}

	var poolDatum SplashV1PoolDatum
	if _, err := cbor.Decode(cborData, &poolDatum); err != nil {
		t.Fatalf("failed to decode datum: %v", err)
	}

	// Verify pool NFT
	if string(poolDatum.PoolNft.PolicyId) != string([]byte{0xab, 0xcd, 0xef}) {
		t.Error("pool NFT policy ID mismatch")
	}
	if string(poolDatum.PoolNft.TokenName) != "NFT" {
		t.Errorf(
			"expected pool NFT token name 'NFT', got %s",
			string(poolDatum.PoolNft.TokenName),
		)
	}

	// Verify pool X (ADA)
	if len(poolDatum.PoolX.PolicyId) != 0 {
		t.Error("expected empty policy ID for ADA")
	}

	// Verify pool Y
	if string(poolDatum.PoolY.TokenName) != "TOKEN" {
		t.Errorf(
			"expected pool Y token name 'TOKEN', got %s",
			string(poolDatum.PoolY.TokenName),
		)
	}

	// Verify pool LQ
	if string(poolDatum.PoolLq.TokenName) != "LP" {
		t.Errorf(
			"expected pool LQ token name 'LP', got %s",
			string(poolDatum.PoolLq.TokenName),
		)
	}

	// Verify fee
	if poolDatum.PoolFeeNum != 30 {
		t.Errorf("expected poolFeeNum 30, got %d", poolDatum.PoolFeeNum)
	}

	// Verify nonce
	if poolDatum.Nonce != 12345678 {
		t.Errorf("expected nonce 12345678, got %d", poolDatum.Nonce)
	}
}

func TestSplashV1ParserParsePoolDatum(t *testing.T) {
	// Build test datum
	poolNft := cbor.NewConstructor(0, cbor.IndefLengthList{
		[]byte{0x01, 0x02, 0x03, 0x04},
		[]byte("NFT"),
	})

	poolX := cbor.NewConstructor(0, cbor.IndefLengthList{
		[]byte{},
		[]byte{},
	})

	poolY := cbor.NewConstructor(0, cbor.IndefLengthList{
		[]byte{0xab, 0xcd},
		[]byte("TEST"),
	})

	poolLq := cbor.NewConstructor(0, cbor.IndefLengthList{
		[]byte{0x01, 0x02, 0x03, 0x04},
		[]byte("LP"),
	})

	unspecified := [][]byte{{0x00}}

	datum := cbor.NewConstructor(0, cbor.IndefLengthList{
		poolNft,
		poolX,
		poolY,
		poolLq,
		uint64(50), // 50 basis points = 0.5% fee
		unspecified,
		int64(999999),
	})

	cborData, err := cbor.Encode(&datum)
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	parser := NewSplashV1Parser()
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

	if state.Protocol != "splash-v1" {
		t.Errorf("expected protocol 'splash-v1', got %s", state.Protocol)
	}
	if state.Slot != 12345 {
		t.Errorf("expected slot 12345, got %d", state.Slot)
	}
	if state.TxHash != "abc123" {
		t.Errorf("expected txHash 'abc123', got %s", state.TxHash)
	}

	// Check fee calculation (50 basis points = 0.5%)
	// FeeNum should be 10000 - 50 = 9950
	if state.FeeNum != 9950 {
		t.Errorf("expected feeNum 9950, got %d", state.FeeNum)
	}
	if state.FeeDenom != 10000 {
		t.Errorf("expected feeDenom 10000, got %d", state.FeeDenom)
	}

	// Check pool ID format
	expectedPoolId := "splash_010203044e4654"
	if state.PoolId != expectedPoolId {
		t.Errorf("expected pool ID %s, got %s", expectedPoolId, state.PoolId)
	}
}

func TestSplashV1PoolDatumGetPoolNftHex(t *testing.T) {
	datum := &SplashV1PoolDatum{
		PoolNft: SplashAsset{
			PolicyId:  []byte{0xab, 0xcd, 0xef},
			TokenName: []byte("NFT"),
		},
	}

	nftHex := datum.GetPoolNftHex()
	expected := "abcdef4e4654" // hex of policy + hex of "NFT"
	if nftHex != expected {
		t.Errorf("expected NFT hex %s, got %s", expected, nftHex)
	}
}

func TestSplashBytesSingletonArrayUnmarshal(t *testing.T) {
	// Test with single element
	arr := [][]byte{{0x01, 0x02, 0x03}}
	cborData, err := cbor.Encode(&arr)
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	var bsa SplashBytesSingletonArray
	if _, err := cbor.Decode(cborData, &bsa); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	if len(bsa.Data) != 3 {
		t.Errorf("expected data length 3, got %d", len(bsa.Data))
	}
	if bsa.Data[0] != 0x01 || bsa.Data[1] != 0x02 || bsa.Data[2] != 0x03 {
		t.Error("data content mismatch")
	}
}

func TestSplashBytesSingletonArrayUnmarshalEmpty(t *testing.T) {
	// Test with empty array
	arr := [][]byte{}
	cborData, err := cbor.Encode(&arr)
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	var bsa SplashBytesSingletonArray
	if _, err := cbor.Decode(cborData, &bsa); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	if len(bsa.Data) != 0 {
		t.Errorf("expected empty data, got length %d", len(bsa.Data))
	}
}

func TestSplashV1ParserHighFee(t *testing.T) {
	// Test with a high fee (should cap at 100%)
	poolNft := cbor.NewConstructor(0, cbor.IndefLengthList{
		[]byte{0x01},
		[]byte("N"),
	})

	poolX := cbor.NewConstructor(0, cbor.IndefLengthList{
		[]byte{},
		[]byte{},
	})

	poolY := cbor.NewConstructor(0, cbor.IndefLengthList{
		[]byte{0x01},
		[]byte("Y"),
	})

	poolLq := cbor.NewConstructor(0, cbor.IndefLengthList{
		[]byte{0x01},
		[]byte("L"),
	})

	unspecified := [][]byte{{0x00}}

	datum := cbor.NewConstructor(0, cbor.IndefLengthList{
		poolNft,
		poolX,
		poolY,
		poolLq,
		uint64(15000), // 150% - invalid but should be handled
		unspecified,
		int64(1),
	})

	cborData, err := cbor.Encode(&datum)
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	parser := NewSplashV1Parser()
	state, err := parser.ParsePoolDatum(
		cborData,
		"test",
		0,
		1,
		time.Now(),
	)
	if err != nil {
		t.Fatalf("failed to parse datum: %v", err)
	}

	// Fee should be capped - feeNum should be 0 (100% fee)
	if state.FeeNum != 0 {
		t.Errorf("expected feeNum 0 for >100%% fee, got %d", state.FeeNum)
	}
}
