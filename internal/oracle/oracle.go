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
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/blinklabs-io/adder/event"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/blinklabs-io/shai/internal/config"
	"github.com/blinklabs-io/shai/internal/indexer"
	"github.com/blinklabs-io/shai/internal/logging"
)

const (
	subscriberBufferSize = 100
	dropLogSampleRate    = 100
)

// CDPParser parses synthetics/CDP datums that are tracked separately from AMM
// pool states.
type CDPParser interface {
	Protocol() string
	ParseCDPDatum(
		datum []byte,
		txHash string,
		txIndex uint32,
		slot uint64,
		timestamp time.Time,
	) (*CDPState, error)
	CDPIdForOutput(txHash string, txIndex uint32) string
}

// Oracle tracks pool states from on-chain data
type Oracle struct {
	idx           *indexer.Indexer
	profile       *config.Profile
	parser        PoolParser
	pools         map[string]*PoolState
	poolsMu       sync.RWMutex
	cdps          map[string]*CDPState
	cdpsMu        sync.RWMutex
	poolAddresses map[string]struct{} // Set for O(1) lookup
	storage       *OracleStorage
	stopChan      chan struct{}
	subscribers   []chan *PriceUpdate
	subMu         sync.RWMutex
	stopped       bool
	dropCount     atomic.Uint64
	mempoolMgr    *MempoolStateManager
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
		cdps:          make(map[string]*CDPState),
		poolAddresses: make(map[string]struct{}),
		stopChan:      make(chan struct{}),
		mempoolMgr:    NewMempoolStateManager(),
	}

	o.addProfileAddresses()

	return o
}

func (o *Oracle) addProfileAddresses() {
	switch profileConfig := o.profile.Config.(type) {
	case config.OracleProfileConfig:
		for _, addr := range profileConfig.PoolAddresses {
			o.poolAddresses[addr.Address] = struct{}{}
		}
	case config.SyntheticsProfileConfig:
		for _, addr := range profileConfig.CDPAddresses {
			o.poolAddresses[addr.Address] = struct{}{}
		}
	case config.BondsProfileConfig:
		for _, addr := range profileConfig.BondAddresses {
			o.poolAddresses[addr.Address] = struct{}{}
		}
		for _, addr := range profileConfig.OADAAddresses {
			o.poolAddresses[addr.Address] = struct{}{}
		}
	}
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
	o.subMu.Lock()
	if o.stopped {
		o.subMu.Unlock()
		return
	}
	o.stopped = true
	for _, ch := range o.subscribers {
		close(ch)
	}
	o.subscribers = nil
	o.subMu.Unlock()

	close(o.stopChan)
	if o.storage != nil {
		if err := o.storage.Close(); err != nil {
			logger := logging.GetLogger()
			logger.Error("failed to close oracle storage", "error", err)
		}
	}
}

// Subscribe returns a receive-only channel that receives price updates.
// The caller should call Unsubscribe when done to prevent leaks.
func (o *Oracle) Subscribe() <-chan *PriceUpdate {
	o.subMu.Lock()
	defer o.subMu.Unlock()

	ch := make(chan *PriceUpdate, subscriberBufferSize)
	if o.stopped {
		close(ch)
		return ch
	}
	o.subscribers = append(o.subscribers, ch)
	return ch
}

// Unsubscribe removes a subscriber channel.
func (o *Oracle) Unsubscribe(ch <-chan *PriceUpdate) {
	o.subMu.Lock()
	defer o.subMu.Unlock()

	for i, sub := range o.subscribers {
		if (<-chan *PriceUpdate)(sub) == ch {
			o.subscribers = append(o.subscribers[:i], o.subscribers[i+1:]...)
			close(sub)
			return
		}
	}
}

// notifySubscribers sends a price update to all subscribers.
func (o *Oracle) notifySubscribers(state *PoolState, prevPrice float64) {
	update := NewPriceUpdate(state, prevPrice)
	if update == nil {
		return
	}
	logger := logging.GetLogger()

	o.subMu.RLock()
	defer o.subMu.RUnlock()

	for i, ch := range o.subscribers {
		updateCopy := clonePriceUpdate(update)
		select {
		case ch <- updateCopy:
		default:
			// Subscriber channel is full. Drop oldest buffered update and try
			// to enqueue the newest one so slow subscribers receive fresher data.
			select {
			case <-ch:
				o.recordDrop(logger, i, updateCopy)
			default:
			}
			select {
			case ch <- updateCopy:
			default:
				o.recordDrop(logger, i, updateCopy)
			}
		}
	}
}

func clonePriceUpdate(update *PriceUpdate) *PriceUpdate {
	if update == nil {
		return nil
	}
	copy := *update
	return &copy
}

func (o *Oracle) recordDrop(
	logger *slog.Logger,
	subscriberIndex int,
	update *PriceUpdate,
) {
	drops := o.dropCount.Add(1)
	if drops == 1 || drops%dropLogSampleRate == 0 {
		logger.Debug(
			"oracle subscriber update dropped",
			"subscriberIndex", subscriberIndex,
			"updateType", "price_update",
			"poolId", update.PoolId,
			"drops", drops,
		)
	}
}

// DroppedNotifications returns the total number of subscriber updates dropped.
func (o *Oracle) DroppedNotifications() uint64 {
	return o.dropCount.Load()
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
	cdpParser, hasCDPParser := o.parser.(CDPParser)
	if hasCDPParser && o.isSyntheticsProfile() {
		o.deleteSpentCDPStates(logger, cdpParser, transactionInputs(txEvt))
	}

	// Check for tracked UTxOs at monitored addresses.
	for _, utxo := range producedUTXOs(txEvt, ctx.TransactionHash) {
		addr := utxo.Output.Address().String()
		if !o.isPoolAddress(addr) {
			continue
		}
		if utxo.Output.Datum() == nil {
			continue
		}

		if hasCDPParser && o.isSyntheticsProfile() {
			o.handleProducedCDPOutput(
				logger,
				cdpParser,
				cfg.Network,
				txEvt.BlockHash,
				ctx,
				utxo,
			)
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
		if state == nil {
			continue
		}

		// Set metadata
		state.Network = cfg.Network
		state.BlockHash = txEvt.BlockHash
		state.UpdatedAt = time.Now()

		// Update pool state
		o.poolsMu.Lock()
		var prevPrice float64
		if prev, ok := o.pools[state.PoolId]; ok {
			prevPrice = prev.PriceXY()
		}
		o.pools[state.PoolId] = state
		o.poolsMu.Unlock()

		// Notify subscribers of price update
		o.notifySubscribers(state, prevPrice)

		// Update mempool manager's confirmed state for this pool
		o.mempoolMgr.UpdateConfirmedState(state.PoolId, state)

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

func (o *Oracle) handleProducedCDPOutput(
	logger *slog.Logger,
	parser CDPParser,
	network string,
	blockHash string,
	ctx event.TransactionContext,
	utxo ledger.Utxo,
) {
	now := time.Now()
	datum := utxo.Output.Datum()
	if datum == nil {
		logger.Debug(
			"skipping produced CDP output without datum",
			"txHash", ctx.TransactionHash,
			"outputIndex", utxo.Id.Index(),
		)
		return
	}

	state, err := parser.ParseCDPDatum(
		datum.Cbor(),
		ctx.TransactionHash,
		utxo.Id.Index(),
		ctx.SlotNumber,
		now,
	)
	if err != nil || state == nil {
		return
	}

	state.Network = network
	state.Protocol = parser.Protocol()
	state.BlockHash = blockHash
	state.UpdatedAt = now

	o.cdpsMu.Lock()
	if o.cdps == nil {
		o.cdps = make(map[string]*CDPState)
	}
	o.cdps[state.CDPId] = state
	o.cdpsMu.Unlock()

	if err := o.storage.SaveCDPState(state); err != nil {
		logger.Error(
			"failed to persist CDP state",
			"error", err,
			"cdpId", state.CDPId,
		)
	}

	logger.Debug(
		"CDP state updated",
		"cdpId", state.CDPId,
		"protocol", state.Protocol,
		"slot", state.Slot,
	)
}

func (o *Oracle) deleteSpentCDPStates(
	logger *slog.Logger,
	parser CDPParser,
	inputs []ledger.TransactionInput,
) {
	for _, input := range inputs {
		cdpId := parser.CDPIdForOutput(input.Id().String(), input.Index())
		state, ok := o.deleteCDPStateByID(cdpId)
		if !ok {
			continue
		}
		network := state.Network
		if network == "" {
			network = config.GetConfig().Network
		}
		protocol := state.Protocol
		if protocol == "" {
			protocol = parser.Protocol()
		}
		if err := o.storage.DeleteCDPState(
			network,
			protocol,
			cdpId,
		); err != nil {
			logger.Error(
				"failed to delete spent CDP state",
				"error", err,
				"cdpId", cdpId,
			)
			continue
		}
		logger.Debug(
			"deleted spent CDP state",
			"cdpId", cdpId,
			"protocol", parser.Protocol(),
		)
	}
}

func (o *Oracle) deleteCDPStateByID(cdpId string) (*CDPState, bool) {
	o.cdpsMu.Lock()
	defer o.cdpsMu.Unlock()
	if o.cdps == nil {
		return nil, false
	}
	state, ok := o.cdps[cdpId]
	if !ok {
		return nil, false
	}
	delete(o.cdps, cdpId)
	return state, true
}

func (o *Oracle) isSyntheticsProfile() bool {
	return o.profile != nil && o.profile.Type == config.ProfileTypeSynthetics
}

func transactionInputs(txEvt event.TransactionEvent) []ledger.TransactionInput {
	if len(txEvt.Inputs) > 0 {
		return txEvt.Inputs
	}
	if txEvt.Transaction == nil {
		return nil
	}
	return txEvt.Transaction.Consumed()
}

func producedUTXOs(
	txEvt event.TransactionEvent,
	txHash string,
) []ledger.Utxo {
	if txEvt.Transaction != nil {
		return txEvt.Transaction.Produced()
	}
	utxos := make([]ledger.Utxo, 0, len(txEvt.Outputs))
	for i, output := range txEvt.Outputs {
		utxos = append(utxos, ledger.Utxo{
			Id:     shelley.NewShelleyTransactionInput(txHash, i),
			Output: output,
		})
	}
	return utxos
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

	o.cdpsMu.Lock()
	var cdpsToDelete []*CDPState
	for _, state := range o.cdps {
		if state.Slot >= evt.SlotNumber {
			cdpsToDelete = append(cdpsToDelete, state)
		}
	}
	for _, state := range cdpsToDelete {
		delete(o.cdps, state.CDPId)
	}
	o.cdpsMu.Unlock()

	// Invalidate mempool tracking for rolled-back pools. Otherwise the reorged
	// confirmed state stays visible to consumers and seeds future pending-tx
	// delta calculations with stale data.
	for _, state := range toDelete {
		o.mempoolMgr.RemovePoolState(state.PoolId)
	}

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
	for _, state := range cdpsToDelete {
		if err := o.storage.DeleteCDPState(
			state.Network,
			state.Protocol,
			state.CDPId,
		); err != nil {
			logger.Error(
				"failed to delete rolled-back CDP state",
				"error", err,
				"cdpId", state.CDPId,
				"slot", state.Slot,
			)
			errs = append(errs, err)
		} else {
			logger.Info(
				"invalidated CDP state due to rollback",
				"cdpId", state.CDPId,
				"slot", state.Slot,
				"rollbackSlot", evt.SlotNumber,
			)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf(
			"failed to delete %d oracle states during rollback",
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

// loadPersistedStates loads oracle states from storage.
func (o *Oracle) loadPersistedStates() error {
	states, err := o.storage.LoadAllPoolStates()
	if err != nil {
		return err
	}
	cdpStates, err := o.storage.LoadAllCDPStates()
	if err != nil {
		return err
	}

	o.poolsMu.Lock()
	if o.pools == nil {
		o.pools = make(map[string]*PoolState)
	}
	for _, state := range states {
		o.pools[state.PoolId] = state
	}
	o.poolsMu.Unlock()

	o.cdpsMu.Lock()
	if o.cdps == nil {
		o.cdps = make(map[string]*CDPState)
	}
	for _, state := range cdpStates {
		o.cdps[state.CDPId] = state
	}
	o.cdpsMu.Unlock()

	if o.mempoolMgr == nil {
		o.mempoolMgr = NewMempoolStateManager()
	}
	for _, state := range states {
		o.mempoolMgr.UpdateConfirmedState(state.PoolId, state)
	}

	logger := logging.GetLogger()
	logger.Info(
		"loaded persisted oracle states",
		"pools", len(states),
		"cdps", len(cdpStates),
	)

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

// GetCDPState returns the current state of a CDP.
func (o *Oracle) GetCDPState(cdpId string) (*CDPState, bool) {
	o.cdpsMu.RLock()
	defer o.cdpsMu.RUnlock()

	state, ok := o.cdps[cdpId]
	return state, ok
}

// GetAllCDPs returns all tracked CDP states.
func (o *Oracle) GetAllCDPs() []*CDPState {
	o.cdpsMu.RLock()
	defer o.cdpsMu.RUnlock()

	cdps := make([]*CDPState, 0, len(o.cdps))
	for _, state := range o.cdps {
		cdps = append(cdps, state)
	}
	return cdps
}

// CDPCount returns the number of tracked CDPs.
func (o *Oracle) CDPCount() int {
	o.cdpsMu.RLock()
	defer o.cdpsMu.RUnlock()
	return len(o.cdps)
}

// PoolCount returns the number of tracked pools
func (o *Oracle) PoolCount() int {
	o.poolsMu.RLock()
	defer o.poolsMu.RUnlock()
	return len(o.pools)
}

// GetMempoolManager returns the mempool state manager.
func (o *Oracle) GetMempoolManager() *MempoolStateManager { return o.mempoolMgr }

// GetPrice returns the price of assetX in terms of assetY for a pool
func (o *Oracle) GetPrice(poolId string) (float64, bool) {
	o.poolsMu.RLock()
	defer o.poolsMu.RUnlock()

	if state, ok := o.pools[poolId]; ok {
		return state.PriceXY(), true
	}
	return 0, false
}
