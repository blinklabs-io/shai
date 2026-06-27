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

package liqwid

import (
	"encoding/hex"
	"fmt"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/shai/internal/common"
)

// MarketState represents the parsed state of a Liqwid lending market
type MarketState struct {
	MarketId         string
	Protocol         string
	UnderlyingAsset  common.AssetAmount
	QTokenAsset      common.AssetClass
	TotalSupply      uint64
	TotalBorrows     uint64
	ReserveAmount    uint64
	InterestRate     uint64 // basis points
	CollateralFactor uint64 // basis points
	UtilizationRate  float64
	Slot             uint64
	TxHash           string
	TxIndex          uint32
	Timestamp        time.Time
}

// Key returns a unique key for this market state
func (m *MarketState) Key() string {
	return fmt.Sprintf("liqwid:%s", m.MarketId)
}

// AvailableLiquidity returns the amount available for borrowing
func (m *MarketState) AvailableLiquidity() uint64 {
	if m.TotalSupply <= m.TotalBorrows {
		return 0
	}
	return m.TotalSupply - m.TotalBorrows
}

// CollateralFactorFloat returns the collateral factor as a decimal
func (m *MarketState) CollateralFactorFloat() float64 {
	return float64(m.CollateralFactor) / float64(BasisPointsDenom)
}

// InterestRateFloat returns the interest rate as a decimal
func (m *MarketState) InterestRateFloat() float64 {
	return float64(m.InterestRate) / float64(BasisPointsDenom)
}

// SupplyState represents a user's supply position
type SupplyState struct {
	PositionId   string
	Owner        string
	MarketId     string
	QTokenAmount uint64
	DepositSlot  uint64
	Slot         uint64
	TxHash       string
	TxIndex      uint32
	Timestamp    time.Time
}

// Key returns a unique key for this supply position
func (s *SupplyState) Key() string {
	return fmt.Sprintf("liqwid:supply:%s", s.PositionId)
}

// BorrowState represents a user's borrow position
type BorrowState struct {
	PositionId   string
	Owner        string
	MarketId     string
	BorrowAmount uint64
	BorrowIndex  uint64
	BorrowSlot   uint64
	Slot         uint64
	TxHash       string
	TxIndex      uint32
	Timestamp    time.Time
}

// Key returns a unique key for this borrow position
func (b *BorrowState) Key() string {
	return fmt.Sprintf("liqwid:borrow:%s", b.PositionId)
}

// OracleState represents price oracle data
type OracleState struct {
	Asset       common.AssetClass
	Price       uint64
	Denominator uint64
	ValidFrom   time.Time
	ValidTo     time.Time
	Slot        uint64
	TxHash      string
	Timestamp   time.Time
}

// PriceFloat returns the price as a float64
func (o *OracleState) PriceFloat() float64 {
	if o.Denominator == 0 {
		return 0
	}
	return float64(o.Price) / float64(o.Denominator)
}

// Key returns a unique key for this oracle state
func (o *OracleState) Key() string {
	return fmt.Sprintf(
		"liqwid:oracle:%s.%s",
		hex.EncodeToString(o.Asset.PolicyId),
		hex.EncodeToString(o.Asset.Name),
	)
}

// Parser implements parsing for Liqwid protocol
type Parser struct{}

// NewParser creates a parser for Liqwid protocol
func NewParser() *Parser {
	return &Parser{}
}

// Protocol returns the protocol name
func (p *Parser) Protocol() string {
	return ProtocolName
}

// ParseMarketDatum parses a Liqwid market datum
func (p *Parser) ParseMarketDatum(
	datum []byte,
	txHash string,
	txIndex uint32,
	slot uint64,
	timestamp time.Time,
) (*MarketState, error) {
	var marketDatum MarketDatum
	if _, err := cbor.Decode(datum, &marketDatum); err != nil {
		return nil, fmt.Errorf("failed to decode Liqwid market datum: %w", err)
	}

	// Generate market ID from the market NFT
	marketId := GenerateMarketId(
		marketDatum.MarketNft.PolicyId,
		marketDatum.MarketNft.AssetName,
	)

	// Calculate utilization rate
	utilization := marketDatum.UtilizationRate()

	state := &MarketState{
		MarketId: marketId,
		Protocol: p.Protocol(),
		UnderlyingAsset: common.AssetAmount{
			Class:  marketDatum.UnderlyingAsset.ToCommonAssetClass(),
			Amount: marketDatum.TotalSupply,
		},
		QTokenAsset:      marketDatum.QTokenAsset.ToCommonAssetClass(),
		TotalSupply:      marketDatum.TotalSupply,
		TotalBorrows:     marketDatum.TotalBorrows,
		ReserveAmount:    marketDatum.ReserveAmount,
		InterestRate:     marketDatum.InterestRate,
		CollateralFactor: marketDatum.CollateralFactor,
		UtilizationRate:  utilization,
		Slot:             slot,
		TxHash:           txHash,
		TxIndex:          txIndex,
		Timestamp:        timestamp,
	}

	return state, nil
}

// ParseMarketDatumSimple parses just the market datum without state conversion
func (p *Parser) ParseMarketDatumSimple(datum []byte) (*MarketDatum, error) {
	var marketDatum MarketDatum
	if _, err := cbor.Decode(datum, &marketDatum); err != nil {
		return nil, fmt.Errorf("failed to decode Liqwid market datum: %w", err)
	}
	return &marketDatum, nil
}

// ParseSupplyPositionDatum parses a supply position datum
func (p *Parser) ParseSupplyPositionDatum(
	datum []byte,
	txHash string,
	txIndex uint32,
	slot uint64,
	timestamp time.Time,
) (*SupplyState, error) {
	var supplyDatum SupplyPositionDatum
	if _, err := cbor.Decode(datum, &supplyDatum); err != nil {
		return nil, fmt.Errorf("failed to decode Liqwid supply datum: %w", err)
	}

	// Generate position ID
	positionId := GeneratePositionId(txHash, txIndex)

	// Get market ID from NFT
	marketId := GenerateMarketId(
		supplyDatum.MarketNft.PolicyId,
		supplyDatum.MarketNft.AssetName,
	)

	// Get owner identifier
	owner := hex.EncodeToString(supplyDatum.Owner.Hash)

	state := &SupplyState{
		PositionId:   positionId,
		Owner:        owner,
		MarketId:     marketId,
		QTokenAmount: supplyDatum.QTokenAmount,
		DepositSlot:  supplyDatum.DepositSlot,
		Slot:         slot,
		TxHash:       txHash,
		TxIndex:      txIndex,
		Timestamp:    timestamp,
	}

	return state, nil
}

// ParseBorrowPositionDatum parses a borrow position datum
func (p *Parser) ParseBorrowPositionDatum(
	datum []byte,
	txHash string,
	txIndex uint32,
	slot uint64,
	timestamp time.Time,
) (*BorrowState, error) {
	var borrowDatum BorrowPositionDatum
	if _, err := cbor.Decode(datum, &borrowDatum); err != nil {
		return nil, fmt.Errorf("failed to decode Liqwid borrow datum: %w", err)
	}

	// Generate position ID
	positionId := GeneratePositionId(txHash, txIndex)

	// Get market ID from NFT
	marketId := GenerateMarketId(
		borrowDatum.MarketNft.PolicyId,
		borrowDatum.MarketNft.AssetName,
	)

	// Get owner identifier
	owner := hex.EncodeToString(borrowDatum.Owner.Hash)

	state := &BorrowState{
		PositionId:   positionId,
		Owner:        owner,
		MarketId:     marketId,
		BorrowAmount: borrowDatum.BorrowAmount,
		BorrowIndex:  borrowDatum.BorrowIndex,
		BorrowSlot:   borrowDatum.BorrowSlot,
		Slot:         slot,
		TxHash:       txHash,
		TxIndex:      txIndex,
		Timestamp:    timestamp,
	}

	return state, nil
}

// ParseOracleDatum parses a Liqwid oracle datum
func (p *Parser) ParseOracleDatum(
	datum []byte,
	txHash string,
	slot uint64,
	timestamp time.Time,
) (*OracleState, error) {
	var oracleDatum OracleDatum
	if _, err := cbor.Decode(datum, &oracleDatum); err != nil {
		return nil, fmt.Errorf("failed to decode Liqwid oracle datum: %w", err)
	}

	state := &OracleState{
		Asset:       oracleDatum.Asset.ToCommonAssetClass(),
		Price:       oracleDatum.Price,
		Denominator: oracleDatum.Denominator,
		ValidFrom:   time.Unix(oracleDatum.ValidFrom/1000, 0),
		ValidTo:     time.Unix(oracleDatum.ValidTo/1000, 0),
		Slot:        slot,
		TxHash:      txHash,
		Timestamp:   timestamp,
	}

	return state, nil
}

// GenerateMarketId generates a unique market ID from the market NFT
func GenerateMarketId(policyId, assetName []byte) string {
	return fmt.Sprintf(
		"liqwid_%s.%s",
		hex.EncodeToString(policyId),
		hex.EncodeToString(assetName),
	)
}

// GeneratePositionId generates a unique position ID
func GeneratePositionId(txHash string, txIndex uint32) string {
	if len(txHash) > 16 {
		return fmt.Sprintf("liqwid_pos_%s#%d", txHash[:16], txIndex)
	}
	return fmt.Sprintf("liqwid_pos_%s#%d", txHash, txIndex)
}

// Token Policy IDs for Liqwid protocol (confirmed on Cardanoscan)
const (
	// LQPolicyId is the policy ID for the LQ governance token
	// See: https://cardanoscan.io/token/da8c30857834c6ae7203935b89278c532b3995245295456f993e1d24.4c51
	LQPolicyId = "da8c30857834c6ae7203935b89278c532b3995245295456f993e1d24"

	// QADAPolicyId is the policy ID for the qADA interest-bearing token
	// See: https://cardanoscan.io/tokenPolicy/a04ce7a52545e5e33c2867e148898d9e667a69602285f6a1298f9d68
	QADAPolicyId = "a04ce7a52545e5e33c2867e148898d9e667a69602285f6a1298f9d68"

	// Charli3OracleAddress is the Charli3 oracle address used by Liqwid
	// Liqwid was the first Cardano protocol to integrate Charli3 oracles
	// See: https://cexplorer.io/address/addr1wyd8cezjr0gcf8nfxuc9trd4hs7ec520jmkwkqzywx6l5jg0al0ya
	Charli3OracleAddress = "addr1wyd8cezjr0gcf8nfxuc9trd4hs7ec520jmkwkqzywx6l5jg0al0ya"
)

// GetMarketAddresses returns mainnet Liqwid market addresses
// NOTE: Liqwid market validator script addresses are not publicly documented.
// The protocol uses Plutarch scripts with liqwid-markets.json internal config.
// Contact Liqwid Labs or analyze on-chain transactions for specific addresses.
// See: https://github.com/Liqwid-Labs
func GetMarketAddresses() []string {
	return []string{
		// Market addresses need to be obtained from Liqwid Labs
		// or by analyzing on-chain transactions via Cardanoscan
		// The protocol architecture uses:
		// - MarketInbox validator (market state and parameters)
		// - BatchFinal validator (batch finalization logic)
		// - DemandAction validator (redeem, borrow actions)
		// - SupplyAction validator (mint, repay actions)
	}
}

// GetOracleAddresses returns mainnet Liqwid oracle addresses
// Liqwid uses Charli3 for price feeds (first Cardano oracle integration)
// and also integrates Chainlink Price Feeds for ADA/USD
func GetOracleAddresses() []string {
	return []string{
		// Charli3 oracle (ADA/USD, SHEN/USD feeds)
		Charli3OracleAddress,
	}
}
