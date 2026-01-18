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

// Package geniusyield implements the Genius Yield order-book DEX batcher
// with Smart Order Routing (SOR) capabilities.
package geniusyield

import (
	"errors"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/shai/internal/common"
)

// ErrNotOrderDatum is returned when CBOR data is not a valid order datum
var ErrNotOrderDatum = errors.New("not an order datum (constructor != 0)")

// OrderConfig represents the order configuration extracted from a datum
// This matches the on-chain PartialOrderDatum structure
type OrderConfig struct {
	cbor.StructAsArray
	OwnerKey                  []byte        // PubKeyHash
	OwnerAddr                 OrderAddress  // Address for payments
	OfferedAsset              OrderAsset    // Asset being offered
	OfferedOriginalAmount     uint64        // Original amount
	OfferedAmount             uint64        // Current amount
	AskedAsset                OrderAsset    // Asset wanted
	Price                     OrderRational // Price as rational
	NFT                       []byte        // Order NFT token name
	Start                     OptionalPOSIX // Start time
	End                       OptionalPOSIX // End time
	PartialFills              uint64        // Number of fills
	MakerLovelaceFlatFee      uint64        // Flat fee
	MakerOfferedPercentFee    OrderRational // Percent fee
	MakerOfferedPercentFeeMax uint64        // Max percent fee
	ContainedFee              ContainedFee  // Fee tracking
	ContainedPayment          uint64        // Payment tracking
}

func (o *OrderConfig) UnmarshalCBOR(cborData []byte) error {
	var tmpConstr cbor.Constructor
	if _, err := cbor.Decode(cborData, &tmpConstr); err != nil {
		return err
	}
	if tmpConstr.Constructor() != 0 {
		return ErrNotOrderDatum
	}
	return cbor.DecodeGeneric(tmpConstr.FieldsCbor(), o)
}

// OrderAsset represents an asset class in order datums
type OrderAsset struct {
	cbor.StructAsArray
	PolicyId  []byte
	AssetName []byte
}

func (a *OrderAsset) UnmarshalCBOR(cborData []byte) error {
	var tmpConstr cbor.Constructor
	if _, err := cbor.Decode(cborData, &tmpConstr); err != nil {
		return err
	}
	return cbor.DecodeGeneric(tmpConstr.FieldsCbor(), a)
}

// ToCommon converts to common.AssetClass
func (a OrderAsset) ToCommon() common.AssetClass {
	return common.AssetClass{
		PolicyId: a.PolicyId,
		Name:     a.AssetName,
	}
}

// IsLovelace returns true if this is ADA
func (a OrderAsset) IsLovelace() bool {
	return len(a.PolicyId) == 0 && len(a.AssetName) == 0
}

// OrderAddress represents a Cardano address in datum format
type OrderAddress struct {
	cbor.StructAsArray
	PaymentCredential OrderCredential
	StakingCredential OptionalCredential
}

func (a *OrderAddress) UnmarshalCBOR(cborData []byte) error {
	var tmpConstr cbor.Constructor
	if _, err := cbor.Decode(cborData, &tmpConstr); err != nil {
		return err
	}
	return cbor.DecodeGeneric(tmpConstr.FieldsCbor(), a)
}

// OrderCredential represents a payment credential
type OrderCredential struct {
	Type int // 0 = PubKeyHash, 1 = ScriptHash
	Hash []byte
}

func (c *OrderCredential) UnmarshalCBOR(cborData []byte) error {
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
	Credential *OrderCredential
}

func (o *OptionalCredential) UnmarshalCBOR(cborData []byte) error {
	var tmpConstr cbor.Constructor
	if _, err := cbor.Decode(cborData, &tmpConstr); err != nil {
		return err
	}
	if tmpConstr.Constructor() == 1 {
		o.IsPresent = false
		o.Credential = nil // Reset to avoid stale data
		return nil
	}
	o.IsPresent = true
	var wrapper struct {
		cbor.StructAsArray
		Inner OrderCredential
	}
	if err := cbor.DecodeGeneric(tmpConstr.FieldsCbor(), &wrapper); err != nil {
		return err
	}
	o.Credential = &wrapper.Inner
	return nil
}

// OrderRational represents a rational number
type OrderRational struct {
	Numerator   int64
	Denominator int64
}

func (r *OrderRational) UnmarshalCBOR(cborData []byte) error {
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

// ToFloat64 converts to float64
func (r OrderRational) ToFloat64() float64 {
	if r.Denominator == 0 {
		return 0
	}
	return float64(r.Numerator) / float64(r.Denominator)
}

// OptionalPOSIX represents an optional timestamp
type OptionalPOSIX struct {
	IsPresent bool
	Time      int64 // POSIX time in milliseconds
}

func (o *OptionalPOSIX) UnmarshalCBOR(cborData []byte) error {
	var tmpConstr cbor.Constructor
	if _, err := cbor.Decode(cborData, &tmpConstr); err != nil {
		return err
	}
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

// ContainedFee tracks fees in an order
type ContainedFee struct {
	cbor.StructAsArray
	LovelaceFee uint64
	OfferedFee  uint64
	AskedFee    uint64
}

func (c *ContainedFee) UnmarshalCBOR(cborData []byte) error {
	var tmpConstr cbor.Constructor
	if _, err := cbor.Decode(cborData, &tmpConstr); err != nil {
		return err
	}
	return cbor.DecodeGeneric(tmpConstr.FieldsCbor(), c)
}

// PartialFillRedeemer is the redeemer for partially filling an order
type PartialFillRedeemer struct {
	cbor.StructAsArray
	FillAmount uint64 // Amount of offered asset being taken
	// Note: Full structure depends on Genius Yield contract version
}

// MarshalCBOR encodes the redeemer
func (r *PartialFillRedeemer) MarshalCBOR() ([]byte, error) {
	// Constructor 0 = PartialFill
	tmpConstr := cbor.NewConstructor(
		0,
		cbor.IndefLengthList{
			r.FillAmount,
		},
	)
	return cbor.Encode(&tmpConstr)
}

// CompleteFillRedeemer is the redeemer for completely filling an order
type CompleteFillRedeemer struct {
	cbor.StructAsArray
}

// MarshalCBOR encodes the redeemer
func (r *CompleteFillRedeemer) MarshalCBOR() ([]byte, error) {
	// Constructor 1 = CompleteFill
	tmpConstr := cbor.NewConstructor(1, cbor.IndefLengthList{})
	return cbor.Encode(&tmpConstr)
}
