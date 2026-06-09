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

// Mempool state tracking with per-transaction granularity.
//
// This module tracks the effect of each mempool transaction individually,
// enabling analysis scenarios such as price-impact estimation and selective
// transaction ordering. Each pending transaction is recorded as a discrete
// effect (the delta it would apply to the confirmed pool state), preserving
// the order in which transactions were observed.

package oracle

import (
	"math"
	"sync"
	"time"
)

// MempoolTxEffect represents the effect of a single mempool transaction on a
// pool. It captures the signed delta (change) relative to the confirmed state,
// allowing for selective application of transactions in any order.
type MempoolTxEffect struct {
	TxHash    string    `json:"txHash"`
	PoolId    string    `json:"poolId"`
	Protocol  string    `json:"protocol"`
	Timestamp time.Time `json:"timestamp"`

	// Delta values are signed to allow for both additions and subtractions.
	// They represent the CHANGE from the confirmed state, not absolute values.
	DeltaX int64 `json:"deltaX"`
	DeltaY int64 `json:"deltaY"`

	// ResultingPriceXY is the price of X in terms of Y if this transaction
	// were applied alone to the confirmed state.
	ResultingPriceXY float64 `json:"resultingPriceXY"`
}

// PriceImpact returns the price change percentage this transaction causes
// relative to the given confirmed price. Returns 0 when confirmedPrice is 0.
func (e *MempoolTxEffect) PriceImpact(confirmedPrice float64) float64 {
	if confirmedPrice == 0 {
		return 0
	}
	return (e.ResultingPriceXY - confirmedPrice) / confirmedPrice * 100
}

// MempoolPoolState tracks the confirmed state and all pending mempool effects
// for a single pool. It is safe for concurrent use.
type MempoolPoolState struct {
	mu sync.RWMutex

	poolId   string
	protocol string

	// confirmedState is the last confirmed on-chain state.
	confirmedState *PoolState

	// pendingTxs holds per-transaction effects, indexed by tx hash.
	pendingTxs map[string]*MempoolTxEffect

	// txOrder preserves the order in which transactions were observed.
	txOrder []string
}

// NewMempoolPoolState creates a new mempool state tracker for a pool.
func NewMempoolPoolState(
	poolId, protocol string,
	confirmed *PoolState,
) *MempoolPoolState {
	return &MempoolPoolState{
		poolId:         poolId,
		protocol:       protocol,
		confirmedState: clonePoolState(confirmed),
		pendingTxs:     make(map[string]*MempoolTxEffect),
		txOrder:        make([]string, 0),
	}
}

// AddPendingTx records the effect of a pending mempool transaction relative to
// the confirmed state. If a transaction with the same hash already exists, the
// existing effect is returned unchanged. The returned effect is a copy; nil
// newState inputs are ignored and return nil.
func (m *MempoolPoolState) AddPendingTx(
	txHash string,
	newState *PoolState,
) *MempoolTxEffect {
	m.mu.Lock()
	defer m.mu.Unlock()

	if existing, ok := m.pendingTxs[txHash]; ok {
		return cloneMempoolTxEffect(existing)
	}
	if newState == nil {
		return nil
	}

	effect := &MempoolTxEffect{
		TxHash:           txHash,
		PoolId:           m.poolId,
		Protocol:         m.protocol,
		Timestamp:        time.Now(),
		ResultingPriceXY: newState.PriceXY(),
	}
	oldX, oldY := poolReserves(m.confirmedState)
	effect.DeltaX = reserveDelta(newState.AssetX.Amount, oldX)
	effect.DeltaY = reserveDelta(newState.AssetY.Amount, oldY)

	m.pendingTxs[txHash] = effect
	m.txOrder = append(m.txOrder, txHash)
	return cloneMempoolTxEffect(effect)
}

// RemovePendingTx removes a pending transaction by hash. Removing a hash that
// does not exist is a no-op.
func (m *MempoolPoolState) RemovePendingTx(txHash string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.removePendingTxLocked(txHash)
}

// removePendingTxLocked removes a pending transaction. Caller must hold m.mu.
func (m *MempoolPoolState) removePendingTxLocked(txHash string) {
	if _, ok := m.pendingTxs[txHash]; !ok {
		return
	}
	delete(m.pendingTxs, txHash)
	for i, h := range m.txOrder {
		if h == txHash {
			m.txOrder = append(m.txOrder[:i], m.txOrder[i+1:]...)
			break
		}
	}
}

// PendingCount returns the number of pending transactions tracked for the pool.
func (m *MempoolPoolState) PendingCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.pendingTxs)
}

// SetConfirmedState updates the confirmed on-chain state and drops any pending
// transaction whose hash now matches the confirmed transaction. Remaining
// pending effects are rebased so their deltas stay relative to the new
// confirmed state.
func (m *MempoolPoolState) SetConfirmedState(newConfirmed *PoolState) {
	m.mu.Lock()
	defer m.mu.Unlock()

	oldX, oldY := poolReserves(m.confirmedState)
	newX, newY := poolReserves(newConfirmed)

	m.confirmedState = clonePoolState(newConfirmed)
	if newConfirmed != nil {
		m.removePendingTxLocked(newConfirmed.TxHash)
	}

	// A pending effect's absolute resulting reserves are fixed at
	// (oldConfirmed + oldDelta). Shifting each delta by (old - new) keeps those
	// resulting reserves constant while making the delta reflect the change from
	// the new confirmed state instead of the stale previous one.
	shiftX := reserveDelta(oldX, newX)
	shiftY := reserveDelta(oldY, newY)
	if shiftX == 0 && shiftY == 0 {
		return
	}
	for _, effect := range m.pendingTxs {
		effect.DeltaX = addDeltaSaturating(effect.DeltaX, shiftX)
		effect.DeltaY = addDeltaSaturating(effect.DeltaY, shiftY)
	}
}

// poolReserves returns the X and Y reserve amounts of a pool state, treating a
// nil state as empty (zero) reserves.
func poolReserves(state *PoolState) (x, y uint64) {
	if state == nil {
		return 0, 0
	}
	return state.AssetX.Amount, state.AssetY.Amount
}

// reserveDelta returns newAmount - oldAmount as a signed int64. The true
// difference between two uint64 reserves can fall outside the int64 range, so
// rather than letting a uint64->int64 cast silently flip the sign, the result
// saturates at the int64 bounds.
func reserveDelta(newAmount, oldAmount uint64) int64 {
	if newAmount >= oldAmount {
		if diff := newAmount - oldAmount; diff <= math.MaxInt64 {
			return int64(diff)
		}
		return math.MaxInt64
	}
	if diff := oldAmount - newAmount; diff <= math.MaxInt64 {
		return -int64(diff)
	}
	return math.MinInt64
}

// addDeltaSaturating returns a + b clamped to the int64 range so that rebasing
// an already-saturated delta cannot itself silently overflow.
func addDeltaSaturating(a, b int64) int64 {
	sum := a + b
	switch {
	case a > 0 && b > 0 && sum < 0:
		return math.MaxInt64
	case a < 0 && b < 0 && sum >= 0:
		return math.MinInt64
	default:
		return sum
	}
}

// GetConfirmedState returns a copy of the current confirmed on-chain state.
func (m *MempoolPoolState) GetConfirmedState() *PoolState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return clonePoolState(m.confirmedState)
}

// GetPendingTxs returns copies of the pending transaction effects in insertion
// order.
func (m *MempoolPoolState) GetPendingTxs() []*MempoolTxEffect {
	m.mu.RLock()
	defer m.mu.RUnlock()

	effects := make([]*MempoolTxEffect, 0, len(m.txOrder))
	for _, h := range m.txOrder {
		if effect, ok := m.pendingTxs[h]; ok {
			effects = append(effects, cloneMempoolTxEffect(effect))
		}
	}
	return effects
}

func cloneMempoolTxEffect(effect *MempoolTxEffect) *MempoolTxEffect {
	if effect == nil {
		return nil
	}
	clone := *effect
	return &clone
}

// MempoolStateManager tracks mempool state across all pools. It is safe for
// concurrent use.
type MempoolStateManager struct {
	mu    sync.RWMutex
	pools map[string]*MempoolPoolState
}

// NewMempoolStateManager creates an empty mempool state manager.
func NewMempoolStateManager() *MempoolStateManager {
	return &MempoolStateManager{
		pools: make(map[string]*MempoolPoolState),
	}
}

// GetOrCreatePoolState returns the existing mempool state for a pool, creating
// one seeded with the given confirmed state if it does not yet exist.
func (mgr *MempoolStateManager) GetOrCreatePoolState(
	poolId, protocol string,
	confirmed *PoolState,
) *MempoolPoolState {
	mgr.mu.Lock()
	defer mgr.mu.Unlock()

	if ps, ok := mgr.pools[poolId]; ok {
		return ps
	}
	ps := NewMempoolPoolState(poolId, protocol, confirmed)
	mgr.pools[poolId] = ps
	return ps
}

// GetPoolState returns the mempool state for a pool, if tracked.
func (mgr *MempoolStateManager) GetPoolState(
	poolId string,
) (*MempoolPoolState, bool) {
	mgr.mu.RLock()
	defer mgr.mu.RUnlock()
	ps, ok := mgr.pools[poolId]
	if !ok || ps == nil {
		return nil, false
	}
	return ps, ok
}

// PoolCount returns the number of pools currently tracked.
func (mgr *MempoolStateManager) PoolCount() int {
	mgr.mu.RLock()
	defer mgr.mu.RUnlock()
	return len(mgr.pools)
}

// AddPendingTx records a pending transaction's effect for the given pool,
// creating the pool's mempool state if necessary.
func (mgr *MempoolStateManager) AddPendingTx(
	poolId, protocol, txHash string,
	newState *PoolState,
) *MempoolTxEffect {
	if newState == nil {
		mgr.mu.RLock()
		ps, ok := mgr.pools[poolId]
		mgr.mu.RUnlock()
		if !ok {
			return nil
		}
		return ps.AddPendingTx(txHash, nil)
	}
	ps := mgr.GetOrCreatePoolState(poolId, protocol, nil)
	return ps.AddPendingTx(txHash, newState)
}

// RemovePendingTx removes a pending transaction by hash from every pool that
// tracks it.
func (mgr *MempoolStateManager) RemovePendingTx(txHash string) {
	mgr.mu.RLock()
	defer mgr.mu.RUnlock()
	for _, ps := range mgr.pools {
		ps.RemovePendingTx(txHash)
	}
}

// TotalPendingTxs returns the total number of pending transactions across all
// pools.
func (mgr *MempoolStateManager) TotalPendingTxs() int {
	mgr.mu.RLock()
	defer mgr.mu.RUnlock()
	total := 0
	for _, ps := range mgr.pools {
		total += ps.PendingCount()
	}
	return total
}

// GetPoolsAffectedByTx returns the IDs of pools that have the given
// transaction hash pending.
func (mgr *MempoolStateManager) GetPoolsAffectedByTx(txHash string) []string {
	mgr.mu.RLock()
	defer mgr.mu.RUnlock()

	var affected []string
	for poolId, ps := range mgr.pools {
		ps.mu.RLock()
		_, ok := ps.pendingTxs[txHash]
		ps.mu.RUnlock()
		if ok {
			affected = append(affected, poolId)
		}
	}
	return affected
}

// RemovePoolState removes all mempool tracking for a pool, including its
// confirmed state and any pending transaction effects. Removing a pool that is
// not tracked is a no-op. This is used on rollback to discard reorged state so
// it is neither served to consumers nor used to seed future pending-tx deltas.
func (mgr *MempoolStateManager) RemovePoolState(poolId string) {
	mgr.mu.Lock()
	defer mgr.mu.Unlock()
	delete(mgr.pools, poolId)
}

// UpdateConfirmedState updates the confirmed state for a pool, creating the
// pool's mempool state if necessary. Any pending transaction matching the
// confirmed transaction hash is dropped.
func (mgr *MempoolStateManager) UpdateConfirmedState(
	poolId string,
	state *PoolState,
) {
	if state == nil {
		return
	}
	ps := mgr.GetOrCreatePoolState(poolId, state.Protocol, state)
	ps.SetConfirmedState(state)
}
