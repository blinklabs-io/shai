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

	"github.com/blinklabs-io/shai/internal/oracle/butane"
)

// Re-export constants for backward compatibility
const (
	ButaneProtocolName = butane.ProtocolName
	ButanePriceDenom   = butane.PriceDenom
)

// Re-export types for backward compatibility
type (
	ButaneMonoDatum     = butane.MonoDatum
	ButaneCDP           = butane.CDP
	ButaneCDPCredential = butane.CDPCredential
	ButaneAssetClass    = butane.AssetClass
	ButanePriceFeed     = butane.PriceFeed
	ButaneCDPState      = butane.CDPState
	ButanePriceState    = butane.PriceState
)

// ButaneParser wraps butane.Parser for backward compatibility
type ButaneParser struct {
	parser *butane.Parser
}

// NewButaneParser creates a parser for Butane protocol
func NewButaneParser() *ButaneParser {
	return &ButaneParser{parser: butane.NewParser()}
}

// Protocol returns the protocol name
func (p *ButaneParser) Protocol() string {
	return p.parser.Protocol()
}

// ParseMonoDatum parses a Butane MonoDatum and returns the CDP if present
func (p *ButaneParser) ParseMonoDatum(
	datum []byte,
	txHash string,
	txIndex uint32,
	slot uint64,
	timestamp time.Time,
) (*ButaneCDPState, error) {
	return p.parser.ParseMonoDatum(datum, txHash, txIndex, slot, timestamp)
}

// ParsePoolDatum implements PoolParser interface for compatibility
// Butane is a synthetics protocol, not an AMM, so this returns nil
func (p *ButaneParser) ParsePoolDatum(
	datum []byte,
	txHash string,
	txIndex uint32,
	slot uint64,
	timestamp time.Time,
) (*PoolState, error) {
	// Butane doesn't have AMM pools, return nil
	return nil, nil
}

// generateButaneCDPId wraps butane.GenerateCDPId for backward compatibility
func generateButaneCDPId(txHash string, txIndex uint32) string {
	return butane.GenerateCDPId(txHash, txIndex)
}

// GetButaneAddresses returns mainnet Butane contract addresses
func GetButaneAddresses() []string {
	return butane.GetAddresses()
}
