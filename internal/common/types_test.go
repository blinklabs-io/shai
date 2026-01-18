// Copyright 2025 Blink Labs Software
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

package common_test

import (
	"testing"

	"github.com/blinklabs-io/shai/internal/common"
)

func TestAssetClassIsLovelace(t *testing.T) {
	// Empty AssetClass should be lovelace
	emptyAsset := common.AssetClass{}
	if !emptyAsset.IsLovelace() {
		t.Errorf("empty AssetClass should be lovelace")
	}

	// Non-empty AssetClass should not be lovelace
	nonEmptyAsset := common.AssetClass{
		PolicyId: []byte{0x01, 0x02, 0x03},
		Name:     []byte{0x04, 0x05},
	}
	if nonEmptyAsset.IsLovelace() {
		t.Errorf("non-empty AssetClass should not be lovelace")
	}

	// PolicyId only should not be lovelace
	policyOnlyAsset := common.AssetClass{
		PolicyId: []byte{0x01, 0x02, 0x03},
	}
	if policyOnlyAsset.IsLovelace() {
		t.Errorf("AssetClass with only PolicyId should not be lovelace")
	}
}

func TestAssetClassFingerprint(t *testing.T) {
	// Lovelace returns "lovelace"
	lovelace := common.AssetClass{}
	if lovelace.Fingerprint() != "lovelace" {
		t.Errorf(
			"lovelace Fingerprint() should return 'lovelace', got %s",
			lovelace.Fingerprint(),
		)
	}

	// Tokens return "policyId.assetName" in hex
	token := common.AssetClass{
		PolicyId: []byte{0xab, 0xcd, 0xef},
		Name:     []byte{0x12, 0x34},
	}
	expected := "abcdef.1234"
	if token.Fingerprint() != expected {
		t.Errorf(
			"token Fingerprint() should return '%s', got %s",
			expected,
			token.Fingerprint(),
		)
	}

	// Token with empty name
	tokenEmptyName := common.AssetClass{
		PolicyId: []byte{0xab, 0xcd, 0xef},
		Name:     []byte{},
	}
	expectedEmptyName := "abcdef."
	if tokenEmptyName.Fingerprint() != expectedEmptyName {
		t.Errorf(
			"token with empty name Fingerprint() should return '%s', got %s",
			expectedEmptyName,
			tokenEmptyName.Fingerprint(),
		)
	}
}

func TestNewAssetClass(t *testing.T) {
	// Valid hex creates AssetClass
	policyId := "abcdef0123456789"
	name := "1234"
	asset, err := common.NewAssetClass(policyId, name)
	if err != nil {
		t.Errorf(
			"NewAssetClass with valid hex should not return error: %v",
			err,
		)
	}
	if asset.PolicyIdHex() != policyId {
		t.Errorf(
			"PolicyIdHex() should return '%s', got '%s'",
			policyId,
			asset.PolicyIdHex(),
		)
	}
	if asset.NameHex() != name {
		t.Errorf(
			"NameHex() should return '%s', got '%s'",
			name,
			asset.NameHex(),
		)
	}

	// Invalid policy ID hex returns error
	_, err = common.NewAssetClass("invalid", "1234")
	if err == nil {
		t.Errorf("NewAssetClass with invalid policyId hex should return error")
	}

	// Invalid name hex returns error
	_, err = common.NewAssetClass("abcdef", "invalid")
	if err == nil {
		t.Errorf("NewAssetClass with invalid name hex should return error")
	}

	// Empty strings create lovelace
	lovelace, err := common.NewAssetClass("", "")
	if err != nil {
		t.Errorf(
			"NewAssetClass with empty strings should not return error: %v",
			err,
		)
	}
	if !lovelace.IsLovelace() {
		t.Errorf(
			"NewAssetClass with empty strings should create lovelace asset",
		)
	}
}

func TestAssetAmountIsAsset(t *testing.T) {
	asset1 := common.AssetClass{
		PolicyId: []byte{0x01, 0x02, 0x03},
		Name:     []byte{0x04, 0x05},
	}
	asset2 := common.AssetClass{
		PolicyId: []byte{0x01, 0x02, 0x03},
		Name:     []byte{0x04, 0x05},
	}
	asset3 := common.AssetClass{
		PolicyId: []byte{0xaa, 0xbb, 0xcc},
		Name:     []byte{0xdd, 0xee},
	}

	amount := common.AssetAmount{
		Class:  asset1,
		Amount: 100,
	}

	// Matching asset returns true
	if !amount.IsAsset(asset2) {
		t.Errorf("IsAsset should return true for matching asset")
	}

	// Non-matching asset returns false
	if amount.IsAsset(asset3) {
		t.Errorf("IsAsset should return false for non-matching asset")
	}
}

func TestLovelace(t *testing.T) {
	lovelace := common.Lovelace()
	if !lovelace.IsLovelace() {
		t.Errorf("Lovelace() should return asset class that IsLovelace()")
	}
}
