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

// Package optim provides datum types and parsing for Optim Finance liquidity
// bonds protocol. Optim Finance allows users to borrow ADA staking rights for
// fixed periods through liquidity bonds, and provides staked ADA derivatives
// (OADA/sOADA).
//
// See: https://www.optim.finance/
// GitHub: https://github.com/OptimFinance (leviathan for Aiken contracts, ply
// for Plutus)
package optim

import (
	"github.com/blinklabs-io/gouroboros/cbor"
)

// Protocol constants
const (
	ProtocolName = "optim"

	// Bond status values
	BondStatusActive  uint64 = 0 // Bond is active, staking rights in effect
	BondStatusMatured uint64 = 1 // Bond has matured, awaiting claim
	BondStatusClaimed uint64 = 2 // Bond has been claimed/closed

	// Interest rate denominator (basis points: 10000 = 100%)
	InterestRateDenom = 10000
)

// BondDatum represents an Optim Finance liquidity bond datum
// The bond allows lenders to provide ADA principal while borrowers acquire
// staking rights for a fixed duration.
//
// CDDL (estimated based on Optim documentation):
// BondDatum = #6.121([
//
//	  bondNFT         : bytes        ; NFT identifying this bond
//	, lenderAddress   : Address      ; Who lent the ADA
//	, borrowerNFT     : bytes        ; NFT given to borrower for staking rights
//	, principalAmount : int          ; ADA principal locked
//	, interestRate    : int          ; Interest rate in basis points
//	, duration        : int          ; Bond duration in epochs or slots
//	, startEpoch      : int          ; When bond started
//	, endEpoch        : int          ; When bond matures
//	, stakePool       : bytes        ; Pool ID for staking (28 bytes)
//	, accruedRewards  : int          ; Rewards accrued so far
//	, status          : int          ; 0=active, 1=matured, 2=claimed
//	])
type BondDatum struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	BondNFT         []byte  // NFT identifying this bond
	LenderAddress   Address // Who lent the ADA
	BorrowerNFT     []byte  // NFT given to borrower for staking rights
	PrincipalAmount uint64  // ADA principal locked (in lovelace)
	InterestRate    uint64  // Interest rate in basis points
	Duration        uint64  // Bond duration in epochs or slots
	StartEpoch      uint64  // When bond started
	EndEpoch        uint64  // When bond matures
	StakePool       []byte  // Pool ID for staking (28 bytes)
	AccruedRewards  uint64  // Rewards accrued so far
	Status          uint64  // 0=active, 1=matured, 2=claimed
}

func (d *BondDatum) UnmarshalCBOR(cborData []byte) error {
	d.SetCbor(cborData)

	var tmpConstr cbor.Constructor
	if _, err := cbor.Decode(cborData, &tmpConstr); err != nil {
		return err
	}

	// Bond datum uses constructor 0 (#6.121)
	if tmpConstr.Constructor() != 0 {
		return nil // Not a bond datum
	}

	var fields struct {
		cbor.StructAsArray
		BondNFT         []byte
		LenderAddress   Address
		BorrowerNFT     []byte
		PrincipalAmount uint64
		InterestRate    uint64
		Duration        uint64
		StartEpoch      uint64
		EndEpoch        uint64
		StakePool       []byte
		AccruedRewards  uint64
		Status          uint64
	}

	if err := cbor.DecodeGeneric(tmpConstr.FieldsCbor(), &fields); err != nil {
		return err
	}

	d.BondNFT = fields.BondNFT
	d.LenderAddress = fields.LenderAddress
	d.BorrowerNFT = fields.BorrowerNFT
	d.PrincipalAmount = fields.PrincipalAmount
	d.InterestRate = fields.InterestRate
	d.Duration = fields.Duration
	d.StartEpoch = fields.StartEpoch
	d.EndEpoch = fields.EndEpoch
	d.StakePool = fields.StakePool
	d.AccruedRewards = fields.AccruedRewards
	d.Status = fields.Status

	return nil
}

// IsActive returns true if the bond is still active
func (d *BondDatum) IsActive() bool {
	return d.Status == BondStatusActive
}

// IsMatured returns true if the bond has matured
func (d *BondDatum) IsMatured() bool {
	return d.Status == BondStatusMatured
}

// IsClaimed returns true if the bond has been claimed
func (d *BondDatum) IsClaimed() bool {
	return d.Status == BondStatusClaimed
}

// InterestRatePercent returns the interest rate as a percentage
func (d *BondDatum) InterestRatePercent() float64 {
	return float64(d.InterestRate) / float64(InterestRateDenom) * 100
}

// Address represents a Cardano address in the Optim datum
// Address = #6.121([ paymentCredential : Credential
//
//	, stakeCredential   : MaybeStakeCredential
//	])
type Address struct {
	cbor.StructAsArray
	PaymentCredential Credential
	StakeCredential   MaybeStakeCredential
}

func (a *Address) UnmarshalCBOR(cborData []byte) error {
	var tmpConstr cbor.Constructor
	if _, err := cbor.Decode(cborData, &tmpConstr); err != nil {
		return err
	}
	if tmpConstr.Constructor() != 0 {
		return nil
	}
	return cbor.DecodeGeneric(tmpConstr.FieldsCbor(), a)
}

// Credential represents either a VerificationKeyCredential or ScriptCredential
// VerificationKeyCredential = #6.121([bytes])
// ScriptCredential = #6.122([bytes])
type Credential struct {
	cbor.StructAsArray
	Type CredentialType
	Hash []byte
}

// CredentialType indicates the type of credential
type CredentialType int

const (
	CredentialTypeVerificationKey CredentialType = 0 // Constructor 0 (#6.121)
	CredentialTypeScript          CredentialType = 1 // Constructor 1 (#6.122)
)

func (c *Credential) UnmarshalCBOR(cborData []byte) error {
	var tmpConstr cbor.Constructor
	if _, err := cbor.Decode(cborData, &tmpConstr); err != nil {
		return err
	}
	c.Type = CredentialType(tmpConstr.Constructor())
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

// IsScript returns true if this is a script credential
func (c *Credential) IsScript() bool {
	return c.Type == CredentialTypeScript
}

// MaybeStakeCredential represents an optional stake credential
// Some = #6.121([ StakeCredential ])
// None = #6.122([])
type MaybeStakeCredential struct {
	IsPresent       bool
	StakeCredential StakeCredential
}

func (m *MaybeStakeCredential) UnmarshalCBOR(cborData []byte) error {
	var tmpConstr cbor.Constructor
	if _, err := cbor.Decode(cborData, &tmpConstr); err != nil {
		return err
	}
	if tmpConstr.Constructor() == 1 { // None
		m.IsPresent = false
		return nil
	}
	m.IsPresent = true
	var wrapper struct {
		cbor.StructAsArray
		StakeCredential StakeCredential
	}
	if err := cbor.DecodeGeneric(tmpConstr.FieldsCbor(), &wrapper); err != nil {
		return err
	}
	m.StakeCredential = wrapper.StakeCredential
	return nil
}

// StakeCredential represents a stake credential (inline or pointer)
// Inline = #6.121([ Credential ])
// Pointer = #6.122([ slotNumber, transactionIndex, certificateIndex ])
type StakeCredential struct {
	IsInline   bool
	Credential Credential
	// Pointer fields (only if !IsInline)
	SlotNumber       int64
	TransactionIndex int64
	CertificateIndex int64
}

func (s *StakeCredential) UnmarshalCBOR(cborData []byte) error {
	var tmpConstr cbor.Constructor
	if _, err := cbor.Decode(cborData, &tmpConstr); err != nil {
		return err
	}
	if tmpConstr.Constructor() == 0 { // Inline
		s.IsInline = true
		var wrapper struct {
			cbor.StructAsArray
			Credential Credential
		}
		if err := cbor.DecodeGeneric(tmpConstr.FieldsCbor(), &wrapper); err != nil {
			return err
		}
		s.Credential = wrapper.Credential
		return nil
	}
	// Pointer
	s.IsInline = false
	var wrapper struct {
		cbor.StructAsArray
		SlotNumber       int64
		TransactionIndex int64
		CertificateIndex int64
	}
	if err := cbor.DecodeGeneric(tmpConstr.FieldsCbor(), &wrapper); err != nil {
		return err
	}
	s.SlotNumber = wrapper.SlotNumber
	s.TransactionIndex = wrapper.TransactionIndex
	s.CertificateIndex = wrapper.CertificateIndex
	return nil
}

// OADADatum represents the OADA staked ADA derivative token datum
// OADA tokens represent staked ADA that accrues yield automatically.
//
// CDDL (estimated):
// OADADatum = #6.121([
//
//	  totalStaked     : int          ; Total ADA staked
//	, exchangeRate    : Rational     ; OADA to ADA exchange rate
//	, lastUpdateEpoch : int          ; Last update epoch
//	, totalOADA       : int          ; Total OADA minted
//	])
type OADADatum struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	TotalStaked     uint64   // Total ADA staked in lovelace
	ExchangeRate    Rational // OADA to ADA exchange rate
	LastUpdateEpoch uint64   // Last update epoch
	TotalOADA       uint64   // Total OADA minted
}

func (d *OADADatum) UnmarshalCBOR(cborData []byte) error {
	d.SetCbor(cborData)

	var tmpConstr cbor.Constructor
	if _, err := cbor.Decode(cborData, &tmpConstr); err != nil {
		return err
	}

	if tmpConstr.Constructor() != 0 {
		return nil
	}

	var fields struct {
		cbor.StructAsArray
		TotalStaked     uint64
		ExchangeRate    Rational
		LastUpdateEpoch uint64
		TotalOADA       uint64
	}

	if err := cbor.DecodeGeneric(tmpConstr.FieldsCbor(), &fields); err != nil {
		return err
	}

	d.TotalStaked = fields.TotalStaked
	d.ExchangeRate = fields.ExchangeRate
	d.LastUpdateEpoch = fields.LastUpdateEpoch
	d.TotalOADA = fields.TotalOADA

	return nil
}

// ExchangeRateFloat returns the exchange rate as a float64
func (d *OADADatum) ExchangeRateFloat() float64 {
	return d.ExchangeRate.Float64()
}

// Rational represents a rational number (numerator/denominator)
// Rational = #6.121([ numerator : int, denominator : int ])
type Rational struct {
	cbor.StructAsArray
	Numerator   uint64
	Denominator uint64
}

func (r *Rational) UnmarshalCBOR(cborData []byte) error {
	var tmpConstr cbor.Constructor
	if _, err := cbor.Decode(cborData, &tmpConstr); err != nil {
		return err
	}
	return cbor.DecodeGeneric(tmpConstr.FieldsCbor(), r)
}

// Float64 returns the rational as a float64
func (r Rational) Float64() float64 {
	if r.Denominator == 0 {
		return 0
	}
	return float64(r.Numerator) / float64(r.Denominator)
}
