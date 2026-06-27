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
	CDP *CDP
}

func (d *MonoDatum) UnmarshalCBOR(cborData []byte) error {
	d.SetCbor(cborData)
	var tmpConstr cbor.ConstructorDecoder
	if _, err := cbor.Decode(cborData, &tmpConstr); err != nil {
		return err
	}
	d.Constructor = tmpConstr.Tag()

	switch d.Constructor {
	case 1: // CDP
		var cdp CDP
		if err := cbor.DecodeGeneric(tmpConstr.Fields(), &cdp); err != nil {
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
	var tmpConstr cbor.ConstructorDecoder
	if _, err := cbor.Decode(cborData, &tmpConstr); err != nil {
		return err
	}
	c.Type = int(tmpConstr.Tag())
	fields, err := constructorFields(tmpConstr.Fields())
	if err != nil {
		return err
	}

	switch c.Type {
	case 0:
		if len(fields) != 1 {
			return fmt.Errorf(
				"AuthorizeWithPubKey: expected 1 field, got %d",
				len(fields),
			)
		}
		if _, err := cbor.Decode([]byte(fields[0]), &c.PubKey); err != nil {
			return err
		}
	case 1:
		if len(fields) != 1 {
			return fmt.Errorf(
				"AuthorizeWithConstraint: expected 1 field, got %d",
				len(fields),
			)
		}
		var constraint CDPConstraint
		if _, err := cbor.Decode([]byte(fields[0]), &constraint); err != nil {
			return err
		}
		if constraint.TokenId == nil {
			return fmt.Errorf("AuthorizeWithConstraint: missing token constraint")
		}
		c.TokenId = constraint.TokenId
	default:
		return fmt.Errorf("unsupported CDPCredential type: %d", c.Type)
	}

	return nil
}

func constructorFields(cborData []byte) ([]cbor.RawMessage, error) {
	var fields []cbor.RawMessage
	if _, err := cbor.Decode(cborData, &fields); err != nil {
		return nil, err
	}
	return fields, nil
}

// CDPConstraint represents a constraint-based CDP owner authorization.
type CDPConstraint struct {
	cbor.StructAsArray
	Type    int
	TokenId *AssetClass
}

func (c *CDPConstraint) UnmarshalCBOR(cborData []byte) error {
	var tmpConstr cbor.ConstructorDecoder
	if _, err := cbor.Decode(cborData, &tmpConstr); err != nil {
		return err
	}
	c.Type = int(tmpConstr.Tag())
	fields, err := constructorFields(tmpConstr.Fields())
	if err != nil {
		return err
	}

	switch c.Type {
	case 0:
		if len(fields) != 1 {
			return fmt.Errorf(
				"MustSpendToken: expected 1 field, got %d",
				len(fields),
			)
		}
		var asset AssetClass
		if _, err := cbor.Decode([]byte(fields[0]), &asset); err != nil {
			return err
		}
		c.TokenId = &asset
	case 1:
		return fmt.Errorf(
			"AuthorizeWithConstraint MustWithdrawFrom is unsupported",
		)
	default:
		return fmt.Errorf("unsupported CDP constraint type: %d", c.Type)
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
	var tmpConstr cbor.ConstructorDecoder
	if _, err := cbor.Decode(cborData, &tmpConstr); err != nil {
		return err
	}
	if tmpConstr.Tag() != 0 {
		return fmt.Errorf(
			"AssetClass: expected constructor 0, got %d",
			tmpConstr.Tag(),
		)
	}
	return cbor.DecodeGeneric(tmpConstr.Fields(), a)
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
	var tmpConstr cbor.ConstructorDecoder
	if _, err := cbor.Decode(cborData, &tmpConstr); err != nil {
		return err
	}
	return cbor.DecodeGeneric(tmpConstr.Fields(), p)
}
