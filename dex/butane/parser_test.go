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

package butane

import (
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
)

func TestNewParser(t *testing.T) {
	parser := NewParser()
	if parser == nil {
		t.Fatal("expected non-nil parser")
	}
	if parser.Protocol() != "butane" {
		t.Errorf("expected protocol 'butane', got %s", parser.Protocol())
	}
}

func TestAssetClassToCommonAssetClass(t *testing.T) {
	asset := AssetClass{
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

	cdpId := GenerateCDPId(txHash, txIndex)
	expected := "butane_cdp_abc123def456789012345678901234567890#2"

	if cdpId != expected {
		t.Errorf("expected CDP ID %s, got %s", expected, cdpId)
	}
}

func TestGenerateButaneCDPIdShortHash(t *testing.T) {
	cdpId := GenerateCDPId("abc123", 0)
	expected := "butane_cdp_abc123#0"

	if cdpId != expected {
		t.Errorf("expected CDP ID %s, got %s", expected, cdpId)
	}
}

func TestCDPCredentialUnmarshal(t *testing.T) {
	// Test AuthorizeWithPubKey (Constructor 0)
	pubKeyHash := make([]byte, 28)
	for i := range pubKeyHash {
		pubKeyHash[i] = byte(i + 1)
	}

	credConstr := cbor.NewConstructorEncoder(0, cbor.IndefLengthList{
		pubKeyHash,
	})

	cborData, err := cbor.Encode(&credConstr)
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	var cred CDPCredential
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

func TestCDPCredentialPubKeyRejectsExtraFields(t *testing.T) {
	pubKeyHash := make([]byte, 28)
	credConstr := cbor.NewConstructorEncoder(0, cbor.IndefLengthList{
		pubKeyHash,
		[]byte{0x01},
	})

	cborData, err := cbor.Encode(&credConstr)
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	var cred CDPCredential
	if _, err := cbor.Decode(cborData, &cred); err == nil {
		t.Fatal("expected extra AuthorizeWithPubKey field to fail")
	}
}

func TestCDPCredentialConstraintToken(t *testing.T) {
	asset := cbor.NewConstructorEncoder(0, cbor.IndefLengthList{
		[]byte{0xab, 0xcd},
		[]byte("owner"),
	})
	constraint := cbor.NewConstructorEncoder(0, cbor.IndefLengthList{
		asset,
	})
	credConstr := cbor.NewConstructorEncoder(1, cbor.IndefLengthList{
		constraint,
	})

	cborData, err := cbor.Encode(&credConstr)
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	var cred CDPCredential
	if _, err := cbor.Decode(cborData, &cred); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if cred.TokenId == nil {
		t.Fatal("expected token ID")
	}
	if string(cred.TokenId.AssetName) != "owner" {
		t.Fatalf("expected owner token name, got %q", cred.TokenId.AssetName)
	}
}

func TestCDPCredentialConstraintUnsupported(t *testing.T) {
	withdrawConstraint := cbor.NewConstructorEncoder(1, cbor.IndefLengthList{
		[]byte{0x01},
	})
	credConstr := cbor.NewConstructorEncoder(1, cbor.IndefLengthList{
		withdrawConstraint,
	})

	cborData, err := cbor.Encode(&credConstr)
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	var cred CDPCredential
	if _, err := cbor.Decode(cborData, &cred); err == nil {
		t.Fatal("expected unsupported constraint to fail")
	}
}

func TestAssetClassRejectsInvalidConstructor(t *testing.T) {
	asset := cbor.NewConstructorEncoder(1, cbor.IndefLengthList{
		[]byte{0xab, 0xcd, 0xef},
		[]byte("bUSD"),
	})

	cborData, err := cbor.Encode(&asset)
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	var decoded AssetClass
	if _, err := cbor.Decode(cborData, &decoded); err == nil {
		t.Fatal("expected invalid AssetClass constructor to fail")
	}
}

func TestMonoDatumCDP(t *testing.T) {
	// Build a CDP datum (Constructor 1)
	// CDP fields: owner, synthetic, minted, startTime

	// Owner: AuthorizeWithPubKey
	pubKeyHash := make([]byte, 28)
	owner := cbor.NewConstructorEncoder(0, cbor.IndefLengthList{
		pubKeyHash,
	})

	// Synthetic asset (bUSD)
	synthetic := cbor.NewConstructorEncoder(0, cbor.IndefLengthList{
		[]byte{0xab, 0xcd, 0xef}, // policy
		[]byte("bUSD"),           // name
	})

	// MonoDatum with CDP constructor (1)
	datum := cbor.NewConstructorEncoder(1, cbor.IndefLengthList{
		owner,
		synthetic,
		uint64(100000000),    // minted: 100 bUSD
		int64(1704067200000), // startTime (ms)
	})

	cborData, err := cbor.Encode(&datum)
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	var monoDatum MonoDatum
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
	owner := cbor.NewConstructorEncoder(0, cbor.IndefLengthList{
		pubKeyHash,
	})

	synthetic := cbor.NewConstructorEncoder(0, cbor.IndefLengthList{
		[]byte{0x12, 0x34},
		[]byte("bBTC"),
	})

	startTimeMs := int64(1704067200123)
	datum := cbor.NewConstructorEncoder(1, cbor.IndefLengthList{
		owner,
		synthetic,
		uint64(50000000), // 0.5 bBTC
		startTimeMs,
	})

	cborData, err := cbor.Encode(&datum)
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	parser := NewParser()
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
	if !state.StartTime.Equal(time.UnixMilli(startTimeMs)) {
		t.Errorf(
			"expected start time %s, got %s",
			time.UnixMilli(startTimeMs),
			state.StartTime,
		)
	}
}

func TestButaneParserNonCDPDatum(t *testing.T) {
	// Build a non-CDP datum (Constructor 0 = ParamsWrapper)
	datum := cbor.NewConstructorEncoder(0, cbor.IndefLengthList{
		[]byte{0x01, 0x02, 0x03}, // dummy params data
	})

	cborData, err := cbor.Encode(&datum)
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	parser := NewParser()
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

func TestCDPStateKey(t *testing.T) {
	state := &CDPState{
		CDPId: "butane_cdp_abc123#0",
	}

	expected := "butane:butane_cdp_abc123#0"
	if state.Key() != expected {
		t.Errorf("expected key %s, got %s", expected, state.Key())
	}
}

func TestGetCDPAddresses(t *testing.T) {
	addrs := GetCDPAddresses()
	if len(addrs) != 1 {
		t.Fatalf("expected 1 Butane address, got %d", len(addrs))
	}
	if addrs[0] != CDPContractAddress {
		t.Fatalf("unexpected Butane CDP address: %q", addrs[0])
	}
}
