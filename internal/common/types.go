// Copyright 2025 Blink Labs Software
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

package common

import (
	"bytes"
	"encoding/hex"
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
)

// AssetClass represents a Cardano native asset identified by policy ID and
// asset name. ADA/lovelace is represented by empty policy ID and name.
type AssetClass struct {
	cbor.StructAsArray
	PolicyId []byte
	Name     []byte
}

// UnmarshalCBOR decodes CBOR data into an AssetClass
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

// MarshalCBOR encodes an AssetClass to CBOR
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
		"AssetClass< policy_id = %s, name = %s >",
		hex.EncodeToString(a.PolicyId),
		hex.EncodeToString(a.Name),
	)
}

// IsLovelace returns true if the AssetClass represents ADA/lovelace
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

// Fingerprint returns a unique identifier for the asset.
// For lovelace, returns "lovelace".
// For native tokens, returns "policyId.assetName" in hex.
func (a AssetClass) Fingerprint() string {
	if a.IsLovelace() {
		return "lovelace"
	}
	return fmt.Sprintf("%s.%s", a.PolicyIdHex(), a.NameHex())
}

// NewAssetClass creates a new AssetClass from hex-encoded policy ID and name.
// Returns an error if the hex strings are invalid.
func NewAssetClass(policyIdHex, nameHex string) (AssetClass, error) {
	policyId, err := hex.DecodeString(policyIdHex)
	if err != nil {
		return AssetClass{}, fmt.Errorf(
			"invalid policy ID hex: %w",
			err,
		)
	}
	name, err := hex.DecodeString(nameHex)
	if err != nil {
		return AssetClass{}, fmt.Errorf("invalid name hex: %w", err)
	}
	return AssetClass{
		PolicyId: policyId,
		Name:     name,
	}, nil
}

// Lovelace returns an AssetClass representing ADA/lovelace
func Lovelace() AssetClass {
	return AssetClass{}
}

// AssetAmount represents an amount of a specific asset
type AssetAmount struct {
	Class  AssetClass
	Amount uint64
}

// IsAsset returns true if the AssetAmount's class matches the given AssetClass
func (a AssetAmount) IsAsset(other AssetClass) bool {
	return bytes.Equal(a.Class.PolicyId, other.PolicyId) &&
		bytes.Equal(a.Class.Name, other.Name)
}

// IsLovelace returns true if the AssetAmount represents ADA/lovelace
func (a AssetAmount) IsLovelace() bool {
	return a.Class.IsLovelace()
}

// String returns a human-readable representation of the AssetAmount
func (a AssetAmount) String() string {
	return fmt.Sprintf(
		"AssetAmount< class = %s, amount = %d >",
		a.Class.String(),
		a.Amount,
	)
}
