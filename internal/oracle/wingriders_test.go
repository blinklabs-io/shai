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

func TestWingRidersAssetToCommonAssetClass(t *testing.T) {
	asset := wingriders.AssetClass{
		PolicyId: []byte{0x01, 0x02, 0x03},
		Name:     []byte("TEST"),
	}

	common := asset.ToCommonAssetClass()
	if string(common.PolicyId) != string(asset.PolicyId) {
		t.Error("policy ID mismatch")
	}
	if string(common.Name) != string(asset.Name) {
		t.Error("asset name mismatch")
	}
}

func TestGenerateWingRidersPoolId(t *testing.T) {
	poolId := wingriders.GeneratePoolId(
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

func TestWingRidersV2ParserParsePoolDatum(t *testing.T) {
	// Build a test WingRiders V2 datum
	// Based on LiquidityPoolDatumV2 structure from dex-serializer

	requestValidatorHash := []byte{
		0x86, 0xae, 0x9e, 0xeb, 0xd8, 0xb9, 0x79, 0x44,
		0xa4, 0x52, 0x01, 0xe4, 0xae, 0xc1, 0x33, 0x0a,
		0x72, 0x29, 0x1a, 0xf2, 0xd0, 0x71, 0x64, 0x4b,
		0xba, 0x01, 0x59, 0x59,
	}

	assetA := cbor.NewConstructor(0, cbor.IndefLengthList{
		[]byte{}, // Empty policy = ADA
		[]byte{},
	})

	assetB := cbor.NewConstructor(0, cbor.IndefLengthList{
		[]byte{0xab, 0xcd, 0xef},
		[]byte("WING"),
	})

	datum := cbor.NewConstructor(0, cbor.IndefLengthList{
		requestValidatorHash,
		assetA,
		assetB,
		uint64(30),    // swapFeeInBasis (0.3%)
		uint64(5),     // protocolFeeInBasis (0.05%)
		uint64(10),    // projectFeeInBasis (0.1%)
		uint64(10000), // feeBasis
		uint64(2000000), // agentFeeAda (2 ADA)
		uint64(1662811586000), // lastInteraction
		uint64(100000000), // treasuryA (100 ADA)
		uint64(200000000), // treasuryB (200 WING)
		uint64(1000000),   // projectTreasuryA (1 ADA)
		uint64(2000000),   // projectTreasuryB (2 WING)
	})

	cborData, err := cbor.Encode(&datum)
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	parser := NewWingRidersV2Parser()
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

	if state.Protocol != "wingriders-v2" {
		t.Errorf("expected protocol 'wingriders-v2', got %s", state.Protocol)
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
	if state.FeeNum != 9970 { // 10000 - 30 = 9970 (0.3% fee)
		t.Errorf("expected fee num 9970, got %d", state.FeeNum)
	}
	if state.FeeDenom != 10000 {
		t.Errorf("expected fee denom 10000, got %d", state.FeeDenom)
	}
	if state.Slot != 12345 {
		t.Errorf("expected slot 12345, got %d", state.Slot)
	}

	// Check price calculation: 200 WING / 100 ADA = 2.0
	expectedPrice := 2.0
	if state.PriceXY() != expectedPrice {
		t.Errorf("expected price %f, got %f", expectedPrice, state.PriceXY())
	}
}