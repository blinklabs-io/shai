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

// Package butane provides datum types and parsing for Butane synthetics protocol.
package butane

import (
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/shai/internal/common"
)

// Protocol constants
const (
	ProtocolName = "butane"

	// Price feed denominator (standard precision)
	PriceDenom = 1000000
)

// MonoDatum represents the main Butane datum wrapper
// This is a sum type (enum) with multiple constructors:
// - Constructor 0: ParamsWrapper
// - Constructor 1: CDP
// - Constructor 2: GovDatum
// - Constructor 3: TreasuryDatum
// - Constructor 4: CompatLockedTokens
// - Constructor 5: StakedSynthetics
type MonoDatum struct {
	cbor.DecodeStoreCbor
	Constructor uint
	// Only one of these will be populated based on constructor
	CDP       *CDP
	PriceFeed *PriceFeed
}

func (d *MonoDatum) UnmarshalCBOR(cborData []byte) error {
	d.SetCbor(cborData)
	var tmpConstr cbor.Constructor
	if _, err := cbor.Decode(cborData, &tmpConstr); err != nil {
		return err
	}
	d.Constructor = tmpConstr.Constructor()

	switch d.Constructor {
	case 1: // CDP
		var cdp CDP
		if err := cbor.DecodeGeneric(tmpConstr.FieldsCbor(), &cdp); err != nil {
			return err
		}
		d.CDP = &cdp
	default:
		// Other constructors (e.g., ParamsWrapper) are valid but not parsed
		// Caller should check CDP != nil to determine if this is a CDP datum
		d.CDP = nil
	}

	return nil
}

// CDP represents a Collateralized Debt Position
// Constructor 1 with fields: owner, synthetic, minted, startTime
type CDP struct {
	cbor.StructAsArray
	Owner     CDPCredential
	Synthetic AssetClass
	Minted    uint64
	StartTime int64
}

// CDPCredential represents CDP owner authorization
// Constructor 0: AuthorizeWithPubKey (pubkey hash)
// Constructor 1: AuthorizeWithConstraint (token/script constraint)
type CDPCredential struct {
	cbor.StructAsArray
	Type    int // 0 = PubKey, 1 = Constraint
	PubKey  []byte
	TokenId *AssetClass
}

func (c *CDPCredential) UnmarshalCBOR(cborData []byte) error {
	var tmpConstr cbor.Constructor
	if _, err := cbor.Decode(cborData, &tmpConstr); err != nil {
		return err
	}
	c.Type = int(tmpConstr.Constructor())

	switch c.Type {
	case 0:
		// AuthorizeWithPubKey - single field: pubkey hash
		var wrapper struct {
			cbor.StructAsArray
			PubKey []byte
		}
		if err := cbor.DecodeGeneric(tmpConstr.FieldsCbor(), &wrapper); err != nil {
			return err
		}
		c.PubKey = wrapper.PubKey
	case 1:
		// AuthorizeWithConstraint - token/script constraint (fields not parsed)
		// Leave TokenId nil for now; add parsing if needed
	default:
		return fmt.Errorf("unsupported CDPCredential type: %d", c.Type)
	}

	return nil
}

// AssetClass represents an asset (PolicyId, AssetName)
type AssetClass struct {
	cbor.StructAsArray
	PolicyId  []byte
	AssetName []byte
}

func (a *AssetClass) UnmarshalCBOR(cborData []byte) error {
	var tmpConstr cbor.Constructor
	if _, err := cbor.Decode(cborData, &tmpConstr); err != nil {
		return err
	}
	return cbor.DecodeGeneric(tmpConstr.FieldsCbor(), a)
}

// ToCommonAssetClass converts to common.AssetClass
func (a AssetClass) ToCommonAssetClass() common.AssetClass {
	return common.AssetClass{
		PolicyId: a.PolicyId,
		Name:     a.AssetName,
	}
}

// PriceFeed represents oracle price data
// Fields: collateralPrices, syntheticAsset, denominator, validityInterval
type PriceFeed struct {
	cbor.StructAsArray
	CollateralPrices []uint64
	SyntheticAsset   []byte
	Denominator      uint64
	ValidFrom        int64
	ValidTo          int64
}

func (p *PriceFeed) UnmarshalCBOR(cborData []byte) error {
	var tmpConstr cbor.Constructor
	if _, err := cbor.Decode(cborData, &tmpConstr); err != nil {
		return err
	}
	return cbor.DecodeGeneric(tmpConstr.FieldsCbor(), p)
}
