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

// Package common provides shared types used across multiple packages
package common

import (
	"bytes"
	"encoding/hex"
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
)

// AssetClass represents a Cardano native asset (policy ID + asset name)
type AssetClass struct {
	cbor.StructAsArray
	PolicyId []byte
	Name     []byte
}

// UnmarshalCBOR implements cbor.Unmarshaler for AssetClass
func (a *AssetClass) UnmarshalCBOR(cborData []byte) error {
	var tmpConstr cbor.Constructor
	if _, err := cbor.Decode(cborData, &tmpConstr); err != nil {
		return err
	}
	return cbor.DecodeGeneric(
		tmpConstr.FieldsCbor(),
		a,
	)
}

// MarshalCBOR implements cbor.Marshaler for AssetClass
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

// String returns a human-readable representation of the AssetClass
func (a AssetClass) String() string {
	return fmt.Sprintf(
		"AssetClass< name = %s, policy_id = %s >",
		a.Name,
		hex.EncodeToString(a.PolicyId),
	)
}

// IsLovelace returns true if this is ADA (empty policy ID and name)
func (a AssetClass) IsLovelace() bool {
	return len(a.PolicyId) == 0 && len(a.Name) == 0
}

// PolicyIdHex returns the policy ID as a hex string
func (a AssetClass) PolicyIdHex() string {
	return hex.EncodeToString(a.PolicyId)
}

// NameHex returns the asset name as a hex string
func (a AssetClass) NameHex() string {
	return hex.EncodeToString(a.Name)
}

// Fingerprint returns a unique identifier for this asset class
func (a AssetClass) Fingerprint() string {
	if a.IsLovelace() {
		return "lovelace"
	}
	return hex.EncodeToString(a.PolicyId) + "." + hex.EncodeToString(a.Name)
}

// AssetAmount represents an asset class with an amount
type AssetAmount struct {
	Class  AssetClass
	Amount uint64
}

// IsAsset checks if this amount is for the given asset class
func (a AssetAmount) IsAsset(asset AssetClass) bool {
	return bytes.Equal(asset.PolicyId, a.Class.PolicyId) &&
		bytes.Equal(asset.Name, a.Class.Name)
}

// IsLovelace returns true if this is ADA
func (a AssetAmount) IsLovelace() bool {
	return a.Class.IsLovelace()
}

// String returns a human-readable representation
func (a AssetAmount) String() string {
	return fmt.Sprintf("%d %s", a.Amount, a.Class.Fingerprint())
}

// NewAssetClass creates a new AssetClass from hex strings
func NewAssetClass(policyIdHex, nameHex string) (AssetClass, error) {
	policyId, err := hex.DecodeString(policyIdHex)
	if err != nil {
		return AssetClass{}, fmt.Errorf("invalid policy ID hex: %w", err)
	}
	name, err := hex.DecodeString(nameHex)
	if err != nil {
		return AssetClass{}, fmt.Errorf("invalid asset name hex: %w", err)
	}
	return AssetClass{PolicyId: policyId, Name: name}, nil
}

// Lovelace returns the AssetClass representing ADA
func Lovelace() AssetClass {
	return AssetClass{}
}
