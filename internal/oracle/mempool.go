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

// Package oracle provides mempool state tracking with per-transaction granularity.
//
// This module tracks the effect of each mempool transaction individually,
// enabling future analysis scenarios like:
// - "What would the price be if tx 1 and 3 were applied but not tx 2?"
// - "What is the price impact of each individual pending transaction?"
// - "Compare multiple transaction ordering scenarios"
//
// Current implementation stores per-transaction effects.
// Granular query capabilities planned for v2.
package oracle

import (
	"sort"
	"sync"
	"time"
)

// MempoolTxEffect represents the effect of a single mempool transaction on a pool.
// This captures the delta (change) that a transaction would cause, allowing
// for selective application of transactions in any order.
type MempoolTxEffect struct {
	TxHash    string    `json:"txHash"`
	PoolId    string    `json:"poolId"`
	Protocol  string    `json:"protocol"`
	Sequence  int       `json:"sequence"`  // Order seen in mempool
	Timestamp time.Time `json:"timestamp"` // When we saw this tx

	// Delta values (signed to allow for both additions and subtractions)
	// These represent the CHANGE from confirmed state, not absolute values
	DeltaX int64 `json:"deltaX"` // Change in asset X reserve
	DeltaY int64 `json:"deltaY"` // Change in asset Y reserve

	// Resulting state if this tx were applied alone to confirmed state
	ResultingReserveX uint64  `json:"resultingReserveX"`
	ResultingReserveY uint64  `json:"resultingReserveY"`
	ResultingPriceXY  float64 `json:"resultingPriceXY"`

	// Fee changes (if the transaction modifies pool fees)
	NewFeeNum   uint64 `json:"newFeeNum,omitempty"`
	NewFeeDenom uint64 `json:"newFeeDenom,omitempty"`
	FeeChanged  bool   `json:"feeChanged"`

	// The full pool state this tx would produce (if applied alone)
	ResultingState *PoolState `json:"resultingState,omitempty"`
}

// PriceImpact returns the price change percentage this tx causes
// relative to the given confirmed price.
func (e *MempoolTxEffect) PriceImpact(confirmedPrice float64) float64 {
	if confirmedPrice == 0 {
		return 0
	}
	return (e.ResultingPriceXY - confirmedPrice) / confirmedPrice * 100
}

// MempoolPoolState tracks the confirmed state and all pending mempool effects
// for a single pool. It supports computing the resulting state for any
// subset of pending transactions.
type MempoolPoolState struct {
	mu sync.RWMutex

	PoolId   string `json:"poolId"`
	Protocol string `json:"protocol"`

	// The last confirmed on-chain state
	ConfirmedState *PoolState `json:"confirmedState"`

	// Per-transaction effects, indexed by tx hash
	PendingTxs map[string]*MempoolTxEffect `json:"pendingTxs"`

	// Ordered list of tx hashes by sequence (order seen in mempool)
	TxOrder []string `json:"txOrder"`

	// Sequence counter for ordering new transactions
	nextSequence int
}

// NewMempoolPoolState creates a new mempool state tracker for a pool.
func NewMempoolPoolState(
	poolId, protocol string,
	confirmedState *PoolState,
) *MempoolPoolState {
	return &MempoolPoolState{
		PoolId:         poolId,
		Protocol:       protocol,
		ConfirmedState: confirmedState,
		PendingTxs:     make(map[string]*MempoolTxEffect),
		TxOrder:        make([]string, 0),
	}
}

// SetConfirmedState updates the confirmed on-chain state.
// Also removes any pending tx that matches this confirmed tx hash.
func (m *MempoolPoolState) SetConfirmedState(state *PoolState) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.ConfirmedState = state

	// Remove pending tx that is now confirmed
	if _, exists := m.PendingTxs[state.TxHash]; exists {
		delete(m.PendingTxs, state.TxHash)
		m.rebuildTxOrder()
	}
}

// AddPendingTx adds a new pending transaction effect.
// Calculates deltas from confirmed state and stores the effect.
func (m *MempoolPoolState) AddPendingTx(
	txHash string,
	newState *PoolState,
) *MempoolTxEffect {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Skip if already tracked
	if existing, exists := m.PendingTxs[txHash]; exists {
		return existing
	}

	// Calculate deltas from confirmed state
	var deltaX, deltaY int64
	if m.ConfirmedState != nil {
		deltaX = int64(
			newState.AssetX.Amount,
		) - int64(
			m.ConfirmedState.AssetX.Amount,
		)
		deltaY = int64(
			newState.AssetY.Amount,
		) - int64(
			m.ConfirmedState.AssetY.Amount,
		)
	}

	effect := &MempoolTxEffect{
		TxHash:            txHash,
		PoolId:            m.PoolId,
		Protocol:          m.Protocol,
		Sequence:          m.nextSequence,
		Timestamp:         time.Now(),
		DeltaX:            deltaX,
		DeltaY:            deltaY,
		ResultingReserveX: newState.AssetX.Amount,
		ResultingReserveY: newState.AssetY.Amount,
		ResultingPriceXY:  newState.PriceXY(),
		ResultingState:    newState,
	}

	// Check for fee changes
	if m.ConfirmedState != nil &&
		(newState.FeeNum != m.ConfirmedState.FeeNum ||
			newState.FeeDenom != m.ConfirmedState.FeeDenom) {
		effect.FeeChanged = true
		effect.NewFeeNum = newState.FeeNum
		effect.NewFeeDenom = newState.FeeDenom
	}

	m.PendingTxs[txHash] = effect
	m.TxOrder = append(m.TxOrder, txHash)
	m.nextSequence++

	return effect
}

// RemovePendingTx removes a pending transaction.
func (m *MempoolPoolState) RemovePendingTx(txHash string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.PendingTxs[txHash]; exists {
		delete(m.PendingTxs, txHash)
		m.rebuildTxOrder()
	}
}

// rebuildTxOrder rebuilds the tx order list (must be called with lock held)
func (m *MempoolPoolState) rebuildTxOrder() {
	m.TxOrder = make([]string, 0, len(m.PendingTxs))
	for txHash := range m.PendingTxs {
		m.TxOrder = append(m.TxOrder, txHash)
	}
	// Sort by sequence
	sort.Slice(m.TxOrder, func(i, j int) bool {
		return m.PendingTxs[m.TxOrder[i]].Sequence <
			m.PendingTxs[m.TxOrder[j]].Sequence
	})
}

// GetConfirmedState returns the confirmed on-chain state.
func (m *MempoolPoolState) GetConfirmedState() *PoolState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.ConfirmedState
}

// GetPendingTxs returns all pending transaction effects in order.
func (m *MempoolPoolState) GetPendingTxs() []*MempoolTxEffect {
	m.mu.RLock()
	defer m.mu.RUnlock()

	effects := make([]*MempoolTxEffect, 0, len(m.TxOrder))
	for _, txHash := range m.TxOrder {
		if effect, ok := m.PendingTxs[txHash]; ok {
			effects = append(effects, effect)
		}
	}
	return effects
}

// GetPendingTx returns a specific pending transaction effect.
func (m *MempoolPoolState) GetPendingTx(
	txHash string,
) (*MempoolTxEffect, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	effect, ok := m.PendingTxs[txHash]
	return effect, ok
}

// PendingCount returns the number of pending transactions.
func (m *MempoolPoolState) PendingCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.PendingTxs)
}

// MempoolStateManager manages mempool states for all tracked pools.
// It provides a centralized view of all pending transactions and their
// effects across the entire system.
type MempoolStateManager struct {
	mu     sync.RWMutex
	pools  map[string]*MempoolPoolState // Indexed by poolId
	byHash map[string][]string          // tx hash -> list of affected pool IDs
}

// NewMempoolStateManager creates a new mempool state manager.
func NewMempoolStateManager() *MempoolStateManager {
	return &MempoolStateManager{
		pools:  make(map[string]*MempoolPoolState),
		byHash: make(map[string][]string),
	}
}

// GetOrCreatePoolState gets or creates a mempool state tracker for a pool.
func (mgr *MempoolStateManager) GetOrCreatePoolState(
	poolId, protocol string,
	confirmedState *PoolState,
) *MempoolPoolState {
	mgr.mu.Lock()
	defer mgr.mu.Unlock()

	if state, ok := mgr.pools[poolId]; ok {
		return state
	}

	state := NewMempoolPoolState(poolId, protocol, confirmedState)
	mgr.pools[poolId] = state
	return state
}

// GetPoolState returns the mempool state for a pool.
func (mgr *MempoolStateManager) GetPoolState(
	poolId string,
) (*MempoolPoolState, bool) {
	mgr.mu.RLock()
	defer mgr.mu.RUnlock()
	state, ok := mgr.pools[poolId]
	return state, ok
}

// UpdateConfirmedState updates the confirmed state for a pool.
// Also cleans up byHash tracking for any transactions that become confirmed.
func (mgr *MempoolStateManager) UpdateConfirmedState(
	poolId string,
	state *PoolState,
) {
	mgr.mu.Lock()
	defer mgr.mu.Unlock()

	if poolState, ok := mgr.pools[poolId]; ok {
		// Check if the confirmed tx was pending before updating
		txHash := state.TxHash
		_, wasPending := poolState.GetPendingTx(txHash)

		poolState.SetConfirmedState(state)

		// Clean up byHash if this tx was pending
		if wasPending && txHash != "" {
			mgr.cleanupByHash(txHash, poolId)
		}
	} else {
		// Create new pool state with this confirmed state
		mgr.pools[poolId] = NewMempoolPoolState(
			poolId,
			state.Protocol,
			state,
		)
	}
}

// cleanupByHash removes a pool from the byHash tracking for a tx.
// Must be called with mgr.mu held.
func (mgr *MempoolStateManager) cleanupByHash(txHash, poolId string) {
	if poolIds, ok := mgr.byHash[txHash]; ok {
		newPoolIds := make([]string, 0, len(poolIds)-1)
		for _, pid := range poolIds {
			if pid != poolId {
				newPoolIds = append(newPoolIds, pid)
			}
		}
		if len(newPoolIds) == 0 {
			delete(mgr.byHash, txHash)
		} else {
			mgr.byHash[txHash] = newPoolIds
		}
	}
}

// AddPendingTx adds a pending transaction effect for a pool.
func (mgr *MempoolStateManager) AddPendingTx(
	poolId, protocol, txHash string,
	newState *PoolState,
) *MempoolTxEffect {
	mgr.mu.Lock()
	defer mgr.mu.Unlock()

	// Get or create pool state
	poolState, ok := mgr.pools[poolId]
	if !ok {
		poolState = NewMempoolPoolState(poolId, protocol, nil)
		mgr.pools[poolId] = poolState
	}

	effect := poolState.AddPendingTx(txHash, newState)

	// Track tx hash -> pool mapping
	mgr.byHash[txHash] = append(mgr.byHash[txHash], poolId)

	return effect
}

// RemovePendingTx removes a pending transaction from all affected pools.
func (mgr *MempoolStateManager) RemovePendingTx(txHash string) {
	mgr.mu.Lock()
	defer mgr.mu.Unlock()

	if poolIds, ok := mgr.byHash[txHash]; ok {
		for _, poolId := range poolIds {
			if poolState, ok := mgr.pools[poolId]; ok {
				poolState.RemovePendingTx(txHash)
			}
		}
		delete(mgr.byHash, txHash)
	}
}

// GetPoolsAffectedByTx returns all pool IDs affected by a transaction.
func (mgr *MempoolStateManager) GetPoolsAffectedByTx(txHash string) []string {
	mgr.mu.RLock()
	defer mgr.mu.RUnlock()

	if poolIds, ok := mgr.byHash[txHash]; ok {
		result := make([]string, len(poolIds))
		copy(result, poolIds)
		return result
	}
	return nil
}

// GetAllPendingTxs returns all pending transactions across all pools.
func (mgr *MempoolStateManager) GetAllPendingTxs() map[string][]*MempoolTxEffect {
	mgr.mu.RLock()
	defer mgr.mu.RUnlock()

	result := make(map[string][]*MempoolTxEffect)
	for poolId, poolState := range mgr.pools {
		effects := poolState.GetPendingTxs()
		if len(effects) > 0 {
			result[poolId] = effects
		}
	}
	return result
}

// PoolCount returns the number of tracked pools.
func (mgr *MempoolStateManager) PoolCount() int {
	mgr.mu.RLock()
	defer mgr.mu.RUnlock()
	return len(mgr.pools)
}

// TotalPendingTxs returns the total number of pending transactions.
func (mgr *MempoolStateManager) TotalPendingTxs() int {
	mgr.mu.RLock()
	defer mgr.mu.RUnlock()
	return len(mgr.byHash)
}
