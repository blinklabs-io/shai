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
	"bytes"
	"encoding/hex"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
)

func TestNewIndigoParser(t *testing.T) {
	parser := NewIndigoParser()
	if parser == nil {
		t.Fatal("expected non-nil parser")
	}
	if parser.Protocol() != "indigo" {
		t.Errorf("expected protocol 'indigo', got %s", parser.Protocol())
	}
}

func TestGenerateIndigoCDPId(t *testing.T) {
	txHash := "abc123def456789012345678901234567890"
	txIndex := uint32(2)

	cdpId := generateIndigoCDPId(txHash, txIndex)
	expected := "indigo_cdp_abc123def456789012345678901234567890#2"

	if cdpId != expected {
		t.Errorf("expected CDP ID %s, got %s", expected, cdpId)
	}

	otherTxHash := "abc123def4567890ffffffffffffffffffffffff"
	otherCDPId := generateIndigoCDPId(otherTxHash, txIndex)
	if cdpId == otherCDPId {
		t.Errorf("expected distinct CDP IDs, got %s", cdpId)
	}
}

func TestIndigoMaybePubKeyHashWithValue(t *testing.T) {
	// Test PubKeyHash (Constructor 0 = #6.121)
	pubKeyHash := make([]byte, 28)
	for i := range pubKeyHash {
		pubKeyHash[i] = byte(i + 1)
	}

	// Create PubKeyHash: #6.121([bytes])
	pubKeyConstr := cbor.NewConstructorEncoder(0, cbor.IndefLengthList{
		pubKeyHash,
	})

	cborData, err := cbor.Encode(&pubKeyConstr)
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	var maybe IndigoMaybePubKeyHash
	if _, err := cbor.Decode(cborData, &maybe); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	if !maybe.IsJust {
		t.Error("expected IsJust to be true")
	}
	if len(maybe.Hash) != 28 {
		t.Errorf("expected 28 byte hash, got %d", len(maybe.Hash))
	}
	if !bytes.Equal(maybe.Hash, pubKeyHash) {
		t.Errorf("decoded hash mismatch: got %x want %x", maybe.Hash, pubKeyHash)
	}
}

func TestIndigoMaybePubKeyHashNothing(t *testing.T) {
	// Test Nothing (Constructor 1 = #6.122)
	nothingConstr := cbor.NewConstructorEncoder(1, cbor.IndefLengthList{})

	cborData, err := cbor.Encode(&nothingConstr)
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	var maybe IndigoMaybePubKeyHash
	if _, err := cbor.Decode(cborData, &maybe); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	if maybe.IsJust {
		t.Error("expected IsJust to be false")
	}
	if maybe.Hash != nil {
		t.Error("expected Hash to be nil")
	}
}

func TestIndigoAccumulatedFeesInterest(t *testing.T) {
	// Test InterestIAssetAmount (Constructor 0 = #6.121)
	interestConstr := cbor.NewConstructorEncoder(0, cbor.IndefLengthList{
		int64(1704067200000), // lastUpdated
		int64(500000),        // iAssetAmount
	})

	cborData, err := cbor.Encode(&interestConstr)
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	var fees IndigoAccumulatedFees
	if _, err := cbor.Decode(cborData, &fees); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	if fees.Type != 0 {
		t.Errorf("expected type 0, got %d", fees.Type)
	}
	if fees.LastUpdated != 1704067200000 {
		t.Errorf("expected lastUpdated 1704067200000, got %d", fees.LastUpdated)
	}
	if fees.IAssetAmount != 500000 {
		t.Errorf("expected iAssetAmount 500000, got %d", fees.IAssetAmount)
	}
}

func TestIndigoAccumulatedFeesLovelaces(t *testing.T) {
	// Test FeesLovelacesAmount (Constructor 1 = #6.122)
	feesConstr := cbor.NewConstructorEncoder(1, cbor.IndefLengthList{
		int64(1000000), // treasury
		int64(2000000), // indyStakers
	})

	cborData, err := cbor.Encode(&feesConstr)
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	var fees IndigoAccumulatedFees
	if _, err := cbor.Decode(cborData, &fees); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	if fees.Type != 1 {
		t.Errorf("expected type 1, got %d", fees.Type)
	}
	if fees.Treasury != 1000000 {
		t.Errorf("expected treasury 1000000, got %d", fees.Treasury)
	}
	if fees.IndyStakers != 2000000 {
		t.Errorf("expected indyStakers 2000000, got %d", fees.IndyStakers)
	}
}

func TestIndigoAccumulatedFeesClearsInactiveFields(t *testing.T) {
	feesConstr := cbor.NewConstructorEncoder(1, cbor.IndefLengthList{
		int64(1000000),
		int64(2000000),
	})
	feesData, err := cbor.Encode(&feesConstr)
	if err != nil {
		t.Fatalf("failed to encode fees: %v", err)
	}

	interestConstr := cbor.NewConstructorEncoder(0, cbor.IndefLengthList{
		int64(1704067200000),
		int64(500000),
	})
	interestData, err := cbor.Encode(&interestConstr)
	if err != nil {
		t.Fatalf("failed to encode interest: %v", err)
	}

	var fees IndigoAccumulatedFees
	if _, err := cbor.Decode(feesData, &fees); err != nil {
		t.Fatalf("failed to decode fees: %v", err)
	}
	if _, err := cbor.Decode(interestData, &fees); err != nil {
		t.Fatalf("failed to decode interest: %v", err)
	}
	if fees.Treasury != 0 {
		t.Errorf("expected inactive Treasury to be cleared, got %d", fees.Treasury)
	}
	if fees.IndyStakers != 0 {
		t.Errorf(
			"expected inactive IndyStakers to be cleared, got %d",
			fees.IndyStakers,
		)
	}

	if _, err := cbor.Decode(feesData, &fees); err != nil {
		t.Fatalf("failed to decode fees again: %v", err)
	}
	if fees.LastUpdated != 0 {
		t.Errorf(
			"expected inactive LastUpdated to be cleared, got %d",
			fees.LastUpdated,
		)
	}
	if fees.IAssetAmount != 0 {
		t.Errorf(
			"expected inactive IAssetAmount to be cleared, got %d",
			fees.IAssetAmount,
		)
	}
}

func TestIndigoCDPContentDatumFull(t *testing.T) {
	// Build a complete CDP content datum following the CDDL:
	// CDPContent = #6.121([#6.121([ owner, iAsset, mintedAmount, accumulatedFees ])])

	// Owner: PubKeyHash (Just)
	pubKeyHash := make([]byte, 28)
	for i := range pubKeyHash {
		pubKeyHash[i] = byte(i)
	}
	owner := cbor.NewConstructorEncoder(0, cbor.IndefLengthList{
		pubKeyHash,
	})

	// iAsset: bytes representing "iUSD"
	iAsset := []byte("iUSD")

	// mintedAmount
	mintedAmount := int64(100000000) // 100 iUSD

	// accumulatedFees: InterestIAssetAmount
	accumulatedFees := cbor.NewConstructorEncoder(0, cbor.IndefLengthList{
		int64(1704067200000), // lastUpdated
		int64(50000),         // iAssetAmount
	})

	// Inner constructor: #6.121([ owner, iAsset, mintedAmount, accumulatedFees ])
	inner := cbor.NewConstructorEncoder(0, cbor.IndefLengthList{
		owner,
		iAsset,
		mintedAmount,
		accumulatedFees,
	})

	// Outer constructor: #6.121([inner])
	outer := cbor.NewConstructorEncoder(0, cbor.IndefLengthList{
		inner,
	})

	cborData, err := cbor.Encode(&outer)
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	var datum IndigoCDPContentDatum
	if _, err := cbor.Decode(cborData, &datum); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	if datum.Inner == nil {
		t.Fatal("expected Inner to be populated")
	}

	if !datum.Inner.Owner.IsJust {
		t.Error("expected owner to be Just")
	}
	if len(datum.Inner.Owner.Hash) != 28 {
		t.Errorf(
			"expected 28 byte owner hash, got %d",
			len(datum.Inner.Owner.Hash),
		)
	}
	if string(datum.Inner.IAsset) != "iUSD" {
		t.Errorf("expected iAsset 'iUSD', got %s", string(datum.Inner.IAsset))
	}
	if datum.Inner.MintedAmount != 100000000 {
		t.Errorf(
			"expected mintedAmount 100000000, got %d",
			datum.Inner.MintedAmount,
		)
	}
	if datum.Inner.AccumulatedFees.Type != 0 {
		t.Errorf(
			"expected fees type 0, got %d",
			datum.Inner.AccumulatedFees.Type,
		)
	}
}

func TestIndigoCDPContentDatumClearsInnerOnNonMatch(t *testing.T) {
	pubKeyHash := make([]byte, 28)
	owner := cbor.NewConstructorEncoder(0, cbor.IndefLengthList{
		pubKeyHash,
	})
	accumulatedFees := cbor.NewConstructorEncoder(0, cbor.IndefLengthList{
		int64(1704067200000),
		int64(50000),
	})
	inner := cbor.NewConstructorEncoder(0, cbor.IndefLengthList{
		owner,
		[]byte("iUSD"),
		int64(100000000),
		accumulatedFees,
	})
	outer := cbor.NewConstructorEncoder(0, cbor.IndefLengthList{
		inner,
	})
	cdpData, err := cbor.Encode(&outer)
	if err != nil {
		t.Fatalf("failed to encode CDP datum: %v", err)
	}

	var datum IndigoCDPContentDatum
	if _, err := cbor.Decode(cdpData, &datum); err != nil {
		t.Fatalf("failed to decode CDP datum: %v", err)
	}
	if datum.Inner == nil {
		t.Fatal("expected Inner to be populated")
	}

	nonMatch := cbor.NewConstructorEncoder(1, cbor.IndefLengthList{})
	nonMatchData, err := cbor.Encode(&nonMatch)
	if err != nil {
		t.Fatalf("failed to encode non-matching datum: %v", err)
	}
	if _, err := cbor.Decode(nonMatchData, &datum); err != nil {
		t.Fatalf("failed to decode non-matching datum: %v", err)
	}
	if datum.Inner != nil {
		t.Fatal("expected Inner to be cleared for non-matching datum")
	}
}

func TestIndigoParserParseCDPDatum(t *testing.T) {
	// Build a complete CDP datum
	pubKeyHash := make([]byte, 28)
	for i := range pubKeyHash {
		pubKeyHash[i] = byte(i)
	}
	owner := cbor.NewConstructorEncoder(0, cbor.IndefLengthList{
		pubKeyHash,
	})

	iAsset := []byte("iBTC")
	mintedAmount := int64(50000000)

	accumulatedFees := cbor.NewConstructorEncoder(1, cbor.IndefLengthList{
		int64(1000000), // treasury
		int64(500000),  // indyStakers
	})

	inner := cbor.NewConstructorEncoder(0, cbor.IndefLengthList{
		owner,
		iAsset,
		mintedAmount,
		accumulatedFees,
	})

	outer := cbor.NewConstructorEncoder(0, cbor.IndefLengthList{
		inner,
	})

	cborData, err := cbor.Encode(&outer)
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	parser := NewIndigoParser()
	txHash := "abc123def456789012345678901234567890"
	txIndex := uint32(3)
	timestamp := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	state, err := parser.ParseCDPDatum(
		cborData,
		txHash,
		txIndex,
		12345,
		timestamp,
	)
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	if state == nil {
		t.Fatal("expected non-nil state")
	}
	expectedCDPId := "indigo_cdp_abc123def456789012345678901234567890#3"
	if state.CDPId != expectedCDPId {
		t.Errorf("expected CDPId %s, got %s", expectedCDPId, state.CDPId)
	}
	if state.TxHash != txHash {
		t.Errorf("expected TxHash %s, got %s", txHash, state.TxHash)
	}
	if state.TxIndex != txIndex {
		t.Errorf("expected TxIndex %d, got %d", txIndex, state.TxIndex)
	}
	if !state.Timestamp.Equal(timestamp) {
		t.Errorf("expected Timestamp %s, got %s", timestamp, state.Timestamp)
	}
	if state.MintedAmount != 50000000 {
		t.Errorf("expected mintedAmount 50000000, got %d", state.MintedAmount)
	}
	if state.Slot != 12345 {
		t.Errorf("expected slot 12345, got %d", state.Slot)
	}
	if state.IAsset != hex.EncodeToString([]byte("iBTC")) {
		t.Errorf(
			"expected iAsset hex of 'iBTC', got %s",
			state.IAsset,
		)
	}
	if !state.HasOwner {
		t.Error("expected HasOwner to be true")
	}
	if state.FeesType != 1 {
		t.Errorf("expected FeesType 1, got %d", state.FeesType)
	}
	if state.Treasury != 1000000 {
		t.Errorf("expected Treasury 1000000, got %d", state.Treasury)
	}
	if state.IndyStakers != 500000 {
		t.Errorf("expected IndyStakers 500000, got %d", state.IndyStakers)
	}
}

func TestIndigoParserNothingOwner(t *testing.T) {
	// Test CDP with Nothing owner
	owner := cbor.NewConstructorEncoder(1, cbor.IndefLengthList{}) // Nothing

	iAsset := []byte("iETH")
	mintedAmount := int64(25000000)

	accumulatedFees := cbor.NewConstructorEncoder(0, cbor.IndefLengthList{
		int64(1704067200000),
		int64(10000),
	})

	inner := cbor.NewConstructorEncoder(0, cbor.IndefLengthList{
		owner,
		iAsset,
		mintedAmount,
		accumulatedFees,
	})

	outer := cbor.NewConstructorEncoder(0, cbor.IndefLengthList{
		inner,
	})

	cborData, err := cbor.Encode(&outer)
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	parser := NewIndigoParser()
	state, err := parser.ParseCDPDatum(
		cborData,
		"def456789012345678901234567890abcdef",
		1,
		54321,
		time.Now(),
	)
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	if state == nil {
		t.Fatal("expected non-nil state")
	}
	if state.HasOwner {
		t.Error("expected HasOwner to be false")
	}
	if state.Owner != "" {
		t.Errorf("expected empty owner, got %s", state.Owner)
	}
}

func TestIndigoParserEmptyOwnerHashHasNoOwner(t *testing.T) {
	owner := cbor.NewConstructorEncoder(0, cbor.IndefLengthList{
		[]byte{},
	})

	iAsset := []byte("iETH")
	mintedAmount := int64(25000000)

	accumulatedFees := cbor.NewConstructorEncoder(0, cbor.IndefLengthList{
		int64(1704067200000),
		int64(10000),
	})

	inner := cbor.NewConstructorEncoder(0, cbor.IndefLengthList{
		owner,
		iAsset,
		mintedAmount,
		accumulatedFees,
	})

	outer := cbor.NewConstructorEncoder(0, cbor.IndefLengthList{
		inner,
	})

	cborData, err := cbor.Encode(&outer)
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	parser := NewIndigoParser()
	state, err := parser.ParseCDPDatum(
		cborData,
		"def456789012345678901234567890abcdef",
		1,
		54321,
		time.Now(),
	)
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	if state == nil {
		t.Fatal("expected non-nil state")
	}
	if state.HasOwner {
		t.Error("expected HasOwner to be false for empty owner hash")
	}
	if state.Owner != "" {
		t.Errorf("expected empty owner, got %s", state.Owner)
	}
}

func TestIndigoCDPStateKey(t *testing.T) {
	state := &IndigoCDPState{
		CDPId: "indigo_cdp_abc123#0",
	}

	expected := "indigo:indigo_cdp_abc123#0"
	if state.Key() != expected {
		t.Errorf("expected key %s, got %s", expected, state.Key())
	}
}

func TestIndigoCDPStateIAssetName(t *testing.T) {
	// Test known iAsset
	state := &IndigoCDPState{
		IAsset: hex.EncodeToString([]byte("iUSD")),
	}
	if state.IAssetName() != "iUSD" {
		t.Errorf("expected iAssetName 'iUSD', got %s", state.IAssetName())
	}

	// Test iBTC
	state.IAsset = hex.EncodeToString([]byte("iBTC"))
	if state.IAssetName() != "iBTC" {
		t.Errorf("expected iAssetName 'iBTC', got %s", state.IAssetName())
	}

	// Test iETH
	state.IAsset = hex.EncodeToString([]byte("iETH"))
	if state.IAssetName() != "iETH" {
		t.Errorf("expected iAssetName 'iETH', got %s", state.IAssetName())
	}

	// Test unknown but printable
	state.IAsset = hex.EncodeToString([]byte("iGOLD"))
	if state.IAssetName() != "iGOLD" {
		t.Errorf("expected iAssetName 'iGOLD', got %s", state.IAssetName())
	}
}

func TestIndigoParserPoolDatum(t *testing.T) {
	// Indigo is not an AMM, so ParsePoolDatum should return nil
	parser := NewIndigoParser()

	datum := cbor.NewConstructorEncoder(0, cbor.IndefLengthList{})
	cborData, _ := cbor.Encode(&datum)

	state, err := parser.ParsePoolDatum(
		cborData,
		nil,
		"abc123",
		0,
		12345,
		time.Now(),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state != nil {
		t.Error("expected nil pool state for Indigo")
	}
}

func TestGetIndigoAddresses(t *testing.T) {
	addresses := GetIndigoAddresses()
	if len(addresses) == 0 {
		t.Error("expected at least one address")
	}

	// Verify the known mainnet address is included
	found := false
	for _, addr := range addresses {
		if addr == IndigoCDPContractAddress {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected CDP contract address to be in the list")
	}
}
