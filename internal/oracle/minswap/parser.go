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

package minswap

import (
	"encoding/hex"
	"fmt"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/shai/internal/common"
)

// PoolState represents the state of a Minswap liquidity pool
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

// Parser implements pool parsing for Minswap
type Parser struct {
	version int // 1 or 2
}

// NewV1Parser creates a parser for Minswap V1 pools
func NewV1Parser() *Parser {
	return &Parser{version: 1}
}

// NewV2Parser creates a parser for Minswap V2 pools
func NewV2Parser() *Parser {
	return &Parser{version: 2}
}

// Protocol returns the protocol name
func (p *Parser) Protocol() string {
	if p.version == 2 {
		return ProtocolName + "-v2"
	}
	return ProtocolName + "-v1"
}

// ParsePoolDatum parses a Minswap pool datum
func (p *Parser) ParsePoolDatum(
	datum []byte,
	txHash string,
	txIndex uint32,
	slot uint64,
	timestamp time.Time,
) (*PoolState, error) {
	if p.version == 2 {
		return p.parseV2Datum(datum, txHash, txIndex, slot, timestamp)
	}
	return p.parseV1Datum(datum, txHash, txIndex, slot, timestamp)
}

// parseV1Datum parses a Minswap V1 pool datum
func (p *Parser) parseV1Datum(
	datum []byte,
	txHash string,
	txIndex uint32,
	slot uint64,
	timestamp time.Time,
) (*PoolState, error) {
	var poolDatum V1PoolDatum
	if _, err := cbor.Decode(datum, &poolDatum); err != nil {
		return nil, fmt.Errorf("failed to decode Minswap V1 datum: %w", err)
	}

	// Generate pool ID from asset pair
	poolId := GeneratePoolId(
		poolDatum.AssetA.PolicyId,
		poolDatum.AssetA.AssetName,
		poolDatum.AssetB.PolicyId,
		poolDatum.AssetB.AssetName,
	)

	// V1 pools don't store reserves directly, only total liquidity
	// The actual reserves need to be calculated from the UTxO value
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
		FeeNum:    9970, // Default 0.3% fee (10000 - 30)
		FeeDenom:  FeeDenom,
		Slot:      slot,
		TxHash:    txHash,
		TxIndex:   txIndex,
		Timestamp: timestamp,
	}

	return state, nil
}

// parseV2Datum parses a Minswap V2 pool datum
func (p *Parser) parseV2Datum(
	datum []byte,
	txHash string,
	txIndex uint32,
	slot uint64,
	timestamp time.Time,
) (*PoolState, error) {
	var poolDatum V2PoolDatum
	if _, err := cbor.Decode(datum, &poolDatum); err != nil {
		return nil, fmt.Errorf("failed to decode Minswap V2 datum: %w", err)
	}

	// Generate pool ID from asset pair
	poolId := GeneratePoolId(
		poolDatum.AssetA.PolicyId,
		poolDatum.AssetA.AssetName,
		poolDatum.AssetB.PolicyId,
		poolDatum.AssetB.AssetName,
	)

	// Calculate effective fee (average of fee A and fee B)
	avgFee := (poolDatum.BaseFee.FeeANumerator + poolDatum.BaseFee.FeeBNumerator) / 2

	state := &PoolState{
		PoolId:   poolId,
		Protocol: p.Protocol(),
		AssetX: common.AssetAmount{
			Class:  poolDatum.AssetA.ToCommonAssetClass(),
			Amount: poolDatum.ReserveA,
		},
		AssetY: common.AssetAmount{
			Class:  poolDatum.AssetB.ToCommonAssetClass(),
			Amount: poolDatum.ReserveB,
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

// GeneratePoolId generates a unique pool ID from asset pair
func GeneratePoolId(policyA, nameA, policyB, nameB []byte) string {
	return fmt.Sprintf(
		"minswap_%s.%s_%s.%s",
		hex.EncodeToString(policyA),
		hex.EncodeToString(nameA),
		hex.EncodeToString(policyB),
		hex.EncodeToString(nameB),
	)
}

// GetV1PoolAddresses returns mainnet V1 pool addresses.
// Note: Minswap V1 pools are deprecated. This returns an empty list as
// V1 is no longer actively used. Add addresses if legacy support is needed.
func GetV1PoolAddresses() []string {
	return []string{}
}

// GetV2PoolAddresses returns mainnet V2 pool addresses.
// These should be configured via profile config rather than hardcoded.
func GetV2PoolAddresses() []string {
	return []string{}
}
