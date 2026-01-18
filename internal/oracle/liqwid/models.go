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

// Package liqwid provides datum types and parsing for Liqwid lending protocol.
// Liqwid is Cardano's leading lending protocol where users deposit assets
// and receive qTokens (interest-bearing tokens) in return.
package liqwid

import (
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/shai/internal/common"
)

// Protocol constants
const (
	ProtocolName = "liqwid"

	// Basis points denominator (100% = 10000 basis points)
	BasisPointsDenom = 10000

	// Interest rate precision (1e18 for high precision math)
	InterestRatePrecision = 1000000000000000000

	// Exchange rate precision for qToken conversion
	ExchangeRatePrecision = 1000000000000000000
)

// MarketDatum represents a Liqwid lending market datum
// Based on Compound/Aave-style lending protocol patterns and Plutus CBOR conventions
// Constructor 0 with fields for market state
type MarketDatum struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	MarketNft        Asset  // Market identifier NFT (unique per market)
	UnderlyingAsset  Asset  // The asset being lent (e.g., ADA, DJED)
	QTokenAsset      Asset  // The qToken for this market (receipt token)
	TotalSupply      uint64 // Total supplied to market (in underlying)
	TotalBorrows     uint64 // Total borrowed from market
	ReserveAmount    uint64 // Protocol reserves
	InterestRate     uint64 // Current interest rate (basis points)
	CollateralFactor uint64 // LTV ratio (basis points, e.g., 7500 = 75%)
}

func (d *MarketDatum) UnmarshalCBOR(cborData []byte) error {
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

// UtilizationRate returns the utilization rate as a float (0.0 - 1.0)
// Utilization = TotalBorrows / TotalSupply
func (d *MarketDatum) UtilizationRate() float64 {
	if d.TotalSupply == 0 {
		return 0
	}
	return float64(d.TotalBorrows) / float64(d.TotalSupply)
}

// AvailableLiquidity returns the amount available for borrowing
func (d *MarketDatum) AvailableLiquidity() uint64 {
	if d.TotalSupply <= d.TotalBorrows {
		return 0
	}
	return d.TotalSupply - d.TotalBorrows
}

// CollateralFactorFloat returns the collateral factor as a decimal (e.g., 0.75)
func (d *MarketDatum) CollateralFactorFloat() float64 {
	return float64(d.CollateralFactor) / float64(BasisPointsDenom)
}

// InterestRateFloat returns the interest rate as a decimal (e.g., 0.05 for 5%)
func (d *MarketDatum) InterestRateFloat() float64 {
	return float64(d.InterestRate) / float64(BasisPointsDenom)
}

// Asset represents an asset in Liqwid datum
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
	if tmpConstr.Constructor() != 0 {
		return fmt.Errorf(
			"expected Asset constructor 0, got %d",
			tmpConstr.Constructor(),
		)
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

// IsLovelace returns true if this asset is ADA (empty policy)
func (a Asset) IsLovelace() bool {
	return len(a.PolicyId) == 0 && len(a.AssetName) == 0
}

// SupplyPositionDatum represents a user's supply position
// Constructor 0 with fields: owner, marketNft, qTokenAmount, depositSlot
type SupplyPositionDatum struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	Owner        Credential
	MarketNft    Asset
	QTokenAmount uint64
	DepositSlot  uint64
}

func (d *SupplyPositionDatum) UnmarshalCBOR(cborData []byte) error {
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

// BorrowPositionDatum represents a user's borrow position
// Constructor 0 with fields: owner, marketNft, borrowAmount, borrowIndex, borrowSlot
type BorrowPositionDatum struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	Owner        Credential
	MarketNft    Asset
	BorrowAmount uint64
	BorrowIndex  uint64 // Index for interest accrual
	BorrowSlot   uint64
}

func (d *BorrowPositionDatum) UnmarshalCBOR(cborData []byte) error {
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

// Credential represents a user credential
// Constructor 0: PubKeyHash, Constructor 1: ScriptHash
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
	if c.Type != 0 && c.Type != 1 {
		return fmt.Errorf(
			"invalid Credential constructor: expected 0 or 1, got %d",
			c.Type,
		)
	}
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

// IsPubKey returns true if this is a public key credential
func (c *Credential) IsPubKey() bool {
	return c.Type == 0
}

// IsScript returns true if this is a script credential
func (c *Credential) IsScript() bool {
	return c.Type == 1
}

// OracleDatum represents price oracle data for Liqwid markets
// Constructor 0 with fields: asset, price, denominator, validFrom, validTo
type OracleDatum struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	Asset       Asset
	Price       uint64
	Denominator uint64
	ValidFrom   int64 // POSIX timestamp (ms)
	ValidTo     int64 // POSIX timestamp (ms)
}

func (d *OracleDatum) UnmarshalCBOR(cborData []byte) error {
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

// PriceFloat returns the price as a float64
func (d *OracleDatum) PriceFloat() float64 {
	if d.Denominator == 0 {
		return 0
	}
	return float64(d.Price) / float64(d.Denominator)
}

// InterestRateModelDatum represents the interest rate model parameters
// Based on kinked/jump rate model (similar to Compound)
// Constructor 0 with fields
type InterestRateModelDatum struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	BaseRatePerSlot       uint64 // Base interest rate per slot
	MultiplierPerSlot     uint64 // Interest rate slope before kink
	JumpMultiplierPerSlot uint64 // Interest rate slope after kink
	Kink                  uint64 // Utilization rate at which jump applies (basis points)
}

func (d *InterestRateModelDatum) UnmarshalCBOR(cborData []byte) error {
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

// KinkFloat returns the kink point as a decimal (e.g., 0.80 for 80%)
func (d *InterestRateModelDatum) KinkFloat() float64 {
	return float64(d.Kink) / float64(BasisPointsDenom)
}
