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

	"github.com/blinklabs-io/shai/internal/oracle/liqwid"
)

// Re-export constants for backward compatibility
const (
	LiqwidProtocolName      = liqwid.ProtocolName
	LiqwidBasisPointsDenom  = liqwid.BasisPointsDenom
	LiqwidInterestPrecision = liqwid.InterestRatePrecision
	LiqwidExchangePrecision = liqwid.ExchangeRatePrecision
)

// Re-export types for backward compatibility
type (
	LiqwidMarketDatum            = liqwid.MarketDatum
	LiqwidAsset                  = liqwid.Asset
	LiqwidCredential             = liqwid.Credential
	LiqwidSupplyPositionDatum    = liqwid.SupplyPositionDatum
	LiqwidBorrowPositionDatum    = liqwid.BorrowPositionDatum
	LiqwidOracleDatum            = liqwid.OracleDatum
	LiqwidInterestRateModelDatum = liqwid.InterestRateModelDatum
	LiqwidMarketState            = liqwid.MarketState
	LiqwidSupplyState            = liqwid.SupplyState
	LiqwidBorrowState            = liqwid.BorrowState
	LiqwidOracleState            = liqwid.OracleState
)

// LiqwidParser wraps liqwid.Parser for backward compatibility
type LiqwidParser struct {
	parser *liqwid.Parser
}

// NewLiqwidParser creates a parser for Liqwid protocol
func NewLiqwidParser() *LiqwidParser {
	return &LiqwidParser{parser: liqwid.NewParser()}
}

// Protocol returns the protocol name
func (p *LiqwidParser) Protocol() string {
	return p.parser.Protocol()
}

// ParseMarketDatum parses a Liqwid market datum
func (p *LiqwidParser) ParseMarketDatum(
	datum []byte,
	txHash string,
	txIndex uint32,
	slot uint64,
	timestamp time.Time,
) (*LiqwidMarketState, error) {
	return p.parser.ParseMarketDatum(datum, txHash, txIndex, slot, timestamp)
}

// ParseMarketDatumSimple parses just the market datum without state conversion
func (p *LiqwidParser) ParseMarketDatumSimple(
	datum []byte,
) (*LiqwidMarketDatum, error) {
	return p.parser.ParseMarketDatumSimple(datum)
}

// ParseSupplyPositionDatum parses a supply position datum
func (p *LiqwidParser) ParseSupplyPositionDatum(
	datum []byte,
	txHash string,
	txIndex uint32,
	slot uint64,
	timestamp time.Time,
) (*LiqwidSupplyState, error) {
	return p.parser.ParseSupplyPositionDatum(
		datum,
		txHash,
		txIndex,
		slot,
		timestamp,
	)
}

// ParseBorrowPositionDatum parses a borrow position datum
func (p *LiqwidParser) ParseBorrowPositionDatum(
	datum []byte,
	txHash string,
	txIndex uint32,
	slot uint64,
	timestamp time.Time,
) (*LiqwidBorrowState, error) {
	return p.parser.ParseBorrowPositionDatum(
		datum,
		txHash,
		txIndex,
		slot,
		timestamp,
	)
}

// ParseOracleDatum parses a Liqwid oracle datum
func (p *LiqwidParser) ParseOracleDatum(
	datum []byte,
	txHash string,
	slot uint64,
	timestamp time.Time,
) (*LiqwidOracleState, error) {
	return p.parser.ParseOracleDatum(datum, txHash, slot, timestamp)
}

// generateLiqwidMarketId wraps liqwid.GenerateMarketId for backward compatibility
func generateLiqwidMarketId(policyId, assetName []byte) string {
	return liqwid.GenerateMarketId(policyId, assetName)
}

// generateLiqwidPositionId wraps liqwid.GeneratePositionId for backward compatibility
func generateLiqwidPositionId(txHash string, txIndex uint32) string {
	return liqwid.GeneratePositionId(txHash, txIndex)
}

// GetLiqwidMarketAddresses returns mainnet market addresses
func GetLiqwidMarketAddresses() []string {
	return liqwid.GetMarketAddresses()
}

// GetLiqwidOracleAddresses returns mainnet oracle addresses
func GetLiqwidOracleAddresses() []string {
	return liqwid.GetOracleAddresses()
}
