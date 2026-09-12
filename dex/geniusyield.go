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

package dex

import (
	"fmt"
	"math/big"
	"time"

	"github.com/blinklabs-io/shai/common"
	"github.com/blinklabs-io/shai/dex/geniusyield"
)

// Re-export constants for backward compatibility
const (
	GeniusYieldProtocolName    = geniusyield.ProtocolName
	GeniusYieldOrderScriptHash = geniusyield.OrderScriptHash
	GeniusYieldOrderNFTPolicy  = geniusyield.OrderNFTPolicy
)

// Re-export types for backward compatibility
type (
	GeniusYieldPartialOrderDatum = geniusyield.PartialOrderDatum
	GeniusYieldOrderState        = geniusyield.OrderState
	GeniusYieldAsset             = geniusyield.Asset
	GeniusYieldAddress           = geniusyield.Address
	GeniusYieldCredential        = geniusyield.Credential
	GeniusYieldRational          = geniusyield.Rational
	GeniusYieldOptionalPOSIX     = geniusyield.OptionalPOSIX
	GeniusYieldContainedFee      = geniusyield.ContainedFee
)

// GeniusYieldParser wraps geniusyield.Parser for backward compatibility
type GeniusYieldParser struct {
	parser *geniusyield.Parser
}

// NewGeniusYieldParser creates a parser for Genius Yield orders
func NewGeniusYieldParser() *GeniusYieldParser {
	return &GeniusYieldParser{parser: geniusyield.NewParser()}
}

// Protocol returns the protocol name
func (p *GeniusYieldParser) Protocol() string {
	return p.parser.Protocol()
}

// PoolAddresses returns the mainnet script addresses holding this protocol's
// order UTxOs. Query your node for UTxOs at these addresses, then feed each
// output's datum and value CBOR to ParsePoolDatum.
func (p *GeniusYieldParser) PoolAddresses() []string {
	return PoolAddresses(p.Protocol())
}

// ParseOrderDatum parses a Genius Yield order datum
func (p *GeniusYieldParser) ParseOrderDatum(
	datum []byte,
	txHash string,
	txIndex uint32,
	slot uint64,
	timestamp time.Time,
) (*geniusyield.OrderState, error) {
	return p.parser.ParseOrderDatum(datum, txHash, txIndex, slot, timestamp)
}

// ParsePoolDatum adapts an order-book order into the generic oracle PoolState.
func (p *GeniusYieldParser) ParsePoolDatum(
	datum []byte,
	utxoValue []byte,
	txHash string,
	txIndex uint32,
	slot uint64,
	timestamp time.Time,
) (*PoolState, error) {
	order, err := p.ParseOrderDatum(datum, txHash, txIndex, slot, timestamp)
	if err != nil {
		return nil, err
	}
	if order == nil || !order.IsActive {
		return nil, nil
	}

	askedAmount, err := geniusYieldAskedAmount(order)
	if err != nil {
		return nil, err
	}

	return &PoolState{
		PoolId:   order.OrderId,
		Protocol: order.Protocol,
		AssetX:   order.OfferedAsset,
		AssetY: common.AssetAmount{
			Class:  order.AskedAsset,
			Amount: askedAmount,
		},
		FeeNum:    1,
		FeeDenom:  1,
		Slot:      order.Slot,
		TxHash:    order.TxHash,
		TxIndex:   order.TxIndex,
		Timestamp: order.Timestamp,
	}, nil
}

func geniusYieldAskedAmount(order *geniusyield.OrderState) (uint64, error) {
	if order.PriceNum <= 0 || order.PriceDenom <= 0 {
		return 0, fmt.Errorf(
			"invalid Genius Yield price %d/%d",
			order.PriceNum,
			order.PriceDenom,
		)
	}
	if order.OfferedAsset.Amount == 0 {
		return 0, nil
	}

	offered := new(big.Int).SetUint64(order.OfferedAsset.Amount)
	num := big.NewInt(order.PriceNum)
	denom := big.NewInt(order.PriceDenom)
	asked := new(big.Int).Mul(offered, num)
	asked.Div(asked, denom)
	if asked.IsUint64() {
		return asked.Uint64(), nil
	}
	return 0, fmt.Errorf(
		"asked amount overflows uint64 for Genius Yield order %s",
		order.OrderId,
	)
}

// GenerateGeniusYieldOrderId wraps geniusyield.GenerateOrderId
func GenerateGeniusYieldOrderId(nftTokenName []byte) string {
	return geniusyield.GenerateOrderId(nftTokenName)
}

// GetGeniusYieldOrderAddresses returns mainnet order addresses
func GetGeniusYieldOrderAddresses() []string {
	return PoolAddresses(GeniusYieldProtocolName)
}

// CalculateGeniusYieldFillAmount calculates fill amounts for an order
func CalculateGeniusYieldFillAmount(
	order *geniusyield.OrderState,
	askedAssetAmount uint64,
) (offeredAmount uint64, remainder uint64) {
	return geniusyield.CalculateFillAmount(order, askedAssetAmount)
}
