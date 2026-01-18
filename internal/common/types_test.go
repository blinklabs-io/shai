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

package common

import (
	"testing"
)

func TestAssetClassIsLovelace(t *testing.T) {
	// Test empty asset class (ADA/lovelace)
	lovelace := AssetClass{}
	if !lovelace.IsLovelace() {
		t.Error("empty AssetClass should be lovelace")
	}

	// Test non-empty asset class
	token := AssetClass{
		PolicyId: []byte{0x01, 0x02, 0x03},
		Name:     []byte("TOKEN"),
	}
	if token.IsLovelace() {
		t.Error("non-empty AssetClass should not be lovelace")
	}
}

func TestAssetClassFingerprint(t *testing.T) {
	// Test lovelace fingerprint
	lovelace := AssetClass{}
	if lovelace.Fingerprint() != "lovelace" {
		t.Errorf(
			"expected fingerprint 'lovelace', got %s",
			lovelace.Fingerprint(),
		)
	}

	// Test token fingerprint
	token := AssetClass{
		PolicyId: []byte{0xab, 0xcd},
		Name:     []byte{0x12, 0x34},
	}
	expected := "abcd.1234"
	if token.Fingerprint() != expected {
		t.Errorf(
			"expected fingerprint %s, got %s",
			expected,
			token.Fingerprint(),
		)
	}
}

func TestNewAssetClass(t *testing.T) {
	// Test valid hex strings
	ac, err := NewAssetClass("abcd1234", "546f6b656e")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ac.PolicyIdHex() != "abcd1234" {
		t.Errorf("expected policy ID 'abcd1234', got %s", ac.PolicyIdHex())
	}
	if string(ac.Name) != "Token" {
		t.Errorf("expected name 'Token', got %s", string(ac.Name))
	}

	// Test invalid policy ID hex
	_, err = NewAssetClass("invalid", "1234")
	if err == nil {
		t.Error("expected error for invalid policy ID hex")
	}

	// Test invalid asset name hex
	_, err = NewAssetClass("abcd", "invalid")
	if err == nil {
		t.Error("expected error for invalid asset name hex")
	}
}

func TestAssetAmountIsAsset(t *testing.T) {
	token := AssetClass{
		PolicyId: []byte{0x01, 0x02},
		Name:     []byte("TEST"),
	}

	amount := AssetAmount{
		Class:  token,
		Amount: 1000,
	}

	// Test matching asset
	if !amount.IsAsset(token) {
		t.Error("expected IsAsset to return true for matching asset")
	}

	// Test non-matching asset
	other := AssetClass{
		PolicyId: []byte{0x03, 0x04},
		Name:     []byte("OTHER"),
	}
	if amount.IsAsset(other) {
		t.Error("expected IsAsset to return false for non-matching asset")
	}
}

func TestLovelace(t *testing.T) {
	l := Lovelace()
	if !l.IsLovelace() {
		t.Error("Lovelace() should return an asset class that IsLovelace()")
	}
}
