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

package wingriders

import (
	"testing"
)

func TestNewV2Parser(t *testing.T) {
	parser := NewV2Parser()
	if parser == nil {
		t.Fatal("expected non-nil parser")
	}
	if parser.Protocol() != "wingriders-v2" {
		t.Errorf("expected protocol 'wingriders-v2', got %s", parser.Protocol())
	}
}

func TestParser_Protocol(t *testing.T) {
	parser := NewV2Parser()
	expected := "wingriders-v2"
	if parser.Protocol() != expected {
		t.Errorf("expected protocol %s, got %s", expected, parser.Protocol())
	}
}

func TestGeneratePoolId(t *testing.T) {
	poolId := GeneratePoolId(
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

func TestAssetClass_ToCommonAssetClass(t *testing.T) {
	asset := AssetClass{
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

func TestAssetClass_IsLovelace(t *testing.T) {
	ada := AssetClass{
		PolicyId: []byte{},
		Name:     []byte{},
	}
	if !ada.IsLovelace() {
		t.Error("expected ADA asset to be lovelace")
	}

	token := AssetClass{
		PolicyId: []byte{0x01},
		Name:     []byte("TOKEN"),
	}
	if token.IsLovelace() {
		t.Error("expected token asset not to be lovelace")
	}
}