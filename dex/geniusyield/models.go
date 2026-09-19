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

package geniusyield

import (
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/shai/common"
)

// Protocol constants
const (
	ProtocolName = "geniusyield"

	// OrderScriptHash is the payment credential of the mainnet partial order
	// script. Orders sit at base addresses that share this script hash and
	// carry the maker's own staking credential, so the order UTxOs of this
	// protocol cannot be enumerated as a fixed address set.
	OrderScriptHash = "edfff663d37fc5f9753bc4222e0da2bfe08aa48db0837d2c329adeb3"

	// OrderNFTPolicy is the mainnet minting policy of the NFT that identifies
	// an order UTxO. It is the podNFT currency symbol recorded in the on-chain
	// PartialOrderConfigDatum.
	OrderNFTPolicy = "22f6999d4effc0ade05f6e1a70b702c65d6b3cdf0e301e4a8267f585"
)

// GetOrderPaymentCredentials returns the mainnet payment credentials that hold
// partial order UTxOs. Orders are located by payment credential rather than by
// bech32 address because each maker's order sits at a base address built from
// OrderScriptHash and that maker's own staking credential.
func GetOrderPaymentCredentials() []string {
	return []string{OrderScriptHash}
}

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
//	    , podMakerOfferedPercentFee :: Integer
//	    , podContainedFee :: ContainedFee
//	    , podContainedPayment :: Integer
//	    }
//
// The datum is a fifteen-field constructor-0 array. Field counts and
// constructor tags are pinned by TestGeniusYieldParseMainnetOrderDatum, which
// decodes a mainnet order datum.
type PartialOrderDatum struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	OwnerKey               []byte        // PubKeyHash for cancellation
	OwnerAddr              Address       // Address for payments
	OfferedAsset           Asset         // Asset being offered
	OfferedOriginalAmount  uint64        // Original units offered
	OfferedAmount          uint64        // Current units offered
	AskedAsset             Asset         // Asset wanted as payment
	Price                  Rational      // Price per unit (num/denom)
	NFT                    []byte        // TokenName identifying this order
	Start                  OptionalPOSIX // Optional start time
	End                    OptionalPOSIX // Optional end time
	PartialFills           uint64        // Number of partial fills
	MakerLovelaceFlatFee   uint64        // Flat fee in lovelace
	MakerOfferedPercentFee uint64        // Offered-asset maker fee units
	ContainedFee           ContainedFee  // Fee tracking
	ContainedPayment       uint64        // Payment tracking
}

func (d *PartialOrderDatum) UnmarshalCBOR(cborData []byte) error {
	d.SetCbor(cborData)
	var tmpConstr cbor.ConstructorDecoder
	if _, err := cbor.Decode(cborData, &tmpConstr); err != nil {
		return err
	}
	if tmpConstr.Tag() != 0 {
		return fmt.Errorf(
			"expected constructor 0, got %d",
			tmpConstr.Tag(),
		)
	}
	return cbor.DecodeGeneric(tmpConstr.Fields(), d)
}

// Asset represents an asset (PolicyId, AssetName) tuple
type Asset struct {
	cbor.StructAsArray
	PolicyId  []byte
	AssetName []byte
}

func (a *Asset) UnmarshalCBOR(cborData []byte) error {
	var tmpConstr cbor.ConstructorDecoder
	if _, err := cbor.Decode(cborData, &tmpConstr); err != nil {
		return err
	}
	return cbor.DecodeGeneric(tmpConstr.Fields(), a)
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
	var tmpConstr cbor.ConstructorDecoder
	if _, err := cbor.Decode(cborData, &tmpConstr); err != nil {
		return err
	}
	return cbor.DecodeGeneric(tmpConstr.Fields(), a)
}

// Credential represents a payment or staking credential
type Credential struct {
	Type int // 0 = PubKeyHash, 1 = ScriptHash
	Hash []byte
}

func (c *Credential) UnmarshalCBOR(cborData []byte) error {
	var tmpConstr cbor.ConstructorDecoder
	if _, err := cbor.Decode(cborData, &tmpConstr); err != nil {
		return err
	}
	tag := tmpConstr.Tag()
	switch tag {
	case 0, 1:
	default:
		return fmt.Errorf("unsupported credential constructor %d", tag)
	}
	var wrapper struct {
		cbor.StructAsArray
		Hash []byte
	}
	if err := cbor.DecodeGeneric(tmpConstr.Fields(), &wrapper); err != nil {
		return err
	}
	c.Type = int(tag)
	c.Hash = wrapper.Hash
	return nil
}

// StakingCredential is the datum's StakingCredential: StakingHash of a
// Credential (constructor 0) or StakingPtr (constructor 1). Only StakingHash
// carries a credential, so Credential is nil when IsPointer is set.
type StakingCredential struct {
	IsPointer  bool
	Credential *Credential
}

func (s *StakingCredential) UnmarshalCBOR(cborData []byte) error {
	var tmpConstr cbor.ConstructorDecoder
	if _, err := cbor.Decode(cborData, &tmpConstr); err != nil {
		return err
	}
	switch tag := tmpConstr.Tag(); tag {
	case 0:
		var wrapper struct {
			cbor.StructAsArray
			Inner Credential
		}
		if err := cbor.DecodeGeneric(tmpConstr.Fields(), &wrapper); err != nil {
			return err
		}
		s.IsPointer = false
		s.Credential = &wrapper.Inner
		return nil
	case 1:
		s.IsPointer = true
		s.Credential = nil
		return nil
	default:
		return fmt.Errorf("unsupported staking credential constructor %d", tag)
	}
}

// OptionalCredential represents the address's optional staking credential. The
// datum holds a Maybe StakingCredential, so the present case wraps the payment
// credential in a StakingHash rather than holding it directly.
type OptionalCredential struct {
	IsPresent  bool
	IsPointer  bool
	Credential *Credential
}

func (o *OptionalCredential) UnmarshalCBOR(cborData []byte) error {
	var tmpConstr cbor.ConstructorDecoder
	if _, err := cbor.Decode(cborData, &tmpConstr); err != nil {
		return err
	}
	// Constructor 0 = Some, Constructor 1 = None
	switch tag := tmpConstr.Tag(); tag {
	case 0:
	case 1:
		// Reset to avoid stale data when struct is reused
		o.IsPresent = false
		o.IsPointer = false
		o.Credential = nil
		return nil
	default:
		return fmt.Errorf("unsupported optional credential constructor %d", tag)
	}
	var wrapper struct {
		cbor.StructAsArray
		Inner StakingCredential
	}
	if err := cbor.DecodeGeneric(tmpConstr.Fields(), &wrapper); err != nil {
		return err
	}
	o.IsPresent = true
	o.IsPointer = wrapper.Inner.IsPointer
	o.Credential = wrapper.Inner.Credential
	return nil
}

// Rational represents a rational number as numerator/denominator pair
type Rational struct {
	Numerator   int64
	Denominator int64
}

func (r *Rational) UnmarshalCBOR(cborData []byte) error {
	var tmpConstr cbor.ConstructorDecoder
	if _, err := cbor.Decode(cborData, &tmpConstr); err != nil {
		return err
	}
	var wrapper struct {
		cbor.StructAsArray
		Numerator   int64
		Denominator int64
	}
	if err := cbor.DecodeGeneric(tmpConstr.Fields(), &wrapper); err != nil {
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
	var tmpConstr cbor.ConstructorDecoder
	if _, err := cbor.Decode(cborData, &tmpConstr); err != nil {
		return err
	}
	// Constructor 0 = Some, Constructor 1 = None
	switch tag := tmpConstr.Tag(); tag {
	case 0:
	case 1:
		o.IsPresent = false
		o.Time = 0 // Reset to avoid stale values when struct is reused
		return nil
	default:
		return fmt.Errorf("unsupported optional POSIX constructor %d", tag)
	}
	var wrapper struct {
		cbor.StructAsArray
		Time int64
	}
	if err := cbor.DecodeGeneric(tmpConstr.Fields(), &wrapper); err != nil {
		return err
	}
	o.IsPresent = true
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
	var tmpConstr cbor.ConstructorDecoder
	if _, err := cbor.Decode(cborData, &tmpConstr); err != nil {
		return err
	}
	return cbor.DecodeGeneric(tmpConstr.Fields(), c)
}
