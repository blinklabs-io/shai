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

// Package minswap provides datum types and parsing for Minswap V2 DEX protocol.
package minswap

import (
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/shai/internal/common"
)

// Protocol constants
const (
	ProtocolName = "minswap"
	FeeDenom     = 10000
)

// V2PoolDatum represents the Minswap V2 pool datum structure
type V2PoolDatum struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	StakeCredential     Credential
	AssetA              Asset
	AssetB              Asset
	TotalLiquidity      uint64
	ReserveA            uint64
	ReserveB            uint64
	BaseFee             BaseFee
	FeeSharingNumerator OptionalUint64
	AllowDynamicFee     Bool
}

func (d *V2PoolDatum) UnmarshalCBOR(cborData []byte) error {
	var tmpConstr cbor.Constructor
	if _, err := cbor.Decode(cborData, &tmpConstr); err != nil {
		return err
	}
	if tmpConstr.Constructor() != 0 {
		return fmt.Errorf(
			"expected constructor 0, got %d",
			tmpConstr.Constructor(),
		)
	}

	type tV2PoolDatum V2PoolDatum
	var tmp tV2PoolDatum
	if _, err := cbor.Decode(tmpConstr.FieldsCbor(), &tmp); err != nil {
		return err
	}
	*d = V2PoolDatum(tmp)
	d.SetCbor(cborData)
	return nil
}

// Bool represents a Plutus boolean (Constructor 0 = false, Constructor 1 = true)
type Bool bool

func (b *Bool) UnmarshalCBOR(cborData []byte) error {
	var tmpConstr cbor.Constructor
	if _, err := cbor.Decode(cborData, &tmpConstr); err != nil {
		return err
	}
	switch tmpConstr.Constructor() {
	case 0:
		*b = false
	case 1:
		*b = true
	default:
		return fmt.Errorf(
			"invalid Bool constructor: expected 0 or 1, got %d",
			tmpConstr.Constructor(),
		)
	}
	return nil
}

// Asset represents an asset in Minswap datum
type Asset struct {
	cbor.StructAsArray
	PolicyId  []byte
	AssetName []byte
}

func (a *Asset) UnmarshalCBOR(cborData []byte) error {
	var tmpConstr cbor.Constructor
	if _, err := cbor.Decode(cborData, &tmpConstr); err != nil {
		return err
	}

	type tAsset Asset
	var tmp tAsset
	if _, err := cbor.Decode(tmpConstr.FieldsCbor(), &tmp); err != nil {
		return err
	}
	*a = Asset(tmp)
	return nil
}

// ToCommonAssetClass converts to common.AssetClass
func (a Asset) ToCommonAssetClass() common.AssetClass {
	return common.AssetClass{
		PolicyId: a.PolicyId,
		Name:     a.AssetName,
	}
}

// Credential represents a credential in Minswap datum
type Credential struct {
	cbor.StructAsArray
	Type int // 0 = PubKeyHash, 1 = ScriptHash
	Hash []byte
}

func (c *Credential) UnmarshalCBOR(cborData []byte) error {
	var tmpConstr cbor.Constructor
	if _, err := cbor.Decode(cborData, &tmpConstr); err != nil {
		return err
	}
	constr := tmpConstr.Constructor()
	if constr != 0 && constr != 1 {
		return fmt.Errorf(
			"invalid Credential constructor: expected 0 or 1, got %d",
			constr,
		)
	}
	c.Type = int(constr)
	var wrapper struct {
		cbor.StructAsArray
		Hash []byte
	}
	type tWrapper struct {
		cbor.StructAsArray
		Hash []byte
	}
	var tmp tWrapper
	if _, err := cbor.Decode(tmpConstr.FieldsCbor(), &tmp); err != nil {
		return err
	}
	wrapper = struct {
		cbor.StructAsArray
		Hash []byte
	}(tmp)
	c.Hash = wrapper.Hash
	return nil
}

// BaseFee represents the base fee structure
type BaseFee struct {
	cbor.StructAsArray
	FeeANumerator uint64
	FeeBNumerator uint64
}

func (f *BaseFee) UnmarshalCBOR(cborData []byte) error {
	var tmpConstr cbor.Constructor
	if _, err := cbor.Decode(cborData, &tmpConstr); err != nil {
		return err
	}

	type tBaseFee BaseFee
	var tmp tBaseFee
	if _, err := cbor.Decode(tmpConstr.FieldsCbor(), &tmp); err != nil {
		return err
	}
	*f = BaseFee(tmp)
	return nil
}

// OptionalUint64 represents an optional uint64 value
type OptionalUint64 struct {
	IsPresent bool
	Value     uint64
}

func (o *OptionalUint64) UnmarshalCBOR(cborData []byte) error {
	var tmpConstr cbor.Constructor
	if _, err := cbor.Decode(cborData, &tmpConstr); err != nil {
		return err
	}
	switch tmpConstr.Constructor() {
	case 0:
		o.IsPresent = true
		var wrapper struct {
			cbor.StructAsArray
			Value uint64
		}
		type tWrapper struct {
			cbor.StructAsArray
			Value uint64
		}
		var tmp tWrapper
		if _, err := cbor.Decode(tmpConstr.FieldsCbor(), &tmp); err != nil {
			return err
		}
		wrapper = struct {
			cbor.StructAsArray
			Value uint64
		}(tmp)
		o.Value = wrapper.Value
	case 1:
		o.IsPresent = false
		o.Value = 0
	default:
		return fmt.Errorf(
			"invalid OptionalUint64 constructor: expected 0 or 1, got %d",
			tmpConstr.Constructor(),
		)
	}
	return nil
}
