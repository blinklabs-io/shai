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

package wingriders

import (
	"encoding/hex"
	"fmt"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/shai/internal/common"
)

// PoolState represents the state of a WingRiders liquidity pool
type PoolState struct {
	PoolId       string
	Protocol     string
	AssetX       common.AssetAmount
	AssetY       common.AssetAmount
	FeeNum       uint64
	FeeDenom     uint64
	IsStableswap bool
	Slot         uint64
	TxHash       string
	TxIndex      uint32
	Timestamp    time.Time
}

// Parser implements pool parsing for WingRiders
type Parser struct {
	version int // 2 for V2
}

// NewV2Parser creates a parser for WingRiders V2 pools
func NewV2Parser() *Parser {
	return &Parser{version: 2}
}

// Protocol returns the protocol name
func (p *Parser) Protocol() string {
	return fmt.Sprintf("%s-v%d", ProtocolName, p.version)
}

// ParsePoolDatum parses a WingRiders pool datum
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
	return nil, fmt.Errorf("unsupported WingRiders version: %d", p.version)
}

// parseV2Datum parses a WingRiders V2 pool datum
func (p *Parser) parseV2Datum(
	datum []byte,
	txHash string,
	txIndex uint32,
	slot uint64,
	timestamp time.Time,
) (*PoolState, error) {
	var poolDatum V2PoolDatum
	if _, err := cbor.Decode(datum, &poolDatum); err != nil {
		return nil, fmt.Errorf("failed to decode WingRiders V2 datum: %w", err)
	}

	// Generate pool ID from asset pair
	poolId := GeneratePoolId(
		poolDatum.AssetASymbol,
		poolDatum.AssetAToken,
		poolDatum.AssetBSymbol,
		poolDatum.AssetBToken,
	)

	// Calculate total fee
	totalFee := poolDatum.TotalFeeInBasis()
	feeDenom := poolDatum.FeeBasis
	if feeDenom == 0 {
		feeDenom = FeeBasis
	}

	// WingRiders doesn't store reserves in datum - they come from UTxO value
	// Treasury values may represent accumulated fees, not reserves
	state := &PoolState{
		PoolId:   poolId,
		Protocol: p.Protocol(),
		AssetX: common.AssetAmount{
			Class:  poolDatum.GetAssetA(),
			Amount: 0, // Will be populated from UTxO value
		},
		AssetY: common.AssetAmount{
			Class:  poolDatum.GetAssetB(),
			Amount: 0, // Will be populated from UTxO value
		},
		FeeNum:       feeDenom - totalFee, // Convert fee to (1 - fee) format
		FeeDenom:     feeDenom,
		IsStableswap: poolDatum.PoolVariant.IsStableswap(),
		Slot:         slot,
		TxHash:       txHash,
		TxIndex:      txIndex,
		Timestamp:    timestamp,
	}

	return state, nil
}

// GeneratePoolId generates a unique pool ID from asset pair
func GeneratePoolId(policyA, nameA, policyB, nameB []byte) string {
	return fmt.Sprintf(
		"wingriders_%s.%s_%s.%s",
		hex.EncodeToString(policyA),
		hex.EncodeToString(nameA),
		hex.EncodeToString(policyB),
		hex.EncodeToString(nameB),
	)
}

// GetV2PoolAddresses returns mainnet V2 pool addresses
func GetV2PoolAddresses() []string {
	return []string{
		// WingRiders V2 pool payment credential address (mainnet)
		"addr1w8nvjzjeydcn4atcd93aac8allvrpjn7pjr2qsweukpnayghhwcpj",
	}
}
