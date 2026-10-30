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

// Package geniusyield implements the Genius Yield order-book DEX batcher
// with Smart Order Routing (SOR) capabilities.
package geniusyield

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/blinklabs-io/adder/event"
	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	dexgy "github.com/blinklabs-io/shai/dex/geniusyield"
	"github.com/blinklabs-io/shai/internal/config"
	"github.com/blinklabs-io/shai/internal/indexer"
	"github.com/blinklabs-io/shai/internal/logging"
	"github.com/blinklabs-io/shai/internal/node"
	"github.com/blinklabs-io/shai/internal/storage"
	"github.com/blinklabs-io/shai/internal/txsubmit"
)

// GeniusYield implements the Genius Yield order-book DEX batcher with SOR
type GeniusYield struct {
	idx                 *indexer.Indexer
	node                *node.Node
	config              config.GeniusYieldProfileConfig
	name                string
	orderAddresses      []string
	sor                 *SmartOrderRouter
	enabled             bool
	mu                  sync.Mutex
	chainOrders         map[string]*trackedOrder
	pendingTransactions map[string]*pendingTransaction
	reservedBy          map[string]map[string]struct{}
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
		idx:                 idx,
		node:                node,
		config:              cfg,
		name:                name,
		orderAddresses:      orderAddresses,
		sor:                 NewSmartOrderRouter(),
		enabled:             true,
		chainOrders:         make(map[string]*trackedOrder),
		pendingTransactions: make(map[string]*pendingTransaction),
		reservedBy:          make(map[string]map[string]struct{}),
	}

	// Configure SOR with multi-hop if enabled
	if cfg.EnableMultiHop {
		gy.sor.EnableMultiHop(true)
	}

	gy.loadPersistedOrders()

	// Register event handlers
	idx.AddEventFunc(gy.handleChainsyncEvent)
	node.AddMempoolNewTransactionFunc(gy.handleMempoolNewTransaction)
	node.AddMempoolRemovedTransactionFunc(
		gy.handleMempoolRemovedTransaction,
	)

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

			var orderConfig dexgy.OrderConfig
			if _, err := cbor.Decode(datum.Cbor(), &orderConfig); err != nil {
				continue
			}

			if len(orderConfig.NFT) == 0 {
				continue
			}
			if !gy.hasOrderNFT(utxo.Output, orderConfig.NFT) {
				continue
			}
			if !hasOfferedAmount(utxo.Output, orderConfig) {
				continue
			}

			orderState := dexgy.OrderConfigToState(
				&orderConfig,
				hex.EncodeToString(utxo.Ref.Id().Bytes()),
				utxo.Ref.Index(),
				0, // Slot unknown for persisted
			)

			tracked := &trackedOrder{
				state:      orderState,
				address:    addr,
				outputCbor: append([]byte(nil), utxo.Output.Cbor()...),
			}
			ref := orderRef(orderState.TxHash, orderState.TxIndex)
			gy.mu.Lock()
			gy.chainOrders[ref] = tracked
			gy.syncOrderLocked(orderState.OrderId)
			gy.mu.Unlock()
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
		orders := make([]*trackedOrder, 0)
		matchOutputs := make([]ledger.TransactionOutput, 0)
		for idx, txOutput := range eventTx.Outputs {
			order, err := gy.parseOrderOutput(
				eventCtx.TransactionHash,
				idx,
				txOutput,
				eventCtx.SlotNumber,
			)
			if err != nil {
				logger.Debug(
					"failed to handle chainsync output",
					"txHash", eventCtx.TransactionHash,
					"index", idx,
					"error", err,
				)
				continue
			}
			if order == nil {
				continue
			}
			if err := storage.GetStorage().AddUtxo(
				order.address,
				order.state.TxHash,
				order.state.TxIndex,
				order.outputCbor,
			); err != nil {
				logger.Warn(
					"failed to store order UTXO",
					"txHash", eventCtx.TransactionHash,
					"error", err,
				)
			}
			orders = append(orders, order)
			matchOutputs = append(matchOutputs, txOutput)
		}
		gy.applyChainTransaction(
			eventCtx.TransactionHash,
			eventTx.Inputs,
			orders,
		)
		if gy.enabled {
			for i, order := range orders {
				if !gy.confirmedOrderEligible(order, time.Now()) {
					continue
				}
				if err := gy.tryMatchOrder(
					order.state,
					matchOutputs[i],
				); err != nil {
					logger.Debug(
						"failed to match confirmed order",
						"orderId", order.state.OrderId,
						"error", err,
					)
				}
			}
		}
	}

	return nil
}

func (gy *GeniusYield) confirmedOrderEligible(
	order *trackedOrder,
	now time.Time,
) bool {
	if order == nil || order.state == nil {
		return false
	}
	ref := orderRef(order.state.TxHash, order.state.TxIndex)
	gy.mu.Lock()
	defer gy.mu.Unlock()
	return gy.chainOrders[ref] == order &&
		len(gy.reservedBy[ref]) == 0 &&
		orderEligible(order.state, now)
}

// handleMempoolNewTransaction processes mempool transactions
func (gy *GeniusYield) handleMempoolNewTransaction(
	mempoolTx node.TxsubmissionMempoolTransaction,
) error {
	if mempoolTx.Source != node.MempoolTransactionSourceOwned {
		return nil
	}

	tx, err := ledger.NewTransactionFromCbor(mempoolTx.Type, mempoolTx.Cbor)
	if err != nil {
		return err
	}

	txHash := tx.Hash().String()
	gy.reservePendingTransaction(txHash, tx.Inputs())
	return nil
}

func (gy *GeniusYield) parseOrderOutput(
	txHash string,
	txOutputIdx int,
	txOutput ledger.TransactionOutput,
	slot uint64,
) (*trackedOrder, error) {
	outputAddress := txOutput.Address().String()
	var paymentAddr string
	if txOutput.Address().PaymentAddress() != nil {
		paymentAddr = txOutput.Address().PaymentAddress().String()
	}

	isOrder := false
	matchedAddress := ""
	for _, addr := range gy.orderAddresses {
		if outputAddress == addr || paymentAddr == addr {
			isOrder = true
			matchedAddress = addr
			break
		}
	}

	if !isOrder {
		return nil, nil
	}

	datum := txOutput.Datum()
	if datum == nil {
		return nil, nil
	}

	var orderConfig dexgy.OrderConfig
	if _, err := cbor.Decode(datum.Cbor(), &orderConfig); err != nil {
		return nil, fmt.Errorf("failed to decode order datum: %w", err)
	}

	if len(orderConfig.NFT) == 0 {
		return nil, nil
	}
	if !gy.hasOrderNFT(txOutput, orderConfig.NFT) {
		return nil, nil
	}
	if !hasOfferedAmount(txOutput, orderConfig) {
		return nil, nil
	}

	orderState := dexgy.OrderConfigToState(
		&orderConfig,
		txHash,
		uint32(txOutputIdx),
		slot,
	)
	logging.GetLogger().Debug(
		"processed order",
		"orderId", orderState.OrderId,
		"offered", orderState.OfferedAsset.Fingerprint(),
		"amount", orderState.OfferedAmount,
		"asked", orderState.AskedAsset.Fingerprint(),
		"price", orderState.Price,
	)
	return &trackedOrder{
		state:      orderState,
		address:    matchedAddress,
		outputCbor: append([]byte(nil), txOutput.Cbor()...),
	}, nil
}

// tryMatchOrder attempts to match a new order against existing orders
func (gy *GeniusYield) tryMatchOrder(
	order *dexgy.OrderState,
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
		order.OfferedAsset,
		order.AskedAsset,
		order.OfferedAmount, // How much they're offering
		maxSlippage,
	)

	if err != nil {
		return err
	}

	if route == nil {
		return fmt.Errorf("no matching route found")
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

func (gy *GeniusYield) hasOrderNFT(
	txOutput ledger.TransactionOutput,
	nftName []byte,
) bool {
	policy, err := hex.DecodeString(gy.config.OrderNFTPolicy)
	if err != nil || len(policy) != 28 || txOutput.Assets() == nil {
		return false
	}
	amount := txOutput.Assets().Asset(ledger.NewBlake2b224(policy), nftName)
	return amount != nil && amount.Sign() > 0
}

func hasOfferedAmount(
	txOutput ledger.TransactionOutput,
	order dexgy.OrderConfig,
) bool {
	want := new(big.Int).SetUint64(order.OfferedAmount)
	if want.Sign() == 0 {
		return false
	}
	if order.OfferedAsset.IsLovelace() {
		amount := txOutput.Amount()
		return amount != nil && amount.Cmp(want) >= 0
	}
	if len(order.OfferedAsset.PolicyId) != 28 ||
		len(order.OfferedAsset.AssetName) > 32 ||
		txOutput.Assets() == nil {
		return false
	}
	amount := txOutput.Assets().Asset(
		ledger.NewBlake2b224(order.OfferedAsset.PolicyId),
		order.OfferedAsset.AssetName,
	)
	return amount != nil && amount.Cmp(want) >= 0
}

// executeRoute builds and submits a transaction to execute a matched route
func (gy *GeniusYield) executeRoute(
	route *Route,
	newOrder *dexgy.OrderState,
	newOrderOutput ledger.TransactionOutput,
) error {
	logger := logging.GetLogger()
	if !routeEligible(route, time.Now()) {
		return fmt.Errorf("route contains an ineligible order")
	}

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
	if !txsubmit.IsStarted() {
		return fmt.Errorf("txsubmit is not started")
	}
	return txsubmit.SubmitTx(txBytes)
}

// cleanupLoop periodically removes expired orders
func (gy *GeniusYield) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now()
		removed := gy.sor.ClearExpired(now)
		gy.mu.Lock()
		orderIDs := make(map[string]struct{})
		for _, order := range gy.chainOrders {
			orderIDs[order.state.OrderId] = struct{}{}
		}
		gy.syncAffectedOrdersLocked(orderIDs)
		gy.mu.Unlock()
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
