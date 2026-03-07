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
	"bytes"
	"encoding/hex"
	"fmt"
	"math"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
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

// Parser implements pool parsing for Minswap V2
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
	if p.version == 1 {
		return ProtocolName + "-v1"
	}
	return ProtocolName + "-v2"
}

// ParsePoolDatum parses a Minswap pool datum.
// utxoValue is consumed by V1 parsing for reserve extraction and ignored by V2.
func (p *Parser) ParsePoolDatum(
	datum []byte,
	utxoValue []byte,
	txHash string,
	txIndex uint32,
	slot uint64,
	timestamp time.Time,
) (*PoolState, error) {
	if p.version == 1 {
		return p.parseV1Datum(datum, utxoValue, txHash, txIndex, slot, timestamp)
	}
	return p.parseV2Datum(datum, txHash, txIndex, slot, timestamp)
}

func (p *Parser) parseV1Datum(
	datum []byte,
	utxoValue []byte,
	txHash string,
	txIndex uint32,
	slot uint64,
	timestamp time.Time,
) (*PoolState, error) {
	var poolDatum V1PoolDatum
	if _, err := cbor.Decode(datum, &poolDatum); err != nil {
		return nil, fmt.Errorf("failed to decode Minswap V1 datum: %w", err)
	}

	poolId := GeneratePoolId(
		poolDatum.AssetA.PolicyId,
		poolDatum.AssetA.AssetName,
		poolDatum.AssetB.PolicyId,
		poolDatum.AssetB.AssetName,
	)

	reserveA, reserveB, err := p.extractReservesFromValue(
		utxoValue,
		poolDatum.AssetA,
		poolDatum.AssetB,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to extract V1 reserves: %w", err)
	}

	return &PoolState{
		PoolId:   poolId,
		Protocol: p.Protocol(),
		AssetX: common.AssetAmount{
			Class:  poolDatum.AssetA.ToCommonAssetClass(),
			Amount: reserveA,
		},
		AssetY: common.AssetAmount{
			Class:  poolDatum.AssetB.ToCommonAssetClass(),
			Amount: reserveB,
		},
		FeeNum:    9970, // default 0.3% swap fee
		FeeDenom:  FeeDenom,
		Slot:      slot,
		TxHash:    txHash,
		TxIndex:   txIndex,
		Timestamp: timestamp,
	}, nil
}

func (p *Parser) extractReservesFromValue(
	utxoValue []byte,
	assetA Asset,
	assetB Asset,
) (uint64, uint64, error) {
	txOut, err := ledger.NewTransactionOutputFromCbor(utxoValue)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to decode UTxO output: %w", err)
	}

	reserveA, err := p.getAssetAmount(txOut, assetA.PolicyId, assetA.AssetName)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get asset A amount: %w", err)
	}
	reserveB, err := p.getAssetAmount(txOut, assetB.PolicyId, assetB.AssetName)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get asset B amount: %w", err)
	}
	return reserveA, reserveB, nil
}

func (p *Parser) getAssetAmount(
	txOut lcommon.TransactionOutput,
	policyId []byte,
	assetName []byte,
) (uint64, error) {
	if len(policyId) == 0 {
		if len(assetName) != 0 {
			return 0, fmt.Errorf(
				"malformed asset: empty policyId with non-empty assetName %x",
				assetName,
			)
		}
		return txOut.Amount().Uint64(), nil
	}

	assets := txOut.Assets()
	if assets == nil {
		return 0, fmt.Errorf("no native assets in UTxO for policy %x", policyId)
	}

	var policyHash lcommon.Blake2b224
	if len(policyId) != len(policyHash) {
		return 0, fmt.Errorf(
			"invalid policy ID length: expected %d, got %d",
			len(policyHash),
			len(policyId),
		)
	}
	copy(policyHash[:], policyId)

	for _, policy := range assets.Policies() {
		if policy != policyHash {
			continue
		}
		for _, name := range assets.Assets(policy) {
			if bytes.Equal(name, assetName) {
				return assets.Asset(policy, name).Uint64(), nil
			}
		}
	}

	return 0, fmt.Errorf(
		"asset not found: policy=%x, name=%x",
		policyId,
		assetName,
	)
}

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
	// Check for overflow before adding
	feeA := poolDatum.BaseFee.FeeANumerator
	feeB := poolDatum.BaseFee.FeeBNumerator
	if feeA > math.MaxUint64-feeB {
		return nil, fmt.Errorf(
			"fee overflow: feeA %d + feeB %d exceeds uint64",
			feeA,
			feeB,
		)
	}
	avgFee := (feeA + feeB) / 2
	if avgFee > FeeDenom {
		return nil, fmt.Errorf(
			"invalid fee: average fee %d exceeds denominator %d",
			avgFee,
			FeeDenom,
		)
	}

	return &PoolState{
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
		FeeNum:    FeeDenom - avgFee,
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
		"minswap_%s.%s_%s.%s",
		hex.EncodeToString(policyA),
		hex.EncodeToString(nameA),
		hex.EncodeToString(policyB),
		hex.EncodeToString(nameB),
	)
}
