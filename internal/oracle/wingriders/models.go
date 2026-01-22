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
	FeeDenom     = 10000
)

// PoolDatum represents the WingRiders V2 pool datum structure
type PoolDatum struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	RequestValidatorHash []byte // Script hash (28 bytes)
	AssetA               AssetClass
	AssetB               AssetClass
	SwapFeeInBasis       uint64
	ProtocolFeeInBasis   uint64
	ProjectFeeInBasis    uint64
	FeeBasis             uint64
	AgentFeeAda          uint64
	LastInteraction      uint64
	TreasuryA            uint64
	TreasuryB            uint64
	ProjectTreasuryA     uint64
	ProjectTreasuryB     uint64
}

func (p *PoolDatum) UnmarshalCBOR(cborData []byte) error {
	p.SetCbor(cborData)
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
	return cbor.DecodeGeneric(tmpConstr.FieldsCbor(), p)
}

func (p PoolDatum) String() string {
	return fmt.Sprintf(
		"PoolDatum< assetA = %s, assetB = %s, treasuryA = %d, treasuryB = %d, swapFee = %d, protocolFee = %d, projectFee = %d >",
		p.AssetA.String(),
		p.AssetB.String(),
		p.TreasuryA,
		p.TreasuryB,
		p.SwapFeeInBasis,
		p.ProtocolFeeInBasis,
		p.ProjectFeeInBasis,
	)
}

// AssetClass represents an asset class (policy ID + asset name)
type AssetClass struct {
	cbor.StructAsArray
	PolicyId []byte
	Name     []byte
}

func (a *AssetClass) UnmarshalCBOR(cborData []byte) error {
	var tmpConstr cbor.Constructor
	if _, err := cbor.Decode(cborData, &tmpConstr); err != nil {
		return err
	}
	if tmpConstr.Constructor() != 0 {
		return fmt.Errorf(
			"AssetClass: expected constructor 0, got %d",
			tmpConstr.Constructor(),
		)
	}
	return cbor.DecodeGeneric(
		tmpConstr.FieldsCbor(),
		a,
	)
}

func (a *AssetClass) MarshalCBOR() ([]byte, error) {
	tmpConstr := cbor.NewConstructor(
		0,
		cbor.IndefLengthList{
			a.PolicyId,
			a.Name,
		},
	)
	return cbor.Encode(&tmpConstr)
}

func (a AssetClass) String() string {
	return fmt.Sprintf(
		"AssetClass< name = %s, policy_id = %s >",
		a.Name,
		fmt.Sprintf("%x", a.PolicyId),
	)
}

func (a AssetClass) IsLovelace() bool {
	return len(a.PolicyId) == 0 && len(a.Name) == 0
}

// ToCommonAssetClass converts to common.AssetClass
func (a AssetClass) ToCommonAssetClass() common.AssetClass {
	return common.AssetClass{
		PolicyId: a.PolicyId,
		Name:     a.Name,
	}
}