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

// LiqwidLendingAdapter wraps the Liqwid parser to implement LendingParser
type LiqwidLendingAdapter struct {
	parser *liqwid.Parser
}

// NewLiqwidLendingAdapter creates a new Liqwid lending adapter
func NewLiqwidLendingAdapter() *LiqwidLendingAdapter {
	return &LiqwidLendingAdapter{
		parser: liqwid.NewParser(),
	}
}

// Protocol returns the protocol name
func (a *LiqwidLendingAdapter) Protocol() string {
	return a.parser.Protocol()
}

// ParseDatum parses a Liqwid datum and returns a unified LendingState
func (a *LiqwidLendingAdapter) ParseDatum(
	datum []byte,
	txHash string,
	txIndex uint32,
	slot uint64,
	timestamp time.Time,
) (*LendingState, error) {
	// Try to parse as market datum first (most common)
	marketState, err := a.parser.ParseMarketDatum(
		datum,
		txHash,
		txIndex,
		slot,
		timestamp,
	)
	if err == nil {
		return a.marketToLendingState(marketState), nil
	}

	// Try supply position
	supplyState, err := a.parser.ParseSupplyPositionDatum(
		datum,
		txHash,
		txIndex,
		slot,
		timestamp,
	)
	if err == nil {
		return a.supplyToLendingState(supplyState), nil
	}

	// Try borrow position
	borrowState, err := a.parser.ParseBorrowPositionDatum(
		datum,
		txHash,
		txIndex,
		slot,
		timestamp,
	)
	if err == nil {
		return a.borrowToLendingState(borrowState), nil
	}

	// Return the original market error if all parsing attempts fail
	return nil, err
}

// GetAddresses returns the monitored addresses for Liqwid
func (a *LiqwidLendingAdapter) GetAddresses() []string {
	return liqwid.GetMarketAddresses()
}

// marketToLendingState converts a Liqwid MarketState to LendingState
func (a *LiqwidLendingAdapter) marketToLendingState(
	m *liqwid.MarketState,
) *LendingState {
	return &LendingState{
		StateId:          m.MarketId,
		StateType:        LendingStateTypeMarket,
		Protocol:         m.Protocol,
		TotalSupply:      m.TotalSupply,
		TotalBorrows:     m.TotalBorrows,
		AvailableLiq:     m.AvailableLiquidity(),
		UtilizationRate:  m.UtilizationRate,
		InterestRate:     m.InterestRate,
		CollateralFactor: m.CollateralFactor,
		InterestRatePct:  m.InterestRateFloat(),
		UnderlyingAsset:  m.UnderlyingAsset.Class,
		LpToken:          m.QTokenAsset,
		Slot:             m.Slot,
		TxHash:           m.TxHash,
		TxIndex:          m.TxIndex,
		Timestamp:        m.Timestamp,
	}
}

// supplyToLendingState converts a Liqwid SupplyState to LendingState
func (a *LiqwidLendingAdapter) supplyToLendingState(
	s *liqwid.SupplyState,
) *LendingState {
	return &LendingState{
		StateId:     s.PositionId,
		StateType:   LendingStateTypePosition,
		Protocol:    liqwid.ProtocolName,
		TotalSupply: s.QTokenAmount,
		Lender:      s.Owner,
		Slot:        s.Slot,
		TxHash:      s.TxHash,
		TxIndex:     s.TxIndex,
		Timestamp:   s.Timestamp,
	}
}

// borrowToLendingState converts a Liqwid BorrowState to LendingState
func (a *LiqwidLendingAdapter) borrowToLendingState(
	b *liqwid.BorrowState,
) *LendingState {
	return &LendingState{
		StateId:    b.PositionId,
		StateType:  LendingStateTypePosition,
		Protocol:   liqwid.ProtocolName,
		LoanAmount: b.BorrowAmount,
		Borrower:   b.Owner,
		Slot:       b.Slot,
		TxHash:     b.TxHash,
		TxIndex:    b.TxIndex,
		Timestamp:  b.Timestamp,
	}
}

// GetLendingParser returns the appropriate LendingParser for a protocol
func GetLendingParser(protocol string) LendingParser {
	switch protocol {
	case "liqwid":
		return NewLiqwidLendingAdapter()
	default:
		return nil
	}
}
