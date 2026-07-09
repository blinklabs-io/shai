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
	"github.com/blinklabs-io/shai/common"
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
	return availableLiquidity(
		m.TotalSupply,
		m.TotalBorrows,
		m.ReserveAmount,
	)
}

// CollateralFactorFloat returns the collateral factor as a decimal
func (m *MarketState) CollateralFactorFloat() float64 {
	return basisPointsFloat(m.CollateralFactor)
}

// InterestRateFloat returns the interest rate as a decimal
func (m *MarketState) InterestRateFloat() float64 {
	return basisPointsFloat(m.InterestRate)
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
	owner := supplyDatum.Owner.Identifier()

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
	owner := borrowDatum.Owner.Identifier()

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
		ValidFrom:   time.UnixMilli(oracleDatum.ValidFrom),
		ValidTo:     time.UnixMilli(oracleDatum.ValidTo),
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
	return fmt.Sprintf("liqwid_pos_%s#%d", txHash, txIndex)
}

// Token Policy IDs for Liqwid protocol (confirmed on Cardanoscan).
// These are exported for consumers that need stable Liqwid asset identifiers
// in configuration, display, or integration code outside the parser itself.
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

	// MarketInboxAddress is the mainnet Liqwid market inbox validator address.
	MarketInboxAddress = "addr1w8dprfgfdxnlwu3948579jrwg0ferf5a63ln8xj0mqcdzegayxmqq"

	// BatchFinalAddress is the mainnet Liqwid batch final validator address.
	BatchFinalAddress = "addr1w9wjz8tjt87gldh2usu8t5mfe4nkmlngp30a387h8s94fyg5uup5n"

	// DemandActionAddress is the mainnet Liqwid demand action validator address.
	DemandActionAddress = "addr1wyw3ap36lnepstpjadwg8cg73llvmju4y94kmfld23lkzjggq4hyj"

	// SupplyActionAddress is the mainnet Liqwid supply action validator address.
	SupplyActionAddress = "addr1wxrxa3ucywn3lqpkzlyucak0a7aavkudh49fqt06yc05sws4l4zs2"
)

// GetMarketAddresses returns mainnet Liqwid market/action validator addresses.
func GetMarketAddresses() []string {
	return []string{
		MarketInboxAddress,
		BatchFinalAddress,
		DemandActionAddress,
		SupplyActionAddress,
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
