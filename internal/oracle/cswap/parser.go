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

package cswap

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

// PoolState represents the state of a CSWAP liquidity pool.
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

// Parser implements pool parsing for CSWAP pools.
type Parser struct{}

// NewParser creates a parser for CSWAP pools.
func NewParser() *Parser {
	return &Parser{}
}

// Protocol returns the protocol name.
func (p *Parser) Protocol() string {
	return ProtocolName
}

// ParsePoolDatum parses a CSWAP pool datum and extracts reserves from utxoValue.
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
		return nil, fmt.Errorf("failed to decode CSWAP datum: %w", err)
	}

	if poolDatum.PoolFee > FeeDenom {
		return nil, fmt.Errorf(
			"invalid fee: pool_fee %d exceeds denominator %d",
			poolDatum.PoolFee,
			FeeDenom,
		)
	}

	reserveQuote, reserveBase, err := p.extractReservesFromValue(utxoValue, poolDatum)
	if err != nil {
		return nil, fmt.Errorf("failed to extract reserves: %w", err)
	}

	return &PoolState{
		PoolId:   GeneratePoolId(poolDatum.LPTokenPolicy, poolDatum.LPTokenName),
		Protocol: p.Protocol(),
		AssetX: common.AssetAmount{
			Class:  poolDatum.QuoteAsset(),
			Amount: reserveQuote,
		},
		AssetY: common.AssetAmount{
			Class:  poolDatum.BaseAsset(),
			Amount: reserveBase,
		},
		FeeNum:    FeeDenom - poolDatum.PoolFee,
		FeeDenom:  FeeDenom,
		Slot:      slot,
		TxHash:    txHash,
		TxIndex:   txIndex,
		Timestamp: timestamp,
	}, nil
}

func (p *Parser) extractReservesFromValue(
	utxoValue []byte,
	poolDatum PoolDatum,
) (uint64, uint64, error) {
	txOut, err := ledger.NewTransactionOutputFromCbor(utxoValue)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to decode UTxO output: %w", err)
	}

	quoteAmount, err := p.getAssetAmount(
		txOut,
		poolDatum.QuotePolicy,
		poolDatum.QuoteName,
	)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get quote reserve: %w", err)
	}

	baseAmount, err := p.getAssetAmount(
		txOut,
		poolDatum.BasePolicy,
		poolDatum.BaseName,
	)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get base reserve: %w", err)
	}

	return quoteAmount, baseAmount, nil
}

func (p *Parser) getAssetAmount(
	txOut lcommon.TransactionOutput,
	policyID []byte,
	assetName []byte,
) (uint64, error) {
	if len(policyID) == 0 {
		if len(assetName) != 0 {
			return 0, fmt.Errorf(
				"malformed asset: empty policy with non-empty asset name %x",
				assetName,
			)
		}
		return txOut.Amount().Uint64(), nil
	}

	assets := txOut.Assets()
	if assets == nil {
		return 0, fmt.Errorf(
			"no native assets in UTxO for policy %x",
			policyID,
		)
	}

	var policyHash lcommon.Blake2b224
	if len(policyID) != len(policyHash) {
		return 0, fmt.Errorf(
			"invalid policy ID length: expected %d, got %d",
			len(policyHash),
			len(policyID),
		)
	}
	copy(policyHash[:], policyID)

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
		policyID,
		assetName,
	)
}

// GeneratePoolId generates a deterministic pool ID from the LP token asset.
func GeneratePoolId(lpPolicy []byte, lpName []byte) string {
	return fmt.Sprintf(
		"%s_%s.%s",
		ProtocolName,
		hex.EncodeToString(lpPolicy),
		hex.EncodeToString(lpName),
	)
}
