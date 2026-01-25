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

package vyfi

import (
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/shai/internal/common"
)

// PoolState represents the state of a VyFi liquidity pool
type PoolState struct {
	PoolId    string
	Protocol  string
	AssetX    common.AssetAmount
	AssetY    common.AssetAmount
	FeeNum    uint64
	FeeDenom  uint64
	Shares    uint64 // Total LP shares
	Slot      uint64
	TxHash    string
	TxIndex   uint32
	Timestamp time.Time
}

// PriceXY returns the price of X in terms of Y (Y per X)
func (p PoolState) PriceXY() float64 {
	if p.AssetX.Amount == 0 {
		return 0.0
	}
	return float64(p.AssetY.Amount) / float64(p.AssetX.Amount)
}

// Parser implements pool parsing for VyFi
type Parser struct{}

// NewParser creates a parser for VyFi pools
func NewParser() *Parser {
	return &Parser{}
}

// Protocol returns the protocol name
func (p *Parser) Protocol() string {
	return ProtocolName
}

// ParsePoolDatum parses a VyFi pool datum
// For VyFi, assets are determined from the UTXO value since the datum doesn't contain asset identifiers
func (p *Parser) ParsePoolDatum(
	datum []byte,
	utxoValue []byte,
	txHash string,
	txIndex uint32,
	slot uint64,
	timestamp time.Time,
) (*PoolState, error) {
	var poolDatum PoolDatum
	if _, err := cbor.Decode(datum, &poolDatum); err != nil {
		return nil, fmt.Errorf("failed to decode VyFi datum: %w", err)
	}

	// Extract assets from UTXO value
	assetA, assetB, poolNFT, err := p.extractAssetsFromUtxoValue(utxoValue)
	if err != nil {
		return nil, fmt.Errorf("failed to extract assets from UTXO: %w", err)
	}

	// Generate pool ID from the pool NFT
	poolId := GeneratePoolId(poolNFT, assetA, assetB)

	// VyFi uses 0.3% swap fee (standard AMM fee)
	state := &PoolState{
		PoolId:   poolId,
		Protocol: p.Protocol(),
		AssetX: common.AssetAmount{
			Class:  assetA,
			Amount: poolDatum.TreasuryA,
		},
		AssetY: common.AssetAmount{
			Class:  assetB,
			Amount: poolDatum.TreasuryB,
		},
		FeeNum:    997, // 0.3% fee
		FeeDenom:  1000,
		Shares:    poolDatum.IssuedShares,
		Slot:      slot,
		TxHash:    txHash,
		TxIndex:   txIndex,
		Timestamp: timestamp,
	}

	return state, nil
}

// extractAssetsFromUtxoValue parses the UTXO value to extract ADA, token, and NFT
// For VyFi pools: ADA + token + NFT (pool identifier)
func (p *Parser) extractAssetsFromUtxoValue(utxoValue []byte) (assetA, assetB common.AssetClass, poolNFT string, err error) {
	// Parse the CBOR-encoded UTXO value
	// UTXO value is a map from asset ID to amount
	var value map[string]uint64
	if _, err := cbor.Decode(utxoValue, &value); err != nil {
		return common.AssetClass{}, common.AssetClass{}, "", fmt.Errorf("failed to decode UTXO value: %w", err)
	}

	var adaAmount uint64
	var nonAdaAssets []struct {
		assetId string
		amount  uint64
	}

	for assetId, amount := range value {
		if assetId == "lovelace" {
			adaAmount = amount
		} else {
			nonAdaAssets = append(nonAdaAssets, struct {
				assetId string
				amount  uint64
			}{assetId, amount})
		}
	}

	if adaAmount == 0 {
		return common.AssetClass{}, common.AssetClass{}, "", fmt.Errorf("no ADA found in UTXO")
	}

	// Validate asset composition: exactly one NFT (amount==1) and exactly one token (amount>1)
	var nftCount, tokenCount int
	var nftAsset, tokenAsset string

	for _, asset := range nonAdaAssets {
		if asset.amount == 1 {
			nftCount++
			nftAsset = asset.assetId
		} else if asset.amount > 1 {
			tokenCount++
			tokenAsset = asset.assetId
		} else {
			// This shouldn't happen as amounts should be positive, but handle it
			return common.AssetClass{}, common.AssetClass{}, "", fmt.Errorf("invalid asset amount 0 for asset %s", asset.assetId)
		}
	}

	if nftCount == 0 {
		return common.AssetClass{}, common.AssetClass{}, "", fmt.Errorf("no NFT found in UTXO")
	}
	if nftCount > 1 {
		return common.AssetClass{}, common.AssetClass{}, "", fmt.Errorf("multiple NFTs found in UTXO (%d), expected exactly one", nftCount)
	}
	if tokenCount == 0 {
		return common.AssetClass{}, common.AssetClass{}, "", fmt.Errorf("no token found in UTXO")
	}
	if tokenCount > 1 {
		return common.AssetClass{}, common.AssetClass{}, "", fmt.Errorf("multiple tokens found in UTXO (%d), expected exactly one", tokenCount)
	}

	// Parse token asset ID
	tokenClass, err := p.parseAssetId(tokenAsset)
	if err != nil {
		return common.AssetClass{}, common.AssetClass{}, "", fmt.Errorf("failed to parse token asset: %w", err)
	}

	// ADA is always asset A
	adaClass := common.AssetClass{
		PolicyId: []byte{},
		Name:     []byte{},
	}

	// For VyFi, ADA is typically AssetA and token is AssetB
	return adaClass, tokenClass, nftAsset, nil
}

// parseAssetId parses an asset ID string into policy ID and name
// Format: {policyId}.{name} or just {policyId} for NFTs with empty name
// Also supports concatenated hex format: {policyId}{name}
func (p *Parser) parseAssetId(assetId string) (common.AssetClass, error) {
	var policyIdHex, nameHex string

	// Check if the asset ID contains a dot (dot-delimited format)
	if dotIndex := strings.Index(assetId, "."); dotIndex != -1 {
		// Dot-delimited format: policyId.name
		policyIdHex = assetId[:dotIndex]
		nameHex = assetId[dotIndex+1:]
	} else {
		// Concatenated hex format: policyId + assetName
		if len(assetId) < 56 { // 56 chars = 28 bytes policy ID
			return common.AssetClass{}, fmt.Errorf("asset ID too short: %s", assetId)
		}
		policyIdHex = assetId[:56] // First 56 chars (28 bytes)
		nameHex = assetId[56:]     // Rest is asset name
	}

	policyId, err := hex.DecodeString(policyIdHex)
	if err != nil {
		return common.AssetClass{}, fmt.Errorf("failed to decode policy ID: %w", err)
	}

	name, err := hex.DecodeString(nameHex)
	if err != nil {
		return common.AssetClass{}, fmt.Errorf("failed to decode asset name: %w", err)
	}

	return common.AssetClass{
		PolicyId: policyId,
		Name:     name,
	}, nil
}

// ParsePoolDatumSimple parses a VyFi pool datum without asset context
// Returns the raw datum values for further processing
func (p *Parser) ParsePoolDatumSimple(datum []byte) (*PoolDatum, error) {
	var poolDatum PoolDatum
	if _, err := cbor.Decode(datum, &poolDatum); err != nil {
		return nil, fmt.Errorf("failed to decode VyFi pool datum: %w", err)
	}
	return &poolDatum, nil
}

// GeneratePoolId generates a unique pool ID
// VyFi pools are typically identified by their NFT
func GeneratePoolId(poolNFT string, assetA, assetB common.AssetClass) string {
	if poolNFT != "" {
		return fmt.Sprintf("vyfi_%s", poolNFT)
	}
	// Fallback to asset pair identification
	return fmt.Sprintf(
		"vyfi_%s.%s_%s.%s",
		hex.EncodeToString(assetA.PolicyId),
		hex.EncodeToString(assetA.Name),
		hex.EncodeToString(assetB.PolicyId),
		hex.EncodeToString(assetB.Name),
	)
}

// GetPoolAddresses returns mainnet pool addresses
// VyFi DEX launched on May 15, 2023 on Cardano mainnet
// See: https://docs.vyfi.io/ and https://app.vyfi.io/
func GetPoolAddresses() []string {
	return []string{
		// VyFi pool script address (mainnet)
		// Script hash: 588fd5e0c8b1da40fd90b4e9878ecb1653fe3201958cd27fe1ee79cd
		// This address hosts VyFi LP pools (e.g., ADA/WMT) using PLUTUS v1
		// Pools are identified by unique pool NFTs
		"addr1z9vgl40qezca5s8ajz6wnpuwevt98l3jqx2ce5nlu8h8nnw60wckas4haxwwclas0g39cc8cvt2r8yalrfa9e8vxx92qsss9sx",
	}
}
