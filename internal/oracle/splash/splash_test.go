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

package splash

import (
	"testing"

	"github.com/blinklabs-io/shai/internal/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewV1Parser(t *testing.T) {
	parser := NewV1Parser()
	require.NotNil(t, parser, "NewV1Parser returned nil")
}

func TestParser_Protocol(t *testing.T) {
	parser := NewV1Parser()
	assert.Equal(t, "splash-v1", parser.Protocol())
}

func TestGeneratePoolId(t *testing.T) {
	policyA := []byte{0x01, 0x02, 0x03}
	nameA := []byte{0x04, 0x05}
	policyB := []byte{0x06, 0x07, 0x08}
	nameB := []byte{0x09, 0x0a}

	result := GeneratePoolId(policyA, nameA, policyB, nameB)
	assert.Equal(t, "010203.0405-060708.090a", result)
}

func TestAssetClass_ToCommonAssetClass(t *testing.T) {
	asset := AssetClass{
		PolicyId: []byte{0x01, 0x02, 0x03},
		Name:     []byte{0x04, 0x05},
	}

	expected := common.AssetClass{
		PolicyId: []byte{0x01, 0x02, 0x03},
		Name:     []byte{0x04, 0x05},
	}

	result := asset.ToCommonAssetClass()
	assert.Equal(t, expected.PolicyId, result.PolicyId)
	assert.Equal(t, expected.Name, result.Name)
}

func TestAssetClass_IsLovelace(t *testing.T) {
	// Test ADA (empty policy and name)
	ada := AssetClass{PolicyId: []byte{}, Name: []byte{}}
	assert.True(t, ada.IsLovelace())

	// Test non-ADA asset
	asset := AssetClass{
		PolicyId: []byte{0x01},
		Name:     []byte{0x02},
	}
	assert.False(t, asset.IsLovelace())
}
