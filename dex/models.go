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

// Package dex provides reusable DEX pool-datum parsing and constant-product
// AMM pricing for Cardano decentralized exchanges. It is independent of any
// node, indexer, storage, or HTTP service: callers supply the raw datum and
// UTxO-value CBOR bytes (e.g. obtained from their own node) and receive a
// PoolState that can compute prices and swap quotes.
package dex

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"time"

	"github.com/blinklabs-io/shai/common"
)

// PoolState represents the current state of a liquidity pool
type PoolState struct {
	PoolId   string             `json:"poolId"`
	Network  string             `json:"network"`
	Protocol string             `json:"protocol"`
	AssetX   common.AssetAmount `json:"assetX"`
	AssetY   common.AssetAmount `json:"assetY"`
	// FeeNum/FeeDenom is the post-fee input multiplier applied by Quote.
	// For a 0.3% fee, parsers must normalize protocol-native fees to
	// 997/1000 or 9970/10000 rather than storing the fee taken.
	FeeNum      uint64    `json:"feeNum"`
	FeeDenom    uint64    `json:"feeDenom"`
	Slot        uint64    `json:"slot"`
	BlockHash   string    `json:"blockHash"`
	TxHash      string    `json:"txHash"`
	TxIndex     uint32    `json:"txIndex"`
	Timestamp   time.Time `json:"timestamp"`
	UpdatedAt   time.Time `json:"updatedAt"`
	FromMempool bool      `json:"fromMempool"` // True if state is from mempool (unconfirmed)
}

// ClonePoolState returns a deep copy of the given pool state, duplicating the
// asset class byte slices so callers can mutate the result independently.
func ClonePoolState(state *PoolState) *PoolState {
	if state == nil {
		return nil
	}
	clone := *state
	clone.AssetX = cloneAssetAmount(state.AssetX)
	clone.AssetY = cloneAssetAmount(state.AssetY)
	return &clone
}

func cloneAssetAmount(amount common.AssetAmount) common.AssetAmount {
	amount.Class.PolicyId = cloneBytes(amount.Class.PolicyId)
	amount.Class.Name = cloneBytes(amount.Class.Name)
	return amount
}

func cloneBytes(src []byte) []byte {
	if src == nil {
		return nil
	}
	// Use make+copy rather than append([]byte(nil), src...): appending zero
	// elements to a nil slice returns nil, which would normalize an empty
	// (non-nil) slice to nil and change the serialized asset class.
	dst := make([]byte, len(src))
	copy(dst, src)
	return dst
}

// PriceXY returns the price of X in terms of Y (Y per X)
func (p *PoolState) PriceXY() float64 {
	if p.AssetX.Amount == 0 {
		return 0
	}
	return float64(p.AssetY.Amount) / float64(p.AssetX.Amount)
}

// PriceYX returns the price of Y in terms of X (X per Y)
func (p *PoolState) PriceYX() float64 {
	if p.AssetY.Amount == 0 {
		return 0
	}
	return float64(p.AssetX.Amount) / float64(p.AssetY.Amount)
}

// TVL returns the total value locked in the pool (sum of both assets)
// Note: This is a raw sum, not normalized to any common unit
func (p *PoolState) TVL() uint64 {
	return p.AssetX.Amount + p.AssetY.Amount
}

// EffectiveFee returns the pool's trading fee as a decimal
func (p *PoolState) EffectiveFee() float64 {
	if p.FeeDenom == 0 {
		return 0
	}
	return 1.0 - (float64(p.FeeNum) / float64(p.FeeDenom))
}

// Quote computes the output amount for swapping amountIn of the asset
// identified by (assetInPolicy, assetInName) into the other asset of the pool,
// using the constant-product (x*y=k) AMM formula with the pool's fee.
//
// ADA/lovelace is identified by an empty policy id and empty asset name (nil or
// zero-length slices are both accepted).
//
// It returns the output amount and the price impact as a percentage: the
// relative difference between the pool's spot price (before the swap) and the
// realized execution price (amountOut/amountIn), expressed as a positive
// percentage. It returns an error if the asset is not part of the pool, if the
// pool fee is invalid, or if either reserve is zero.
func (p *PoolState) Quote(
	assetInPolicy, assetInName []byte,
	amountIn uint64,
) (amountOut uint64, priceImpactPct float64, err error) {
	if p.FeeDenom == 0 {
		return 0, 0, fmt.Errorf("invalid pool fee: zero denominator")
	}
	if p.FeeNum > p.FeeDenom {
		return 0, 0, fmt.Errorf(
			"invalid pool fee: numerator %d exceeds denominator %d",
			p.FeeNum,
			p.FeeDenom,
		)
	}

	matchesX := bytes.Equal(p.AssetX.Class.PolicyId, assetInPolicy) &&
		bytes.Equal(p.AssetX.Class.Name, assetInName)
	matchesY := bytes.Equal(p.AssetY.Class.PolicyId, assetInPolicy) &&
		bytes.Equal(p.AssetY.Class.Name, assetInName)

	var reserveIn, reserveOut uint64
	switch {
	case matchesX:
		reserveIn, reserveOut = p.AssetX.Amount, p.AssetY.Amount
	case matchesY:
		reserveIn, reserveOut = p.AssetY.Amount, p.AssetX.Amount
	default:
		return 0, 0, fmt.Errorf(
			"asset %x.%x is not part of pool %s",
			assetInPolicy,
			assetInName,
			p.PoolId,
		)
	}

	if reserveIn == 0 || reserveOut == 0 {
		return 0, 0, fmt.Errorf(
			"pool %s has zero reserves (in=%d, out=%d)",
			p.PoolId,
			reserveIn,
			reserveOut,
		)
	}
	if amountIn == 0 {
		return 0, 0, nil
	}

	// amountOut = reserveOut * amountIn * FeeNum /
	//             (reserveIn * FeeDenom + amountIn * FeeNum)
	// Use big.Int for the intermediate products to avoid uint64 overflow.
	amountInFee := new(big.Int).Mul(
		new(big.Int).SetUint64(amountIn),
		new(big.Int).SetUint64(p.FeeNum),
	)
	numerator := new(big.Int).Mul(
		new(big.Int).SetUint64(reserveOut),
		amountInFee,
	)
	denominator := new(big.Int).Add(
		new(big.Int).Mul(
			new(big.Int).SetUint64(reserveIn),
			new(big.Int).SetUint64(p.FeeDenom),
		),
		amountInFee,
	)
	if denominator.Sign() == 0 {
		return 0, 0, fmt.Errorf("pool %s: zero swap denominator", p.PoolId)
	}
	out := new(big.Int).Quo(numerator, denominator)
	if !out.IsUint64() {
		return 0, 0, fmt.Errorf(
			"pool %s: output amount overflows uint64",
			p.PoolId,
		)
	}
	amountOut = out.Uint64()

	// Spot price (output per input, before the swap) = reserveOut / reserveIn.
	// Execution price = amountOut / amountIn.
	// Price impact = (spot - execution) / spot * 100.
	spot := float64(reserveOut) / float64(reserveIn)
	if spot > 0 {
		execution := float64(amountOut) / float64(amountIn)
		priceImpactPct = (spot - execution) / spot * 100
	}

	return amountOut, priceImpactPct, nil
}

// Key returns a unique identifier for this pool state
func (p *PoolState) Key() string {
	return fmt.Sprintf("%s:%s:%s", p.Network, p.Protocol, p.PoolId)
}

// String returns a human-readable representation
func (p PoolState) String() string {
	poolIdDisplay := p.PoolId
	if len(poolIdDisplay) > 16 {
		poolIdDisplay = poolIdDisplay[:16] + "..."
	}
	return fmt.Sprintf(
		"Pool[%s] %s/%s: %d/%d (price: %.6f)",
		poolIdDisplay,
		p.AssetX.Class.Fingerprint(),
		p.AssetY.Class.Fingerprint(),
		p.AssetX.Amount,
		p.AssetY.Amount,
		p.PriceXY(),
	)
}

// MarshalJSON implements json.Marshaler with computed fields
func (p PoolState) MarshalJSON() ([]byte, error) {
	type Alias PoolState
	return json.Marshal(&struct {
		Alias
		PriceXY      float64 `json:"priceXY"`
		PriceYX      float64 `json:"priceYX"`
		EffectiveFee float64 `json:"effectiveFee"`
	}{
		Alias:        Alias(p),
		PriceXY:      p.PriceXY(),
		PriceYX:      p.PriceYX(),
		EffectiveFee: p.EffectiveFee(),
	})
}

// PriceUpdate represents a price change event
type PriceUpdate struct {
	PoolId       string    `json:"poolId"`
	Protocol     string    `json:"protocol"`
	AssetX       string    `json:"assetX"`
	AssetY       string    `json:"assetY"`
	PriceXY      float64   `json:"priceXY"`
	PriceYX      float64   `json:"priceYX"`
	ReserveX     uint64    `json:"reserveX"`
	ReserveY     uint64    `json:"reserveY"`
	Slot         uint64    `json:"slot"`
	Timestamp    time.Time `json:"timestamp"`
	PrevPriceXY  float64   `json:"prevPriceXY,omitempty"`
	PriceChangeX float64   `json:"priceChangeX,omitempty"`
}

// NewPriceUpdate creates a PriceUpdate from a PoolState.
// Returns nil if state is nil.
func NewPriceUpdate(state *PoolState, prevPrice float64) *PriceUpdate {
	if state == nil {
		return nil
	}
	update := &PriceUpdate{
		PoolId:      state.PoolId,
		Protocol:    state.Protocol,
		AssetX:      state.AssetX.Class.Fingerprint(),
		AssetY:      state.AssetY.Class.Fingerprint(),
		PriceXY:     state.PriceXY(),
		PriceYX:     state.PriceYX(),
		ReserveX:    state.AssetX.Amount,
		ReserveY:    state.AssetY.Amount,
		Slot:        state.Slot,
		Timestamp:   state.Timestamp,
		PrevPriceXY: prevPrice,
	}
	if prevPrice > 0 {
		update.PriceChangeX = (update.PriceXY - prevPrice) / prevPrice * 100
	}
	return update
}

// PoolParser is the interface that protocol-specific parsers must implement
type PoolParser interface {
	// ParsePoolDatum parses a pool datum and returns the pool state
	ParsePoolDatum(
		datum []byte,
		utxoValue []byte, // CBOR-encoded UTXO value containing token amounts
		txHash string,
		txIndex uint32,
		slot uint64,
		timestamp time.Time,
	) (*PoolState, error)

	// Protocol returns the name of the protocol
	Protocol() string
}
