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
		uint64(1000000000), // treasuryA (1000 ADA worth)
		uint64(500000000),  // treasuryB
		uint64(750000000),  // issuedShares
	})

	cborData, err := cbor.Encode(&datum)
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	var poolDatum VyFiPoolDatum
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
		uint64(100000),
	})

	cborData, err := cbor.Encode(&orderDetails)
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	var details VyFiOrderDetails
	if _, err := cbor.Decode(cborData, &details); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	if details.Type != VyFiOrderTypeAddLiquidity {
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
		uint64(50000),
	})

	cborData, err := cbor.Encode(&orderDetails)
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	var details VyFiOrderDetails
	if _, err := cbor.Decode(cborData, &details); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	if details.Type != VyFiOrderTypeTradeAToB {
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

func TestVyFiParserParsePoolDatum(t *testing.T) {
	// Build a pool datum
	datum := cbor.NewConstructor(0, cbor.IndefLengthList{
		uint64(2000000000), // treasuryA
		uint64(1000000000), // treasuryB
		uint64(1500000000), // issuedShares
	})

	cborData, err := cbor.Encode(&datum)
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	parser := NewVyFiParser()

	// Create mock UTXO value with ADA, token, and NFT
	// ADA: lovelace -> 1000000
	// Token: policyId + name -> 500000
	// NFT: policyId + name -> 1
	mockUtxoValue := map[string]uint64{
		"lovelace": 1000000,
		"1234567890abcdef1234567890abcdef1234567890abcdef1234567890ab" + "cdef1234567890abcdef": 500000, // token
		"nftpolicyid1234567890abcdef1234567890abcdef1234567890abcdef" + "nftname1234567890ab":   1,      // NFT
	}
	utxoValueCbor, err := cbor.Encode(mockUtxoValue)
	if err != nil {
		t.Fatalf("failed to encode mock UTXO value: %v", err)
	}

	state, err := parser.ParsePoolDatum(
		cborData,
		utxoValueCbor,
		"abc123def456",
		0,
		12345,
		time.Now(),
	)
	if err != nil {
		t.Fatalf("failed to parse datum: %v", err)
	}

	if state.Protocol != "vyfi" {
		t.Errorf("expected protocol 'vyfi', got %s", state.Protocol)
	}
	if state.PoolId != "vyfi_nftpolicyid1234567890abcdef1234567890abcdef1234567890abcdefnftname1234567890ab" {
		t.Errorf("expected pool ID 'vyfi_nftpolicyid1234567890abcdef1234567890abcdef1234567890abcdefnftname1234567890ab', got %s", state.PoolId)
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
		uint64(500000000),
		uint64(250000000),
		uint64(375000000),
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
