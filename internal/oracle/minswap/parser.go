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
	"math"
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

// Parser implements pool parsing for Minswap V2
type Parser struct{}

// NewV2Parser creates a parser for Minswap V2 pools
func NewV2Parser() *Parser {
	return &Parser{}
}

// Protocol returns the protocol name
func (p *Parser) Protocol() string {
	return ProtocolName + "-v2"
}

// ParsePoolDatum parses a Minswap V2 pool datum
func (p *Parser) ParsePoolDatum(
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
