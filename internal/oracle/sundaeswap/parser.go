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

package sundaeswap

import (
	"encoding/hex"
	"fmt"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/shai/internal/common"
)

// PoolState represents the state of a SundaeSwap liquidity pool
type PoolState struct {
	PoolId    string
	Protocol  string
	AssetX    common.AssetAmount
	AssetY    common.AssetAmount
	FeeNum    uint64
	FeeDenom  uint64
	Slot      uint64
	TxHash    string
	TxIndex   uint32
	Timestamp time.Time
}

// Parser implements pool parsing for SundaeSwap
type Parser struct {
	version int // 1 for V1, 3 for V3
}

// NewV1Parser creates a parser for SundaeSwap V1 pools
func NewV1Parser() *Parser {
	return &Parser{version: 1}
}

// NewV3Parser creates a parser for SundaeSwap V3 pools
func NewV3Parser() *Parser {
	return &Parser{version: 3}
}

// Protocol returns the protocol name
func (p *Parser) Protocol() string {
	return fmt.Sprintf("%s-v%d", ProtocolName, p.version)
}

// ParsePoolDatum parses a SundaeSwap pool datum
func (p *Parser) ParsePoolDatum(
	datum []byte,
	txHash string,
	txIndex uint32,
	slot uint64,
	timestamp time.Time,
) (*PoolState, error) {
	switch p.version {
	case 1:
		return p.parseV1Datum(datum, txHash, txIndex, slot, timestamp)
	case 3:
		return p.parseV3Datum(datum, txHash, txIndex, slot, timestamp)
	default:
		return nil, fmt.Errorf("unsupported SundaeSwap version: %d", p.version)
	}
}

// parseV1Datum parses a SundaeSwap V1 pool datum
func (p *Parser) parseV1Datum(
	datum []byte,
	txHash string,
	txIndex uint32,
	slot uint64,
	timestamp time.Time,
) (*PoolState, error) {
	var poolDatum V1PoolDatum
	if _, err := cbor.Decode(datum, &poolDatum); err != nil {
		return nil, fmt.Errorf("failed to decode SundaeSwap V1 datum: %w", err)
	}

	// Generate pool ID from the identifier
	poolId := GeneratePoolId(poolDatum.Ident)

	// Use the fee from datum, or default to 30 basis points (0.3%)
	feeNum := poolDatum.FeeNumerator
	if feeNum == 0 {
		feeNum = V1DefaultFee
	}

	// SundaeSwap V1 doesn't store reserves in datum - they come from UTxO value
	state := &PoolState{
		PoolId:   poolId,
		Protocol: p.Protocol(),
		AssetX: common.AssetAmount{
			Class:  poolDatum.AssetA.ToCommonAssetClass(),
			Amount: 0, // Will be populated from UTxO value
		},
		AssetY: common.AssetAmount{
			Class:  poolDatum.AssetB.ToCommonAssetClass(),
			Amount: 0, // Will be populated from UTxO value
		},
		FeeNum:    FeeDenom - feeNum, // Convert fee to (1 - fee) format
		FeeDenom:  FeeDenom,
		Slot:      slot,
		TxHash:    txHash,
		TxIndex:   txIndex,
		Timestamp: timestamp,
	}

	return state, nil
}

// parseV3Datum parses a SundaeSwap V3 pool datum
func (p *Parser) parseV3Datum(
	datum []byte,
	txHash string,
	txIndex uint32,
	slot uint64,
	timestamp time.Time,
) (*PoolState, error) {
	var poolDatum V3PoolDatum
	if _, err := cbor.Decode(datum, &poolDatum); err != nil {
		return nil, fmt.Errorf("failed to decode SundaeSwap V3 datum: %w", err)
	}

	// Generate pool ID from the identifier
	poolId := GeneratePoolId(poolDatum.Identifier)

	// Calculate average fee from bid and ask fees
	avgFee := (poolDatum.BidFeesPer10Thousand + poolDatum.AskFeesPer10Thousand) / 2

	// SundaeSwap V3 doesn't store reserves in datum - they come from UTxO value
	state := &PoolState{
		PoolId:   poolId,
		Protocol: p.Protocol(),
		AssetX: common.AssetAmount{
			Class:  poolDatum.Assets.AssetA.ToCommonAssetClass(),
			Amount: 0, // Will be populated from UTxO value
		},
		AssetY: common.AssetAmount{
			Class:  poolDatum.Assets.AssetB.ToCommonAssetClass(),
			Amount: 0, // Will be populated from UTxO value
		},
		FeeNum:    FeeDenom - avgFee, // Convert fee to (1 - fee) format
		FeeDenom:  FeeDenom,
		Slot:      slot,
		TxHash:    txHash,
		TxIndex:   txIndex,
		Timestamp: timestamp,
	}

	return state, nil
}

// GeneratePoolId generates a unique pool ID from the identifier
func GeneratePoolId(identifier []byte) string {
	return fmt.Sprintf("sundaeswap_%s", hex.EncodeToString(identifier))
}

// GetV1PoolAddresses returns mainnet V1 pool addresses
func GetV1PoolAddresses() []string {
	return []string{
		// SundaeSwap V1 pool script address (mainnet)
		"addr1wyx22z2s4kasd3w976pnjf9xdty88epjqfvgkmfnfpsdacqe7utc8",
	}
}

// GetV3PoolAddresses returns mainnet V3 pool addresses
func GetV3PoolAddresses() []string {
	return []string{
		// Main pool contract address (mainnet)
		"addr1w8srqftqemf0mjlukfszd97ljuxdp68yz8zvsfyguke3e5ce47xcd",
	}
}
