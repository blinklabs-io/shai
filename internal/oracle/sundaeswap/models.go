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

// Package sundaeswap provides datum types and parsing for SundaeSwap DEX protocol.
package sundaeswap

import (
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/shai/internal/common"
)

// Protocol constants
const (
	ProtocolName = "sundaeswap"

	// V1 Constants (mainnet)
	// V1 was closed source Plutus V1 contracts from 2021
	V1PoolScriptHash = "4020e7fc2de75a0729c3cc3af715b34d98381e0c5a5b06f9c38c2ccd"

	// V3 Constants (mainnet)
	V3PoolScriptHash = "e0302560ced2fdcbfcb2602697df970cd0d6a38f94b32703f51c312b"

	// Fee denominator for SundaeSwap (basis points)
	FeeDenom = 10000

	// V1 default fee (0.3% = 30 basis points for each side)
	V1DefaultFee = 30
)

// V3PoolDatum represents the SundaeSwap V3 pool datum structure
// Constructor 0 with fields:
// - identifier: ByteArray (28 bytes)
// - assets: ((PolicyId, AssetName), (PolicyId, AssetName))
// - circulatingLp: Int
// - bidFeesPer10Thousand: Int
// - askFeesPer10Thousand: Int
// - feeManager: Optional<MultisigScript>
// - marketOpen: Int (POSIX timestamp)
// - protocolFees: Int
type V3PoolDatum struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	Identifier           []byte
	Assets               AssetPair
	CirculatingLp        uint64
	BidFeesPer10Thousand uint64
	AskFeesPer10Thousand uint64
	FeeManager           OptionalMultisig
	MarketOpen           int64
	ProtocolFees         uint64
}

func (d *V3PoolDatum) UnmarshalCBOR(cborData []byte) error {
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

// AssetPair represents a pair of assets in the pool
type AssetPair struct {
	cbor.StructAsArray
	AssetA Asset
	AssetB Asset
}

func (p *AssetPair) UnmarshalCBOR(cborData []byte) error {
	var tmpConstr cbor.Constructor
	if _, err := cbor.Decode(cborData, &tmpConstr); err != nil {
		return err
	}
	return cbor.DecodeGeneric(tmpConstr.FieldsCbor(), p)
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

// OptionalMultisig represents an optional multisig script
type OptionalMultisig struct {
	IsPresent bool
	// We don't need to parse the actual multisig for oracle purposes
}

func (o *OptionalMultisig) UnmarshalCBOR(cborData []byte) error {
	var tmpConstr cbor.Constructor
	if _, err := cbor.Decode(cborData, &tmpConstr); err != nil {
		return err
	}
	// Constructor 0 = Some, Constructor 1 = None
	o.IsPresent = tmpConstr.Constructor() == 0
	return nil
}

// V1PoolDatum represents the SundaeSwap V1 pool datum structure
// V1 was closed source but based on SDK interfaces and on-chain analysis:
// Constructor 0 with fields:
// - ident: ByteArray (pool identifier, typically 28 bytes from pool NFT)
// - assetA: (PolicyId, AssetName)
// - assetB: (PolicyId, AssetName)
// - circulatingLp: Int (total LP tokens in circulation)
// - feeNumerator: Int (fee in basis points, typically 30 for 0.3%)
type V1PoolDatum struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	Ident         []byte
	AssetA        Asset
	AssetB        Asset
	CirculatingLp uint64
	FeeNumerator  uint64
}

func (d *V1PoolDatum) UnmarshalCBOR(cborData []byte) error {
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

// GetPoolIdent returns the pool identifier as hex string
func (d *V1PoolDatum) GetPoolIdent() string {
	return fmt.Sprintf("%x", d.Ident)
}
