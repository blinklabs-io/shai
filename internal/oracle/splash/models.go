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

// Package splash provides datum types and parsing for Splash DEX protocol.
package splash

import (
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/shai/internal/common"
)

// Protocol constants
const (
	ProtocolName = "splash"

	// V1 Constants (mainnet)
	// Splash V1 pool script hash - to be confirmed with actual deployment
	V1PoolScriptHash = ""

	// Fee denominator for Splash (basis points)
	FeeDenom = 10000
)

// V1PoolDatum represents the Splash V1 BasicPoolDatum structure
// Based on cardano-datum-registry:
// - poolNft: Asset (policyId: bytes, tokenName: bytes)
// - poolX: Asset
// - poolY: Asset
// - poolLq: Asset (LP token)
// - poolFeeNum: int (fee in basis points, 10000 = 100%)
// - unspecifiedField: BytesSingletonArray (single-element array of bytes)
// - nonce: int
type V1PoolDatum struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	PoolNft          Asset
	PoolX            Asset
	PoolY            Asset
	PoolLq           Asset
	PoolFeeNum       uint64
	UnspecifiedField BytesSingletonArray
	Nonce            int64
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

// GetPoolNftHex returns the pool NFT identifier as hex string
func (d *V1PoolDatum) GetPoolNftHex() string {
	return fmt.Sprintf("%x%x", d.PoolNft.PolicyId, d.PoolNft.TokenName)
}

// Asset represents an asset (PolicyId, TokenName) tuple for Splash protocol
type Asset struct {
	cbor.StructAsArray
	PolicyId  []byte
	TokenName []byte
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
		Name:     a.TokenName,
	}
}

// BytesSingletonArray represents a single-element array of bytes
type BytesSingletonArray struct {
	Data []byte
}

func (b *BytesSingletonArray) UnmarshalCBOR(cborData []byte) error {
	// Decode as array with single element
	var arr [][]byte
	if _, err := cbor.Decode(cborData, &arr); err != nil {
		return err
	}
	if len(arr) > 0 {
		b.Data = arr[0]
	}
	return nil
}
