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

// Package minswap provides datum types and parsing for Minswap DEX protocol.
package minswap

import (
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/shai/internal/common"
)

// Protocol constants
const (
	ProtocolName = "minswap"

	// V1 Constants (mainnet)
	V1PoolScriptHash = "e628bfd41e731f83cea4fd5f97d899bd044f8be4a60afcf7fc6b0c8"
	V1FactoryPolicy  = "13aa2accf2e1561723aa26871e071fdf32c867cff7e7d50ad470d62f"
	V1LPPolicy       = "e4214b7cce62ac6fbba385d164df48e157eae5863521b4b67ca71d86"
	V1PoolNFTPolicy  = "0be55d262b29f564998ff81efe21bdc0022621c12f15af08d0f2ddb1"

	// V2 Constants (mainnet)
	V2PoolScriptHash = "ea07b733d932129c378af627436e7cbc2ef0bf96e0036bb51b3bde6b"
	V2FactoryPolicy  = "7bc5fbd41a95f561be84369631e0e35895efb0b73e0a7480bb9ed730"
	V2LPPolicy       = "f5808c2c990d86da54bfc97d89cee6efa20cd8461616359478d96b4c"

	// Fee denominator for Minswap
	FeeDenom = 10000
)

// V1PoolDatum represents the Minswap V1 pool datum structure
// Constructor 0 with fields: assetA, assetB, totalLiquidity, rootKLast, feeSharing
type V1PoolDatum struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	AssetA         Asset
	AssetB         Asset
	TotalLiquidity uint64
	RootKLast      uint64
	FeeSharing     FeeSharing
}

func (d *V1PoolDatum) UnmarshalCBOR(cborData []byte) error {
	d.SetCbor(cborData)
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
	return cbor.DecodeGeneric(tmpConstr.FieldsCbor(), d)
}

// V2PoolDatum represents the Minswap V2 pool datum structure
// Constructor 0 with fields: stakeCredential, assetA, assetB, totalLiquidity,
// reserveA, reserveB, baseFee, feeSharingNumerator, allowDynamicFee
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
	d.SetCbor(cborData)
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
	return cbor.DecodeGeneric(tmpConstr.FieldsCbor(), d)
}

// Bool represents a Plutus boolean (Constructor 0 = true, Constructor 1 = false)
type Bool bool

func (b *Bool) UnmarshalCBOR(cborData []byte) error {
	var tmpConstr cbor.Constructor
	if _, err := cbor.Decode(cborData, &tmpConstr); err != nil {
		return err
	}
	// Constructor 0 = True, Constructor 1 = False
	switch tmpConstr.Constructor() {
	case 0:
		*b = true
	case 1:
		*b = false
	default:
		return fmt.Errorf(
			"invalid Bool constructor: expected 0 or 1, got %d",
			tmpConstr.Constructor(),
		)
	}
	return nil
}

// Asset represents an asset in Minswap datum
// Constructor 0 with fields: policyId, assetName
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
	return cbor.DecodeGeneric(tmpConstr.FieldsCbor(), a)
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
	c.Type = int(tmpConstr.Constructor())
	var wrapper struct {
		cbor.StructAsArray
		Hash []byte
	}
	if err := cbor.DecodeGeneric(tmpConstr.FieldsCbor(), &wrapper); err != nil {
		return err
	}
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
	return cbor.DecodeGeneric(tmpConstr.FieldsCbor(), f)
}

// FeeSharing represents optional fee sharing in V1
type FeeSharing struct {
	IsPresent      bool
	FeeTo          []byte
	FeeToDatumHash []byte
}

func (f *FeeSharing) UnmarshalCBOR(cborData []byte) error {
	var tmpConstr cbor.Constructor
	if _, err := cbor.Decode(cborData, &tmpConstr); err != nil {
		return err
	}
	// Constructor 0 = Some, Constructor 1 = None
	if tmpConstr.Constructor() == 1 {
		f.IsPresent = false
		return nil
	}
	f.IsPresent = true
	// Decode fields into wrapper struct to avoid including IsPresent
	var wrapper struct {
		cbor.StructAsArray
		FeeTo          []byte
		FeeToDatumHash []byte
	}
	if err := cbor.DecodeGeneric(tmpConstr.FieldsCbor(), &wrapper); err != nil {
		return err
	}
	f.FeeTo = wrapper.FeeTo
	f.FeeToDatumHash = wrapper.FeeToDatumHash
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
		if err := cbor.DecodeGeneric(tmpConstr.FieldsCbor(), &wrapper); err != nil {
			return err
		}
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
