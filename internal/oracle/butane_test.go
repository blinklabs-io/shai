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

func TestNewButaneParser(t *testing.T) {
	parser := NewButaneParser()
	if parser == nil {
		t.Fatal("expected non-nil parser")
	}
	if parser.Protocol() != "butane" {
		t.Errorf("expected protocol 'butane', got %s", parser.Protocol())
	}
}

func TestButaneAssetClassToCommonAssetClass(t *testing.T) {
	asset := ButaneAssetClass{
		PolicyId:  []byte{0x01, 0x02, 0x03},
		AssetName: []byte("bUSD"),
	}

	common := asset.ToCommonAssetClass()
	if string(common.PolicyId) != string(asset.PolicyId) {
		t.Error("policy ID mismatch")
	}
	if string(common.Name) != string(asset.AssetName) {
		t.Error("asset name mismatch")
	}
}

func TestGenerateButaneCDPId(t *testing.T) {
	txHash := "abc123def456789012345678901234567890"
	txIndex := uint32(2)

	cdpId := generateButaneCDPId(txHash, txIndex)
	expected := "butane_cdp_abc123def4567890#2"

	if cdpId != expected {
		t.Errorf("expected CDP ID %s, got %s", expected, cdpId)
	}
}

func TestButaneCDPCredentialUnmarshal(t *testing.T) {
	// Test AuthorizeWithPubKey (Constructor 0)
	pubKeyHash := make([]byte, 28)
	for i := range pubKeyHash {
		pubKeyHash[i] = byte(i + 1)
	}

	credConstr := cbor.NewConstructor(0, cbor.IndefLengthList{
		pubKeyHash,
	})

	cborData, err := cbor.Encode(&credConstr)
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	var cred ButaneCDPCredential
	if _, err := cbor.Decode(cborData, &cred); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	if cred.Type != 0 {
		t.Errorf("expected type 0, got %d", cred.Type)
	}
	if len(cred.PubKey) != 28 {
		t.Errorf("expected 28 byte pubkey, got %d", len(cred.PubKey))
	}
}

func TestButaneMonoDatumCDP(t *testing.T) {
	// Build a CDP datum (Constructor 1)
	// CDP fields: owner, synthetic, minted, startTime

	// Owner: AuthorizeWithPubKey
	pubKeyHash := make([]byte, 28)
	owner := cbor.NewConstructor(0, cbor.IndefLengthList{
		pubKeyHash,
	})

	// Synthetic asset (bUSD)
	synthetic := cbor.NewConstructor(0, cbor.IndefLengthList{
		[]byte{0xab, 0xcd, 0xef}, // policy
		[]byte("bUSD"),           // name
	})

	// MonoDatum with CDP constructor (1)
	datum := cbor.NewConstructor(1, cbor.IndefLengthList{
		owner,
		synthetic,
		uint64(100000000),    // minted: 100 bUSD
		int64(1704067200000), // startTime (ms)
	})

	cborData, err := cbor.Encode(&datum)
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	var monoDatum ButaneMonoDatum
	if _, err := cbor.Decode(cborData, &monoDatum); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	if monoDatum.Constructor != 1 {
		t.Errorf("expected constructor 1, got %d", monoDatum.Constructor)
	}
	if monoDatum.CDP == nil {
		t.Fatal("expected CDP to be populated")
	}
	if monoDatum.CDP.Minted != 100000000 {
		t.Errorf("expected minted 100000000, got %d", monoDatum.CDP.Minted)
	}
	if string(monoDatum.CDP.Synthetic.AssetName) != "bUSD" {
		t.Errorf(
			"expected synthetic 'bUSD', got %s",
			string(monoDatum.CDP.Synthetic.AssetName),
		)
	}
}

func TestButaneParserParseMonoDatum(t *testing.T) {
	// Build a CDP datum
	pubKeyHash := make([]byte, 28)
	for i := range pubKeyHash {
		pubKeyHash[i] = byte(i)
	}
	owner := cbor.NewConstructor(0, cbor.IndefLengthList{
		pubKeyHash,
	})

	synthetic := cbor.NewConstructor(0, cbor.IndefLengthList{
		[]byte{0x12, 0x34},
		[]byte("bBTC"),
	})

	datum := cbor.NewConstructor(1, cbor.IndefLengthList{
		owner,
		synthetic,
		uint64(50000000),     // 0.5 bBTC
		int64(1704067200000), // start time
	})

	cborData, err := cbor.Encode(&datum)
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	parser := NewButaneParser()
	state, err := parser.ParseMonoDatum(
		cborData,
		"abc123def456789012345678901234567890",
		0,
		12345,
		time.Now(),
	)
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	if state == nil {
		t.Fatal("expected non-nil state")
	}
	if state.MintedAmount != 50000000 {
		t.Errorf("expected minted 50000000, got %d", state.MintedAmount)
	}
	if state.Slot != 12345 {
		t.Errorf("expected slot 12345, got %d", state.Slot)
	}
	if string(state.Synthetic.Name) != "bBTC" {
		t.Errorf(
			"expected synthetic 'bBTC', got %s",
			string(state.Synthetic.Name),
		)
	}
}

func TestButaneParserNonCDPDatum(t *testing.T) {
	// Build a non-CDP datum (Constructor 0 = ParamsWrapper)
	datum := cbor.NewConstructor(0, cbor.IndefLengthList{
		[]byte{0x01, 0x02, 0x03}, // dummy params data
	})

	cborData, err := cbor.Encode(&datum)
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	parser := NewButaneParser()
	state, err := parser.ParseMonoDatum(
		cborData,
		"abc123",
		0,
		12345,
		time.Now(),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should return nil for non-CDP datums
	if state != nil {
		t.Error("expected nil state for non-CDP datum")
	}
}

func TestButaneCDPStateKey(t *testing.T) {
	state := &ButaneCDPState{
		CDPId: "butane_cdp_abc123#0",
	}

	expected := "butane:butane_cdp_abc123#0"
	if state.Key() != expected {
		t.Errorf("expected key %s, got %s", expected, state.Key())
	}
}

func TestButanePriceStateFloat(t *testing.T) {
	price := &ButanePriceState{
		Price:       1500000,
		Denominator: 1000000,
	}

	expected := 1.5
	if price.PriceFloat() != expected {
		t.Errorf("expected price %f, got %f", expected, price.PriceFloat())
	}

	// Test zero denominator
	price.Denominator = 0
	if price.PriceFloat() != 0 {
		t.Error("expected 0 for zero denominator")
	}
}

func TestButaneParserPoolDatum(t *testing.T) {
	// Butane is not an AMM, so ParsePoolDatum should return nil
	parser := NewButaneParser()

	datum := cbor.NewConstructor(0, cbor.IndefLengthList{})
	cborData, _ := cbor.Encode(&datum)

	state, err := parser.ParsePoolDatum(
		cborData,
		"abc123",
		0,
		12345,
		time.Now(),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state != nil {
		t.Error("expected nil pool state for Butane")
	}
}
