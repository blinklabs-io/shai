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

// Package wingriders provides datum types and parsing for WingRiders DEX protocol.
package wingriders

import (
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/shai/internal/common"
)

// Protocol constants
const (
	ProtocolName = "wingriders"

	// V2 Constants (mainnet)
	V2PoolScriptHash = "9b248967234e27ab5e1a0aa82ff7f96103a4fc1e07601b9cbe4139d1"
	V2LPTokenPolicy  = "026a18d04a0c642759bb3d83b12e3344894e5c1c7b2aeb1a2113a570"

	// Fee basis (denominator for fee calculations)
	FeeBasis = 10000
)

// V2PoolDatum represents the WingRiders V2 pool datum structure
// Based on CDDL schema from WingRiders datum registry
type V2PoolDatum struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	RequestValidatorHash []byte
	AssetASymbol         []byte
	AssetAToken          []byte
	AssetBSymbol         []byte
	AssetBToken          []byte
	SwapFeeInBasis       uint64
	ProtocolFeeInBasis   uint64
	ProjectFeeInBasis    uint64
	ReserveFeeInBasis    uint64
	FeeBasis             uint64
	AgentFeeAda          uint64
	LastInteraction      int64
	TreasuryA            OptionalInt
	TreasuryB            OptionalInt
	ProjectTreasury      uint64
	ReserveTreasury      uint64
	ProjectBeneficiary   OptionalAddress
	ReserveBeneficiary   OptionalAddress
	PoolVariant          PoolVariant
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

// GetAssetA returns the asset A as a common.AssetClass
func (d *V2PoolDatum) GetAssetA() common.AssetClass {
	return common.AssetClass{
		PolicyId: d.AssetASymbol,
		Name:     d.AssetAToken,
	}
}

// GetAssetB returns the asset B as a common.AssetClass
func (d *V2PoolDatum) GetAssetB() common.AssetClass {
	return common.AssetClass{
		PolicyId: d.AssetBSymbol,
		Name:     d.AssetBToken,
	}
}

// TotalFeeInBasis returns the total fee in basis points
func (d *V2PoolDatum) TotalFeeInBasis() uint64 {
	return d.SwapFeeInBasis + d.ProtocolFeeInBasis +
		d.ProjectFeeInBasis + d.ReserveFeeInBasis
}

// OptionalInt represents an optional integer value (Plutus Maybe Int)
type OptionalInt struct {
	IsPresent bool
	Value     int64
}

func (o *OptionalInt) UnmarshalCBOR(cborData []byte) error {
	var tmpConstr cbor.Constructor
	if _, err := cbor.Decode(cborData, &tmpConstr); err != nil {
		return err
	}
	// Constructor 0 = Just, Constructor 1 = Nothing
	if tmpConstr.Constructor() == 1 {
		o.IsPresent = false
		return nil
	}
	o.IsPresent = true
	var wrapper struct {
		cbor.StructAsArray
		Value int64
	}
	if err := cbor.DecodeGeneric(tmpConstr.FieldsCbor(), &wrapper); err != nil {
		return err
	}
	o.Value = wrapper.Value
	return nil
}

// OptionalAddress represents an optional address (Plutus Maybe Address)
type OptionalAddress struct {
	IsPresent bool
	Address   Address
}

func (o *OptionalAddress) UnmarshalCBOR(cborData []byte) error {
	var tmpConstr cbor.Constructor
	if _, err := cbor.Decode(cborData, &tmpConstr); err != nil {
		return err
	}
	// Constructor 0 = Just, Constructor 1 = Nothing
	if tmpConstr.Constructor() == 1 {
		o.IsPresent = false
		return nil
	}
	o.IsPresent = true
	// The Just constructor contains a single element: the Address
	var wrapper struct {
		cbor.StructAsArray
		Address Address
	}
	if err := cbor.DecodeGeneric(tmpConstr.FieldsCbor(), &wrapper); err != nil {
		return err
	}
	o.Address = wrapper.Address
	return nil
}

// Address represents a Cardano address in datum
type Address struct {
	cbor.StructAsArray
	Credential        Credential
	StakingCredential OptionalStakingCredential
}

func (a *Address) UnmarshalCBOR(cborData []byte) error {
	var tmpConstr cbor.Constructor
	if _, err := cbor.Decode(cborData, &tmpConstr); err != nil {
		return err
	}
	return cbor.DecodeGeneric(tmpConstr.FieldsCbor(), a)
}

// Credential represents a payment credential
type Credential struct {
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

// OptionalStakingCredential represents an optional staking credential
type OptionalStakingCredential struct {
	IsPresent         bool
	StakingCredential StakingCredential
}

func (o *OptionalStakingCredential) UnmarshalCBOR(cborData []byte) error {
	var tmpConstr cbor.Constructor
	if _, err := cbor.Decode(cborData, &tmpConstr); err != nil {
		return err
	}
	// Constructor 0 = Just, Constructor 1 = Nothing
	if tmpConstr.Constructor() == 1 {
		o.IsPresent = false
		return nil
	}
	o.IsPresent = true
	return cbor.DecodeGeneric(tmpConstr.FieldsCbor(), &o.StakingCredential)
}

// StakingCredential represents a staking credential
type StakingCredential struct {
	Type int // 0 = StakingHash, 1 = StakingPtr
	Hash []byte
	// For StakingPtr (type 1)
	BlockIndex       uint64
	TxIndex          uint64
	CertificateIndex uint64
}

func (s *StakingCredential) UnmarshalCBOR(cborData []byte) error {
	var tmpConstr cbor.Constructor
	if _, err := cbor.Decode(cborData, &tmpConstr); err != nil {
		return err
	}
	s.Type = int(tmpConstr.Constructor())
	if s.Type == 0 {
		// StakingHash
		var wrapper struct {
			cbor.StructAsArray
			Credential Credential
		}
		if err := cbor.DecodeGeneric(tmpConstr.FieldsCbor(), &wrapper); err != nil {
			return err
		}
		s.Hash = wrapper.Credential.Hash
	} else {
		// StakingPtr
		var wrapper struct {
			cbor.StructAsArray
			BlockIndex       uint64
			TxIndex          uint64
			CertificateIndex uint64
		}
		if err := cbor.DecodeGeneric(tmpConstr.FieldsCbor(), &wrapper); err != nil {
			return err
		}
		s.BlockIndex = wrapper.BlockIndex
		s.TxIndex = wrapper.TxIndex
		s.CertificateIndex = wrapper.CertificateIndex
	}
	return nil
}

// PoolVariant represents the type of pool (ConstantProduct or Stableswap)
type PoolVariant struct {
	Type int // 0 = ConstantProduct, 1 = Stableswap
	// For Stableswap (type 1)
	ParameterD int64
	ScaleA     int64
	ScaleB     int64
}

func (p *PoolVariant) UnmarshalCBOR(cborData []byte) error {
	var tmpConstr cbor.Constructor
	if _, err := cbor.Decode(cborData, &tmpConstr); err != nil {
		return err
	}
	p.Type = int(tmpConstr.Constructor())
	if p.Type == 1 {
		// Stableswap
		var wrapper struct {
			cbor.StructAsArray
			ParameterD int64
			ScaleA     int64
			ScaleB     int64
		}
		if err := cbor.DecodeGeneric(tmpConstr.FieldsCbor(), &wrapper); err != nil {
			return err
		}
		p.ParameterD = wrapper.ParameterD
		p.ScaleA = wrapper.ScaleA
		p.ScaleB = wrapper.ScaleB
	}
	return nil
}

// IsConstantProduct returns true if this is a constant product pool
func (p *PoolVariant) IsConstantProduct() bool {
	return p.Type == 0
}

// IsStableswap returns true if this is a stableswap pool
func (p *PoolVariant) IsStableswap() bool {
	return p.Type == 1
}
