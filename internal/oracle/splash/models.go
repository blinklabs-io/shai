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

// Package splash provides datum types and parsing for Splash DEX protocol.
package splash

import (
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/shai/internal/common"
)

// Protocol constants
const (
	ProtocolName = "splash"
	FeeDenom     = 10000
)

// PoolDatum represents the Splash pool datum structure
type PoolDatum struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	Nft         AssetClass
	X           AssetClass
	Y           AssetClass
	Lq          AssetClass
	FeeNum      uint64
	AdminPolicy [][]byte
	LqBound     uint64
}

func (p *PoolDatum) UnmarshalCBOR(cborData []byte) error {
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

	type tPoolDatum PoolDatum
	var tmp tPoolDatum
	if _, err := cbor.Decode(tmpConstr.FieldsCbor(), &tmp); err != nil {
		return err
	}
	*p = PoolDatum(tmp)
	p.SetCbor(cborData)
	return nil
}

func (p *PoolDatum) MarshalCBOR() ([]byte, error) {
	var tmpAdminPolicy any = []any{}
	if len(p.AdminPolicy) > 0 {
		tmpAdminPolicyItems := []any{}
		for _, adminPolicy := range p.AdminPolicy {
			tmpAdminPolicyItems = append(tmpAdminPolicyItems, adminPolicy)
		}
		tmpAdminPolicy = cbor.IndefLengthList(tmpAdminPolicyItems)
	}
	tmpConstr := cbor.NewConstructor(
		0,
		cbor.IndefLengthList{
			p.Nft,
			p.X,
			p.Y,
			p.Lq,
			p.FeeNum,
			tmpAdminPolicy,
			p.LqBound,
		},
	)
	return cbor.Encode(&tmpConstr)
}

func (p PoolDatum) String() string {
	return fmt.Sprintf(
		"PoolDatum< nft = %s, x = %s, y = %s, lq = %s, fee_num = %d, admin_policy = %v, lq_bound = %d >",
		p.Nft.String(),
		p.X.String(),
		p.Y.String(),
		p.Lq.String(),
		p.FeeNum,
		p.AdminPolicy,
		p.LqBound,
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

	type tAssetClass AssetClass
	var tmp tAssetClass
	if _, err := cbor.Decode(tmpConstr.FieldsCbor(), &tmp); err != nil {
		return err
	}
	*a = AssetClass(tmp)
	return nil
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
