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
	"sync"
	"time"

	"github.com/blinklabs-io/adder/event"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/shai/internal/config"
	"github.com/blinklabs-io/shai/internal/indexer"
	"github.com/blinklabs-io/shai/internal/logging"
	"github.com/blinklabs-io/shai/internal/node"
)

// Oracle tracks pool states and provides price data
type Oracle struct {
	idx           *indexer.Indexer
	node          *node.Node
	profile       *config.Profile
	parser        PoolParser
	pools         map[string]*PoolState
	poolsMu       sync.RWMutex
	subscribers   []chan *PriceUpdate
	subscribersMu sync.RWMutex
	poolAddresses []string
	storage       *OracleStorage
	mempoolMgr    *MempoolStateManager // Per-tx mempool effect tracking
	stopChan      chan struct{}
	stopped       bool
}

// New creates a new Oracle instance
func New(
	idx *indexer.Indexer,
	n *node.Node,
	profile *config.Profile,
	parser PoolParser,
) *Oracle {
	o := &Oracle{
		idx:        idx,
		node:       n,
		profile:    profile,
		parser:     parser,
		pools:      make(map[string]*PoolState),
		mempoolMgr: NewMempoolStateManager(),
		stopChan:   make(chan struct{}),
	}

	// Extract pool addresses from profile config
	if oracleConfig, ok := profile.Config.(config.OracleProfileConfig); ok {
		for _, addr := range oracleConfig.PoolAddresses {
			o.poolAddresses = append(o.poolAddresses, addr.Address)
		}
	}

	return o
}

// Start begins tracking pool states
func (o *Oracle) Start() error {
	logger := logging.GetLogger()

	// Initialize storage
	var err error
	o.storage, err = NewOracleStorage()
	if err != nil {
		return err
	}

	// Load persisted pool states
	if err := o.loadPersistedStates(); err != nil {
		logger.Warn("failed to load persisted pool states", "error", err)
	}

	// Register event handler with indexer
	o.idx.AddEventFunc(o.HandleChainsyncEvent)

	// Register mempool handler with node (if available)
	if o.node != nil {
		o.node.AddMempoolNewTransactionFunc(o.HandleMempoolTransaction)
	}

	logger.Info(
		"Oracle started",
		"profile", o.profile.Name,
		"protocol", o.parser.Protocol(),
		"addresses", len(o.poolAddresses),
		"mempoolEnabled", o.node != nil,
	)

	return nil
}

// Stop stops the oracle (idempotent - safe to call multiple times)
func (o *Oracle) Stop() {
	o.poolsMu.Lock()
	if o.stopped {
		o.poolsMu.Unlock()
		return
	}
	o.stopped = true
	o.poolsMu.Unlock()

	close(o.stopChan)

	// Close all subscriber channels
	o.subscribersMu.Lock()
	for _, ch := range o.subscribers {
		close(ch)
	}
	o.subscribers = nil
	o.subscribersMu.Unlock()
}

// HandleChainsyncEvent processes chain sync events
func (o *Oracle) HandleChainsyncEvent(evt event.Event) error {
	switch payload := evt.Payload.(type) {
	case event.TransactionEvent:
		return o.handleTransaction(evt, payload)
	case event.RollbackEvent:
		return o.handleRollback(payload)
	}
	return nil
}

// HandleMempoolTransaction processes mempool transactions for early pool updates
func (o *Oracle) HandleMempoolTransaction(
	mempoolTx node.TxsubmissionMempoolTransaction,
) error {
	logger := logging.GetLogger()

	// Parse the transaction
	tx, err := ledger.NewTransactionFromCbor(mempoolTx.Type, mempoolTx.Cbor)
	if err != nil {
		return err
	}

	txHash := tx.Hash().String()

	// Check outputs for pool UTxOs
	for _, txOutput := range tx.Outputs() {
		addr := txOutput.Address().String()
		if !o.isPoolAddress(addr) {
			continue
		}

		// Try to parse datum
		if txOutput.Datum() == nil {
			continue
		}

		// Parse the pool state using the protocol-specific parser
		timestamp := time.Now()
		state, err := o.parser.ParsePoolDatum(
			txOutput.Datum().Cbor(),
			txHash,
			0, // Index unknown in mempool context
			0, // Slot unknown for mempool
			timestamp,
		)
		if err != nil {
			// Not a valid pool datum for this protocol
			continue
		}

		// Mark as from mempool
		state.Network = o.profile.Name
		state.UpdatedAt = time.Now()
		state.FromMempool = true

		// Get previous price for update notification
		var prevPrice float64
		o.poolsMu.RLock()
		if prev, ok := o.pools[state.PoolId]; ok {
			prevPrice = prev.PriceXY()
		}
		o.poolsMu.RUnlock()

		// Track per-transaction effect in mempool manager
		o.mempoolMgr.AddPendingTx(
			state.PoolId,
			state.Protocol,
			txHash,
			state,
		)

		// Update pool state (aggregate view)
		o.poolsMu.Lock()
		o.pools[state.PoolId] = state
		o.poolsMu.Unlock()

		// Note: We don't persist mempool states to storage
		// They will be overwritten when confirmed on-chain

		// Notify subscribers
		update := NewPriceUpdate(state, prevPrice)
		o.notifySubscribers(update)

		logger.Debug(
			"mempool pool state updated",
			"poolId", state.PoolId,
			"protocol", state.Protocol,
			"priceXY", state.PriceXY(),
			"txHash", txHash,
		)
	}

	return nil
}

// handleTransaction processes a transaction event
func (o *Oracle) handleTransaction(
	evt event.Event,
	txEvt event.TransactionEvent,
) error {
	logger := logging.GetLogger()
	ctx := evt.Context.(event.TransactionContext)

	// Check for pool UTxOs at monitored addresses
	for _, utxo := range txEvt.Transaction.Produced() {
		addr := utxo.Output.Address().String()
		if !o.isPoolAddress(addr) {
			continue
		}

		// Try to parse datum
		if utxo.Output.Datum() == nil {
			continue
		}

		// Parse the pool state using the protocol-specific parser
		timestamp := time.Now() // TODO: Get actual block timestamp
		state, err := o.parser.ParsePoolDatum(
			utxo.Output.Datum().Cbor(),
			ctx.TransactionHash,
			utxo.Id.Index(),
			ctx.SlotNumber,
			timestamp,
		)
		if err != nil {
			// Not a valid pool datum for this protocol
			continue
		}

		// Set additional metadata
		state.Network = o.profile.Name
		state.UpdatedAt = time.Now()

		// Get previous price for update notification
		var prevPrice float64
		o.poolsMu.RLock()
		if prev, ok := o.pools[state.PoolId]; ok {
			prevPrice = prev.PriceXY()
		}
		o.poolsMu.RUnlock()

		// Update pool state
		o.poolsMu.Lock()
		o.pools[state.PoolId] = state
		o.poolsMu.Unlock()

		// Update mempool manager with confirmed state
		// (removes any pending tx that matches this confirmed tx)
		o.mempoolMgr.UpdateConfirmedState(state.PoolId, state)

		// Persist to storage
		if o.storage != nil {
			if err := o.storage.SavePoolState(state); err != nil {
				logger.Error(
					"failed to persist pool state",
					"error", err,
					"poolId", state.PoolId,
				)
			}
		}

		// Notify subscribers
		update := NewPriceUpdate(state, prevPrice)
		o.notifySubscribers(update)

		logger.Debug(
			"pool state updated",
			"poolId", state.PoolId,
			"protocol", state.Protocol,
			"priceXY", state.PriceXY(),
			"slot", state.Slot,
		)
	}

	return nil
}

// handleRollback processes a rollback event
func (o *Oracle) handleRollback(evt event.RollbackEvent) error {
	logger := logging.GetLogger()
	logger.Warn(
		"rollback detected in oracle",
		"slot", evt.SlotNumber,
		"blockHash", evt.BlockHash,
	)
	// For simplicity, we don't remove pool states on rollback
	// The next update will overwrite with correct data
	return nil
}

// isPoolAddress checks if an address is a monitored pool address
func (o *Oracle) isPoolAddress(addr string) bool {
	for _, poolAddr := range o.poolAddresses {
		if addr == poolAddr {
			return true
		}
	}
	return false
}

// loadPersistedStates loads pool states from storage
func (o *Oracle) loadPersistedStates() error {
	if o.storage == nil {
		return nil
	}

	states, err := o.storage.LoadAllPoolStates()
	if err != nil {
		return err
	}

	o.poolsMu.Lock()
	for _, state := range states {
		o.pools[state.PoolId] = state
	}
	o.poolsMu.Unlock()

	logger := logging.GetLogger()
	logger.Info("loaded persisted pool states", "count", len(states))

	return nil
}

// notifySubscribers sends a price update to all subscribers
func (o *Oracle) notifySubscribers(update *PriceUpdate) {
	o.subscribersMu.RLock()
	defer o.subscribersMu.RUnlock()

	for _, ch := range o.subscribers {
		select {
		case ch <- update:
		default:
			// Channel full, skip
		}
	}
}

// Subscribe returns a channel that receives price updates
func (o *Oracle) Subscribe() <-chan *PriceUpdate {
	ch := make(chan *PriceUpdate, 100)

	o.subscribersMu.Lock()
	o.subscribers = append(o.subscribers, ch)
	o.subscribersMu.Unlock()

	return ch
}

// Unsubscribe removes a subscription channel
func (o *Oracle) Unsubscribe(ch <-chan *PriceUpdate) {
	o.subscribersMu.Lock()
	defer o.subscribersMu.Unlock()

	for i, sub := range o.subscribers {
		if sub == ch {
			o.subscribers = append(o.subscribers[:i], o.subscribers[i+1:]...)
			close(sub)
			break
		}
	}
}

// GetPoolState returns the current state of a pool
func (o *Oracle) GetPoolState(poolId string) (*PoolState, bool) {
	o.poolsMu.RLock()
	defer o.poolsMu.RUnlock()

	state, ok := o.pools[poolId]
	return state, ok
}

// GetAllPools returns all tracked pool states
func (o *Oracle) GetAllPools() []*PoolState {
	o.poolsMu.RLock()
	defer o.poolsMu.RUnlock()

	pools := make([]*PoolState, 0, len(o.pools))
	for _, state := range o.pools {
		pools = append(pools, state)
	}
	return pools
}

// PoolCount returns the number of tracked pools
func (o *Oracle) PoolCount() int {
	o.poolsMu.RLock()
	defer o.poolsMu.RUnlock()
	return len(o.pools)
}

// GetPrice returns the price of assetX in terms of assetY for a pool
func (o *Oracle) GetPrice(poolId string) (float64, bool) {
	o.poolsMu.RLock()
	defer o.poolsMu.RUnlock()

	if state, ok := o.pools[poolId]; ok {
		return state.PriceXY(), true
	}
	return 0, false
}

// GetMempoolPendingCount returns the total number of pending mempool transactions
func (o *Oracle) GetMempoolPendingCount() int {
	return o.mempoolMgr.TotalPendingTxs()
}

// GetMempoolPoolCount returns the number of pools with pending mempool transactions
func (o *Oracle) GetMempoolPoolCount() int {
	return o.mempoolMgr.PoolCount()
}

// GetMempoolPendingTxs returns all pending mempool transaction effects by pool
func (o *Oracle) GetMempoolPendingTxs() map[string][]*MempoolTxEffect {
	return o.mempoolMgr.GetAllPendingTxs()
}

// GetPoolMempoolState returns the mempool state for a specific pool
func (o *Oracle) GetPoolMempoolState(poolId string) (*MempoolPoolState, bool) {
	return o.mempoolMgr.GetPoolState(poolId)
}
