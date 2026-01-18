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
	FeeNum    uint64 // VyFi uses 0.3% fee (997/1000)
	FeeDenom  uint64
	Shares    int64 // Total LP shares
	Slot      uint64
	TxHash    string
	TxIndex   uint32
	Timestamp time.Time
}

// Parser implements pool parsing for VyFi
type Parser struct {
	version int
}

// NewParser creates a parser for VyFi pools
func NewParser() *Parser {
	return &Parser{version: 1}
}

// Protocol returns the protocol name
func (p *Parser) Protocol() string {
	return ProtocolName
}

// ParsePoolDatum parses a VyFi pool datum
// Note: VyFi pools store reserves in the datum (treasuryA/treasuryB),
// but asset identifiers come from the pool's NFT or address context
func (p *Parser) ParsePoolDatum(
	datum []byte,
	txHash string,
	txIndex uint32,
	slot uint64,
	timestamp time.Time,
	assetA common.AssetClass,
	assetB common.AssetClass,
	poolNFT string,
) (*PoolState, error) {
	var poolDatum PoolDatum
	if _, err := cbor.Decode(datum, &poolDatum); err != nil {
		return nil, fmt.Errorf("failed to decode VyFi pool datum: %w", err)
	}

	// Generate pool ID from the pool NFT or asset pair
	poolId := GeneratePoolId(poolNFT, assetA, assetB)

	// VyFi uses 0.3% swap fee (standard AMM fee)
	// FeeNum = 997, FeeDenom = 1000 means 0.3% fee
	state := &PoolState{
		PoolId:   poolId,
		Protocol: p.Protocol(),
		AssetX: common.AssetAmount{
			Class:  assetA,
			Amount: uint64(poolDatum.TreasuryA),
		},
		AssetY: common.AssetAmount{
			Class:  assetB,
			Amount: uint64(poolDatum.TreasuryB),
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
