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

package splash

import (
	"encoding/hex"
	"fmt"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/shai/internal/common"
)

// PoolState represents the state of a Splash liquidity pool
type PoolState struct {
	PoolId    string
	Protocol  string
	AssetX    common.AssetAmount
	AssetY    common.AssetAmount
	AssetLq   common.AssetClass // LP token asset class
	FeeNum    uint64
	FeeDenom  uint64
	Nonce     int64
	Slot      uint64
	TxHash    string
	TxIndex   uint32
	Timestamp time.Time
}

// Parser implements pool parsing for Splash
type Parser struct {
	version int // 1 for V1
}

// NewV1Parser creates a parser for Splash V1 pools
func NewV1Parser() *Parser {
	return &Parser{version: 1}
}

// Protocol returns the protocol name
func (p *Parser) Protocol() string {
	return fmt.Sprintf("%s-v%d", ProtocolName, p.version)
}

// ParsePoolDatum parses a Splash pool datum
func (p *Parser) ParsePoolDatum(
	datum []byte,
	txHash string,
	txIndex uint32,
	slot uint64,
	timestamp time.Time,
) (*PoolState, error) {
	if p.version == 1 {
		return p.parseV1Datum(datum, txHash, txIndex, slot, timestamp)
	}
	return nil, fmt.Errorf("unsupported Splash version: %d", p.version)
}

// parseV1Datum parses a Splash V1 pool datum
func (p *Parser) parseV1Datum(
	datum []byte,
	txHash string,
	txIndex uint32,
	slot uint64,
	timestamp time.Time,
) (*PoolState, error) {
	var poolDatum V1PoolDatum
	if _, err := cbor.Decode(datum, &poolDatum); err != nil {
		return nil, fmt.Errorf("failed to decode Splash V1 datum: %w", err)
	}

	// Generate pool ID from the pool NFT
	poolId := GeneratePoolId(
		poolDatum.PoolNft.PolicyId,
		poolDatum.PoolNft.TokenName,
	)

	// Fee calculation: poolFeeNum is in basis points (10000 = 100%)
	// FeeNum represents the portion retained after fee (1 - fee)
	feeNum := FeeDenom - poolDatum.PoolFeeNum
	if poolDatum.PoolFeeNum > FeeDenom {
		feeNum = 0 // Cap at 100% fee
	}

	// Splash V1 doesn't store reserves in datum - they come from UTxO value
	state := &PoolState{
		PoolId:   poolId,
		Protocol: p.Protocol(),
		AssetX: common.AssetAmount{
			Class:  poolDatum.PoolX.ToCommonAssetClass(),
			Amount: 0, // Will be populated from UTxO value
		},
		AssetY: common.AssetAmount{
			Class:  poolDatum.PoolY.ToCommonAssetClass(),
			Amount: 0, // Will be populated from UTxO value
		},
		AssetLq:   poolDatum.PoolLq.ToCommonAssetClass(),
		FeeNum:    feeNum,
		FeeDenom:  FeeDenom,
		Nonce:     poolDatum.Nonce,
		Slot:      slot,
		TxHash:    txHash,
		TxIndex:   txIndex,
		Timestamp: timestamp,
	}

	return state, nil
}

// GeneratePoolId generates a unique pool ID from the pool NFT
func GeneratePoolId(policyId, tokenName []byte) string {
	return fmt.Sprintf(
		"splash_%s%s",
		hex.EncodeToString(policyId),
		hex.EncodeToString(tokenName),
	)
}

// GetV1PoolAddresses returns mainnet V1 pool addresses
// Note: Splash pool addresses need to be confirmed with actual deployment
func GetV1PoolAddresses() []string {
	return []string{
		// Splash V1 pool script address (mainnet) - to be configured
		// Pool addresses are typically identified by pool NFTs
	}
}
