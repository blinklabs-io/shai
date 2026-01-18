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
	"github.com/blinklabs-io/shai/internal/oracle/wingriders"
)

func TestNewWingRidersV2Parser(t *testing.T) {
	parser := NewWingRidersV2Parser()
	if parser == nil {
		t.Fatal("expected non-nil parser")
	}
	if parser.Protocol() != "wingriders-v2" {
		t.Errorf("expected protocol 'wingriders-v2', got %s", parser.Protocol())
	}
}

func TestWingRidersGeneratePoolId(t *testing.T) {
	poolId := generateWingRidersPoolId(
		[]byte{0xab, 0xcd},
		[]byte("TokenA"),
		[]byte{0x12, 0x34},
		[]byte("TokenB"),
	)

	expected := "wingriders_abcd.546f6b656e41_1234.546f6b656e42"
	if poolId != expected {
		t.Errorf("expected pool ID %s, got %s", expected, poolId)
	}
}

func TestWingRidersOptionalIntNone(t *testing.T) {
	// Test Nothing case (Constructor 1)
	noneConstr := cbor.NewConstructor(1, cbor.IndefLengthList{})
	noneData, err := cbor.Encode(&noneConstr)
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	var optNone wingriders.OptionalInt
	if _, err := cbor.Decode(noneData, &optNone); err != nil {
		t.Fatalf("failed to decode None: %v", err)
	}
	if optNone.IsPresent {
		t.Error("expected IsPresent to be false for Nothing")
	}
}

func TestWingRidersOptionalIntSome(t *testing.T) {
	// Test Just case (Constructor 0)
	someConstr := cbor.NewConstructor(0, cbor.IndefLengthList{int64(12345)})
	someData, err := cbor.Encode(&someConstr)
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	var optSome wingriders.OptionalInt
	if _, err := cbor.Decode(someData, &optSome); err != nil {
		t.Fatalf("failed to decode Some: %v", err)
	}
	if !optSome.IsPresent {
		t.Error("expected IsPresent to be true for Just")
	}
	if optSome.Value != 12345 {
		t.Errorf("expected value 12345, got %d", optSome.Value)
	}
}

func TestWingRidersPoolVariantConstantProduct(t *testing.T) {
	// ConstantProduct is Constructor 0 with no fields
	cpConstr := cbor.NewConstructor(0, cbor.IndefLengthList{})
	cpData, err := cbor.Encode(&cpConstr)
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	var variant wingriders.PoolVariant
	if _, err := cbor.Decode(cpData, &variant); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if !variant.IsConstantProduct() {
		t.Error("expected ConstantProduct pool type")
	}
	if variant.IsStableswap() {
		t.Error("expected not to be Stableswap")
	}
}

func TestWingRidersPoolVariantStableswap(t *testing.T) {
	// Stableswap is Constructor 1 with parameterD, scaleA, scaleB
	ssConstr := cbor.NewConstructor(1, cbor.IndefLengthList{
		int64(1000), // parameterD
		int64(1),    // scaleA
		int64(1),    // scaleB
	})
	ssData, err := cbor.Encode(&ssConstr)
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	var variant wingriders.PoolVariant
	if _, err := cbor.Decode(ssData, &variant); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if !variant.IsStableswap() {
		t.Error("expected Stableswap pool type")
	}
	if variant.IsConstantProduct() {
		t.Error("expected not to be ConstantProduct")
	}
	if variant.ParameterD != 1000 {
		t.Errorf("expected parameterD 1000, got %d", variant.ParameterD)
	}
}

func TestWingRidersCredential(t *testing.T) {
	// PubKeyCredential (Constructor 0)
	pubKeyHash := make([]byte, 28)
	for i := range pubKeyHash {
		pubKeyHash[i] = byte(i + 1)
	}
	pkConstr := cbor.NewConstructor(0, cbor.IndefLengthList{pubKeyHash})
	pkData, err := cbor.Encode(&pkConstr)
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	var cred wingriders.Credential
	if _, err := cbor.Decode(pkData, &cred); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if cred.Type != 0 {
		t.Errorf("expected type 0 (PubKey), got %d", cred.Type)
	}
	if len(cred.Hash) != 28 {
		t.Errorf("expected 28 byte hash, got %d", len(cred.Hash))
	}
}

func TestWingRidersV2PoolDatumTotalFee(t *testing.T) {
	datum := &wingriders.V2PoolDatum{
		SwapFeeInBasis:     30,
		ProtocolFeeInBasis: 5,
		ProjectFeeInBasis:  3,
		ReserveFeeInBasis:  2,
		FeeBasis:           10000,
	}

	totalFee := datum.TotalFeeInBasis()
	if totalFee != 40 {
		t.Errorf("expected total fee 40, got %d", totalFee)
	}
}

func TestWingRidersV2PoolDatumGetAssets(t *testing.T) {
	datum := &wingriders.V2PoolDatum{
		AssetASymbol: []byte{},
		AssetAToken:  []byte{},
		AssetBSymbol: []byte{0xab, 0xcd},
		AssetBToken:  []byte("TEST"),
	}

	assetA := datum.GetAssetA()
	assetB := datum.GetAssetB()

	// Asset A should be ADA (empty)
	if len(assetA.PolicyId) != 0 {
		t.Error("expected empty policy ID for ADA")
	}

	// Asset B should have the token
	if string(assetB.Name) != "TEST" {
		t.Errorf("expected asset name 'TEST', got %s", string(assetB.Name))
	}
}

func TestWingRidersV2ParserParsePoolDatum(t *testing.T) {
	// Build a simplified V2 pool datum
	// This is a minimal structure for testing
	requestHash := make([]byte, 28)

	// Create credential for address
	credential := cbor.NewConstructor(0, cbor.IndefLengthList{
		make([]byte, 28),
	})

	// Create optional staking credential (Nothing)
	stakingNone := cbor.NewConstructor(1, cbor.IndefLengthList{})

	// Create address
	address := cbor.NewConstructor(0, cbor.IndefLengthList{
		credential,
		stakingNone,
	})

	// Optional address (Just)
	optAddress := cbor.NewConstructor(0, cbor.IndefLengthList{address})

	// Optional address (Nothing)
	optAddressNone := cbor.NewConstructor(1, cbor.IndefLengthList{})

	// Optional int (Nothing)
	optIntNone := cbor.NewConstructor(1, cbor.IndefLengthList{})

	// Pool variant (ConstantProduct)
	poolVariant := cbor.NewConstructor(0, cbor.IndefLengthList{})

	datum := cbor.NewConstructor(0, cbor.IndefLengthList{
		requestHash,          // requestValidatorHash
		[]byte{},             // assetASymbol (ADA)
		[]byte{},             // assetAToken
		[]byte{0xab, 0xcd},   // assetBSymbol
		[]byte("WRT"),        // assetBToken
		uint64(30),           // swapFeeInBasis
		uint64(5),            // protocolFeeInBasis
		uint64(3),            // projectFeeInBasis
		uint64(2),            // reserveFeeInBasis
		uint64(10000),        // feeBasis
		uint64(2000000),      // agentFeeAda
		int64(1704067200000), // lastInteraction
		optIntNone,           // treasuryA
		optIntNone,           // treasuryB
		uint64(0),            // projectTreasury
		uint64(0),            // reserveTreasury
		optAddress,           // projectBeneficiary
		optAddressNone,       // reserveBeneficiary
		poolVariant,          // poolVariant
	})

	cborData, err := cbor.Encode(&datum)
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	parser := NewWingRidersV2Parser()
	state, err := parser.ParsePoolDatum(
		cborData,
		"abc123def456",
		0,
		12345,
		time.Now(),
	)
	if err != nil {
		t.Fatalf("failed to parse datum: %v", err)
	}

	if state.Protocol != "wingriders-v2" {
		t.Errorf("expected protocol 'wingriders-v2', got %s", state.Protocol)
	}
	if state.Slot != 12345 {
		t.Errorf("expected slot 12345, got %d", state.Slot)
	}

	// Check fee calculation (40 basis points total)
	// FeeNum should be 10000 - 40 = 9960
	if state.FeeNum != 9960 {
		t.Errorf("expected feeNum 9960, got %d", state.FeeNum)
	}
	if state.FeeDenom != 10000 {
		t.Errorf("expected feeDenom 10000, got %d", state.FeeDenom)
	}
}
