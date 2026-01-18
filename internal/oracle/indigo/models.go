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

// Package indigo provides datum types and parsing for Indigo CDP protocol.
package indigo

import (
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
)

// Protocol constants
const (
	ProtocolName = "indigo"

	// Indigo mainnet CDP contract address
	// Script hash: e4d2fb0b8d275852103fd75801e2c7dcf6ed3e276c74cabadbe5b8b6
	CDPContractAddress = "addr1z8jd97ct35n4s5ss8lt4sq0zclw0dmf7yak8fj46m0jm3dhzfjvtm0pg7ms9fvw5luec6euwzku8wqjpt5gv0q86052qv9nxuw"

	// Indigo Stability Pool contract address (mainnet)
	// Script hash: de1585e046f16fdf79767300233c1affbe9d30340656acfde45e9142
	StabilityPoolAddress = "addr1w80ptp0qgmcklhmeweesqgeurtlma8fsxsr9dt8au30fzss0czhl9"

	// iAsset minting policy ID - shared by all iAssets (iUSD, iBTC, iETH)
	IAssetPolicyID = "f66d78b4a3cb3d37afa0ec36461e51ecbde00f26c8f0a68f94b69880"

	// INDY governance token policy ID
	INDYPolicyID = "533bb94a8850ee3ccbe483106489399112b74c905342cb1792a797a0"
)

// CDPContentDatum represents the Indigo CDP datum wrapper
// CDDL: CDPContent = #6.121([#6.121([ owner, iAsset, mintedAmount, accumulatedFees ])])
// This is a double-wrapped constructor (outer #6.121, inner #6.121)
type CDPContentDatum struct {
	cbor.DecodeStoreCbor
	Inner *CDPInner
}

func (d *CDPContentDatum) UnmarshalCBOR(cborData []byte) error {
	d.SetCbor(cborData)

	// Parse outer constructor (#6.121)
	var outerConstr cbor.Constructor
	if _, err := cbor.Decode(cborData, &outerConstr); err != nil {
		return err
	}

	// Outer constructor must be 0 (corresponds to #6.121)
	if outerConstr.Constructor() != 0 {
		return nil // Not a CDP content datum
	}

	// The outer constructor wraps a single field which is the inner constructor
	// FieldsCbor() returns the CBOR of the array containing the inner constructor
	// We need to decode it as a wrapper struct to extract the inner constructor
	var wrapper struct {
		cbor.StructAsArray
		Inner CDPInner
	}
	if err := cbor.DecodeGeneric(outerConstr.FieldsCbor(), &wrapper); err != nil {
		return err
	}
	d.Inner = &wrapper.Inner

	return nil
}

// CDPInner represents the inner CDP data
// CDDL: #6.121([ owner, iAsset, mintedAmount, accumulatedFees ])
// Fields: owner (MaybePubKeyHash), iAsset (bytes), mintedAmount (int),
// accumulatedFees (AccumulatedFees)
type CDPInner struct {
	cbor.StructAsArray
	Owner           MaybePubKeyHash
	IAsset          []byte
	MintedAmount    int64
	AccumulatedFees AccumulatedFees
}

func (c *CDPInner) UnmarshalCBOR(cborData []byte) error {
	// The inner is wrapped in a constructor (#6.121)
	var tmpConstr cbor.Constructor
	if _, err := cbor.Decode(cborData, &tmpConstr); err != nil {
		return err
	}

	// Inner constructor must be 0 (corresponds to #6.121)
	if tmpConstr.Constructor() != 0 {
		return fmt.Errorf(
			"unexpected CDP inner constructor: %d, expected 0",
			tmpConstr.Constructor(),
		)
	}

	// Decode the fields array
	var fields struct {
		cbor.StructAsArray
		Owner           MaybePubKeyHash
		IAsset          []byte
		MintedAmount    int64
		AccumulatedFees AccumulatedFees
	}
	if err := cbor.DecodeGeneric(tmpConstr.FieldsCbor(), &fields); err != nil {
		return err
	}

	c.Owner = fields.Owner
	c.IAsset = fields.IAsset
	c.MintedAmount = fields.MintedAmount
	c.AccumulatedFees = fields.AccumulatedFees

	return nil
}

// MaybePubKeyHash represents an optional public key hash
// CDDL: MaybePubKeyHash = PubKeyHash / Nothing
// PubKeyHash = #6.121([bytes])   -> Constructor 0
// Nothing = #6.122([])           -> Constructor 1
type MaybePubKeyHash struct {
	IsJust bool   // true if PubKeyHash, false if Nothing
	Hash   []byte // Only populated if IsJust is true
}

func (m *MaybePubKeyHash) UnmarshalCBOR(cborData []byte) error {
	var tmpConstr cbor.Constructor
	if _, err := cbor.Decode(cborData, &tmpConstr); err != nil {
		return err
	}

	switch tmpConstr.Constructor() {
	case 0: // PubKeyHash (#6.121)
		m.IsJust = true
		var wrapper struct {
			cbor.StructAsArray
			Hash []byte
		}
		if err := cbor.DecodeGeneric(tmpConstr.FieldsCbor(), &wrapper); err != nil {
			return err
		}
		m.Hash = wrapper.Hash
	case 1: // Nothing (#6.122)
		m.IsJust = false
		m.Hash = nil
	default:
		return fmt.Errorf(
			"unexpected MaybePubKeyHash constructor: %d",
			tmpConstr.Constructor(),
		)
	}

	return nil
}

// AccumulatedFees represents the fees structure
// CDDL: AccumulatedFees = InterestIAssetAmount / FeesLovelacesAmount
// InterestIAssetAmount = #6.121([ lastUpdated : int, iAssetAmount : int ])
// FeesLovelacesAmount = #6.122([ treasury : int, indyStakers : int ])
type AccumulatedFees struct {
	Type int // 0 = InterestIAssetAmount, 1 = FeesLovelacesAmount

	// InterestIAssetAmount fields (Type == 0)
	LastUpdated  int64
	IAssetAmount int64

	// FeesLovelacesAmount fields (Type == 1)
	Treasury    int64
	IndyStakers int64
}

func (a *AccumulatedFees) UnmarshalCBOR(cborData []byte) error {
	var tmpConstr cbor.Constructor
	if _, err := cbor.Decode(cborData, &tmpConstr); err != nil {
		return err
	}

	a.Type = int(tmpConstr.Constructor())

	switch a.Type {
	case 0: // InterestIAssetAmount (#6.121)
		var interest struct {
			cbor.StructAsArray
			LastUpdated  int64
			IAssetAmount int64
		}
		if err := cbor.DecodeGeneric(tmpConstr.FieldsCbor(), &interest); err != nil {
			return err
		}
		a.LastUpdated = interest.LastUpdated
		a.IAssetAmount = interest.IAssetAmount

	case 1: // FeesLovelacesAmount (#6.122)
		var fees struct {
			cbor.StructAsArray
			Treasury    int64
			IndyStakers int64
		}
		if err := cbor.DecodeGeneric(tmpConstr.FieldsCbor(), &fees); err != nil {
			return err
		}
		a.Treasury = fees.Treasury
		a.IndyStakers = fees.IndyStakers
	default:
		return fmt.Errorf(
			"unexpected AccumulatedFees constructor: %d",
			a.Type,
		)
	}

	return nil
}
