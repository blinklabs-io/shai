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

package geniusyield

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/shai/common"
)

// OrderState represents the parsed state of a Genius Yield order
type OrderState struct {
	OrderId        string             `json:"orderId"`
	Protocol       string             `json:"protocol"`
	Owner          string             `json:"owner"`
	OfferedAsset   common.AssetAmount `json:"offeredAsset"`
	OriginalAmount uint64             `json:"originalAmount"`
	AskedAsset     common.AssetClass  `json:"askedAsset"`
	Price          float64            `json:"price"`
	PriceNum       int64              `json:"priceNum"`
	PriceDenom     int64              `json:"priceDenom"`
	IsActive       bool               `json:"isActive"`
	StartTime      *time.Time         `json:"startTime"`
	EndTime        *time.Time         `json:"endTime"`
	PartialFills   uint64             `json:"partialFills"`
	Slot           uint64             `json:"slot"`
	TxHash         string             `json:"txHash"`
	TxIndex        uint32             `json:"txIndex"`
	Timestamp      time.Time          `json:"timestamp"`
	UpdatedAt      time.Time          `json:"updatedAt"`

	// Fee and datum fields preserved for partial fill updates
	NFT                  []byte `json:"nft"`
	MakerLovelaceFlatFee uint64 `json:"makerLovelaceFlatFee"`
	MakerFeeNum          int64  `json:"makerFeeNum"`
	MakerFeeDenom        int64  `json:"makerFeeDenom"`
	MakerFeeMax          uint64 `json:"makerFeeMax"`
	ContainedLovelaceFee uint64 `json:"containedLovelaceFee"`
	ContainedOfferedFee  uint64 `json:"containedOfferedFee"`
	ContainedAskedFee    uint64 `json:"containedAskedFee"`
	ContainedPayment     uint64 `json:"containedPayment"`
}

// Key returns a unique identifier for this order state
func (o *OrderState) Key() string {
	return fmt.Sprintf("geniusyield:%s", o.OrderId)
}

// FillPercent returns the percentage of the order that has been filled
func (o *OrderState) FillPercent() float64 {
	if o.OriginalAmount == 0 {
		return 0
	}
	// Guard against underflow if OfferedAsset.Amount > OriginalAmount
	if o.OfferedAsset.Amount >= o.OriginalAmount {
		return 0
	}
	filled := o.OriginalAmount - o.OfferedAsset.Amount
	return float64(filled) / float64(o.OriginalAmount) * 100
}

// RemainingValue returns the value of remaining offered assets at current price
func (o *OrderState) RemainingValue() float64 {
	return float64(o.OfferedAsset.Amount) * o.Price
}

// Parser implements order parsing for Genius Yield DEX
type Parser struct{}

// NewParser creates a new Genius Yield order parser
func NewParser() *Parser {
	return &Parser{}
}

// Protocol returns the protocol name
func (p *Parser) Protocol() string {
	return ProtocolName
}

// ParseOrderDatum parses a Genius Yield order datum
func (p *Parser) ParseOrderDatum(
	datum []byte,
	txHash string,
	txIndex uint32,
	slot uint64,
	timestamp time.Time,
) (*OrderState, error) {
	var orderDatum PartialOrderDatum
	if _, err := cbor.Decode(datum, &orderDatum); err != nil {
		return nil, fmt.Errorf("failed to decode Genius Yield datum: %w", err)
	}
	if orderDatum.Price.Numerator <= 0 || orderDatum.Price.Denominator <= 0 {
		return nil, fmt.Errorf(
			"invalid Genius Yield price: numerator=%d denominator=%d",
			orderDatum.Price.Numerator,
			orderDatum.Price.Denominator,
		)
	}

	// Generate order ID from the NFT token name
	orderId := GenerateOrderId(orderDatum.NFT)

	// Check if order is active
	isActive := p.isOrderActive(orderDatum, timestamp)

	// Convert timestamps
	var startTime, endTime *time.Time
	if orderDatum.Start.IsPresent {
		t := time.UnixMilli(orderDatum.Start.Time)
		startTime = &t
	}
	if orderDatum.End.IsPresent {
		t := time.UnixMilli(orderDatum.End.Time)
		endTime = &t
	}

	state := &OrderState{
		OrderId:  orderId,
		Protocol: p.Protocol(),
		Owner:    hex.EncodeToString(orderDatum.OwnerKey),
		OfferedAsset: common.AssetAmount{
			Class:  orderDatum.OfferedAsset.ToCommonAssetClass(),
			Amount: orderDatum.OfferedAmount,
		},
		OriginalAmount: orderDatum.OfferedOriginalAmount,
		AskedAsset:     orderDatum.AskedAsset.ToCommonAssetClass(),
		Price:          orderDatum.Price.ToFloat64(),
		PriceNum:       orderDatum.Price.Numerator,
		PriceDenom:     orderDatum.Price.Denominator,
		IsActive:       isActive,
		StartTime:      startTime,
		EndTime:        endTime,
		PartialFills:   orderDatum.PartialFills,
		Slot:           slot,
		TxHash:         txHash,
		TxIndex:        txIndex,
		Timestamp:      timestamp,
		UpdatedAt:      time.Now(),

		// Preserve fee and datum fields for partial fill reconstruction
		NFT:                  orderDatum.NFT,
		MakerLovelaceFlatFee: orderDatum.MakerLovelaceFlatFee,
		MakerFeeNum:          orderDatum.MakerOfferedPercentFee.Numerator,
		MakerFeeDenom:        orderDatum.MakerOfferedPercentFee.Denominator,
		MakerFeeMax:          orderDatum.MakerOfferedPercentFeeMax,
		ContainedLovelaceFee: orderDatum.ContainedFee.LovelaceFee,
		ContainedOfferedFee:  orderDatum.ContainedFee.OfferedFee,
		ContainedAskedFee:    orderDatum.ContainedFee.AskedFee,
		ContainedPayment:     orderDatum.ContainedPayment,
	}

	return state, nil
}

// isOrderActive checks if an order is currently active
func (p *Parser) isOrderActive(datum PartialOrderDatum, now time.Time) bool {
	// Order is inactive if no amount remaining
	if datum.OfferedAmount == 0 {
		return false
	}

	nowMs := now.UnixMilli()

	// Check start time constraint
	if datum.Start.IsPresent && datum.Start.Time > nowMs {
		return false // Order hasn't started yet
	}

	// Check end time constraint
	if datum.End.IsPresent && datum.End.Time < nowMs {
		return false // Order has expired
	}

	return true
}

// GenerateOrderId generates a unique order ID from the NFT token name
func GenerateOrderId(nftTokenName []byte) string {
	return fmt.Sprintf("gy_%s", hex.EncodeToString(nftTokenName))
}

// GetOrderAddresses returns mainnet order contract addresses
func GetOrderAddresses() []string {
	return []string{
		// Genius Yield order-book DEX contract address (mainnet)
		// The actual address derived from OrderScriptHash
		"addr1w8lj5fvnqvx8rtp8k6e6kcp7g76twqv2ad2hg7avfqtj7qgc5rquk",
	}
}

// CalculateFillAmount calculates how much of an order can be filled
// given a specific amount of the asked asset.
// Uses integer arithmetic to avoid float64 precision loss on large amounts.
func CalculateFillAmount(
	order *OrderState,
	askedAssetAmount uint64,
) (offeredAmount uint64, remainder uint64) {
	if order.PriceNum <= 0 || order.PriceDenom <= 0 {
		return 0, askedAssetAmount
	}

	// Calculate offered amount based on price using big.Int to avoid overflow
	// offeredAmount = askedAssetAmount * priceDenom / priceNum
	asked := new(big.Int).SetUint64(askedAssetAmount)
	denom := new(big.Int).SetInt64(order.PriceDenom)
	num := new(big.Int).SetInt64(order.PriceNum)

	// maxOffered = asked * denom / num
	maxOfferedBig := new(big.Int).Mul(asked, denom)
	maxOfferedBig.Div(maxOfferedBig, num)

	// Cap at uint64 max and available amount
	maxOffered := uint64(0)
	if maxOfferedBig.Sign() >= 0 && maxOfferedBig.IsUint64() {
		maxOffered = maxOfferedBig.Uint64()
	} else if maxOfferedBig.Sign() > 0 {
		maxOffered = ^uint64(0) // max uint64 if overflow
	}

	// Cap at available amount
	if maxOffered > order.OfferedAsset.Amount {
		offeredAmount = order.OfferedAsset.Amount
	} else {
		offeredAmount = maxOffered
	}

	// Calculate how much asked asset was actually used for the offered amount
	// usedAsked = ceil(offeredAmount * priceNum / priceDenom)
	// This accounts for integer division truncation in both branches
	offered := new(big.Int).SetUint64(offeredAmount)
	usedAskedBig := new(big.Int).Mul(offered, num)
	usedAskedBig = ceilDivPositiveBig(usedAskedBig, denom)

	usedAsked := uint64(0)
	if usedAskedBig.Sign() >= 0 && usedAskedBig.IsUint64() {
		usedAsked = usedAskedBig.Uint64()
	}
	if usedAsked > askedAssetAmount {
		remainder = 0
	} else {
		remainder = askedAssetAmount - usedAsked
	}

	return offeredAmount, remainder
}

func ceilDivPositiveBig(numerator *big.Int, denominator *big.Int) *big.Int {
	quotient, remainder := new(big.Int).QuoRem(
		numerator,
		denominator,
		new(big.Int),
	)
	if remainder.Sign() != 0 {
		quotient.Add(quotient, big.NewInt(1))
	}
	return quotient
}
