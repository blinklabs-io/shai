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

	"github.com/blinklabs-io/adder/event"
	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/shai/internal/config"
	"github.com/blinklabs-io/shai/internal/indexer"
	"github.com/blinklabs-io/shai/internal/logging"
	"github.com/blinklabs-io/shai/internal/node"
	"github.com/blinklabs-io/shai/internal/storage"
	"github.com/blinklabs-io/shai/internal/txsubmit"
)

// GeniusYield implements the Genius Yield order-book DEX batcher with SOR
type GeniusYield struct {
	idx            *indexer.Indexer
	node           *node.Node
	config         config.GeniusYieldProfileConfig
	name           string
	orderAddresses []string
	sor            *SmartOrderRouter
	enabled        bool
}

// New creates a new Genius Yield batcher with SOR
func New(
	idx *indexer.Indexer,
	node *node.Node,
	name string,
	cfg config.GeniusYieldProfileConfig,
) *GeniusYield {
	// Extract order addresses from config
	orderAddresses := make([]string, 0, len(cfg.OrderAddresses))
	for _, addr := range cfg.OrderAddresses {
		orderAddresses = append(orderAddresses, addr.Address)
	}

	gy := &GeniusYield{
		idx:            idx,
		node:           node,
		config:         cfg,
		name:           name,
		orderAddresses: orderAddresses,
		sor:            NewSmartOrderRouter(),
		enabled:        true,
	}

	// Configure SOR with multi-hop if enabled
	if cfg.EnableMultiHop {
		gy.sor.EnableMultiHop(true)
	}

	// Register event handlers
	idx.AddEventFunc(gy.handleChainsyncEvent)
	node.AddMempoolNewTransactionFunc(gy.handleMempoolNewTransaction)

	// Load persisted order book from storage
	go gy.loadPersistedOrders()

	// Start background tasks
	go gy.cleanupLoop()

	return gy
}

// loadPersistedOrders loads orders from storage on startup
func (gy *GeniusYield) loadPersistedOrders() {
	logger := logging.GetLogger()
	count := 0

	for _, addr := range gy.orderAddresses {
		utxosBytes, err := storage.GetStorage().GetUtxos(addr)
		if err != nil {
			logger.Debug(
				"failed to load persisted orders for address",
				"address", addr,
				"error", err,
			)
			continue
		}

		for _, utxoBytes := range utxosBytes {
			// Try to parse as order
			var utxo storage.Utxo
			if _, err := cbor.Decode(utxoBytes, &utxo); err != nil {
				continue
			}

			datum := utxo.Output.Datum()
			if datum == nil {
				continue
			}

			var orderConfig OrderConfig
			if _, err := cbor.Decode(datum.Cbor(), &orderConfig); err != nil {
				continue
			}

			if len(orderConfig.NFT) == 0 {
				continue
			}

			// Create order state
			orderState := gy.orderConfigToState(
				&orderConfig,
				hex.EncodeToString(utxo.Ref.Id().Bytes()),
				utxo.Ref.Index(),
				0, // Slot unknown for persisted
			)

			gy.sor.UpdateOrder(orderState)
			count++
		}
	}

	if count > 0 {
		logger.Info(
			"loaded persisted orders",
			"count", count,
		)
	}
}

// handleChainsyncEvent processes on-chain events
func (gy *GeniusYield) handleChainsyncEvent(evt event.Event) error {
	logger := logging.GetLogger()

	switch evt.Payload.(type) {
	case event.TransactionEvent:
		eventTx := evt.Payload.(event.TransactionEvent)
		eventCtx := evt.Context.(event.TransactionContext)

		// Process outputs (new/updated orders)
		for idx, txOutput := range eventTx.Outputs {
			if err := gy.handleTransactionOutput(
				eventCtx.TransactionHash,
				idx,
				txOutput,
				eventCtx.SlotNumber,
				eventCtx.BlockNumber,
				false,
			); err != nil {
				logger.Debug(
					"failed to handle chainsync output",
					"txHash", eventCtx.TransactionHash,
					"index", idx,
					"error", err,
				)
			}
		}

		// Process inputs (consumed orders)
		for _, txInput := range eventTx.Inputs {
			gy.handleConsumedOrder(txInput.Id().String(), txInput.Index())
		}
	}

	return nil
}

// handleMempoolNewTransaction processes mempool transactions
func (gy *GeniusYield) handleMempoolNewTransaction(
	mempoolTx node.TxsubmissionMempoolTransaction,
) error {
	logger := logging.GetLogger()

	tx, err := ledger.NewTransactionFromCbor(mempoolTx.Type, mempoolTx.Cbor)
	if err != nil {
		return err
	}

	txHash := tx.Hash().String()

	// Process outputs (new orders in mempool)
	for idx, txOutput := range tx.Outputs() {
		if err := gy.handleTransactionOutput(
			txHash,
			idx,
			txOutput,
			0, // Slot unknown for mempool
			0, // Block unknown for mempool
			true,
		); err != nil {
			logger.Debug(
				"failed to handle mempool output",
				"txHash", txHash,
				"index", idx,
				"error", err,
			)
		}
	}

	return nil
}

// handleTransactionOutput processes a transaction output
func (gy *GeniusYield) handleTransactionOutput(
	txHash string,
	txOutputIdx int,
	txOutput ledger.TransactionOutput,
	slot uint64,
	block uint64,
	fromMempool bool,
) error {
	logger := logging.GetLogger()

	// Check if this is an order address
	outputAddress := txOutput.Address().String()
	var paymentAddr string
	if txOutput.Address().PaymentAddress() != nil {
		paymentAddr = txOutput.Address().PaymentAddress().String()
	}

	// Check against configured addresses
	isOrder := false
	for _, addr := range gy.orderAddresses {
		if outputAddress == addr || paymentAddr == addr {
			isOrder = true
			break
		}
	}

	if !isOrder {
		return nil
	}

	// Get datum
	datum := txOutput.Datum()
	if datum == nil {
		return nil
	}

	// Parse order config
	var orderConfig OrderConfig
	if _, err := cbor.Decode(datum.Cbor(), &orderConfig); err != nil {
		return fmt.Errorf("failed to decode order datum: %w", err)
	}

	// Validate order has NFT (required for identification)
	if len(orderConfig.NFT) == 0 {
		return nil
	}

	// Create order state for SOR
	orderState := gy.orderConfigToState(
		&orderConfig,
		txHash,
		uint32(txOutputIdx),
		slot,
	)

	// Store order UTXO
	if !fromMempool {
		if err := storage.GetStorage().AddUtxo(
			outputAddress,
			txHash,
			uint32(txOutputIdx),
			txOutput.Cbor(),
		); err != nil {
			logger.Warn(
				"failed to store order UTXO",
				"txHash", txHash,
				"error", err,
			)
		}
	}

	// Update SOR
	gy.sor.UpdateOrder(orderState)

	logger.Debug(
		"processed order",
		"orderId", orderState.OrderId,
		"offered", orderState.OfferedAsset.Fingerprint(),
		"amount", orderState.OfferedAmount,
		"asked", orderState.AskedAsset.Fingerprint(),
		"price", orderState.Price,
		"fromMempool", fromMempool,
	)

	// If from mempool and SOR is enabled, try to match orders
	if fromMempool && gy.enabled {
		if err := gy.tryMatchOrder(orderState, txOutput); err != nil {
			logger.Debug(
				"failed to match order",
				"orderId", orderState.OrderId,
				"error", err,
			)
		}
	}

	return nil
}

// handleConsumedOrder removes a consumed order from SOR
func (gy *GeniusYield) handleConsumedOrder(txHash string, txIndex uint32) {
	// We need to look up the order ID from the UTXO
	// For now, we iterate through order books (could be optimized with index)
	for _, pair := range gy.sor.GetAllPairs() {
		ob := gy.sor.GetOrderBook(pair)
		if ob == nil {
			continue
		}

		// Check bids
		for _, entry := range ob.Bids {
			if entry.TxHash == txHash && entry.TxIndex == txIndex {
				gy.sor.RemoveOrder(entry.Order.OrderId)
				return
			}
		}

		// Check asks
		for _, entry := range ob.Asks {
			if entry.TxHash == txHash && entry.TxIndex == txIndex {
				gy.sor.RemoveOrder(entry.Order.OrderId)
				return
			}
		}
	}
}

// orderConfigToState converts an OrderConfig to OrderState
func (gy *GeniusYield) orderConfigToState(
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
	}
}

// tryMatchOrder attempts to match a new order against existing orders
func (gy *GeniusYield) tryMatchOrder(
	order *OrderState,
	txOutput ledger.TransactionOutput,
) error {
	logger := logging.GetLogger()

	// Use configured max slippage or default to 5%
	maxSlippage := gy.config.MaxSlippageBps
	if maxSlippage == 0 {
		maxSlippage = 500 // Default 5%
	}

	// Find orders that can be matched against this one
	// We look for orders on the opposite side of the book
	route, err := gy.sor.FindRoute(
		order.AskedAsset,    // We want the asset they're asking for
		order.OfferedAsset,  // They're offering what we want
		order.OfferedAmount, // How much they're offering
		maxSlippage,
	)

	if err != nil {
		return err
	}

	if len(route.Legs) == 0 {
		return fmt.Errorf("no matching orders found")
	}

	logger.Info(
		"found matching route",
		"orderId", order.OrderId,
		"legs", len(route.Legs),
		"totalInput", route.TotalInput,
		"totalOutput", route.TotalOutput,
		"avgPrice", route.AvgPrice,
		"priceImpact", route.PriceImpact,
	)

	// Build and submit matching transaction
	if err := gy.executeRoute(route, order, txOutput); err != nil {
		return fmt.Errorf("failed to execute route: %w", err)
	}

	return nil
}

// executeRoute builds and submits a transaction to execute a matched route
func (gy *GeniusYield) executeRoute(
	route *Route,
	newOrder *OrderState,
	newOrderOutput ledger.TransactionOutput,
) error {
	logger := logging.GetLogger()

	// Build matching transaction
	txBytes, err := gy.buildMatchTx(route, newOrder, newOrderOutput)
	if err != nil {
		return fmt.Errorf("failed to build match tx: %w", err)
	}

	logger.Info(
		"submitting match transaction",
		"orderId", newOrder.OrderId,
		"legs", len(route.Legs),
		"txSize", len(txBytes),
	)

	// Submit transaction
	txsubmit.SubmitTx(txBytes)

	return nil
}

// cleanupLoop periodically removes expired orders
func (gy *GeniusYield) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		removed := gy.sor.ClearExpired(time.Now())
		if removed > 0 {
			logger := logging.GetLogger()
			logger.Debug(
				"removed expired orders",
				"count", removed,
			)
		}
	}
}

// GetSOR returns the Smart Order Router
func (gy *GeniusYield) GetSOR() *SmartOrderRouter {
	return gy.sor
}

// SetEnabled enables or disables order matching
func (gy *GeniusYield) SetEnabled(enabled bool) {
	gy.enabled = enabled
}

// IsEnabled returns whether order matching is enabled
func (gy *GeniusYield) IsEnabled() bool {
	return gy.enabled
}

// GetStats returns current batcher statistics
func (gy *GeniusYield) GetStats() SORStats {
	return gy.sor.GetStats()
}

// wrapTxOutput wraps a transaction output in UTXO structure
func wrapTxOutput(
	txId string,
	txOutputIdx int,
	txOutBytes []byte,
) ([]byte, error) {
	txIdBytes, err := hex.DecodeString(txId)
	if err != nil {
		return nil, err
	}

	utxoTmp := []any{
		[]any{
			txIdBytes,
			uint32(txOutputIdx),
		},
		cbor.RawMessage(txOutBytes),
	}

	cborBytes, err := cbor.Encode(&utxoTmp)
	if err != nil {
		return nil, err
	}

	return cborBytes, nil
}
