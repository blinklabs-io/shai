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

// Package sundaeswap provides datum types and parsing for SundaeSwap V3 DEX protocol.
package sundaeswap

import (
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/shai/internal/common"
)

// Protocol constants
const (
	ProtocolName = "sundaeswap"
	FeeDenom     = 10000
)

// V3PoolDatum represents the SundaeSwap V3 pool datum structure
type V3PoolDatum struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	Identifier           []byte
	Assets               Assets
	CirculatingLp        uint64
	BidFeesPer10Thousand Fees
	AskFeesPer10Thousand Fees
	FeeManager           OptionalMultisigScript
	MarketOpen           uint64
	FeeFinalized         uint64
	ProtocolFees         uint64
}

func (d *V3PoolDatum) UnmarshalCBOR(cborData []byte) error {
	type tV3PoolDatum V3PoolDatum
	var tmp tV3PoolDatum
	if _, err := cbor.Decode(cborData, &tmp); err != nil {
		return err
	}
	*d = V3PoolDatum(tmp)
	d.SetCbor(cborData)
	return nil
}

// Assets represents the asset pair in SundaeSwap datum
type Assets struct {
	cbor.StructAsArray
	AssetA AssetClass
	AssetB AssetClass
}

func (a *Assets) UnmarshalCBOR(cborData []byte) error {
	type tAssets Assets
	var tmp tAssets
	if _, err := cbor.Decode(cborData, &tmp); err != nil {
		return err
	}
	*a = Assets(tmp)
	return nil
}

// AssetClass represents an asset class (policy ID + asset name)
type AssetClass struct {
	cbor.StructAsArray
	PolicyId  []byte
	AssetName []byte
}

func (a *AssetClass) UnmarshalCBOR(cborData []byte) error {
	type tAssetClass AssetClass
	var tmp tAssetClass
	if _, err := cbor.Decode(cborData, &tmp); err != nil {
		return err
	}
	*a = AssetClass(tmp)
	return nil
}

// ToCommonAssetClass converts to common.AssetClass
func (a AssetClass) ToCommonAssetClass() common.AssetClass {
	return common.AssetClass{
		PolicyId: a.PolicyId,
		Name:     a.AssetName,
	}
}

// Fees represents fee structure (numerator, denominator)
type Fees struct {
	cbor.StructAsArray
	Numerator   uint64
	Denominator uint64
}

func (f *Fees) UnmarshalCBOR(cborData []byte) error {
	type tFees Fees
	var tmp tFees
	if _, err := cbor.Decode(cborData, &tmp); err != nil {
		return err
	}
	*f = Fees(tmp)
	return nil
}

// OptionalMultisigScript represents an optional multisig script
type OptionalMultisigScript struct {
	IsPresent bool
	Value     MultisigScript
}

func (o *OptionalMultisigScript) UnmarshalCBOR(cborData []byte) error {
	var tmpConstr cbor.Constructor
	if _, err := cbor.Decode(cborData, &tmpConstr); err != nil {
		return err
	}
	switch tmpConstr.Constructor() {
	case 0:
		o.IsPresent = true
		// The field is a nested Constructor (MultisigScript)
		// Decode the fields array to get the inner script's CBOR
		var fields struct {
			cbor.StructAsArray
			Script cbor.RawMessage
		}
		type tFields struct {
			cbor.StructAsArray
			Script cbor.RawMessage
		}
		var tmp tFields
		if _, err := cbor.Decode(tmpConstr.FieldsCbor(), &tmp); err != nil {
			return fmt.Errorf("failed to decode Optional Some fields: %w", err)
		}
		fields = struct {
			cbor.StructAsArray
			Script cbor.RawMessage
		}(tmp)
		// Now decode the script
		if _, err := cbor.Decode(fields.Script, &o.Value); err != nil {
			return fmt.Errorf("failed to decode MultisigScript: %w", err)
		}
	case 1:
		o.IsPresent = false
		o.Value = MultisigScript{}
	default:
		return fmt.Errorf(
			"invalid OptionalMultisigScript constructor: expected 0 or 1, got %d",
			tmpConstr.Constructor(),
		)
	}
	return nil
}

// MultisigScript represents a Cardano native script.
// Native scripts can be one of:
//   - Constructor 0: Signature (single pubkey hash)
//   - Constructor 1: AllOf (all child scripts must validate)
//   - Constructor 2: AnyOf (any child script must validate)
//   - Constructor 3: AtLeast (k of n child scripts must validate)
//   - Constructor 4: TimeBefore (valid before slot)
//   - Constructor 5: TimeAfter (valid after slot)
//
// For SundaeSwap pool fee managers, we only need to parse the structure
// without fully interpreting it. The Signature field is populated only
// for constructor 0; other script types store their raw CBOR.
type MultisigScript struct {
	Constructor uint
	Signature   []byte // Only set for constructor 0 (Signature)
	RawCbor     []byte // Raw CBOR for non-signature types
}

func (m *MultisigScript) UnmarshalCBOR(cborData []byte) error {
	var tmpConstr cbor.Constructor
	if _, err := cbor.Decode(cborData, &tmpConstr); err != nil {
		return err
	}

	m.Constructor = tmpConstr.Constructor()
	m.RawCbor = cborData

	switch tmpConstr.Constructor() {
	case 0: // Signature - extract the pubkey hash
		// The fields are encoded as a CBOR array containing the pubkey hash
		var fields struct {
			cbor.StructAsArray
			PubKeyHash []byte
		}
		type tFields struct {
			cbor.StructAsArray
			PubKeyHash []byte
		}
		var tmp tFields
		if _, err := cbor.Decode(tmpConstr.FieldsCbor(), &tmp); err != nil {
			return fmt.Errorf(
				"failed to decode Signature script fields: %w",
				err,
			)
		}
		fields = struct {
			cbor.StructAsArray
			PubKeyHash []byte
		}(tmp)
		m.Signature = fields.PubKeyHash
	case 1, 2, 3, 4, 5:
		// AllOf, AnyOf, AtLeast, TimeBefore, TimeAfter
		// We don't need to interpret these for pool parsing,
		// just store the raw CBOR
	default:
		return fmt.Errorf(
			"unknown MultisigScript constructor: %d (expected 0-5)",
			tmpConstr.Constructor(),
		)
	}
	return nil
}
