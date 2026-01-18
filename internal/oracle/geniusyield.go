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
	"time"

	"github.com/blinklabs-io/shai/internal/oracle/geniusyield"
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

// GenerateGeniusYieldOrderId wraps geniusyield.GenerateOrderId
func GenerateGeniusYieldOrderId(nftTokenName []byte) string {
	return geniusyield.GenerateOrderId(nftTokenName)
}

// GetGeniusYieldOrderAddresses returns mainnet order addresses
func GetGeniusYieldOrderAddresses() []string {
	return geniusyield.GetOrderAddresses()
}

// CalculateGeniusYieldFillAmount calculates fill amounts for an order
func CalculateGeniusYieldFillAmount(
	order *geniusyield.OrderState,
	askedAssetAmount uint64,
) (offeredAmount uint64, remainder uint64) {
	return geniusyield.CalculateFillAmount(order, askedAssetAmount)
}
