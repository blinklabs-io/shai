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
	"time"

	"github.com/blinklabs-io/shai/common"
)

// OrderState represents the parsed state of a Genius Yield order
type OrderState struct {
	OrderId        string            `json:"orderId"`
	Protocol       string            `json:"protocol"`
	Owner          string            `json:"owner"`
	OfferedAsset   common.AssetClass `json:"offeredAsset"`
	OfferedAmount  uint64            `json:"offeredAmount"`
	OriginalAmount uint64            `json:"originalAmount"`
	AskedAsset     common.AssetClass `json:"askedAsset"`
	Price          float64           `json:"price"`
	PriceNum       int64             `json:"priceNum"`
	PriceDenom     int64             `json:"priceDenom"`
	IsActive       bool              `json:"isActive"`
	StartTime      *time.Time        `json:"startTime"`
	EndTime        *time.Time        `json:"endTime"`
	PartialFills   uint64            `json:"partialFills"`
	Slot           uint64            `json:"slot"`
	TxHash         string            `json:"txHash"`
	TxIndex        uint32            `json:"txIndex"`
	Timestamp      time.Time         `json:"timestamp"`
	UpdatedAt      time.Time         `json:"updatedAt"`

	// Fields preserved for partial fill datum reconstruction
	OwnerAddr            OrderAddress `json:"ownerAddr"`            // Owner payment address
	NFT                  []byte       `json:"nft"`                  // Order NFT token name
	MakerLovelaceFlatFee uint64       `json:"makerLovelaceFlatFee"` // Flat maker fee
	MakerFeeNum          int64        `json:"makerFeeNum"`          // Maker fee numerator
	MakerFeeDenom        int64        `json:"makerFeeDenom"`        // Maker fee denominator
	MakerFeeMax          uint64       `json:"makerFeeMax"`          // Max maker fee
	ContainedLovelaceFee uint64       `json:"containedLovelaceFee"` // Contained lovelace fee
	ContainedOfferedFee  uint64       `json:"containedOfferedFee"`  // Contained offered fee
	ContainedAskedFee    uint64       `json:"containedAskedFee"`    // Contained asked fee
	ContainedPayment     uint64       `json:"containedPayment"`     // Contained payment
}

// OrderConfigToState converts an OrderConfig to OrderState
func OrderConfigToState(
	cfg *OrderConfig,
	txHash string,
	txIndex uint32,
	slot uint64,
) *OrderState {
	orderId := fmt.Sprintf("gy_%s", hex.EncodeToString(cfg.NFT))

	var startTime, endTime *time.Time
	if cfg.Start.IsPresent {
		t := time.UnixMilli(cfg.Start.Time)
		startTime = &t
	}
	if cfg.End.IsPresent {
		t := time.UnixMilli(cfg.End.Time)
		endTime = &t
	}

	isActive := cfg.OfferedAmount > 0
	now := time.Now()
	if cfg.Start.IsPresent && time.UnixMilli(cfg.Start.Time).After(now) {
		isActive = false
	}
	if cfg.End.IsPresent && time.UnixMilli(cfg.End.Time).Before(now) {
		isActive = false
	}

	return &OrderState{
		OrderId:        orderId,
		Protocol:       "geniusyield",
		Owner:          hex.EncodeToString(cfg.OwnerKey),
		OfferedAsset:   cfg.OfferedAsset.ToCommon(),
		OfferedAmount:  cfg.OfferedAmount,
		OriginalAmount: cfg.OfferedOriginalAmount,
		AskedAsset:     cfg.AskedAsset.ToCommon(),
		Price:          cfg.Price.ToFloat64(),
		PriceNum:       cfg.Price.Numerator,
		PriceDenom:     cfg.Price.Denominator,
		IsActive:       isActive,
		StartTime:      startTime,
		EndTime:        endTime,
		PartialFills:   cfg.PartialFills,
		Slot:           slot,
		TxHash:         txHash,
		TxIndex:        txIndex,
		Timestamp:      time.Now(),
		UpdatedAt:      time.Now(),

		OwnerAddr:            cfg.OwnerAddr,
		NFT:                  cfg.NFT,
		MakerLovelaceFlatFee: cfg.MakerLovelaceFlatFee,
		MakerFeeNum:          cfg.MakerOfferedPercentFee.Numerator,
		MakerFeeDenom:        cfg.MakerOfferedPercentFee.Denominator,
		MakerFeeMax:          cfg.MakerOfferedPercentFeeMax,
		ContainedLovelaceFee: cfg.ContainedFee.LovelaceFee,
		ContainedOfferedFee:  cfg.ContainedFee.OfferedFee,
		ContainedAskedFee:    cfg.ContainedFee.AskedFee,
		ContainedPayment:     cfg.ContainedPayment,
	}
}
