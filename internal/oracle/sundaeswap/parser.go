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
	"bytes"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
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

// Parser implements pool parsing for SundaeSwap V3
type Parser struct{}

// NewV3Parser creates a parser for SundaeSwap V3 pools
func NewV3Parser() *Parser {
	return &Parser{}
}

// Protocol returns the protocol name
func (p *Parser) Protocol() string {
	return ProtocolName + "-v3"
}

// ParsePoolDatum parses a SundaeSwap V3 pool datum
func (p *Parser) ParsePoolDatum(
	datum []byte,
	utxoValue []byte,
	txHash string,
	txIndex uint32,
	slot uint64,
	timestamp time.Time,
) (*PoolState, error) {
	var poolDatum V3PoolDatum
	if _, err := cbor.Decode(datum, &poolDatum); err != nil {
		return nil, fmt.Errorf("failed to decode SundaeSwap V3 datum: %w", err)
	}

	// Generate pool ID from asset pair
	poolId := GeneratePoolId(
		poolDatum.Assets.AssetA.PolicyId,
		poolDatum.Assets.AssetA.AssetName,
		poolDatum.Assets.AssetB.PolicyId,
		poolDatum.Assets.AssetB.AssetName,
	)

	// Calculate effective fee from bid/ask fees using integer arithmetic
	// Fees are stored as (numerator, denominator) fractions
	bidNum := poolDatum.BidFeesPer10Thousand.Numerator
	bidDen := poolDatum.BidFeesPer10Thousand.Denominator
	askNum := poolDatum.AskFeesPer10Thousand.Numerator
	askDen := poolDatum.AskFeesPer10Thousand.Denominator

	// Validate denominators are non-zero
	if bidDen == 0 || askDen == 0 {
		return nil, fmt.Errorf(
			"invalid fee denominators: bid=%d, ask=%d",
			bidDen,
			askDen,
		)
	}

	// Calculate average fee as a fraction using integer arithmetic:
	// avgFee = (bidNum/bidDen + askNum/askDen) / 2
	//        = (bidNum*askDen + askNum*bidDen) / (2*bidDen*askDen)
	//
	// We need: effectiveFeeNum = FeeDenom * (1 - avgFee)
	//        = FeeDenom * (avgDen - avgNum) / avgDen
	// where avgNum = bidNum*askDen + askNum*bidDen
	//       avgDen = 2*bidDen*askDen

	// Check for overflow before multiplication
	// For typical fee values (< 10000), these won't overflow uint64
	avgNum := bidNum*askDen + askNum*bidDen
	avgDen := 2 * bidDen * askDen

	// Validate avgNum doesn't exceed avgDen (fee rate must be <= 1)
	if avgNum > avgDen {
		return nil, fmt.Errorf(
			"invalid fee: average fee numerator %d exceeds denominator %d",
			avgNum,
			avgDen,
		)
	}

	// effectiveFeeNum = FeeDenom * (avgDen - avgNum) / avgDen
	// Use integer division with rounding toward zero
	effectiveFeeNum := FeeDenom * (avgDen - avgNum) / avgDen

	// Sanity check: effectiveFeeNum should be <= FeeDenom
	if effectiveFeeNum > FeeDenom {
		return nil, fmt.Errorf(
			"invalid fee: effective fee %d exceeds denominator %d",
			effectiveFeeNum,
			FeeDenom,
		)
	}

	// Parse the UTXO value to extract token amounts
	reserveA, reserveB, err := p.extractReservesFromValue(utxoValue, poolDatum)
	if err != nil {
		return nil, fmt.Errorf("failed to extract reserves: %w", err)
	}

	return &PoolState{
		PoolId:   poolId,
		Protocol: p.Protocol(),
		AssetX: common.AssetAmount{
			Class:  poolDatum.Assets.AssetA.ToCommonAssetClass(),
			Amount: reserveA,
		},
		AssetY: common.AssetAmount{
			Class:  poolDatum.Assets.AssetB.ToCommonAssetClass(),
			Amount: reserveB,
		},
		FeeNum:    effectiveFeeNum,
		FeeDenom:  FeeDenom,
		Slot:      slot,
		TxHash:    txHash,
		TxIndex:   txIndex,
		Timestamp: timestamp,
	}, nil
}

// extractReservesFromValue extracts the reserve amounts for AssetA and AssetB from the UTXO value
func (p *Parser) extractReservesFromValue(
	utxoValue []byte,
	poolDatum V3PoolDatum,
) (uint64, uint64, error) {
	// Parse the UTXO output CBOR using the generic decoder
	// This handles all eras (Mary, Alonzo, Babbage, Conway)
	txOut, err := ledger.NewTransactionOutputFromCbor(utxoValue)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to decode UTXO output: %w", err)
	}

	// Extract reserve for AssetA
	reserveA, err := p.getAssetAmount(
		txOut,
		poolDatum.Assets.AssetA.PolicyId,
		poolDatum.Assets.AssetA.AssetName,
	)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get AssetA amount: %w", err)
	}

	// Extract reserve for AssetB
	reserveB, err := p.getAssetAmount(
		txOut,
		poolDatum.Assets.AssetB.PolicyId,
		poolDatum.Assets.AssetB.AssetName,
	)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get AssetB amount: %w", err)
	}

	return reserveA, reserveB, nil
}

// getAssetAmount extracts the amount of a specific asset from a transaction output
func (p *Parser) getAssetAmount(
	txOut lcommon.TransactionOutput,
	policyId []byte,
	assetName []byte,
) (uint64, error) {
	// Check if this is ADA (empty policy ID)
	if len(policyId) == 0 {
		return txOut.Amount(), nil
	}

	// Get native assets
	assets := txOut.Assets()
	if assets == nil {
		return 0, fmt.Errorf(
			"no native assets in UTXO for policy %x",
			policyId,
		)
	}

	// Convert policy ID to Blake2b224
	var policyHash lcommon.Blake2b224
	if len(policyId) != len(policyHash) {
		return 0, fmt.Errorf(
			"invalid policy ID length: expected %d, got %d",
			len(policyHash),
			len(policyId),
		)
	}
	copy(policyHash[:], policyId)

	// Look up the asset amount
	// The Asset method returns the amount for the given policy and asset name
	for _, policy := range assets.Policies() {
		if policy == policyHash {
			for _, name := range assets.Assets(policy) {
				if bytes.Equal(name, assetName) {
					return assets.Asset(policy, name), nil
				}
			}
		}
	}

	return 0, fmt.Errorf(
		"asset not found: policy=%x, name=%x",
		policyId,
		assetName,
	)
}

// GeneratePoolId generates a unique pool ID from asset pair
func GeneratePoolId(policyA, nameA, policyB, nameB []byte) string {
	return fmt.Sprintf(
		"sundaeswap_%s.%s_%s.%s",
		hex.EncodeToString(policyA),
		hex.EncodeToString(nameA),
		hex.EncodeToString(policyB),
		hex.EncodeToString(nameB),
	)
}
