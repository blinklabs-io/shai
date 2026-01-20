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

package sundaeswap

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/shai/internal/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewV3Parser(t *testing.T) {
	parser := NewV3Parser()
	require.NotNil(t, parser, "NewV3Parser returned nil")
}

func TestParser_Protocol(t *testing.T) {
	parser := NewV3Parser()
	assert.Equal(t, "sundaeswap-v3", parser.Protocol())
}

func TestGeneratePoolId(t *testing.T) {
	policyA := []byte{0x01, 0x02, 0x03}
	nameA := []byte{0x04, 0x05}
	policyB := []byte{0x06, 0x07, 0x08}
	nameB := []byte{0x09, 0x0a}

	result := GeneratePoolId(policyA, nameA, policyB, nameB)
	assert.Equal(t, "sundaeswap_010203.0405_060708.090a", result)
}

func TestAssetClass_ToCommonAssetClass(t *testing.T) {
	asset := AssetClass{
		PolicyId:  []byte{0x01, 0x02, 0x03},
		AssetName: []byte{0x04, 0x05},
	}

	expected := common.AssetClass{
		PolicyId: []byte{0x01, 0x02, 0x03},
		Name:     []byte{0x04, 0x05},
	}

	result := asset.ToCommonAssetClass()
	assert.Equal(t, expected.PolicyId, result.PolicyId)
	assert.Equal(t, expected.Name, result.Name)
}

func TestFeesUnmarshal(t *testing.T) {
	// Fees are encoded as a CBOR array [numerator, denominator]
	feesData := cbor.IndefLengthList{uint64(30), uint64(10000)}
	cborData, err := cbor.Encode(&feesData)
	require.NoError(t, err, "failed to encode fees")

	var fees Fees
	_, err = cbor.Decode(cborData, &fees)
	require.NoError(t, err, "failed to decode fees")

	assert.Equal(t, uint64(30), fees.Numerator)
	assert.Equal(t, uint64(10000), fees.Denominator)
}

func TestOptionalMultisigScriptNone(t *testing.T) {
	// None case: Constructor 1 with no fields
	noneConstr := cbor.NewConstructor(1, cbor.IndefLengthList{})
	cborData, err := cbor.Encode(&noneConstr)
	require.NoError(t, err, "failed to encode None")

	var opt OptionalMultisigScript
	_, err = cbor.Decode(cborData, &opt)
	require.NoError(t, err, "failed to decode None")

	assert.False(t, opt.IsPresent, "expected IsPresent to be false for None")
}

func TestOptionalMultisigScriptSome(t *testing.T) {
	// Some case: Constructor 0 with a MultisigScript (Signature variant)
	// MultisigScript Signature is Constructor 0 with a single field (pubkey hash)
	signatureBytes := []byte{0x01, 0x02, 0x03}
	// Signature script: Constructor 0 with [pubkey_hash]
	signatureScript := cbor.NewConstructor(
		0,
		cbor.IndefLengthList{signatureBytes},
	)
	// Optional Some: Constructor 0 wrapping the script
	someConstr := cbor.NewConstructor(0, cbor.IndefLengthList{signatureScript})
	cborData, err := cbor.Encode(&someConstr)
	require.NoError(t, err, "failed to encode Some")

	var opt OptionalMultisigScript
	_, err = cbor.Decode(cborData, &opt)
	require.NoError(t, err, "failed to decode Some")

	assert.True(t, opt.IsPresent, "expected IsPresent to be true for Some")
	assert.Equal(t, uint(0), opt.Value.Constructor, "expected Signature constructor (0)")
	assert.Equal(t, signatureBytes, opt.Value.Signature, "expected matching signature bytes")
}

func TestMultisigScriptAllConstructors(t *testing.T) {
	tests := []struct {
		name        string
		constructor uint
		fields      cbor.IndefLengthList
		wantErr     bool
	}{
		{
			name:        "Signature",
			constructor: 0,
			fields:      cbor.IndefLengthList{[]byte{0xab, 0xcd}},
			wantErr:     false,
		},
		{
			name:        "AllOf",
			constructor: 1,
			fields:      cbor.IndefLengthList{cbor.IndefLengthList{}}, // empty list of scripts
			wantErr:     false,
		},
		{
			name:        "AnyOf",
			constructor: 2,
			fields:      cbor.IndefLengthList{cbor.IndefLengthList{}},
			wantErr:     false,
		},
		{
			name:        "AtLeast",
			constructor: 3,
			fields:      cbor.IndefLengthList{uint64(1), cbor.IndefLengthList{}},
			wantErr:     false,
		},
		{
			name:        "TimeBefore",
			constructor: 4,
			fields:      cbor.IndefLengthList{uint64(12345)},
			wantErr:     false,
		},
		{
			name:        "TimeAfter",
			constructor: 5,
			fields:      cbor.IndefLengthList{uint64(12345)},
			wantErr:     false,
		},
		{
			name:        "Unknown",
			constructor: 99,
			fields:      cbor.IndefLengthList{},
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			constr := cbor.NewConstructor(tt.constructor, tt.fields)
			cborData, err := cbor.Encode(&constr)
			require.NoError(t, err)

			var ms MultisigScript
			_, err = cbor.Decode(cborData, &ms)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.constructor, ms.Constructor)
			}
		})
	}
}

func TestV3PoolDatumUnmarshal(t *testing.T) {
	// Build a V3PoolDatum as a CBOR array
	// Fields: Identifier, Assets, CirculatingLp, BidFees, AskFees, FeeManager, MarketOpen, FeeFinalized, ProtocolFees

	// Asset A (ADA - empty policy and name)
	assetA := cbor.IndefLengthList{[]byte{}, []byte{}}

	// Asset B (some token)
	assetB := cbor.IndefLengthList{
		[]byte{0xab, 0xcd, 0xef},
		[]byte("SUNDAE"),
	}

	// Assets pair
	assets := cbor.IndefLengthList{assetA, assetB}

	// Fees (30 / 10000 = 0.3%)
	bidFees := cbor.IndefLengthList{uint64(30), uint64(10000)}
	askFees := cbor.IndefLengthList{uint64(30), uint64(10000)}

	// FeeManager: None (Constructor 1)
	feeManagerNone := cbor.NewConstructor(1, cbor.IndefLengthList{})

	// Build the full datum as an array
	datum := cbor.IndefLengthList{
		[]byte{0x01, 0x02, 0x03}, // Identifier
		assets,                   // Assets
		uint64(1000000000),       // CirculatingLp
		bidFees,                  // BidFeesPer10Thousand
		askFees,                  // AskFeesPer10Thousand
		feeManagerNone,           // FeeManager
		uint64(0),                // MarketOpen
		uint64(0),                // FeeFinalized
		uint64(0),                // ProtocolFees
	}

	cborData, err := cbor.Encode(&datum)
	require.NoError(t, err, "failed to encode datum")

	var poolDatum V3PoolDatum
	_, err = cbor.Decode(cborData, &poolDatum)
	require.NoError(t, err, "failed to decode datum")

	// Verify fields
	assert.Len(t, poolDatum.Identifier, 3)
	assert.Equal(t, uint64(1000000000), poolDatum.CirculatingLp)
	assert.Equal(t, uint64(30), poolDatum.BidFeesPer10Thousand.Numerator)
	assert.Equal(t, uint64(10000), poolDatum.AskFeesPer10Thousand.Denominator)

	// Verify assets
	assert.Empty(t, poolDatum.Assets.AssetA.PolicyId, "expected AssetA to be ADA")
	assert.Equal(t, "SUNDAE", string(poolDatum.Assets.AssetB.AssetName))
}

func TestGeneratePoolIdWithADA(t *testing.T) {
	// Test pool ID generation with ADA (empty policy/name)
	// Empty bytes encode to empty strings, so format is: sundaeswap_._abcd.544f4b454e
	poolId := GeneratePoolId(
		[]byte{},
		[]byte{},
		[]byte{0xab, 0xcd},
		[]byte("TOKEN"),
	)

	assert.Equal(t, "sundaeswap_._abcd.544f4b454e", poolId)
}
