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

package oracle

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/blinklabs-io/shai/internal/common"
)

// PoolState represents the current state of a liquidity pool
type PoolState struct {
	PoolId      string             `json:"poolId"`
	Network     string             `json:"network"`
	Protocol    string             `json:"protocol"`
	AssetX      common.AssetAmount `json:"assetX"`
	AssetY      common.AssetAmount `json:"assetY"`
	FeeNum      uint64             `json:"feeNum"`
	FeeDenom    uint64             `json:"feeDenom"`
	Slot        uint64             `json:"slot"`
	BlockHash   string             `json:"blockHash"`
	TxHash      string             `json:"txHash"`
	TxIndex     uint32             `json:"txIndex"`
	Timestamp   time.Time          `json:"timestamp"`
	UpdatedAt   time.Time          `json:"updatedAt"`
	FromMempool bool               `json:"fromMempool"` // True if state is from mempool (unconfirmed)
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

// NewPriceUpdate creates a PriceUpdate from a PoolState
func NewPriceUpdate(state *PoolState, prevPrice float64) *PriceUpdate {
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
		txHash string,
		txIndex uint32,
		slot uint64,
		timestamp time.Time,
	) (*PoolState, error)

	// Protocol returns the name of the protocol
	Protocol() string
}
