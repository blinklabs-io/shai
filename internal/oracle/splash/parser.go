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
	"bytes"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/shai/internal/common"
)

// PoolState represents the state of a Splash liquidity pool
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

// Parser implements pool parsing for Splash
type Parser struct{}

// NewV1Parser creates a parser for Splash v1 pools
func NewV1Parser() *Parser {
	return &Parser{}
}

// Protocol returns the protocol name
func (p *Parser) Protocol() string {
	return ProtocolName + "-v1"
}

// ParsePoolDatum parses a Splash pool datum
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
		return nil, fmt.Errorf("failed to decode Splash datum: %w", err)
	}

	// Generate pool ID from asset pair
	poolId := GeneratePoolId(
		poolDatum.X.PolicyId,
		poolDatum.X.Name,
		poolDatum.Y.PolicyId,
		poolDatum.Y.Name,
	)

	// Validate fee
	if poolDatum.FeeNum > FeeDenom {
		return nil, fmt.Errorf(
			"invalid fee: fee_num %d exceeds denominator %d",
			poolDatum.FeeNum,
			FeeDenom,
		)
	}

	// Parse the UTXO value to extract token amounts
	reserveX, reserveY, err := p.extractReservesFromValue(utxoValue, poolDatum)
	if err != nil {
		return nil, fmt.Errorf("failed to extract reserves: %w", err)
	}

	return &PoolState{
		PoolId:   poolId,
		Protocol: p.Protocol(),
		AssetX: common.AssetAmount{
			Class:  poolDatum.X.ToCommonAssetClass(),
			Amount: reserveX,
		},
		AssetY: common.AssetAmount{
			Class:  poolDatum.Y.ToCommonAssetClass(),
			Amount: reserveY,
		},
		FeeNum:    poolDatum.FeeNum,
		FeeDenom:  FeeDenom,
		Slot:      slot,
		TxHash:    txHash,
		TxIndex:   txIndex,
		Timestamp: timestamp,
	}, nil
}

// extractReservesFromValue extracts the reserve amounts for AssetX and AssetY from the UTXO value
func (p *Parser) extractReservesFromValue(
	utxoValue []byte,
	poolDatum PoolDatum,
) (uint64, uint64, error) {
	// Parse the UTXO output CBOR using the generic decoder
	// This handles all eras (Mary, Alonzo, Babbage, Conway)
	txOut, err := ledger.NewTransactionOutputFromCbor(utxoValue)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to decode UTXO output: %w", err)
	}

	// Extract reserve for AssetX
	reserveX, err := p.getAssetAmount(
		txOut,
		poolDatum.X.PolicyId,
		poolDatum.X.Name,
	)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get AssetX amount: %w", err)
	}

	// Extract reserve for AssetY
	reserveY, err := p.getAssetAmount(
		txOut,
		poolDatum.Y.PolicyId,
		poolDatum.Y.Name,
	)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get AssetY amount: %w", err)
	}

	return reserveX, reserveY, nil
}

// getAssetAmount extracts the amount of a specific asset from a transaction output
func (p *Parser) getAssetAmount(
	txOut lcommon.TransactionOutput,
	policyId []byte,
	assetName []byte,
) (uint64, error) {
	// Check if this is ADA (empty policy ID AND empty asset name)
	if len(policyId) == 0 {
		if len(assetName) != 0 {
			return 0, fmt.Errorf(
				"malformed asset: empty policyId with non-empty assetName %x",
				assetName,
			)
		}
		amount := txOut.Amount()
		return amount.Uint64(), nil
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
					amount := assets.Asset(policy, name)
					return amount.Uint64(), nil
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

// GeneratePoolId generates a deterministic pool ID from asset pair
func GeneratePoolId(
	policyIdA []byte,
	assetNameA []byte,
	policyIdB []byte,
	assetNameB []byte,
) string {
	// Create asset class strings
	assetA := fmt.Sprintf("%s.%s", hex.EncodeToString(policyIdA), hex.EncodeToString(assetNameA))
	assetB := fmt.Sprintf("%s.%s", hex.EncodeToString(policyIdB), hex.EncodeToString(assetNameB))

	// Ensure consistent ordering (lexicographically smaller first)
	if assetA > assetB {
		assetA, assetB = assetB, assetA
	}

	return fmt.Sprintf("%s-%s", assetA, assetB)
}
