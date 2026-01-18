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

// Package geniusyield provides datum types and parsing for Genius Yield
// order-book DEX protocol.
package geniusyield

import (
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/shai/internal/common"
)

// Protocol constants
const (
	ProtocolName = "geniusyield"

	// Mainnet order script hash
	// Genius Yield order-book DEX contract
	OrderScriptHash = "f95cab2d4cf78cc5ffa1c6f0bdb17a6f35df9a60e442d59e2d576e32"

	// Mainnet NFT policy ID for order identification
	OrderNFTPolicy = "2e5f2c41e0a58f5a5a7b1f5c5e5f5e5f5e5f5e5f5e5f5e5f5e5f5e5f"
)

// PartialOrderDatum represents the Genius Yield order datum structure
// Based on the Haskell definition:
//
//	data PartialOrderDatum = PartialOrderDatum
//	    { podOwnerKey :: PubKeyHash
//	    , podOwnerAddr :: Address
//	    , podOfferedAsset :: AssetClass
//	    , podOfferedOriginalAmount :: Integer
//	    , podOfferedAmount :: Integer
//	    , podAskedAsset :: AssetClass
//	    , podPrice :: Rational  -- (numerator, denominator)
//	    , podNFT :: TokenName
//	    , podStart :: Maybe POSIXTime
//	    , podEnd :: Maybe POSIXTime
//	    , podPartialFills :: Integer
//	    , podMakerLovelaceFlatFee :: Integer
//	    , podMakerOfferedPercentFee :: Rational
//	    , podMakerOfferedPercentFeeMax :: Integer
//	    , podContainedFee :: ContainedFee
//	    , podContainedPayment :: Integer
//	    }
type PartialOrderDatum struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	OwnerKey                  []byte        // PubKeyHash for cancellation
	OwnerAddr                 Address       // Address for payments
	OfferedAsset              Asset         // Asset being offered
	OfferedOriginalAmount     uint64        // Original units offered
	OfferedAmount             uint64        // Current units offered
	AskedAsset                Asset         // Asset wanted as payment
	Price                     Rational      // Price per unit (num/denom)
	NFT                       []byte        // TokenName identifying this order
	Start                     OptionalPOSIX // Optional start time
	End                       OptionalPOSIX // Optional end time
	PartialFills              uint64        // Number of partial fills
	MakerLovelaceFlatFee      uint64        // Flat fee in lovelace
	MakerOfferedPercentFee    Rational      // Percentage fee
	MakerOfferedPercentFeeMax uint64        // Max percentage fee
	ContainedFee              ContainedFee  // Fee tracking
	ContainedPayment          uint64        // Payment tracking
}

func (d *PartialOrderDatum) UnmarshalCBOR(cborData []byte) error {
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

// Asset represents an asset (PolicyId, AssetName) tuple
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

// Address represents a Cardano address in datum format
// Constructor 0: PubKeyHash credential
// Constructor 1: ScriptHash credential
type Address struct {
	cbor.StructAsArray
	PaymentCredential Credential
	StakingCredential OptionalCredential
}

func (a *Address) UnmarshalCBOR(cborData []byte) error {
	var tmpConstr cbor.Constructor
	if _, err := cbor.Decode(cborData, &tmpConstr); err != nil {
		return err
	}
	return cbor.DecodeGeneric(tmpConstr.FieldsCbor(), a)
}

// Credential represents a payment or staking credential
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

// OptionalCredential represents an optional staking credential
type OptionalCredential struct {
	IsPresent  bool
	Credential *Credential
}

func (o *OptionalCredential) UnmarshalCBOR(cborData []byte) error {
	var tmpConstr cbor.Constructor
	if _, err := cbor.Decode(cborData, &tmpConstr); err != nil {
		return err
	}
	// Constructor 0 = Some, Constructor 1 = None
	if tmpConstr.Constructor() == 1 {
		o.IsPresent = false
		o.Credential = nil // Reset to avoid stale data when struct is reused
		return nil
	}
	o.IsPresent = true
	o.Credential = &Credential{}
	var wrapper struct {
		cbor.StructAsArray
		Inner Credential
	}
	if err := cbor.DecodeGeneric(tmpConstr.FieldsCbor(), &wrapper); err != nil {
		return err
	}
	o.Credential = &wrapper.Inner
	return nil
}

// Rational represents a rational number as numerator/denominator pair
type Rational struct {
	Numerator   int64
	Denominator int64
}

func (r *Rational) UnmarshalCBOR(cborData []byte) error {
	var tmpConstr cbor.Constructor
	if _, err := cbor.Decode(cborData, &tmpConstr); err != nil {
		return err
	}
	var wrapper struct {
		cbor.StructAsArray
		Numerator   int64
		Denominator int64
	}
	if err := cbor.DecodeGeneric(tmpConstr.FieldsCbor(), &wrapper); err != nil {
		return err
	}
	r.Numerator = wrapper.Numerator
	r.Denominator = wrapper.Denominator
	return nil
}

// ToFloat64 converts the rational to a float64 value
func (r Rational) ToFloat64() float64 {
	if r.Denominator == 0 {
		return 0
	}
	return float64(r.Numerator) / float64(r.Denominator)
}

// OptionalPOSIX represents an optional POSIX timestamp (in milliseconds)
type OptionalPOSIX struct {
	IsPresent bool
	Time      int64 // POSIX time in milliseconds
}

func (o *OptionalPOSIX) UnmarshalCBOR(cborData []byte) error {
	var tmpConstr cbor.Constructor
	if _, err := cbor.Decode(cborData, &tmpConstr); err != nil {
		return err
	}
	// Constructor 0 = Some, Constructor 1 = None
	if tmpConstr.Constructor() == 1 {
		o.IsPresent = false
		o.Time = 0 // Reset to avoid stale values when struct is reused
		return nil
	}
	o.IsPresent = true
	var wrapper struct {
		cbor.StructAsArray
		Time int64
	}
	if err := cbor.DecodeGeneric(tmpConstr.FieldsCbor(), &wrapper); err != nil {
		return err
	}
	o.Time = wrapper.Time
	return nil
}

// ContainedFee tracks fee amounts contained in the order
type ContainedFee struct {
	cbor.StructAsArray
	LovelaceFee uint64 // Lovelace fees contained
	OfferedFee  uint64 // Offered asset fees contained
	AskedFee    uint64 // Asked asset fees contained
}

func (c *ContainedFee) UnmarshalCBOR(cborData []byte) error {
	var tmpConstr cbor.Constructor
	if _, err := cbor.Decode(cborData, &tmpConstr); err != nil {
		return err
	}
	return cbor.DecodeGeneric(tmpConstr.FieldsCbor(), c)
}
