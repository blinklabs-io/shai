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

package dex

import (
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/shai/dex/geniusyield"
)

func TestNewGeniusYieldParser(t *testing.T) {
	parser := NewGeniusYieldParser()
	if parser == nil {
		t.Fatal("expected non-nil parser")
	}
	if parser.Protocol() != "geniusyield" {
		t.Errorf("expected protocol 'geniusyield', got %s", parser.Protocol())
	}
}

func TestGeniusYieldAssetToCommon(t *testing.T) {
	asset := geniusyield.OrderAsset{
		PolicyId:  []byte{0x01, 0x02, 0x03},
		AssetName: []byte("TEST"),
	}

	common := asset.ToCommon()
	if string(common.PolicyId) != string(asset.PolicyId) {
		t.Error("policy ID mismatch")
	}
	if string(common.Name) != string(asset.AssetName) {
		t.Error("asset name mismatch")
	}
}

func TestGeniusYieldRationalToFloat64(t *testing.T) {
	tests := []struct {
		name     string
		num      int64
		denom    int64
		expected float64
	}{
		{"simple ratio", 1, 2, 0.5},
		{"whole number", 10, 1, 10.0},
		{"price ratio", 150, 100, 1.5},
		{"zero denominator", 1, 0, 0.0},
		{"zero numerator", 0, 100, 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := geniusyield.OrderRational{
				Numerator:   tt.num,
				Denominator: tt.denom,
			}
			result := r.ToFloat64()
			if result != tt.expected {
				t.Errorf("expected %f, got %f", tt.expected, result)
			}
		})
	}
}

func TestGeniusYieldOptionalPOSIXUnmarshal(t *testing.T) {
	// Test None case (Constructor 1)
	noneConstr := cbor.NewConstructorEncoder(1, cbor.IndefLengthList{})
	noneData, err := cbor.Encode(&noneConstr)
	if err != nil {
		t.Fatalf("failed to encode None: %v", err)
	}

	var optNone geniusyield.OptionalPOSIX
	if _, err := cbor.Decode(noneData, &optNone); err != nil {
		t.Fatalf("failed to decode None: %v", err)
	}
	if optNone.IsPresent {
		t.Error("expected IsPresent to be false for None")
	}

	// Test Some case (Constructor 0)
	timestamp := int64(1704067200000) // 2024-01-01 00:00:00 UTC
	someConstr := cbor.NewConstructorEncoder(0, cbor.IndefLengthList{timestamp})
	someData, err := cbor.Encode(&someConstr)
	if err != nil {
		t.Fatalf("failed to encode Some: %v", err)
	}

	var optSome geniusyield.OptionalPOSIX
	if _, err := cbor.Decode(someData, &optSome); err != nil {
		t.Fatalf("failed to decode Some: %v", err)
	}
	if !optSome.IsPresent {
		t.Error("expected IsPresent to be true for Some")
	}
	if optSome.Time != timestamp {
		t.Errorf("expected time %d, got %d", timestamp, optSome.Time)
	}

	invalidConstr := cbor.NewConstructorEncoder(2, cbor.IndefLengthList{})
	invalidData, err := cbor.Encode(&invalidConstr)
	if err != nil {
		t.Fatalf("failed to encode invalid constructor: %v", err)
	}

	var optInvalid geniusyield.OptionalPOSIX
	if _, err := cbor.Decode(invalidData, &optInvalid); err == nil {
		t.Fatal("expected unsupported constructor error")
	}
}

func TestGeniusYieldContainedFeeUnmarshal(t *testing.T) {
	feeConstr := cbor.NewConstructorEncoder(0, cbor.IndefLengthList{
		uint64(1000000), // lovelaceFee
		uint64(500000),  // offeredFee
		uint64(250000),  // askedFee
	})

	cborData, err := cbor.Encode(&feeConstr)
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	var fee geniusyield.ContainedFee
	if _, err := cbor.Decode(cborData, &fee); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	if fee.LovelaceFee != 1000000 {
		t.Errorf("expected lovelaceFee 1000000, got %d", fee.LovelaceFee)
	}
	if fee.OfferedFee != 500000 {
		t.Errorf("expected offeredFee 500000, got %d", fee.OfferedFee)
	}
	if fee.AskedFee != 250000 {
		t.Errorf("expected askedFee 250000, got %d", fee.AskedFee)
	}
}

func TestGeniusYieldCredentialUnmarshal(t *testing.T) {
	// Test PubKeyHash (Constructor 0)
	pubKeyHash := []byte{0xab, 0xcd, 0xef, 0x12, 0x34}
	pkConstr := cbor.NewConstructorEncoder(0, cbor.IndefLengthList{pubKeyHash})
	pkData, err := cbor.Encode(&pkConstr)
	if err != nil {
		t.Fatalf("failed to encode PubKeyHash: %v", err)
	}

	var pkCred geniusyield.OrderCredential
	if _, err := cbor.Decode(pkData, &pkCred); err != nil {
		t.Fatalf("failed to decode PubKeyHash: %v", err)
	}
	if pkCred.Type != 0 {
		t.Errorf("expected type 0 for PubKeyHash, got %d", pkCred.Type)
	}
	if string(pkCred.Hash) != string(pubKeyHash) {
		t.Error("PubKeyHash mismatch")
	}

	// Test ScriptHash (Constructor 1)
	scriptHash := []byte{0x11, 0x22, 0x33, 0x44, 0x55}
	shConstr := cbor.NewConstructorEncoder(1, cbor.IndefLengthList{scriptHash})
	shData, err := cbor.Encode(&shConstr)
	if err != nil {
		t.Fatalf("failed to encode ScriptHash: %v", err)
	}

	var shCred geniusyield.OrderCredential
	if _, err := cbor.Decode(shData, &shCred); err != nil {
		t.Fatalf("failed to decode ScriptHash: %v", err)
	}
	if shCred.Type != 1 {
		t.Errorf("expected type 1 for ScriptHash, got %d", shCred.Type)
	}
	if string(shCred.Hash) != string(scriptHash) {
		t.Error("ScriptHash mismatch")
	}

	invalidConstr := cbor.NewConstructorEncoder(
		2,
		cbor.IndefLengthList{scriptHash},
	)
	invalidData, err := cbor.Encode(&invalidConstr)
	if err != nil {
		t.Fatalf("failed to encode invalid credential: %v", err)
	}

	var invalidCred geniusyield.OrderCredential
	if _, err := cbor.Decode(invalidData, &invalidCred); err == nil {
		t.Fatal("expected unsupported constructor error")
	}
}

func TestGeniusYieldOptionalCredentialUnmarshalRejectsInvalidConstructor(
	t *testing.T,
) {
	invalidConstr := cbor.NewConstructorEncoder(2, cbor.IndefLengthList{})
	invalidData, err := cbor.Encode(&invalidConstr)
	if err != nil {
		t.Fatalf("failed to encode invalid optional credential: %v", err)
	}

	var optionalCred geniusyield.OptionalCredential
	if _, err := cbor.Decode(invalidData, &optionalCred); err == nil {
		t.Fatal("expected unsupported constructor error")
	}
}

func TestGeniusYieldOrderConfigRejectsNonZeroConstructor(t *testing.T) {
	notOrder := cbor.NewConstructorEncoder(1, cbor.IndefLengthList{})
	data, err := cbor.Encode(&notOrder)
	if err != nil {
		t.Fatalf("failed to encode constructor: %v", err)
	}

	var cfg geniusyield.OrderConfig
	if err := cfg.UnmarshalCBOR(data); err == nil {
		t.Fatal("expected ErrNotOrderDatum for non-zero constructor")
	}
}

func TestGeniusYieldOrderConfigUnmarshal(t *testing.T) {
	cborData := buildTestGeniusYieldDatum(t, testDatumParams{
		offeredOriginal: 10000000,
		offeredAmount:   5000000,
		priceNum:        3,
		priceDenom:      2,
		nft:             []byte{0x01, 0x02, 0x03, 0x04},
		partialFills:    3,
		// End set, Start unset
		endTime:         int64(1735689600000), // 2025-01-01 00:00:00 UTC
		hasEnd:          true,
		containedFeeLov: 1000000,
	})

	var cfg geniusyield.OrderConfig
	if err := cfg.UnmarshalCBOR(cborData); err != nil {
		t.Fatalf("failed to decode order config: %v", err)
	}

	if cfg.OfferedOriginalAmount != 10000000 {
		t.Errorf(
			"expected offeredOriginalAmount 10000000, got %d",
			cfg.OfferedOriginalAmount,
		)
	}
	if cfg.OfferedAmount != 5000000 {
		t.Errorf("expected offeredAmount 5000000, got %d", cfg.OfferedAmount)
	}
	if cfg.PartialFills != 3 {
		t.Errorf("expected partialFills 3, got %d", cfg.PartialFills)
	}
	if cfg.Price.Numerator != 3 || cfg.Price.Denominator != 2 {
		t.Errorf(
			"expected price 3/2, got %d/%d",
			cfg.Price.Numerator,
			cfg.Price.Denominator,
		)
	}
	if cfg.Start.IsPresent {
		t.Error("expected start to be None")
	}
	if !cfg.End.IsPresent {
		t.Error("expected end to be Some")
	}
	if cfg.End.Time != 1735689600000 {
		t.Errorf("expected end time 1735689600000, got %d", cfg.End.Time)
	}
	if cfg.ContainedFee.LovelaceFee != 1000000 {
		t.Errorf(
			"expected containedFee.lovelaceFee 1000000, got %d",
			cfg.ContainedFee.LovelaceFee,
		)
	}
}

func TestGeniusYieldOrderConfigToState(t *testing.T) {
	cborData := buildTestGeniusYieldDatum(t, testDatumParams{
		offeredOriginal: 5000000,
		offeredAmount:   5000000,
		priceNum:        15,
		priceDenom:      10,
		nft:             []byte{0xde, 0xad, 0xbe, 0xef},
	})

	var cfg geniusyield.OrderConfig
	if err := cfg.UnmarshalCBOR(cborData); err != nil {
		t.Fatalf("failed to decode order config: %v", err)
	}

	state := geniusyield.OrderConfigToState(&cfg, "abc123def456", 0, 12345)

	if state.Protocol != "geniusyield" {
		t.Errorf("expected protocol 'geniusyield', got %s", state.Protocol)
	}
	if state.Slot != 12345 {
		t.Errorf("expected slot 12345, got %d", state.Slot)
	}
	if state.TxHash != "abc123def456" {
		t.Errorf("expected txHash 'abc123def456', got %s", state.TxHash)
	}
	if state.Price != 1.5 {
		t.Errorf("expected price 1.5, got %f", state.Price)
	}
	if state.OrderId != "gy_deadbeef" {
		t.Errorf("expected orderId 'gy_deadbeef', got %s", state.OrderId)
	}
	if !state.IsActive {
		t.Error("expected order to be active")
	}
	if state.OriginalAmount != 5000000 {
		t.Errorf("expected originalAmount 5000000, got %d", state.OriginalAmount)
	}
}

func TestGeniusYieldParsePoolDatum(t *testing.T) {
	// Offered 1000 of ADA, price 2/1 => asked 2000 of asked asset.
	cborData := buildTestGeniusYieldDatum(t, testDatumParams{
		offeredOriginal: 1000,
		offeredAmount:   1000,
		priceNum:        2,
		priceDenom:      1,
		nft:             []byte{0xaa},
	})

	parser := NewGeniusYieldParser()
	state, err := parser.ParsePoolDatum(
		cborData,
		nil,
		"tx",
		0,
		7,
		time.Now(),
	)
	if err != nil {
		t.Fatalf("failed to parse pool datum: %v", err)
	}
	if state == nil {
		t.Fatal("expected non-nil pool state")
	}
	if state.Protocol != "geniusyield" {
		t.Errorf("expected protocol 'geniusyield', got %s", state.Protocol)
	}
	if state.PoolId != "gy_aa" {
		t.Errorf("expected poolId 'gy_aa', got %s", state.PoolId)
	}
	if state.AssetX.Amount != 1000 {
		t.Errorf("expected AssetX amount 1000, got %d", state.AssetX.Amount)
	}
	if state.AssetY.Amount != 2000 {
		t.Errorf("expected AssetY amount 2000, got %d", state.AssetY.Amount)
	}
	if state.FeeNum != 1 || state.FeeDenom != 1 {
		t.Errorf("expected fee 1/1, got %d/%d", state.FeeNum, state.FeeDenom)
	}
	if state.Slot != 7 {
		t.Errorf("expected slot 7, got %d", state.Slot)
	}
}

func TestGeniusYieldParsePoolDatumRejectsNonPositivePrice(t *testing.T) {
	tests := []struct {
		name       string
		priceNum   int64
		priceDenom int64
	}{
		{"zero numerator", 0, 1},
		{"negative numerator", -1, 1},
		{"negative denominator", 1, -1},
	}

	parser := NewGeniusYieldParser()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cborData := buildTestGeniusYieldDatum(t, testDatumParams{
				offeredOriginal: 1000,
				offeredAmount:   1000,
				priceNum:        tt.priceNum,
				priceDenom:      tt.priceDenom,
				nft:             []byte{0x01},
			})
			state, err := parser.ParsePoolDatum(
				cborData,
				nil,
				"tx",
				0,
				0,
				time.Now(),
			)
			if err == nil {
				t.Fatalf("expected invalid price error, got state %#v", state)
			}
		})
	}
}

func TestGeniusYieldParsePoolDatumRejectsAskedAmountOverflow(t *testing.T) {
	parser := NewGeniusYieldParser()
	cborData := buildTestGeniusYieldDatum(t, testDatumParams{
		offeredOriginal: 4,
		offeredAmount:   4,
		priceNum:        1 << 62,
		priceDenom:      1,
		nft:             []byte{0x01},
	})

	state, err := parser.ParsePoolDatum(cborData, nil, "tx", 0, 0, time.Now())
	if err == nil {
		t.Fatalf("expected asked amount overflow error, got state %#v", state)
	}
	if state != nil {
		t.Fatalf("expected nil state on overflow, got %#v", state)
	}
}

func TestGeniusYieldParsePoolDatumInactiveReturnsNil(t *testing.T) {
	parser := NewGeniusYieldParser()
	// Zero offered amount => inactive order => nil state.
	cborData := buildTestGeniusYieldDatum(t, testDatumParams{
		offeredOriginal: 1000,
		offeredAmount:   0,
		priceNum:        1,
		priceDenom:      1,
		nft:             []byte{0x01},
	})

	state, err := parser.ParsePoolDatum(cborData, nil, "tx", 0, 0, time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state != nil {
		t.Fatalf("expected nil state for inactive order, got %#v", state)
	}
}

func TestGeniusYieldOrderIsActive(t *testing.T) {
	now := time.Now()
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)

	tests := []struct {
		name           string
		offeredAmount  uint64
		startTime      *time.Time
		endTime        *time.Time
		expectedActive bool
	}{
		{
			name:           "active - no time constraints",
			offeredAmount:  1000,
			startTime:      nil,
			endTime:        nil,
			expectedActive: true,
		},
		{
			name:           "inactive - no amount",
			offeredAmount:  0,
			startTime:      nil,
			endTime:        nil,
			expectedActive: false,
		},
		{
			name:           "inactive - not started",
			offeredAmount:  1000,
			startTime:      &future,
			endTime:        nil,
			expectedActive: false,
		},
		{
			name:           "inactive - expired",
			offeredAmount:  1000,
			startTime:      nil,
			endTime:        &past,
			expectedActive: false,
		},
		{
			name:           "active - within time window",
			offeredAmount:  1000,
			startTime:      &past,
			endTime:        &future,
			expectedActive: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := testDatumParams{
				offeredOriginal: tt.offeredAmount,
				offeredAmount:   tt.offeredAmount,
				priceNum:        1,
				priceDenom:      1,
				nft:             []byte{0x01},
			}
			if tt.startTime != nil {
				params.startTime = tt.startTime.UnixMilli()
				params.hasStart = true
			}
			if tt.endTime != nil {
				params.endTime = tt.endTime.UnixMilli()
				params.hasEnd = true
			}

			cborData := buildTestGeniusYieldDatum(t, params)

			var cfg geniusyield.OrderConfig
			if err := cfg.UnmarshalCBOR(cborData); err != nil {
				t.Fatalf("failed to decode: %v", err)
			}
			state := geniusyield.OrderConfigToState(&cfg, "tx", 0, 0)

			if state.IsActive != tt.expectedActive {
				t.Errorf(
					"expected isActive=%v, got %v",
					tt.expectedActive,
					state.IsActive,
				)
			}
		})
	}
}

// testDatumParams configures buildTestGeniusYieldDatum.
type testDatumParams struct {
	offeredOriginal uint64
	offeredAmount   uint64
	priceNum        int64
	priceDenom      int64
	nft             []byte
	partialFills    uint64
	startTime       int64
	hasStart        bool
	endTime         int64
	hasEnd          bool
	containedFeeLov uint64
}

// buildTestGeniusYieldDatum builds a CBOR-encoded PartialOrderDatum
// (constructor 0) for tests.
func buildTestGeniusYieldDatum(t *testing.T, p testDatumParams) []byte {
	t.Helper()

	ownerKey := make([]byte, 28)
	for i := range ownerKey {
		ownerKey[i] = byte(i)
	}

	paymentCred := cbor.NewConstructorEncoder(0, cbor.IndefLengthList{ownerKey})
	stakingCred := cbor.NewConstructorEncoder(1, cbor.IndefLengthList{})
	ownerAddr := cbor.NewConstructorEncoder(0, cbor.IndefLengthList{
		paymentCred,
		stakingCred,
	})

	// Offered asset (ADA)
	offeredAsset := cbor.NewConstructorEncoder(0, cbor.IndefLengthList{
		[]byte{},
		[]byte{},
	})

	// Asked asset (some token)
	askedAsset := cbor.NewConstructorEncoder(0, cbor.IndefLengthList{
		[]byte{0xab, 0xcd},
		[]byte("TEST"),
	})

	price := cbor.NewConstructorEncoder(0, cbor.IndefLengthList{
		p.priceNum,
		p.priceDenom,
	})

	var startConstr, endConstr cbor.ConstructorEncoder
	if p.hasStart {
		startConstr = cbor.NewConstructorEncoder(
			0,
			cbor.IndefLengthList{p.startTime},
		)
	} else {
		startConstr = cbor.NewConstructorEncoder(1, cbor.IndefLengthList{})
	}
	if p.hasEnd {
		endConstr = cbor.NewConstructorEncoder(
			0,
			cbor.IndefLengthList{p.endTime},
		)
	} else {
		endConstr = cbor.NewConstructorEncoder(1, cbor.IndefLengthList{})
	}

	makerPercentFee := cbor.NewConstructorEncoder(0, cbor.IndefLengthList{
		int64(3),
		int64(1000),
	})
	containedFee := cbor.NewConstructorEncoder(0, cbor.IndefLengthList{
		p.containedFeeLov,
		uint64(0),
		uint64(0),
	})

	datum := cbor.NewConstructorEncoder(0, cbor.IndefLengthList{
		ownerKey,
		ownerAddr,
		offeredAsset,
		p.offeredOriginal,
		p.offeredAmount,
		askedAsset,
		price,
		p.nft,
		startConstr,
		endConstr,
		p.partialFills,
		uint64(1000000),
		makerPercentFee,
		uint64(50000),
		containedFee,
		uint64(0),
	})

	cborData, err := cbor.Encode(&datum)
	if err != nil {
		t.Fatalf("failed to encode test datum: %v", err)
	}
	return cborData
}
