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
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
)

const (
	liveMainnetCDPDatum       = "d87a9fd8799f581c3258f32901c7ac8acfb0815ac78515d7e27f949e7ec71f23ee1aa7bc40ff454d494441531b00000003383ef9821b0000019c07cd50b0ff"
	deployedSyntheticPolicyId = "00000000000410c2d9e01e8ec78ab1dc6bbc383fae76cbe2689beb02"
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
		[]byte{},
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
		[]byte{0x02},
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
	if cred.Constraint == nil || cred.Constraint.TokenId == nil {
		t.Fatal("expected token ID")
	}
	if string(cred.Constraint.TokenId.AssetName) != "owner" {
		t.Fatalf(
			"expected owner token name, got %q",
			cred.Constraint.TokenId.AssetName,
		)
	}
}

func TestCDPCredentialWithdrawalConstraint(t *testing.T) {
	withdrawConstraint := cbor.NewConstructorEncoder(1, cbor.IndefLengthList{
		cbor.NewConstructorEncoder(0, cbor.IndefLengthList{
			[]byte{0x01},
		}),
	})
	credConstr := cbor.NewConstructorEncoder(1, cbor.IndefLengthList{
		withdrawConstraint,
	})

	cborData, err := cbor.Encode(&credConstr)
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	var cred CDPCredential
	if _, err := cbor.Decode(cborData, &cred); err != nil {
		t.Fatalf("failed to decode withdrawal constraint: %v", err)
	}
	if cred.Constraint == nil || cred.Constraint.Type != 1 {
		t.Fatalf("unexpected withdrawal constraint: %#v", cred.Constraint)
	}
	if len(cred.Constraint.WithdrawalCredential) == 0 {
		t.Fatal("expected encoded withdrawal credential")
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
		[]byte{},
	})

	// Synthetic asset (bUSD)
	synthetic := []byte("bUSD")

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
	if string(monoDatum.CDP.SyntheticName) != "bUSD" {
		t.Errorf(
			"expected synthetic 'bUSD', got %s",
			string(monoDatum.CDP.SyntheticName),
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
		[]byte{},
	})

	synthetic := []byte("bBTC")

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

func TestButaneParserRejectsNegativeMintedAmount(t *testing.T) {
	owner := cbor.NewConstructorEncoder(0, cbor.IndefLengthList{
		make([]byte, 28),
		[]byte{},
	})
	datum := cbor.NewConstructorEncoder(1, cbor.IndefLengthList{
		owner,
		[]byte("bBTC"),
		int64(-1),
		int64(1_704_067_200_123),
	})
	cborData, err := cbor.Encode(&datum)
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}
	var monoDatum MonoDatum
	if _, err := cbor.Decode(cborData, &monoDatum); err == nil {
		t.Fatal("expected negative minted amount to fail during model decoding")
	}

	_, err = NewParser().ParseMonoDatum(
		cborData,
		strings.Repeat("a", 64),
		0,
		1,
		time.Unix(0, 0),
	)
	if err == nil {
		t.Fatal("expected negative minted amount to fail")
	}
}

func TestButaneParserParsesLiveMainnetCDPDatum(t *testing.T) {
	datum, err := hex.DecodeString(liveMainnetCDPDatum)
	if err != nil {
		t.Fatalf("failed to decode live datum fixture: %v", err)
	}

	state, err := NewParser().ParseMonoDatum(
		datum,
		"89fe03b8fc7cc9e4fae4de3fc7ee3b3be7326d0092b51d1b43902cf8ea479a5e",
		0,
		196_882_173,
		time.Unix(1_769_657_573, 0),
	)
	if err != nil {
		t.Fatalf("failed to parse live mainnet CDP datum: %v", err)
	}
	if state == nil {
		t.Fatal("expected live mainnet datum to produce a CDP state")
	}
	if state.Owner != "3258f32901c7ac8acfb0815ac78515d7e27f949e7ec71f23ee1aa7bc" {
		t.Fatalf("unexpected owner hash: %q", state.Owner)
	}
	if string(state.Synthetic.PolicyId) != string(mustDecodeHex(t, deployedSyntheticPolicyId)) {
		t.Fatalf("unexpected synthetic policy: %x", state.Synthetic.PolicyId)
	}
	if string(state.Synthetic.Name) != "MIDAS" {
		t.Fatalf("unexpected synthetic name: %q", state.Synthetic.Name)
	}
	if state.MintedAmount != 13_828_553_090 {
		t.Fatalf("unexpected minted amount: %d", state.MintedAmount)
	}
	if !state.StartTime.Equal(time.UnixMilli(1_769_657_422_000)) {
		t.Fatalf("unexpected start time: %s", state.StartTime)
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

func TestGetCDPPaymentCredentials(t *testing.T) {
	credentials := GetCDPPaymentCredentials()
	if len(credentials) != 1 {
		t.Fatalf("expected 1 Butane payment credential, got %d", len(credentials))
	}
	if credentials[0] != CDPPaymentCredential {
		t.Fatalf("unexpected Butane CDP payment credential: %q", credentials[0])
	}
}

func mustDecodeHex(t *testing.T, value string) []byte {
	t.Helper()
	decoded, err := hex.DecodeString(value)
	if err != nil {
		t.Fatalf("failed to decode %q: %v", value, err)
	}
	return decoded
}
