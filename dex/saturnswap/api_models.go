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

package saturnswap

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/blinklabs-io/shai/common"
)

const FeeDenom = 10000

// DecimalString preserves GraphQL decimal values that may be encoded as JSON
// strings or numbers.
type DecimalString string

func (d *DecimalString) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if bytes.Equal(data, []byte("null")) {
		*d = ""
		return nil
	}
	if len(data) == 0 {
		return fmt.Errorf("empty decimal value")
	}
	if data[0] == '"' {
		var value string
		if err := json.Unmarshal(data, &value); err != nil {
			return err
		}
		*d = DecimalString(value)
		return nil
	}
	*d = DecimalString(string(data))
	return nil
}

func (d DecimalString) String() string {
	return string(d)
}

func (d DecimalString) Float64() (float64, error) {
	value := strings.TrimSpace(string(d))
	if value == "" {
		return 0, fmt.Errorf("missing decimal value")
	}
	ret, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid decimal value %q: %w", value, err)
	}
	return ret, nil
}

func (d DecimalString) Uint64() (uint64, error) {
	value := strings.TrimSpace(string(d))
	if value == "" {
		return 0, fmt.Errorf("missing decimal value")
	}
	if strings.HasPrefix(value, "-") {
		return 0, fmt.Errorf("decimal value %q out of uint64 range", d)
	}
	if before, after, ok := strings.Cut(value, "."); ok {
		if strings.TrimRight(after, "0") != "" {
			return 0, fmt.Errorf("decimal value %q is not an integer", d)
		}
		value = before
	}
	ret, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid uint64 decimal value %q: %w", d, err)
	}
	return ret, nil
}

// TokenProject is the token metadata shape used in SaturnSwap pool responses.
type TokenProject struct {
	ID         string        `json:"id,omitempty"`
	Name       string        `json:"name,omitempty"`
	Image      string        `json:"image,omitempty"`
	PolicyID   string        `json:"policy_id,omitempty"`
	AssetName  string        `json:"asset_name,omitempty"`
	Decimals   uint32        `json:"decimals,omitempty"`
	Precision  uint32        `json:"precision,omitempty"`
	Ticker     string        `json:"ticker,omitempty"`
	Price      DecimalString `json:"price,omitempty"`
	IsVerified bool          `json:"is_verified,omitempty"`
	IsActive   bool          `json:"is_active,omitempty"`
}

func (t TokenProject) AssetClass() (common.AssetClass, error) {
	policyID := cleanHex(t.PolicyID)
	assetName := cleanHex(t.AssetName)
	if policyID == "" && assetName == "" {
		return common.Lovelace(), nil
	}
	if policyID == "" {
		return common.AssetClass{}, fmt.Errorf(
			"token %q has asset name %q without policy ID",
			t.Ticker,
			t.AssetName,
		)
	}
	asset, err := common.NewAssetClass(policyID, assetName)
	if err != nil {
		return common.AssetClass{}, fmt.Errorf(
			"token %q asset class: %w",
			t.Ticker,
			err,
		)
	}
	return asset, nil
}

func cleanHex(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "0x")
	value = strings.TrimPrefix(value, "0X")
	return strings.ToLower(value)
}

// PoolStats contains the reserve and market fields needed for oracle snapshots.
type PoolStats struct {
	PoolID            string        `json:"pool_id,omitempty"`
	Price             DecimalString `json:"price,omitempty"`
	HighestBid        DecimalString `json:"highest_bid,omitempty"`
	LowestAsk         DecimalString `json:"lowest_ask,omitempty"`
	ReserveTokenOne   DecimalString `json:"reserve_token_one,omitempty"`
	ReserveTokenTwo   DecimalString `json:"reserve_token_two,omitempty"`
	TVL               DecimalString `json:"tvl,omitempty"`
	Volume1D          DecimalString `json:"volume_1d,omitempty"`
	BuyVolume1D       DecimalString `json:"buy_volume_1d,omitempty"`
	SellVolume1D      DecimalString `json:"sell_volume_1d,omitempty"`
	Transactions1D    uint64        `json:"transactions_1d,omitempty"`
	Buys1D            uint64        `json:"buys_1d,omitempty"`
	Sells1D           uint64        `json:"sells_1d,omitempty"`
	UserFeesEarned1D  DecimalString `json:"user_fees_earned_1d,omitempty"`
	Volume7D          DecimalString `json:"volume_7d,omitempty"`
	BuyVolume7D       DecimalString `json:"buy_volume_7d,omitempty"`
	SellVolume7D      DecimalString `json:"sell_volume_7d,omitempty"`
	Transactions7D    uint64        `json:"transactions_7d,omitempty"`
	Buys7D            uint64        `json:"buys_7d,omitempty"`
	Sells7D           uint64        `json:"sells_7d,omitempty"`
	UserFeesEarned7D  DecimalString `json:"user_fees_earned_7d,omitempty"`
	VolumeAll         DecimalString `json:"volume_all,omitempty"`
	BuyVolumeAll      DecimalString `json:"buy_volume_all,omitempty"`
	SellVolumeAll     DecimalString `json:"sell_volume_all,omitempty"`
	TransactionsAll   uint64        `json:"transactions_all,omitempty"`
	BuysAll           uint64        `json:"buys_all,omitempty"`
	SellsAll          uint64        `json:"sells_all,omitempty"`
	UserFeesEarnedAll DecimalString `json:"user_fees_earned_all,omitempty"`
	AverageAPY1D      DecimalString `json:"average_apy_1d,omitempty"`
	AverageAPY7D      DecimalString `json:"average_apy_7d,omitempty"`
	AverageAPY30D     DecimalString `json:"average_apy_30d,omitempty"`
	Volume30D         DecimalString `json:"volume_30d,omitempty"`
	BuyVolume30D      DecimalString `json:"buy_volume_30d,omitempty"`
	SellVolume30D     DecimalString `json:"sell_volume_30d,omitempty"`
	Transactions30D   uint64        `json:"transactions_30d,omitempty"`
	Buys30D           uint64        `json:"buys_30d,omitempty"`
	Sells30D          uint64        `json:"sells_30d,omitempty"`
	UserFeesEarned30D DecimalString `json:"user_fees_earned_30d,omitempty"`
}

// Pool is the public SaturnSwap pool shape used by the web app and docs.
type Pool struct {
	ID                 string        `json:"id"`
	Name               string        `json:"name,omitempty"`
	Ticker             string        `json:"ticker,omitempty"`
	LPFeePercent       DecimalString `json:"lp_fee_percent,omitempty"`
	ProtocolFeePercent DecimalString `json:"protocol_fee_percent,omitempty"`
	IsSwapActive       bool          `json:"is_swap_active,omitempty"`
	IsLiquidityActive  bool          `json:"is_liquidity_active,omitempty"`
	IsVerified         bool          `json:"is_verified,omitempty"`
	TokenProjectOne    TokenProject  `json:"token_project_one,omitempty"`
	TokenProjectTwo    TokenProject  `json:"token_project_two,omitempty"`
	PoolStats          PoolStats     `json:"pool_stats,omitempty"`
}

func (p Pool) ToPoolState(slot uint64, timestamp time.Time) (*PoolState, error) {
	if strings.TrimSpace(p.ID) == "" {
		return nil, fmt.Errorf("pool ID is required")
	}
	assetX, err := p.TokenProjectOne.AssetClass()
	if err != nil {
		return nil, fmt.Errorf("asset X: %w", err)
	}
	assetY, err := p.TokenProjectTwo.AssetClass()
	if err != nil {
		return nil, fmt.Errorf("asset Y: %w", err)
	}
	reserveX, err := p.PoolStats.ReserveTokenOne.Uint64()
	if err != nil {
		return nil, fmt.Errorf("reserve token one: %w", err)
	}
	reserveY, err := p.PoolStats.ReserveTokenTwo.Uint64()
	if err != nil {
		return nil, fmt.Errorf("reserve token two: %w", err)
	}
	feeNum, feeDenom, err := p.EffectiveFeeParts()
	if err != nil {
		return nil, err
	}
	return &PoolState{
		PoolId:   p.ID,
		Protocol: ProtocolName,
		AssetX: common.AssetAmount{
			Class:  assetX,
			Amount: reserveX,
		},
		AssetY: common.AssetAmount{
			Class:  assetY,
			Amount: reserveY,
		},
		FeeNum:    feeNum,
		FeeDenom:  feeDenom,
		Slot:      slot,
		Timestamp: timestamp,
	}, nil
}

func (p Pool) EffectiveFeeParts() (uint64, uint64, error) {
	lpPercent, err := p.LPFeePercent.Float64()
	if err != nil {
		return 0, 0, fmt.Errorf("lp_fee_percent: %w", err)
	}
	// protocol_fee_percent is optional; treat a missing value as zero
	// rather than erroring so pools without a protocol surcharge still
	// produce a valid fee.
	protocolPercent := 0.0
	if strings.TrimSpace(p.ProtocolFeePercent.String()) != "" {
		protocolPercent, err = p.ProtocolFeePercent.Float64()
		if err != nil {
			return 0, 0, fmt.Errorf("protocol_fee_percent: %w", err)
		}
	}
	percent := lpPercent + protocolPercent
	feeBasisPoints := math.Round(percent * 100)
	if feeBasisPoints < 0 || feeBasisPoints > FeeDenom {
		return 0, 0, fmt.Errorf(
			"fee percent %q + %q outside supported range",
			p.LPFeePercent,
			p.ProtocolFeePercent,
		)
	}
	return FeeDenom - uint64(feeBasisPoints), FeeDenom, nil
}

// PoolConnection is the connection wrapper returned by SaturnSwap list queries.
type PoolConnection struct {
	Nodes      []Pool     `json:"nodes,omitempty"`
	Edges      []PoolEdge `json:"edges,omitempty"`
	TotalCount uint64     `json:"totalCount,omitempty"`
}

func (p PoolConnection) Pools() []Pool {
	if len(p.Nodes) > 0 {
		return p.Nodes
	}
	ret := make([]Pool, 0, len(p.Edges))
	for _, edge := range p.Edges {
		ret = append(ret, edge.Node)
	}
	return ret
}

type PoolEdge struct {
	Cursor string `json:"cursor,omitempty"`
	Node   Pool   `json:"node"`
}

// PoolByTokensInput is the documented poolByTokens lookup input.
type PoolByTokensInput struct {
	PolicyIDOne  string `json:"policyIdOne"`
	AssetNameOne string `json:"assetNameOne"`
	PolicyIDTwo  string `json:"policyIdTwo"`
	AssetNameTwo string `json:"assetNameTwo"`
}

// PoolUtxoType names SaturnSwap order component and order-book UTxO types.
type PoolUtxoType string

const (
	PoolUtxoTypeLimitBuyOrder   PoolUtxoType = "LIMIT_BUY_ORDER"
	PoolUtxoTypeLimitSellOrder  PoolUtxoType = "LIMIT_SELL_ORDER"
	PoolUtxoTypeMarketBuyOrder  PoolUtxoType = "MARKET_BUY_ORDER"
	PoolUtxoTypeMarketSellOrder PoolUtxoType = "MARKET_SELL_ORDER"
	PoolUtxoTypeCancel          PoolUtxoType = "CANCEL"
)

// OrderBookPoolUtxo is intentionally permissive: the docs publish the
// order-book field names but not the full nested UTxO schema.
type OrderBookPoolUtxo struct {
	ID              string          `json:"id,omitempty"`
	PoolID          string          `json:"pool_id,omitempty"`
	PoolUtxoID      string          `json:"pool_utxo_id,omitempty"`
	TxHash          string          `json:"tx_hash,omitempty"`
	TxIndex         uint32          `json:"tx_index,omitempty"`
	Type            PoolUtxoType    `json:"pool_utxo_type,omitempty"`
	Price           DecimalString   `json:"price,omitempty"`
	TokenAmountBuy  DecimalString   `json:"token_amount_buy,omitempty"`
	TokenAmountSell DecimalString   `json:"token_amount_sell,omitempty"`
	Raw             json.RawMessage `json:"-"`
}

func (u *OrderBookPoolUtxo) UnmarshalJSON(data []byte) error {
	type orderBookPoolUtxo OrderBookPoolUtxo

	var decoded orderBookPoolUtxo
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*u = OrderBookPoolUtxo(decoded)
	u.Raw = append(json.RawMessage(nil), data...)
	return nil
}

// OrderTransaction is the CBOR transaction handle returned by create and
// consumed by submit.
type OrderTransaction struct {
	TransactionID  string `json:"transactionId"`
	HexTransaction string `json:"hexTransaction"`
}

type CreateOrderTransactionInput struct {
	PaymentAddress        string                 `json:"paymentAddress"`
	MarketOrderComponents []MarketOrderComponent `json:"marketOrderComponents,omitempty"`
	LimitOrderComponents  []LimitOrderComponent  `json:"limitOrderComponents,omitempty"`
	CancelComponents      []CancelComponent      `json:"cancelComponents,omitempty"`
}

type MarketOrderComponent struct {
	PoolID          string       `json:"poolId"`
	TokenAmountSell float64      `json:"tokenAmountSell"`
	TokenAmountBuy  float64      `json:"tokenAmountBuy"`
	MarketOrderType PoolUtxoType `json:"marketOrderType"`
	Slippage        float64      `json:"slippage,omitempty"`
	Version         int          `json:"version,omitempty"`
}

type LimitOrderComponent struct {
	PoolID          string       `json:"poolId"`
	TokenAmountSell float64      `json:"tokenAmountSell"`
	TokenAmountBuy  float64      `json:"tokenAmountBuy"`
	LimitOrderType  PoolUtxoType `json:"limitOrderType"`
	Version         int          `json:"version,omitempty"`
}

type CancelComponent struct {
	PoolUtxoID string `json:"poolUtxoId"`
}

type SaturnAPIError struct {
	Message string `json:"message"`
	Code    string `json:"code,omitempty"`
	Link    string `json:"link,omitempty"`
}

func (e SaturnAPIError) Error() string {
	if e.Code == "" {
		return e.Message
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

type FailedTransaction struct {
	Error *SaturnAPIError `json:"error,omitempty"`
}

type CreateOrderTransactionResult struct {
	SuccessTransactions []OrderTransaction  `json:"successTransactions"`
	FailTransactions    []FailedTransaction `json:"failTransactions"`
	Error               *SaturnAPIError     `json:"error,omitempty"`
}

type SubmitOrderTransactionInput struct {
	PaymentAddress      string             `json:"paymentAddress"`
	SuccessTransactions []OrderTransaction `json:"successTransactions"`
}

type SubmitOrderTransactionResult struct {
	TransactionIDs []string        `json:"transactionIds"`
	Error          *SaturnAPIError `json:"error,omitempty"`
}
