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
	"bytes"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/blinklabs-io/shai/internal/common"
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
	NFT                  []byte `json:"nft"`                  // Order NFT token name
	MakerLovelaceFlatFee uint64 `json:"makerLovelaceFlatFee"` // Flat maker fee
	MakerFeeNum          int64  `json:"makerFeeNum"`          // Maker fee numerator
	MakerFeeDenom        int64  `json:"makerFeeDenom"`        // Maker fee denominator
	MakerFeeMax          uint64 `json:"makerFeeMax"`          // Max maker fee
	ContainedLovelaceFee uint64 `json:"containedLovelaceFee"` // Contained lovelace fee
	ContainedOfferedFee  uint64 `json:"containedOfferedFee"`  // Contained offered fee
	ContainedAskedFee    uint64 `json:"containedAskedFee"`    // Contained asked fee
	ContainedPayment     uint64 `json:"containedPayment"`     // Contained payment
}

// TradingPair represents a pair of assets that can be traded
type TradingPair struct {
	Base  common.AssetClass // The asset being bought/sold
	Quote common.AssetClass // The asset used for pricing
}

// String returns a string representation of the trading pair
func (tp TradingPair) String() string {
	return fmt.Sprintf("%s/%s", tp.Base.Fingerprint(), tp.Quote.Fingerprint())
}

// Reverse returns the inverse trading pair
func (tp TradingPair) Reverse() TradingPair {
	return TradingPair{Base: tp.Quote, Quote: tp.Base}
}

// OrderSide represents buy or sell
type OrderSide int

const (
	OrderSideBuy  OrderSide = iota // Buying base asset
	OrderSideSell                  // Selling base asset
)

// OrderBookEntry represents an order in the order book
type OrderBookEntry struct {
	Order      *OrderState
	TxHash     string
	TxIndex    uint32
	Side       OrderSide
	EffPrice   float64
	AvailQty   uint64
	LastUpdate time.Time
}

// OrderBook maintains a sorted list of orders for a trading pair
type OrderBook struct {
	Pair    TradingPair
	Bids    []*OrderBookEntry
	Asks    []*OrderBookEntry
	mu      sync.RWMutex
	updated time.Time
}

// NewOrderBook creates a new order book for a trading pair
func NewOrderBook(pair TradingPair) *OrderBook {
	return &OrderBook{
		Pair:    pair,
		Bids:    make([]*OrderBookEntry, 0),
		Asks:    make([]*OrderBookEntry, 0),
		updated: time.Now(),
	}
}

// AddOrder adds an order to the order book
func (ob *OrderBook) AddOrder(order *OrderState) {
	ob.mu.Lock()
	defer ob.mu.Unlock()

	entry := &OrderBookEntry{
		Order:      order,
		TxHash:     order.TxHash,
		TxIndex:    order.TxIndex,
		AvailQty:   order.OfferedAmount,
		LastUpdate: time.Now(),
	}

	if ob.assetEquals(order.OfferedAsset, ob.Pair.Quote) &&
		ob.assetEquals(order.AskedAsset, ob.Pair.Base) {
		entry.Side = OrderSideBuy
		if order.Price != 0 {
			entry.EffPrice = 1.0 / order.Price
		} // else EffPrice remains 0, order won't match
		ob.Bids = append(ob.Bids, entry)
		ob.sortBids()
	} else if ob.assetEquals(order.OfferedAsset, ob.Pair.Base) &&
		ob.assetEquals(order.AskedAsset, ob.Pair.Quote) {
		entry.Side = OrderSideSell
		if order.Price != 0 {
			entry.EffPrice = order.Price
		} // else EffPrice remains 0, order won't match
		ob.Asks = append(ob.Asks, entry)
		ob.sortAsks()
	}

	ob.updated = time.Now()
}

// RemoveOrder removes an order from the order book by order ID
func (ob *OrderBook) RemoveOrder(orderId string) {
	ob.mu.Lock()
	defer ob.mu.Unlock()

	for i, entry := range ob.Bids {
		if entry.Order.OrderId == orderId {
			ob.Bids = append(ob.Bids[:i], ob.Bids[i+1:]...)
			break
		}
	}

	for i, entry := range ob.Asks {
		if entry.Order.OrderId == orderId {
			ob.Asks = append(ob.Asks[:i], ob.Asks[i+1:]...)
			break
		}
	}

	ob.updated = time.Now()
}

// UpdateOrder updates an existing order in the order book
func (ob *OrderBook) UpdateOrder(order *OrderState) {
	ob.RemoveOrder(order.OrderId)
	if order.IsActive && order.OfferedAmount > 0 {
		ob.AddOrder(order)
	}
}

// GetBestBid returns the best bid
func (ob *OrderBook) GetBestBid() *OrderBookEntry {
	ob.mu.RLock()
	defer ob.mu.RUnlock()
	if len(ob.Bids) == 0 {
		return nil
	}
	return ob.Bids[0]
}

// GetBestAsk returns the best ask
func (ob *OrderBook) GetBestAsk() *OrderBookEntry {
	ob.mu.RLock()
	defer ob.mu.RUnlock()
	if len(ob.Asks) == 0 {
		return nil
	}
	return ob.Asks[0]
}

// GetSpread returns the bid-ask spread
func (ob *OrderBook) GetSpread() (spread float64, spreadPercent float64) {
	bestBid := ob.GetBestBid()
	bestAsk := ob.GetBestAsk()
	if bestBid == nil || bestAsk == nil {
		return 0, 0
	}
	spread = bestAsk.EffPrice - bestBid.EffPrice
	midPrice := (bestAsk.EffPrice + bestBid.EffPrice) / 2
	if midPrice > 0 {
		spreadPercent = (spread / midPrice) * 100
	}
	return spread, spreadPercent
}

// GetDepth returns depth at each price level
func (ob *OrderBook) GetDepth(levels int) (bids, asks []PriceLevel) {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	bidLevels := make(map[float64]uint64)
	for _, entry := range ob.Bids {
		bidLevels[entry.EffPrice] += entry.AvailQty
	}

	askLevels := make(map[float64]uint64)
	for _, entry := range ob.Asks {
		askLevels[entry.EffPrice] += entry.AvailQty
	}

	for price, qty := range bidLevels {
		bids = append(bids, PriceLevel{Price: price, Quantity: qty})
	}
	sort.Slice(bids, func(i, j int) bool {
		return bids[i].Price > bids[j].Price
	})

	for price, qty := range askLevels {
		asks = append(asks, PriceLevel{Price: price, Quantity: qty})
	}
	sort.Slice(asks, func(i, j int) bool {
		return asks[i].Price < asks[j].Price
	})

	if len(bids) > levels {
		bids = bids[:levels]
	}
	if len(asks) > levels {
		asks = asks[:levels]
	}

	return bids, asks
}

func (ob *OrderBook) sortBids() {
	sort.Slice(ob.Bids, func(i, j int) bool {
		if ob.Bids[i].EffPrice != ob.Bids[j].EffPrice {
			return ob.Bids[i].EffPrice > ob.Bids[j].EffPrice
		}
		return ob.Bids[i].Order.Timestamp.Before(ob.Bids[j].Order.Timestamp)
	})
}

func (ob *OrderBook) sortAsks() {
	sort.Slice(ob.Asks, func(i, j int) bool {
		if ob.Asks[i].EffPrice != ob.Asks[j].EffPrice {
			return ob.Asks[i].EffPrice < ob.Asks[j].EffPrice
		}
		return ob.Asks[i].Order.Timestamp.Before(ob.Asks[j].Order.Timestamp)
	})
}

func (ob *OrderBook) assetEquals(a, b common.AssetClass) bool {
	return bytes.Equal(a.PolicyId, b.PolicyId) &&
		bytes.Equal(a.Name, b.Name)
}

// PriceLevel represents aggregated quantity at a price level
type PriceLevel struct {
	Price    float64
	Quantity uint64
}

// RouteLeg represents a single order fill in a route
type RouteLeg struct {
	Order        *OrderState
	TxHash       string
	TxIndex      uint32
	InputAmount  uint64
	OutputAmount uint64
	Price        float64
}

// Route represents an execution plan across multiple orders
type Route struct {
	InputAsset   common.AssetClass
	OutputAsset  common.AssetClass
	Legs         []RouteLeg
	TotalInput   uint64
	TotalOutput  uint64
	AvgPrice     float64
	PriceImpact  float64
	EstimatedFee uint64
	IsMultiHop   bool              // True if this route goes through an intermediate asset
	Intermediate common.AssetClass // The intermediate asset for multi-hop routes
}

// SmartOrderRouter finds optimal execution paths
type SmartOrderRouter struct {
	orderBooks      map[string]*OrderBook
	multiHopEnabled bool
	mu              sync.RWMutex
}

// NewSmartOrderRouter creates a new SOR
func NewSmartOrderRouter() *SmartOrderRouter {
	return &SmartOrderRouter{
		orderBooks:      make(map[string]*OrderBook),
		multiHopEnabled: false,
	}
}

// EnableMultiHop enables or disables multi-hop routing
func (sor *SmartOrderRouter) EnableMultiHop(enabled bool) {
	sor.mu.Lock()
	defer sor.mu.Unlock()
	sor.multiHopEnabled = enabled
}

// IsMultiHopEnabled returns whether multi-hop routing is enabled
func (sor *SmartOrderRouter) IsMultiHopEnabled() bool {
	sor.mu.RLock()
	defer sor.mu.RUnlock()
	return sor.multiHopEnabled
}

// GetOrCreateOrderBook gets or creates an order book
func (sor *SmartOrderRouter) GetOrCreateOrderBook(pair TradingPair) *OrderBook {
	sor.mu.Lock()
	defer sor.mu.Unlock()

	key := pair.String()
	if ob, exists := sor.orderBooks[key]; exists {
		return ob
	}

	ob := NewOrderBook(pair)
	sor.orderBooks[key] = ob
	return ob
}

// GetOrderBook returns an order book
func (sor *SmartOrderRouter) GetOrderBook(pair TradingPair) *OrderBook {
	sor.mu.RLock()
	defer sor.mu.RUnlock()
	return sor.orderBooks[pair.String()]
}

// AddOrder adds an order
func (sor *SmartOrderRouter) AddOrder(order *OrderState) {
	pair := TradingPair{
		Base:  order.OfferedAsset,
		Quote: order.AskedAsset,
	}
	normalizedPair := sor.normalizePair(pair)
	ob := sor.GetOrCreateOrderBook(normalizedPair)
	ob.AddOrder(order)
}

// RemoveOrder removes an order
func (sor *SmartOrderRouter) RemoveOrder(orderId string) {
	sor.mu.RLock()
	defer sor.mu.RUnlock()
	for _, ob := range sor.orderBooks {
		ob.RemoveOrder(orderId)
	}
}

// UpdateOrder updates an order
func (sor *SmartOrderRouter) UpdateOrder(order *OrderState) {
	sor.RemoveOrder(order.OrderId)
	if order.IsActive && order.OfferedAmount > 0 {
		sor.AddOrder(order)
	}
}

// FindRoute finds the optimal execution route
func (sor *SmartOrderRouter) FindRoute(
	inputAsset common.AssetClass,
	outputAsset common.AssetClass,
	inputAmount uint64,
	maxSlippageBps uint64,
) (*Route, error) {
	// Try direct route first
	directRoute, directErr := sor.findDirectRoute(
		inputAsset,
		outputAsset,
		inputAmount,
		maxSlippageBps,
	)

	// If multi-hop is disabled or direct route works well, return it
	if !sor.IsMultiHopEnabled() {
		return directRoute, directErr
	}

	// If direct route found and fills most of the order, use it
	if directErr == nil && directRoute.TotalInput >= inputAmount*80/100 {
		return directRoute, nil
	}

	// Try multi-hop routes through common intermediaries
	multiHopRoute, multiHopErr := sor.findMultiHopRoute(
		inputAsset,
		outputAsset,
		inputAmount,
		maxSlippageBps,
	)

	// Choose the better route
	if directErr != nil && multiHopErr != nil {
		return nil, fmt.Errorf(
			"no route found: direct=%v, multihop=%v",
			directErr,
			multiHopErr,
		)
	}

	if directErr != nil {
		return multiHopRoute, nil
	}

	if multiHopErr != nil {
		return directRoute, nil
	}

	// Both routes found - compare output amounts
	if multiHopRoute.TotalOutput > directRoute.TotalOutput {
		return multiHopRoute, nil
	}

	return directRoute, nil
}

// findDirectRoute finds a direct route between two assets
func (sor *SmartOrderRouter) findDirectRoute(
	inputAsset common.AssetClass,
	outputAsset common.AssetClass,
	inputAmount uint64,
	maxSlippageBps uint64,
) (*Route, error) {
	pair := TradingPair{Base: outputAsset, Quote: inputAsset}
	normalizedPair := sor.normalizePair(pair)

	ob := sor.GetOrderBook(normalizedPair)
	if ob == nil {
		return nil, fmt.Errorf("no order book for pair %s", pair.String())
	}

	isBuyingBase := sor.assetEquals(outputAsset, normalizedPair.Base)

	var entries []*OrderBookEntry
	if isBuyingBase {
		entries = ob.Asks
	} else {
		entries = ob.Bids
	}

	if len(entries) == 0 {
		return nil, fmt.Errorf("no orders available")
	}

	route := &Route{
		InputAsset:  inputAsset,
		OutputAsset: outputAsset,
		Legs:        make([]RouteLeg, 0),
	}

	remainingInput := inputAmount
	bestPrice := entries[0].EffPrice

	for _, entry := range entries {
		if remainingInput == 0 {
			break
		}

		priceDeviation := (entry.EffPrice - bestPrice) / bestPrice
		if isBuyingBase {
			if priceDeviation*10000 > float64(maxSlippageBps) {
				break
			}
		} else {
			priceDeviation = (bestPrice - entry.EffPrice) / bestPrice
			if priceDeviation*10000 > float64(maxSlippageBps) {
				break
			}
		}

		var legInput, legOutput uint64

		if isBuyingBase {
			maxOutput := entry.AvailQty
			maxInput := uint64(float64(maxOutput) * entry.EffPrice)
			if maxInput >= remainingInput {
				legInput = remainingInput
				legOutput = uint64(float64(legInput) / entry.EffPrice)
			} else {
				legInput = maxInput
				legOutput = maxOutput
			}
		} else {
			maxInput := entry.AvailQty
			if maxInput >= remainingInput {
				legInput = remainingInput
			} else {
				legInput = maxInput
			}
			legOutput = uint64(float64(legInput) * entry.EffPrice)
		}

		if legOutput == 0 {
			continue
		}

		leg := RouteLeg{
			Order:        entry.Order,
			TxHash:       entry.TxHash,
			TxIndex:      entry.TxIndex,
			InputAmount:  legInput,
			OutputAmount: legOutput,
			Price:        entry.EffPrice,
		}
		route.Legs = append(route.Legs, leg)

		route.TotalInput += legInput
		route.TotalOutput += legOutput
		remainingInput -= legInput
	}

	if len(route.Legs) == 0 {
		return nil, fmt.Errorf("no orders matched")
	}

	if route.TotalInput > 0 {
		route.AvgPrice = float64(route.TotalOutput) / float64(route.TotalInput)
	}

	if isBuyingBase {
		effectivePrice := float64(route.TotalInput) / float64(route.TotalOutput)
		route.PriceImpact = (effectivePrice - bestPrice) / bestPrice * 100
	} else {
		route.PriceImpact = (bestPrice - route.AvgPrice) / bestPrice * 100
	}

	route.EstimatedFee = 200000 + uint64(len(route.Legs))*50000

	return route, nil
}

// findMultiHopRoute finds a route through intermediate assets
func (sor *SmartOrderRouter) findMultiHopRoute(
	inputAsset common.AssetClass,
	outputAsset common.AssetClass,
	inputAmount uint64,
	maxSlippageBps uint64,
) (*Route, error) {
	// Get all possible intermediate assets (assets that have order books with both input and output)
	intermediates := sor.findIntermediateAssets(inputAsset, outputAsset)

	if len(intermediates) == 0 {
		return nil, fmt.Errorf("no intermediate assets for multi-hop")
	}

	var bestRoute *Route
	var bestOutput uint64

	for _, intermediate := range intermediates {
		// Find route: input -> intermediate
		route1, err1 := sor.findDirectRoute(
			inputAsset,
			intermediate,
			inputAmount,
			maxSlippageBps/2, // Split slippage allowance
		)
		if err1 != nil || route1.TotalOutput == 0 {
			continue
		}

		// Find route: intermediate -> output
		route2, err2 := sor.findDirectRoute(
			intermediate,
			outputAsset,
			route1.TotalOutput, // Use output from first hop as input
			maxSlippageBps/2,
		)
		if err2 != nil || route2.TotalOutput == 0 {
			continue
		}

		// Combine routes
		combinedRoute := &Route{
			InputAsset:   inputAsset,
			OutputAsset:  outputAsset,
			Legs:         append(route1.Legs, route2.Legs...),
			TotalInput:   route1.TotalInput,
			TotalOutput:  route2.TotalOutput,
			IsMultiHop:   true,
			Intermediate: intermediate,
		}

		// Calculate combined metrics
		if combinedRoute.TotalInput > 0 {
			combinedRoute.AvgPrice = float64(
				combinedRoute.TotalOutput,
			) / float64(
				combinedRoute.TotalInput,
			)
		}
		combinedRoute.PriceImpact = route1.PriceImpact + route2.PriceImpact
		combinedRoute.EstimatedFee = route1.EstimatedFee + route2.EstimatedFee

		// Track best route
		if combinedRoute.TotalOutput > bestOutput {
			bestOutput = combinedRoute.TotalOutput
			bestRoute = combinedRoute
		}
	}

	if bestRoute == nil {
		return nil, fmt.Errorf("no multi-hop route found")
	}

	return bestRoute, nil
}

// findIntermediateAssets finds assets that could serve as intermediaries
func (sor *SmartOrderRouter) findIntermediateAssets(
	inputAsset common.AssetClass,
	outputAsset common.AssetClass,
) []common.AssetClass {
	sor.mu.RLock()
	defer sor.mu.RUnlock()

	// Collect all unique assets from order books
	assetSet := make(map[string]common.AssetClass)

	for _, ob := range sor.orderBooks {
		// Add base and quote assets
		baseKey := ob.Pair.Base.Fingerprint()
		quoteKey := ob.Pair.Quote.Fingerprint()

		assetSet[baseKey] = ob.Pair.Base
		assetSet[quoteKey] = ob.Pair.Quote
	}

	// Filter to find valid intermediates
	inputKey := inputAsset.Fingerprint()
	outputKey := outputAsset.Fingerprint()

	var intermediates []common.AssetClass

	for key, asset := range assetSet {
		// Skip input and output assets
		if key == inputKey || key == outputKey {
			continue
		}

		// Check if there's liquidity for both hops
		// Hop 1: input -> intermediate
		pair1 := sor.normalizePair(TradingPair{Base: asset, Quote: inputAsset})
		ob1 := sor.orderBooks[pair1.String()]

		// Hop 2: intermediate -> output
		pair2 := sor.normalizePair(TradingPair{Base: outputAsset, Quote: asset})
		ob2 := sor.orderBooks[pair2.String()]

		if ob1 != nil && ob2 != nil {
			// Both order books exist - valid intermediate
			intermediates = append(intermediates, asset)
		}
	}

	// Prioritize ADA (lovelace) as intermediate if available
	sortedIntermediates := make([]common.AssetClass, 0, len(intermediates))
	for _, asset := range intermediates {
		if asset.IsLovelace() {
			// Put ADA first
			sortedIntermediates = append(
				[]common.AssetClass{asset},
				sortedIntermediates...)
		} else {
			sortedIntermediates = append(sortedIntermediates, asset)
		}
	}

	return sortedIntermediates
}

// GetQuote returns a quick quote
func (sor *SmartOrderRouter) GetQuote(
	inputAsset common.AssetClass,
	outputAsset common.AssetClass,
	inputAmount uint64,
) (outputAmount uint64, avgPrice float64, err error) {
	route, err := sor.FindRoute(inputAsset, outputAsset, inputAmount, 10000)
	if err != nil {
		return 0, 0, err
	}
	return route.TotalOutput, route.AvgPrice, nil
}

// GetBestPrice returns the best available price
func (sor *SmartOrderRouter) GetBestPrice(
	inputAsset common.AssetClass,
	outputAsset common.AssetClass,
) (price float64, available uint64, err error) {
	pair := TradingPair{Base: outputAsset, Quote: inputAsset}
	normalizedPair := sor.normalizePair(pair)

	ob := sor.GetOrderBook(normalizedPair)
	if ob == nil {
		return 0, 0, fmt.Errorf("no order book for pair")
	}

	isBuyingBase := sor.assetEquals(outputAsset, normalizedPair.Base)

	if isBuyingBase {
		bestAsk := ob.GetBestAsk()
		if bestAsk == nil {
			return 0, 0, fmt.Errorf("no asks available")
		}
		return bestAsk.EffPrice, bestAsk.AvailQty, nil
	}

	bestBid := ob.GetBestBid()
	if bestBid == nil {
		return 0, 0, fmt.Errorf("no bids available")
	}
	return bestBid.EffPrice, bestBid.AvailQty, nil
}

// GetAllPairs returns all trading pairs
func (sor *SmartOrderRouter) GetAllPairs() []TradingPair {
	sor.mu.RLock()
	defer sor.mu.RUnlock()

	pairs := make([]TradingPair, 0, len(sor.orderBooks))
	for _, ob := range sor.orderBooks {
		pairs = append(pairs, ob.Pair)
	}
	return pairs
}

// GetOrderCount returns total active orders
func (sor *SmartOrderRouter) GetOrderCount() int {
	sor.mu.RLock()
	defer sor.mu.RUnlock()

	count := 0
	for _, ob := range sor.orderBooks {
		ob.mu.RLock()
		count += len(ob.Bids) + len(ob.Asks)
		ob.mu.RUnlock()
	}
	return count
}

// ClearExpired removes expired orders
func (sor *SmartOrderRouter) ClearExpired(now time.Time) int {
	sor.mu.RLock()
	defer sor.mu.RUnlock()

	removed := 0
	for _, ob := range sor.orderBooks {
		ob.mu.Lock()
		newBids := make([]*OrderBookEntry, 0, len(ob.Bids))
		for _, entry := range ob.Bids {
			if entry.Order.EndTime == nil || entry.Order.EndTime.After(now) {
				newBids = append(newBids, entry)
			} else {
				removed++
			}
		}
		ob.Bids = newBids

		newAsks := make([]*OrderBookEntry, 0, len(ob.Asks))
		for _, entry := range ob.Asks {
			if entry.Order.EndTime == nil || entry.Order.EndTime.After(now) {
				newAsks = append(newAsks, entry)
			} else {
				removed++
			}
		}
		ob.Asks = newAsks
		ob.mu.Unlock()
	}

	return removed
}

func (sor *SmartOrderRouter) normalizePair(pair TradingPair) TradingPair {
	if pair.Quote.IsLovelace() {
		return pair
	}
	if pair.Base.IsLovelace() {
		return pair.Reverse()
	}
	if pair.Base.Fingerprint() < pair.Quote.Fingerprint() {
		return pair
	}
	return pair.Reverse()
}

func (sor *SmartOrderRouter) assetEquals(a, b common.AssetClass) bool {
	return bytes.Equal(a.PolicyId, b.PolicyId) &&
		bytes.Equal(a.Name, b.Name)
}

// SORStats contains SOR statistics
type SORStats struct {
	TotalPairs       int
	TotalOrders      int
	TotalBids        int
	TotalAsks        int
	TotalBidVolume   uint64
	TotalAskVolume   uint64
	LastUpdate       time.Time
	MostActivePair   string
	MostActiveOrders int
}

// GetStats returns current statistics
func (sor *SmartOrderRouter) GetStats() SORStats {
	sor.mu.RLock()
	defer sor.mu.RUnlock()

	stats := SORStats{
		TotalPairs: len(sor.orderBooks),
	}

	for pairStr, ob := range sor.orderBooks {
		ob.mu.RLock()
		bidCount := len(ob.Bids)
		askCount := len(ob.Asks)
		pairOrders := bidCount + askCount

		stats.TotalBids += bidCount
		stats.TotalAsks += askCount
		stats.TotalOrders += pairOrders

		for _, entry := range ob.Bids {
			stats.TotalBidVolume += entry.AvailQty
		}
		for _, entry := range ob.Asks {
			stats.TotalAskVolume += entry.AvailQty
		}

		if stats.LastUpdate.IsZero() || ob.updated.After(stats.LastUpdate) {
			stats.LastUpdate = ob.updated
		}

		if pairOrders > stats.MostActiveOrders {
			stats.MostActiveOrders = pairOrders
			stats.MostActivePair = pairStr
		}
		ob.mu.RUnlock()
	}

	return stats
}
