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
	"encoding/hex"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
)

func TestNewOptimParser(t *testing.T) {
	parser := NewOptimParser()
	if parser == nil {
		t.Fatal("expected non-nil parser")
	}
	if parser.Protocol() != "optim" {
		t.Errorf("expected protocol 'optim', got %s", parser.Protocol())
	}
}

func TestGenerateOptimBondId(t *testing.T) {
	bondNFT := []byte{
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18,
	}

	bondId := generateOptimBondId(bondNFT)
	expected := "optim_bond_0102030405060708090a0b0c0d0e0f10"

	if bondId != expected {
		t.Errorf("expected bond ID %s, got %s", expected, bondId)
	}
}

func TestOptimCredentialUnmarshal(t *testing.T) {
	// Test VerificationKeyCredential (Constructor 0)
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

	var cred OptimCredential
	if _, err := cbor.Decode(cborData, &cred); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	if cred.Type != OptimCredentialTypeVerificationKey {
		t.Errorf(
			"expected type %d, got %d",
			OptimCredentialTypeVerificationKey,
			cred.Type,
		)
	}
	if len(cred.Hash) != 28 {
		t.Errorf("expected 28 byte hash, got %d", len(cred.Hash))
	}
}

func TestOptimCredentialScriptUnmarshal(t *testing.T) {
	// Test ScriptCredential (Constructor 1)
	scriptHash := make([]byte, 28)
	for i := range scriptHash {
		scriptHash[i] = byte(i + 0x10)
	}

	credConstr := cbor.NewConstructor(1, cbor.IndefLengthList{
		scriptHash,
	})

	cborData, err := cbor.Encode(&credConstr)
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	var cred OptimCredential
	if _, err := cbor.Decode(cborData, &cred); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	if cred.Type != OptimCredentialTypeScript {
		t.Errorf(
			"expected type %d, got %d",
			OptimCredentialTypeScript,
			cred.Type,
		)
	}
	if !cred.IsScript() {
		t.Error("expected IsScript() to be true")
	}
}

func TestOptimMaybeStakeCredentialNone(t *testing.T) {
	// Test None (Constructor 1)
	noneConstr := cbor.NewConstructor(1, cbor.IndefLengthList{})

	cborData, err := cbor.Encode(&noneConstr)
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	var maybe OptimMaybeStakeCredential
	if _, err := cbor.Decode(cborData, &maybe); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	if maybe.IsPresent {
		t.Error("expected IsPresent to be false")
	}
}

func TestOptimRational(t *testing.T) {
	// Test Rational (Constructor 0)
	rationalConstr := cbor.NewConstructor(0, cbor.IndefLengthList{
		uint64(105), // numerator
		uint64(100), // denominator
	})

	cborData, err := cbor.Encode(&rationalConstr)
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	var rational OptimRational
	if _, err := cbor.Decode(cborData, &rational); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	if rational.Numerator != 105 {
		t.Errorf("expected numerator 105, got %d", rational.Numerator)
	}
	if rational.Denominator != 100 {
		t.Errorf("expected denominator 100, got %d", rational.Denominator)
	}

	expectedFloat := 1.05
	if rational.Float64() != expectedFloat {
		t.Errorf(
			"expected float64 %f, got %f",
			expectedFloat,
			rational.Float64(),
		)
	}
}

func TestOptimRationalZeroDenominator(t *testing.T) {
	rational := OptimRational{
		Numerator:   100,
		Denominator: 0,
	}

	if rational.Float64() != 0 {
		t.Error("expected 0 for zero denominator")
	}
}

func TestOptimBondDatumFull(t *testing.T) {
	// Build a complete bond datum

	// Bond NFT (32 bytes)
	bondNFT := make([]byte, 32)
	for i := range bondNFT {
		bondNFT[i] = byte(i)
	}

	// Lender address: payment credential (pubkey)
	pubKeyHash := make([]byte, 28)
	for i := range pubKeyHash {
		pubKeyHash[i] = byte(i + 0x10)
	}
	paymentCred := cbor.NewConstructor(0, cbor.IndefLengthList{
		pubKeyHash,
	})

	// Stake credential: None
	stakeCred := cbor.NewConstructor(1, cbor.IndefLengthList{})

	// Address
	address := cbor.NewConstructor(0, cbor.IndefLengthList{
		paymentCred,
		stakeCred,
	})

	// Borrower NFT
	borrowerNFT := make([]byte, 32)
	for i := range borrowerNFT {
		borrowerNFT[i] = byte(i + 0x20)
	}

	// Stake pool ID (28 bytes)
	stakePool := make([]byte, 28)
	for i := range stakePool {
		stakePool[i] = byte(i + 0x30)
	}

	// BondDatum constructor
	bondDatum := cbor.NewConstructor(0, cbor.IndefLengthList{
		bondNFT,
		address,
		borrowerNFT,
		uint64(1000000000), // principal: 1000 ADA
		uint64(500),        // interest rate: 5% (500 basis points)
		uint64(10),         // duration: 10 epochs
		uint64(400),        // start epoch
		uint64(410),        // end epoch
		stakePool,
		uint64(50000000), // accrued rewards: 50 ADA
		uint64(0),        // status: active
	})

	cborData, err := cbor.Encode(&bondDatum)
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	var datum OptimBondDatum
	if _, err := cbor.Decode(cborData, &datum); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	if len(datum.BondNFT) != 32 {
		t.Errorf("expected 32 byte bond NFT, got %d", len(datum.BondNFT))
	}
	if datum.PrincipalAmount != 1000000000 {
		t.Errorf(
			"expected principal 1000000000, got %d",
			datum.PrincipalAmount,
		)
	}
	if datum.InterestRate != 500 {
		t.Errorf("expected interest rate 500, got %d", datum.InterestRate)
	}
	if datum.Duration != 10 {
		t.Errorf("expected duration 10, got %d", datum.Duration)
	}
	if datum.StartEpoch != 400 {
		t.Errorf("expected start epoch 400, got %d", datum.StartEpoch)
	}
	if datum.EndEpoch != 410 {
		t.Errorf("expected end epoch 410, got %d", datum.EndEpoch)
	}
	if datum.AccruedRewards != 50000000 {
		t.Errorf(
			"expected accrued rewards 50000000, got %d",
			datum.AccruedRewards,
		)
	}
	if datum.Status != 0 {
		t.Errorf("expected status 0, got %d", datum.Status)
	}
	if !datum.IsActive() {
		t.Error("expected IsActive() to be true")
	}
	if datum.IsMatured() {
		t.Error("expected IsMatured() to be false")
	}

	// Check interest rate percent
	expectedRate := 5.0
	if datum.InterestRatePercent() != expectedRate {
		t.Errorf(
			"expected interest rate %.2f%%, got %.2f%%",
			expectedRate,
			datum.InterestRatePercent(),
		)
	}
}

func TestOptimParserParseBondDatum(t *testing.T) {
	// Build a bond datum
	bondNFT := make([]byte, 32)
	for i := range bondNFT {
		bondNFT[i] = byte(i)
	}

	pubKeyHash := make([]byte, 28)
	for i := range pubKeyHash {
		pubKeyHash[i] = byte(i + 0x10)
	}
	paymentCred := cbor.NewConstructor(0, cbor.IndefLengthList{
		pubKeyHash,
	})
	stakeCred := cbor.NewConstructor(1, cbor.IndefLengthList{})
	address := cbor.NewConstructor(0, cbor.IndefLengthList{
		paymentCred,
		stakeCred,
	})

	borrowerNFT := make([]byte, 32)
	stakePool := make([]byte, 28)

	bondDatum := cbor.NewConstructor(0, cbor.IndefLengthList{
		bondNFT,
		address,
		borrowerNFT,
		uint64(5000000000), // 5000 ADA
		uint64(300),        // 3%
		uint64(20),         // 20 epochs
		uint64(450),
		uint64(470),
		stakePool,
		uint64(100000000), // 100 ADA rewards
		uint64(1),         // matured
	})

	cborData, err := cbor.Encode(&bondDatum)
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	parser := NewOptimParser()
	state, err := parser.ParseBondDatum(
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
	if state.Principal != 5000000000 {
		t.Errorf("expected principal 5000000000, got %d", state.Principal)
	}
	if state.InterestRate != 300 {
		t.Errorf("expected interest rate 300, got %d", state.InterestRate)
	}
	if state.Status != 1 {
		t.Errorf("expected status 1, got %d", state.Status)
	}
	if !state.IsMatured {
		t.Error("expected IsMatured to be true")
	}
	if state.Slot != 12345 {
		t.Errorf("expected slot 12345, got %d", state.Slot)
	}
	if state.Protocol != "optim" {
		t.Errorf("expected protocol 'optim', got %s", state.Protocol)
	}
	if state.Lender != hex.EncodeToString(pubKeyHash) {
		t.Errorf("expected lender %s, got %s",
			hex.EncodeToString(pubKeyHash), state.Lender)
	}
	if !state.LenderIsKey {
		t.Error("expected LenderIsKey to be true")
	}

	// Test helper methods
	if state.PrincipalADA() != 5000.0 {
		t.Errorf("expected 5000 ADA, got %f", state.PrincipalADA())
	}
	if state.RewardsADA() != 100.0 {
		t.Errorf("expected 100 ADA rewards, got %f", state.RewardsADA())
	}
	if state.TotalValueADA() != 5100.0 {
		t.Errorf("expected 5100 ADA total, got %f", state.TotalValueADA())
	}
}

func TestOptimBondStateKey(t *testing.T) {
	state := &OptimBondState{
		BondId: "optim_bond_abc123",
	}

	expected := "optim:optim_bond_abc123"
	if state.Key() != expected {
		t.Errorf("expected key %s, got %s", expected, state.Key())
	}
}

func TestOptimBondStateStatusString(t *testing.T) {
	tests := []struct {
		status   uint64
		expected string
	}{
		{0, "active"},
		{1, "matured"},
		{2, "claimed"},
		{99, "unknown(99)"},
	}

	for _, test := range tests {
		state := &OptimBondState{Status: test.status}
		if state.StatusString() != test.expected {
			t.Errorf(
				"status %d: expected %s, got %s",
				test.status,
				test.expected,
				state.StatusString(),
			)
		}
	}
}

func TestOptimOADADatumFull(t *testing.T) {
	// Build exchange rate rational
	exchangeRate := cbor.NewConstructor(0, cbor.IndefLengthList{
		uint64(105), // 1.05 exchange rate
		uint64(100),
	})

	// OADADatum
	oadaDatum := cbor.NewConstructor(0, cbor.IndefLengthList{
		uint64(1000000000000), // 1M ADA staked
		exchangeRate,
		uint64(450),          // last update epoch
		uint64(950000000000), // ~950k OADA minted
	})

	cborData, err := cbor.Encode(&oadaDatum)
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	var datum OptimOADADatum
	if _, err := cbor.Decode(cborData, &datum); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	if datum.TotalStaked != 1000000000000 {
		t.Errorf(
			"expected total staked 1000000000000, got %d",
			datum.TotalStaked,
		)
	}
	if datum.TotalOADA != 950000000000 {
		t.Errorf("expected total OADA 950000000000, got %d", datum.TotalOADA)
	}
	if datum.LastUpdateEpoch != 450 {
		t.Errorf(
			"expected last update epoch 450, got %d",
			datum.LastUpdateEpoch,
		)
	}

	expectedRate := 1.05
	if datum.ExchangeRateFloat() != expectedRate {
		t.Errorf(
			"expected exchange rate %f, got %f",
			expectedRate,
			datum.ExchangeRateFloat(),
		)
	}
}

func TestOptimParserParseOADADatum(t *testing.T) {
	exchangeRate := cbor.NewConstructor(0, cbor.IndefLengthList{
		uint64(110),
		uint64(100),
	})

	oadaDatum := cbor.NewConstructor(0, cbor.IndefLengthList{
		uint64(500000000000), // 500k ADA
		exchangeRate,
		uint64(460),
		uint64(450000000000), // 450k OADA
	})

	cborData, err := cbor.Encode(&oadaDatum)
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	parser := NewOptimParser()
	state, err := parser.ParseOADADatum(
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
	if state.TotalStaked != 500000000000 {
		t.Errorf(
			"expected total staked 500000000000, got %d",
			state.TotalStaked,
		)
	}
	if state.TotalOADA != 450000000000 {
		t.Errorf("expected total OADA 450000000000, got %d", state.TotalOADA)
	}
	if state.ExchangeRate != 1.1 {
		t.Errorf("expected exchange rate 1.1, got %f", state.ExchangeRate)
	}
	if state.Slot != 54321 {
		t.Errorf("expected slot 54321, got %d", state.Slot)
	}
}

func TestOptimOADAStateKey(t *testing.T) {
	state := &OptimOADAState{}
	expected := "optim:oada"
	if state.Key() != expected {
		t.Errorf("expected key %s, got %s", expected, state.Key())
	}
}

func TestOptimOADAStateTotalStakedADA(t *testing.T) {
	state := &OptimOADAState{
		TotalStaked: 1000000000000, // 1M lovelace = 1M ADA... wait no
	}

	// 1000000000000 lovelace = 1000000 ADA
	expected := 1000000.0
	if state.TotalStakedADA() != expected {
		t.Errorf("expected %f ADA, got %f", expected, state.TotalStakedADA())
	}
}

func TestOptimParserPoolDatum(t *testing.T) {
	// Optim is not an AMM, so ParsePoolDatum should return nil
	parser := NewOptimParser()

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
		t.Error("expected nil pool state for Optim")
	}
}

func TestGetOptimAddresses(t *testing.T) {
	addresses := GetOptimAddresses()
	// Currently returns empty slice (placeholder)
	if addresses == nil {
		t.Error("expected non-nil addresses slice")
	}
}

func TestOptimBondStateString(t *testing.T) {
	state := &OptimBondState{
		BondId:       "optim_bond_0102030405060708",
		Status:       0,
		Principal:    1000000000,
		InterestRate: 500,
		Rewards:      50000000,
	}

	str := state.String()
	if str == "" {
		t.Error("expected non-empty string")
	}
	// Should contain key info
	if len(str) < 20 {
		t.Error("expected more detailed string representation")
	}
}
