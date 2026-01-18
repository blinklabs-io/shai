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

package fluidtokens

import (
	"fmt"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
)

// RentDatum represents the on-chain datum for a FluidTokens rental
type RentDatum struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	OwnerPaymentCred  Credential
	OwnerStakingCred  StakingCredential
	DailyRentPolicy   []byte
	DailyRentAsset    []byte
	DailyRentAmount   uint64
	PoolPolicy        []byte
	PoolAsset         []byte
	PoolAmount        uint64
	PoolDivider       uint64
	TenantPaymentCred Credential
	TenantStakingCred StakingCredential
	DeadlineMs        uint64 // Unix timestamp in milliseconds
	ExpirationMs      uint64 // Unix timestamp in milliseconds
	FluidAddress      DatumAddress
	FeePercent        uint64
	MinDays           uint64
	Multiplier        uint64
}

// Credential represents a payment credential (pub key hash or script hash)
type Credential struct {
	cbor.StructAsArray
	Type int    // 0 = pub key hash, 1 = script hash
	Hash []byte // 28 bytes
}

func (c *Credential) UnmarshalCBOR(cborData []byte) error {
	var tmpConstr cbor.Constructor
	if _, err := cbor.Decode(cborData, &tmpConstr); err != nil {
		return err
	}
	c.Type = int(tmpConstr.Constructor())
	var hashWrapper struct {
		cbor.StructAsArray
		Hash []byte
	}
	if err := cbor.DecodeGeneric(
		tmpConstr.FieldsCbor(),
		&hashWrapper,
	); err != nil {
		return err
	}
	c.Hash = hashWrapper.Hash
	return nil
}

// StakingCredential represents an optional staking credential
type StakingCredential struct {
	cbor.StructAsArray
	IsPresent bool
	Cred      Credential
}

func (s *StakingCredential) UnmarshalCBOR(cborData []byte) error {
	var tmpConstr cbor.Constructor
	if _, err := cbor.Decode(cborData, &tmpConstr); err != nil {
		return err
	}
	// Constructor 0 = Some, Constructor 1 = None
	if tmpConstr.Constructor() == 1 {
		s.IsPresent = false
		return nil
	}
	s.IsPresent = true
	// The credential is wrapped inside
	var wrapper struct {
		cbor.StructAsArray
		Inner Credential
	}
	if err := cbor.DecodeGeneric(
		tmpConstr.FieldsCbor(),
		&wrapper,
	); err != nil {
		return err
	}
	s.Cred = wrapper.Inner
	return nil
}

// DatumAddress represents a Cardano address in datum form
type DatumAddress struct {
	cbor.StructAsArray
	PaymentCred Credential
	StakingCred StakingCredential
}

func (a *DatumAddress) UnmarshalCBOR(cborData []byte) error {
	var tmpConstr cbor.Constructor
	if _, err := cbor.Decode(cborData, &tmpConstr); err != nil {
		return err
	}
	return cbor.DecodeGeneric(tmpConstr.FieldsCbor(), a)
}

func (r *RentDatum) UnmarshalCBOR(cborData []byte) error {
	r.SetCbor(cborData)
	var tmpConstr cbor.Constructor
	if _, err := cbor.Decode(cborData, &tmpConstr); err != nil {
		return err
	}
	return cbor.DecodeGeneric(tmpConstr.FieldsCbor(), r)
}

// Deadline returns the deadline as a time.Time
func (r *RentDatum) Deadline() time.Time {
	return time.UnixMilli(int64(r.DeadlineMs))
}

// Expiration returns the expiration as a time.Time
func (r *RentDatum) Expiration() time.Time {
	return time.UnixMilli(int64(r.ExpirationMs))
}

// IsExpired returns true if the rental has passed its deadline
func (r *RentDatum) IsExpired() bool {
	return time.Now().After(r.Deadline())
}

// IsLent returns true if the rental is currently lent
// (owner != tenant)
func (r *RentDatum) IsLent() bool {
	return string(r.OwnerPaymentCred.Hash) != string(r.TenantPaymentCred.Hash)
}

// CanBeReturned returns true if the rental can be liquidated
// (expired and currently lent)
func (r *RentDatum) CanBeReturned() bool {
	return r.IsExpired() && r.IsLent()
}

func (r RentDatum) String() string {
	return fmt.Sprintf(
		"RentDatum< owner = %x, tenant = %x, deadline = %s, expired = %v, lent = %v >",
		r.OwnerPaymentCred.Hash,
		r.TenantPaymentCred.Hash,
		r.Deadline().Format(time.RFC3339),
		r.IsExpired(),
		r.IsLent(),
	)
}

// ReturnRedeemer is the redeemer used for returning NFTs
// Constructor 4 with batch index
type ReturnRedeemer struct {
	cbor.StructAsArray
	BatchIndex uint64
}

func (r *ReturnRedeemer) MarshalCBOR() ([]byte, error) {
	tmpConstr := cbor.NewConstructor(
		4,
		cbor.IndefLengthList{
			r.BatchIndex,
		},
	)
	return cbor.Encode(&tmpConstr)
}

func NewReturnRedeemer(batchIndex uint64) *ReturnRedeemer {
	return &ReturnRedeemer{BatchIndex: batchIndex}
}
