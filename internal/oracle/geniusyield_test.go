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
	"github.com/blinklabs-io/shai/internal/oracle/geniusyield"
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

func TestGeniusYieldAssetToCommonAssetClass(t *testing.T) {
	asset := GeniusYieldAsset{
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

func TestGenerateGeniusYieldOrderId(t *testing.T) {
	nftName := []byte{
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
	}
	orderId := GenerateGeniusYieldOrderId(nftName)

	expected := "gy_0102030405060708090a0b0c0d0e0f10"
	if orderId != expected {
		t.Errorf("expected order ID %s, got %s", expected, orderId)
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
			r := GeniusYieldRational{
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
	noneConstr := cbor.NewConstructor(1, cbor.IndefLengthList{})
	noneData, err := cbor.Encode(&noneConstr)
	if err != nil {
		t.Fatalf("failed to encode None: %v", err)
	}

	var optNone GeniusYieldOptionalPOSIX
	if _, err := cbor.Decode(noneData, &optNone); err != nil {
		t.Fatalf("failed to decode None: %v", err)
	}
	if optNone.IsPresent {
		t.Error("expected IsPresent to be false for None")
	}

	// Test Some case (Constructor 0)
	timestamp := int64(1704067200000) // 2024-01-01 00:00:00 UTC
	someConstr := cbor.NewConstructor(0, cbor.IndefLengthList{timestamp})
	someData, err := cbor.Encode(&someConstr)
	if err != nil {
		t.Fatalf("failed to encode Some: %v", err)
	}

	var optSome GeniusYieldOptionalPOSIX
	if _, err := cbor.Decode(someData, &optSome); err != nil {
		t.Fatalf("failed to decode Some: %v", err)
	}
	if !optSome.IsPresent {
		t.Error("expected IsPresent to be true for Some")
	}
	if optSome.Time != timestamp {
		t.Errorf("expected time %d, got %d", timestamp, optSome.Time)
	}
}

func TestGeniusYieldContainedFeeUnmarshal(t *testing.T) {
	feeConstr := cbor.NewConstructor(0, cbor.IndefLengthList{
		uint64(1000000), // lovelaceFee
		uint64(500000),  // offeredFee
		uint64(250000),  // askedFee
	})

	cborData, err := cbor.Encode(&feeConstr)
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	var fee GeniusYieldContainedFee
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
	pkConstr := cbor.NewConstructor(0, cbor.IndefLengthList{pubKeyHash})
	pkData, err := cbor.Encode(&pkConstr)
	if err != nil {
		t.Fatalf("failed to encode PubKeyHash: %v", err)
	}

	var pkCred GeniusYieldCredential
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
	shConstr := cbor.NewConstructor(1, cbor.IndefLengthList{scriptHash})
	shData, err := cbor.Encode(&shConstr)
	if err != nil {
		t.Fatalf("failed to encode ScriptHash: %v", err)
	}

	var shCred GeniusYieldCredential
	if _, err := cbor.Decode(shData, &shCred); err != nil {
		t.Fatalf("failed to decode ScriptHash: %v", err)
	}
	if shCred.Type != 1 {
		t.Errorf("expected type 1 for ScriptHash, got %d", shCred.Type)
	}
	if string(shCred.Hash) != string(scriptHash) {
		t.Error("ScriptHash mismatch")
	}
}

func TestGeniusYieldOrderStateKey(t *testing.T) {
	state := &geniusyield.OrderState{
		OrderId: "gy_abc123",
	}
	key := state.Key()
	expected := "geniusyield:gy_abc123"
	if key != expected {
		t.Errorf("expected key %s, got %s", expected, key)
	}
}

func TestGeniusYieldOrderStateFillPercent(t *testing.T) {
	tests := []struct {
		name         string
		original     uint64
		remaining    uint64
		expectedFill float64
	}{
		{"no fill", 1000, 1000, 0.0},
		{"half filled", 1000, 500, 50.0},
		{"fully filled", 1000, 0, 100.0},
		{"quarter filled", 1000, 750, 25.0},
		{"zero original", 0, 0, 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := &geniusyield.OrderState{
				OriginalAmount: tt.original,
			}
			state.OfferedAsset.Amount = tt.remaining
			result := state.FillPercent()
			if result != tt.expectedFill {
				t.Errorf("expected %f%%, got %f%%", tt.expectedFill, result)
			}
		})
	}
}

func TestCalculateGeniusYieldFillAmount(t *testing.T) {
	tests := []struct {
		name              string
		offeredAmount     uint64
		priceNum          int64
		priceDenom        int64
		askedAmount       uint64
		expectedOffered   uint64
		expectedRemainder uint64
	}{
		{
			name:              "exact fill",
			offeredAmount:     1000,
			priceNum:          1,
			priceDenom:        1,
			askedAmount:       1000,
			expectedOffered:   1000,
			expectedRemainder: 0,
		},
		{
			name:              "partial fill",
			offeredAmount:     1000,
			priceNum:          1,
			priceDenom:        1,
			askedAmount:       500,
			expectedOffered:   500,
			expectedRemainder: 0,
		},
		{
			name:              "overfill capped",
			offeredAmount:     1000,
			priceNum:          1,
			priceDenom:        1,
			askedAmount:       2000,
			expectedOffered:   1000,
			expectedRemainder: 1000,
		},
		{
			name:              "price ratio 2:1",
			offeredAmount:     1000,
			priceNum:          2,
			priceDenom:        1,
			askedAmount:       1000,
			expectedOffered:   500,
			expectedRemainder: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			order := &geniusyield.OrderState{
				PriceNum:   tt.priceNum,
				PriceDenom: tt.priceDenom,
				Price:      float64(tt.priceNum) / float64(tt.priceDenom),
			}
			order.OfferedAsset.Amount = tt.offeredAmount

			offered, remainder := CalculateGeniusYieldFillAmount(
				order,
				tt.askedAmount,
			)

			if offered != tt.expectedOffered {
				t.Errorf(
					"expected offered %d, got %d",
					tt.expectedOffered,
					offered,
				)
			}
			if remainder != tt.expectedRemainder {
				t.Errorf(
					"expected remainder %d, got %d",
					tt.expectedRemainder,
					remainder,
				)
			}
		})
	}
}

func TestGeniusYieldPartialOrderDatumUnmarshal(t *testing.T) {
	// Build a test PartialOrderDatum
	// Constructor 0 with all required fields

	ownerKey := make([]byte, 28)
	for i := range ownerKey {
		ownerKey[i] = byte(i + 1)
	}

	// Payment credential (PubKeyHash)
	paymentCred := cbor.NewConstructor(0, cbor.IndefLengthList{ownerKey})

	// Staking credential (None)
	stakingCred := cbor.NewConstructor(1, cbor.IndefLengthList{})

	// Owner address
	ownerAddr := cbor.NewConstructor(0, cbor.IndefLengthList{
		paymentCred,
		stakingCred,
	})

	// Offered asset (ADA)
	offeredAsset := cbor.NewConstructor(0, cbor.IndefLengthList{
		[]byte{},
		[]byte{},
	})

	// Asked asset (some token)
	askedAsset := cbor.NewConstructor(0, cbor.IndefLengthList{
		[]byte{0xab, 0xcd, 0xef},
		[]byte("TOKEN"),
	})

	// Price rational (1.5 = 3/2)
	price := cbor.NewConstructor(0, cbor.IndefLengthList{
		int64(3),
		int64(2),
	})

	// NFT token name
	nftName := []byte{0x01, 0x02, 0x03, 0x04}

	// Start time (None)
	startTime := cbor.NewConstructor(1, cbor.IndefLengthList{})

	// End time (Some timestamp)
	endTimestamp := int64(1735689600000) // 2025-01-01 00:00:00 UTC
	endTime := cbor.NewConstructor(0, cbor.IndefLengthList{endTimestamp})

	// Maker percent fee (0.3% = 3/1000)
	makerPercentFee := cbor.NewConstructor(0, cbor.IndefLengthList{
		int64(3),
		int64(1000),
	})

	// Contained fee
	containedFee := cbor.NewConstructor(0, cbor.IndefLengthList{
		uint64(1000000), // lovelaceFee
		uint64(0),       // offeredFee
		uint64(0),       // askedFee
	})

	datum := cbor.NewConstructor(0, cbor.IndefLengthList{
		ownerKey,         // podOwnerKey
		ownerAddr,        // podOwnerAddr
		offeredAsset,     // podOfferedAsset
		uint64(10000000), // podOfferedOriginalAmount
		uint64(5000000),  // podOfferedAmount (partially filled)
		askedAsset,       // podAskedAsset
		price,            // podPrice
		nftName,          // podNFT
		startTime,        // podStart
		endTime,          // podEnd
		uint64(3),        // podPartialFills
		uint64(2000000),  // podMakerLovelaceFlatFee
		makerPercentFee,  // podMakerOfferedPercentFee
		uint64(100000),   // podMakerOfferedPercentFeeMax
		containedFee,     // podContainedFee
		uint64(7500000),  // podContainedPayment
	})

	cborData, err := cbor.Encode(&datum)
	if err != nil {
		t.Fatalf("failed to encode test datum: %v", err)
	}

	var orderDatum GeniusYieldPartialOrderDatum
	if _, err := cbor.Decode(cborData, &orderDatum); err != nil {
		t.Fatalf("failed to decode datum: %v", err)
	}

	// Verify fields
	if orderDatum.OfferedOriginalAmount != 10000000 {
		t.Errorf(
			"expected offeredOriginalAmount 10000000, got %d",
			orderDatum.OfferedOriginalAmount,
		)
	}
	if orderDatum.OfferedAmount != 5000000 {
		t.Errorf(
			"expected offeredAmount 5000000, got %d",
			orderDatum.OfferedAmount,
		)
	}
	if orderDatum.PartialFills != 3 {
		t.Errorf(
			"expected partialFills 3, got %d",
			orderDatum.PartialFills,
		)
	}
	if orderDatum.Price.Numerator != 3 || orderDatum.Price.Denominator != 2 {
		t.Errorf(
			"expected price 3/2, got %d/%d",
			orderDatum.Price.Numerator,
			orderDatum.Price.Denominator,
		)
	}
	if orderDatum.Start.IsPresent {
		t.Error("expected start to be None")
	}
	if !orderDatum.End.IsPresent {
		t.Error("expected end to be Some")
	}
	if orderDatum.End.Time != endTimestamp {
		t.Errorf(
			"expected end time %d, got %d",
			endTimestamp,
			orderDatum.End.Time,
		)
	}
	if orderDatum.ContainedFee.LovelaceFee != 1000000 {
		t.Errorf(
			"expected containedFee.lovelaceFee 1000000, got %d",
			orderDatum.ContainedFee.LovelaceFee,
		)
	}
}

func TestGeniusYieldParserParseOrderDatum(t *testing.T) {
	// Build test datum
	ownerKey := make([]byte, 28)
	for i := range ownerKey {
		ownerKey[i] = byte(i)
	}

	paymentCred := cbor.NewConstructor(0, cbor.IndefLengthList{ownerKey})
	stakingCred := cbor.NewConstructor(1, cbor.IndefLengthList{})
	ownerAddr := cbor.NewConstructor(0, cbor.IndefLengthList{
		paymentCred,
		stakingCred,
	})

	offeredAsset := cbor.NewConstructor(0, cbor.IndefLengthList{
		[]byte{},
		[]byte{},
	})

	askedAsset := cbor.NewConstructor(0, cbor.IndefLengthList{
		[]byte{0xab, 0xcd},
		[]byte("TEST"),
	})

	price := cbor.NewConstructor(0, cbor.IndefLengthList{
		int64(15),
		int64(10),
	})

	nftName := []byte{0xde, 0xad, 0xbe, 0xef}
	startTime := cbor.NewConstructor(1, cbor.IndefLengthList{})
	endTime := cbor.NewConstructor(1, cbor.IndefLengthList{})
	makerPercentFee := cbor.NewConstructor(0, cbor.IndefLengthList{
		int64(3),
		int64(1000),
	})
	containedFee := cbor.NewConstructor(0, cbor.IndefLengthList{
		uint64(0),
		uint64(0),
		uint64(0),
	})

	datum := cbor.NewConstructor(0, cbor.IndefLengthList{
		ownerKey,
		ownerAddr,
		offeredAsset,
		uint64(5000000),
		uint64(5000000),
		askedAsset,
		price,
		nftName,
		startTime,
		endTime,
		uint64(0),
		uint64(1000000),
		makerPercentFee,
		uint64(50000),
		containedFee,
		uint64(0),
	})

	cborData, err := cbor.Encode(&datum)
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	parser := NewGeniusYieldParser()
	state, err := parser.ParseOrderDatum(
		cborData,
		"abc123def456",
		0,
		12345,
		time.Now(),
	)
	if err != nil {
		t.Fatalf("failed to parse datum: %v", err)
	}

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
		t.Errorf(
			"expected originalAmount 5000000, got %d",
			state.OriginalAmount,
		)
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
			// Build datum
			ownerKey := make([]byte, 28)
			paymentCred := cbor.NewConstructor(
				0,
				cbor.IndefLengthList{ownerKey},
			)
			stakingCred := cbor.NewConstructor(1, cbor.IndefLengthList{})
			ownerAddr := cbor.NewConstructor(0, cbor.IndefLengthList{
				paymentCred,
				stakingCred,
			})
			offeredAsset := cbor.NewConstructor(0, cbor.IndefLengthList{
				[]byte{},
				[]byte{},
			})
			askedAsset := cbor.NewConstructor(0, cbor.IndefLengthList{
				[]byte{0xab},
				[]byte("T"),
			})
			price := cbor.NewConstructor(0, cbor.IndefLengthList{
				int64(1),
				int64(1),
			})
			nftName := []byte{0x01}
			makerFee := cbor.NewConstructor(0, cbor.IndefLengthList{
				int64(0),
				int64(1),
			})
			containedFee := cbor.NewConstructor(0, cbor.IndefLengthList{
				uint64(0),
				uint64(0),
				uint64(0),
			})

			var startConstr, endConstr interface{}
			if tt.startTime != nil {
				startConstr = cbor.NewConstructor(0, cbor.IndefLengthList{
					tt.startTime.UnixMilli(),
				})
			} else {
				startConstr = cbor.NewConstructor(1, cbor.IndefLengthList{})
			}
			if tt.endTime != nil {
				endConstr = cbor.NewConstructor(0, cbor.IndefLengthList{
					tt.endTime.UnixMilli(),
				})
			} else {
				endConstr = cbor.NewConstructor(1, cbor.IndefLengthList{})
			}

			datum := cbor.NewConstructor(0, cbor.IndefLengthList{
				ownerKey,
				ownerAddr,
				offeredAsset,
				tt.offeredAmount,
				tt.offeredAmount,
				askedAsset,
				price,
				nftName,
				startConstr,
				endConstr,
				uint64(0),
				uint64(0),
				makerFee,
				uint64(0),
				containedFee,
				uint64(0),
			})

			cborData, err := cbor.Encode(&datum)
			if err != nil {
				t.Fatalf("failed to encode: %v", err)
			}

			parser := NewGeniusYieldParser()
			state, err := parser.ParseOrderDatum(cborData, "tx", 0, 0, now)
			if err != nil {
				t.Fatalf("failed to parse: %v", err)
			}

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
