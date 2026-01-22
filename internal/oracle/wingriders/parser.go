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

// PriceXY returns the price of X in terms of Y (Y per X)
func (p PoolState) PriceXY() float64 {
	if p.AssetX.Amount == 0 {
		return 0.0
	}
	return float64(p.AssetY.Amount) / float64(p.AssetX.Amount)
}

// Parser implements pool parsing for WingRiders V2
type Parser struct{}

// NewV2Parser creates a parser for WingRiders V2 pools
func NewV2Parser() *Parser {
	return &Parser{}
}

// Protocol returns the protocol name
func (p *Parser) Protocol() string {
	return ProtocolName + "-v2"
}

// ParsePoolDatum parses a WingRiders V2 pool datum
func (p *Parser) ParsePoolDatum(
	datum []byte,
	utxoValue []byte, // Not used for WingRiders v2
	txHash string,
	txIndex uint32,
	slot uint64,
	timestamp time.Time,
) (*PoolState, error) {
	var poolDatum PoolDatum
	if _, err := cbor.Decode(datum, &poolDatum); err != nil {
		return nil, fmt.Errorf("failed to decode WingRiders V2 datum: %w", err)
	}

	// Generate pool ID from asset pair
	poolId := GeneratePoolId(
		poolDatum.AssetA.PolicyId,
		poolDatum.AssetA.Name,
		poolDatum.AssetB.PolicyId,
		poolDatum.AssetB.Name,
	)

	// Calculate effective fee from swap fee in basis points
	// WingRiders uses basis points (1/10000), so convert to our format
	swapFee := poolDatum.SwapFeeInBasis
	if swapFee > FeeDenom {
		return nil, fmt.Errorf(
			"invalid swap fee: %d exceeds denominator %d",
			swapFee,
			FeeDenom,
		)
	}

	// FeeNum represents the fee taken, so FeeDenom - FeeNum = effective fee
	effectiveFeeNum := FeeDenom - swapFee

	return &PoolState{
		PoolId:   poolId,
		Protocol: p.Protocol(),
		AssetX: common.AssetAmount{
			Class:  poolDatum.AssetA.ToCommonAssetClass(),
			Amount: poolDatum.TreasuryA,
		},
		AssetY: common.AssetAmount{
			Class:  poolDatum.AssetB.ToCommonAssetClass(),
			Amount: poolDatum.TreasuryB,
		},
		FeeNum:    effectiveFeeNum,
		FeeDenom:  FeeDenom,
		Slot:      slot,
		TxHash:    txHash,
		TxIndex:   txIndex,
		Timestamp: timestamp,
	}, nil
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
