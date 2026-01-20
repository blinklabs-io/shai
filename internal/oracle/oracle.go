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
	"fmt"
	"sync"
	"time"

	"github.com/blinklabs-io/adder/event"
	"github.com/blinklabs-io/shai/internal/config"
	"github.com/blinklabs-io/shai/internal/indexer"
	"github.com/blinklabs-io/shai/internal/logging"
)

// Oracle tracks pool states from on-chain data
type Oracle struct {
	idx           *indexer.Indexer
	profile       *config.Profile
	parser        PoolParser
	pools         map[string]*PoolState
	poolsMu       sync.RWMutex
	poolAddresses map[string]struct{} // Set for O(1) lookup
	storage       *OracleStorage
	stopChan      chan struct{}
}

// New creates a new Oracle instance
func New(
	idx *indexer.Indexer,
	profile *config.Profile,
	parser PoolParser,
) *Oracle {
	o := &Oracle{
		idx:           idx,
		profile:       profile,
		parser:        parser,
		pools:         make(map[string]*PoolState),
		poolAddresses: make(map[string]struct{}),
		stopChan:      make(chan struct{}),
	}

	// Extract pool addresses from profile config
	if oracleConfig, ok := profile.Config.(config.OracleProfileConfig); ok {
		for _, addr := range oracleConfig.PoolAddresses {
			o.poolAddresses[addr.Address] = struct{}{}
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

	logger.Info(
		"Oracle started",
		"profile", o.profile.Name,
		"protocol", o.parser.Protocol(),
		"addresses", len(o.poolAddresses),
	)

	return nil
}

// Stop stops the oracle
func (o *Oracle) Stop() {
	close(o.stopChan)
	if o.storage != nil {
		if err := o.storage.Close(); err != nil {
			logger := logging.GetLogger()
			logger.Error("failed to close oracle storage", "error", err)
		}
	}
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

// handleTransaction processes a transaction event
func (o *Oracle) handleTransaction(
	evt event.Event,
	txEvt event.TransactionEvent,
) error {
	logger := logging.GetLogger()
	ctx, ok := evt.Context.(event.TransactionContext)
	if !ok {
		logger.Error(
			"unexpected event context type",
			"expected", "event.TransactionContext",
			"got", fmt.Sprintf("%T", evt.Context),
		)
		return nil
	}
	cfg := config.GetConfig()

	// Check for pool UTxOs at monitored addresses
	for _, utxo := range txEvt.Transaction.Produced() {
		addr := utxo.Output.Address().String()
		if !o.isPoolAddress(addr) {
			continue
		}

		// Skip UTxOs without datum
		if utxo.Output.Datum() == nil {
			continue
		}

		// Parse the pool state using the protocol-specific parser
		state, err := o.parser.ParsePoolDatum(
			utxo.Output.Datum().Cbor(),
			utxo.Output.Cbor(),
			ctx.TransactionHash,
			utxo.Id.Index(),
			ctx.SlotNumber,
			time.Now(),
		)
		if err != nil {
			// Not a valid pool datum for this protocol
			continue
		}

		// Set metadata
		state.Network = cfg.Network
		state.BlockHash = txEvt.BlockHash
		state.UpdatedAt = time.Now()

		// Update pool state
		o.poolsMu.Lock()
		o.pools[state.PoolId] = state
		o.poolsMu.Unlock()

		// Persist to storage
		if err := o.storage.SavePoolState(state); err != nil {
			logger.Error(
				"failed to persist pool state",
				"error", err,
				"poolId", state.PoolId,
			)
		}

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

// handleRollback processes a rollback event by invalidating pool states
// that were recorded at or after the rollback slot
func (o *Oracle) handleRollback(evt event.RollbackEvent) error {
	logger := logging.GetLogger()
	logger.Warn(
		"rollback detected in oracle",
		"slot", evt.SlotNumber,
		"blockHash", evt.BlockHash,
	)

	// Collect pool IDs to invalidate (states with Slot >= rollback slot)
	o.poolsMu.Lock()
	var toDelete []*PoolState
	for _, state := range o.pools {
		if state.Slot >= evt.SlotNumber {
			toDelete = append(toDelete, state)
		}
	}

	// Remove from in-memory map
	for _, state := range toDelete {
		delete(o.pools, state.PoolId)
	}
	o.poolsMu.Unlock()

	// Delete from persistent storage
	var errs []error
	for _, state := range toDelete {
		if err := o.storage.DeletePoolState(state); err != nil {
			logger.Error(
				"failed to delete rolled-back pool state",
				"error", err,
				"poolId", state.PoolId,
				"slot", state.Slot,
			)
			errs = append(errs, err)
		} else {
			logger.Info(
				"invalidated pool state due to rollback",
				"poolId", state.PoolId,
				"slot", state.Slot,
				"rollbackSlot", evt.SlotNumber,
			)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf(
			"failed to delete %d pool states during rollback",
			len(errs),
		)
	}

	return nil
}

// isPoolAddress checks if an address is a monitored pool address
func (o *Oracle) isPoolAddress(addr string) bool {
	_, ok := o.poolAddresses[addr]
	return ok
}

// loadPersistedStates loads pool states from storage
func (o *Oracle) loadPersistedStates() error {
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
