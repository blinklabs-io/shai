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
	"testing"
	"time"

	"github.com/blinklabs-io/shai/internal/common"
)

// Helper to create a test pool state
func createTestPoolState(
	poolId, protocol, txHash string,
	reserveX, reserveY uint64,
) *PoolState {
	return &PoolState{
		PoolId:   poolId,
		Protocol: protocol,
		AssetX: common.AssetAmount{
			Class:  common.AssetClass{PolicyId: []byte{}, Name: []byte{}},
			Amount: reserveX,
		},
		AssetY: common.AssetAmount{
			Class: common.AssetClass{
				PolicyId: []byte{0x01},
				Name:     []byte("TOKEN"),
			},
			Amount: reserveY,
		},
		FeeNum:    997,
		FeeDenom:  1000,
		TxHash:    txHash,
		Timestamp: time.Now(),
	}
}

// ==================== MempoolTxEffect Tests ====================

func TestMempoolTxEffectPriceImpact(t *testing.T) {
	tests := []struct {
		name           string
		resultingPrice float64
		confirmedPrice float64
		expectedImpact float64
	}{
		{
			name:           "no change",
			resultingPrice: 1.5,
			confirmedPrice: 1.5,
			expectedImpact: 0,
		},
		{
			name:           "10% increase",
			resultingPrice: 1.65,
			confirmedPrice: 1.5,
			expectedImpact: 10.0,
		},
		{
			name:           "10% decrease",
			resultingPrice: 1.35,
			confirmedPrice: 1.5,
			expectedImpact: -10.0,
		},
		{
			name:           "zero confirmed price",
			resultingPrice: 1.5,
			confirmedPrice: 0,
			expectedImpact: 0,
		},
		{
			name:           "double price",
			resultingPrice: 3.0,
			confirmedPrice: 1.5,
			expectedImpact: 100.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			effect := &MempoolTxEffect{
				ResultingPriceXY: tt.resultingPrice,
			}
			impact := effect.PriceImpact(tt.confirmedPrice)
			// Use tolerance for floating point comparison
			diff := impact - tt.expectedImpact
			if diff < 0 {
				diff = -diff
			}
			if diff > 0.0001 {
				t.Errorf(
					"expected impact %.4f, got %.4f",
					tt.expectedImpact,
					impact,
				)
			}
		})
	}
}

func TestMempoolTxEffectFields(t *testing.T) {
	now := time.Now()
	effect := &MempoolTxEffect{
		TxHash:            "abc123",
		PoolId:            "pool_123",
		Protocol:          "minswap",
		Sequence:          5,
		Timestamp:         now,
		DeltaX:            1000000,
		DeltaY:            -500000,
		ResultingReserveX: 101000000,
		ResultingReserveY: 99500000,
		ResultingPriceXY:  0.985,
		FeeChanged:        true,
		NewFeeNum:         995,
		NewFeeDenom:       1000,
	}

	if effect.TxHash != "abc123" {
		t.Errorf("expected txHash 'abc123', got %s", effect.TxHash)
	}
	if effect.PoolId != "pool_123" {
		t.Errorf("expected poolId 'pool_123', got %s", effect.PoolId)
	}
	if effect.Sequence != 5 {
		t.Errorf("expected sequence 5, got %d", effect.Sequence)
	}
	if effect.DeltaX != 1000000 {
		t.Errorf("expected deltaX 1000000, got %d", effect.DeltaX)
	}
	if effect.DeltaY != -500000 {
		t.Errorf("expected deltaY -500000, got %d", effect.DeltaY)
	}
	if !effect.FeeChanged {
		t.Error("expected feeChanged to be true")
	}
}

// ==================== MempoolPoolState Tests ====================

func TestNewMempoolPoolState(t *testing.T) {
	confirmedState := createTestPoolState(
		"pool_abc",
		"spectrum",
		"tx_confirmed",
		100000000,
		200000000,
	)

	mps := NewMempoolPoolState("pool_abc", "spectrum", confirmedState)

	if mps.PoolId != "pool_abc" {
		t.Errorf("expected poolId 'pool_abc', got %s", mps.PoolId)
	}
	if mps.Protocol != "spectrum" {
		t.Errorf("expected protocol 'spectrum', got %s", mps.Protocol)
	}
	if mps.ConfirmedState != confirmedState {
		t.Error("confirmed state mismatch")
	}
	if len(mps.PendingTxs) != 0 {
		t.Errorf("expected 0 pending txs, got %d", len(mps.PendingTxs))
	}
	if len(mps.TxOrder) != 0 {
		t.Errorf("expected 0 tx order, got %d", len(mps.TxOrder))
	}
}

func TestMempoolPoolStateAddPendingTx(t *testing.T) {
	confirmedState := createTestPoolState(
		"pool_abc",
		"minswap",
		"tx_confirmed",
		100000000,
		200000000,
	)

	mps := NewMempoolPoolState("pool_abc", "minswap", confirmedState)

	// Add first pending tx
	newState1 := createTestPoolState(
		"pool_abc",
		"minswap",
		"tx_mempool_1",
		101000000,
		199000000,
	)
	effect1 := mps.AddPendingTx("tx_mempool_1", newState1)

	if effect1 == nil {
		t.Fatal("expected non-nil effect")
	}
	if effect1.TxHash != "tx_mempool_1" {
		t.Errorf("expected txHash 'tx_mempool_1', got %s", effect1.TxHash)
	}
	if effect1.Sequence != 0 {
		t.Errorf("expected sequence 0, got %d", effect1.Sequence)
	}
	if effect1.DeltaX != 1000000 {
		t.Errorf("expected deltaX 1000000, got %d", effect1.DeltaX)
	}
	if effect1.DeltaY != -1000000 {
		t.Errorf("expected deltaY -1000000, got %d", effect1.DeltaY)
	}
	if mps.PendingCount() != 1 {
		t.Errorf("expected 1 pending tx, got %d", mps.PendingCount())
	}

	// Add second pending tx
	newState2 := createTestPoolState(
		"pool_abc",
		"minswap",
		"tx_mempool_2",
		102000000,
		198000000,
	)
	effect2 := mps.AddPendingTx("tx_mempool_2", newState2)

	if effect2.Sequence != 1 {
		t.Errorf("expected sequence 1, got %d", effect2.Sequence)
	}
	if mps.PendingCount() != 2 {
		t.Errorf("expected 2 pending txs, got %d", mps.PendingCount())
	}

	// Try to add duplicate tx
	duplicateEffect := mps.AddPendingTx("tx_mempool_1", newState1)
	if duplicateEffect != effect1 {
		t.Error("expected same effect for duplicate tx")
	}
	if mps.PendingCount() != 2 {
		t.Errorf(
			"expected still 2 pending txs after duplicate, got %d",
			mps.PendingCount(),
		)
	}
}

func TestMempoolPoolStateRemovePendingTx(t *testing.T) {
	confirmedState := createTestPoolState(
		"pool_abc",
		"minswap",
		"tx_confirmed",
		100000000,
		200000000,
	)

	mps := NewMempoolPoolState("pool_abc", "minswap", confirmedState)

	// Add pending txs
	newState1 := createTestPoolState(
		"pool_abc",
		"minswap",
		"tx_1",
		101000000,
		199000000,
	)
	newState2 := createTestPoolState(
		"pool_abc",
		"minswap",
		"tx_2",
		102000000,
		198000000,
	)
	mps.AddPendingTx("tx_1", newState1)
	mps.AddPendingTx("tx_2", newState2)

	if mps.PendingCount() != 2 {
		t.Errorf("expected 2 pending txs, got %d", mps.PendingCount())
	}

	// Remove first tx
	mps.RemovePendingTx("tx_1")
	if mps.PendingCount() != 1 {
		t.Errorf(
			"expected 1 pending tx after removal, got %d",
			mps.PendingCount(),
		)
	}

	// Verify tx_1 is gone but tx_2 remains
	_, exists := mps.GetPendingTx("tx_1")
	if exists {
		t.Error("tx_1 should be removed")
	}
	_, exists = mps.GetPendingTx("tx_2")
	if !exists {
		t.Error("tx_2 should still exist")
	}

	// Remove non-existent tx (should not panic)
	mps.RemovePendingTx("tx_nonexistent")
	if mps.PendingCount() != 1 {
		t.Errorf("expected still 1 pending tx, got %d", mps.PendingCount())
	}
}

func TestMempoolPoolStateSetConfirmedState(t *testing.T) {
	confirmedState := createTestPoolState(
		"pool_abc",
		"minswap",
		"tx_confirmed",
		100000000,
		200000000,
	)

	mps := NewMempoolPoolState("pool_abc", "minswap", confirmedState)

	// Add a pending tx
	newState := createTestPoolState(
		"pool_abc",
		"minswap",
		"tx_mempool",
		101000000,
		199000000,
	)
	mps.AddPendingTx("tx_mempool", newState)

	if mps.PendingCount() != 1 {
		t.Errorf("expected 1 pending tx, got %d", mps.PendingCount())
	}

	// Set confirmed state with the same tx hash (simulates tx being confirmed)
	newConfirmed := createTestPoolState(
		"pool_abc",
		"minswap",
		"tx_mempool",
		101000000,
		199000000,
	)
	mps.SetConfirmedState(newConfirmed)

	// Pending tx should be removed
	if mps.PendingCount() != 0 {
		t.Errorf(
			"expected 0 pending txs after confirmation, got %d",
			mps.PendingCount(),
		)
	}

	// Confirmed state should be updated
	if mps.GetConfirmedState().TxHash != "tx_mempool" {
		t.Error("confirmed state should be updated")
	}
}

func TestMempoolPoolStateGetPendingTxs(t *testing.T) {
	confirmedState := createTestPoolState(
		"pool_abc",
		"minswap",
		"tx_confirmed",
		100000000,
		200000000,
	)

	mps := NewMempoolPoolState("pool_abc", "minswap", confirmedState)

	// Add multiple pending txs
	for i := 0; i < 5; i++ {
		state := createTestPoolState(
			"pool_abc",
			"minswap",
			"tx_"+string(rune('a'+i)),
			uint64(100000000+i*1000000),
			uint64(200000000-i*1000000),
		)
		mps.AddPendingTx("tx_"+string(rune('a'+i)), state)
	}

	pendingTxs := mps.GetPendingTxs()
	if len(pendingTxs) != 5 {
		t.Errorf("expected 5 pending txs, got %d", len(pendingTxs))
	}

	// Verify they are in sequence order
	for i, tx := range pendingTxs {
		if tx.Sequence != i {
			t.Errorf("expected sequence %d, got %d", i, tx.Sequence)
		}
	}
}

func TestMempoolPoolStateGetPendingTx(t *testing.T) {
	confirmedState := createTestPoolState(
		"pool_abc",
		"minswap",
		"tx_confirmed",
		100000000,
		200000000,
	)

	mps := NewMempoolPoolState("pool_abc", "minswap", confirmedState)

	state := createTestPoolState(
		"pool_abc",
		"minswap",
		"tx_test",
		101000000,
		199000000,
	)
	mps.AddPendingTx("tx_test", state)

	// Get existing tx
	effect, exists := mps.GetPendingTx("tx_test")
	if !exists {
		t.Error("expected tx to exist")
	}
	if effect.TxHash != "tx_test" {
		t.Errorf("expected txHash 'tx_test', got %s", effect.TxHash)
	}

	// Get non-existent tx
	_, exists = mps.GetPendingTx("tx_nonexistent")
	if exists {
		t.Error("expected tx to not exist")
	}
}

func TestMempoolPoolStateNoConfirmedState(t *testing.T) {
	// Test with nil confirmed state
	mps := NewMempoolPoolState("pool_abc", "minswap", nil)

	state := createTestPoolState(
		"pool_abc",
		"minswap",
		"tx_test",
		101000000,
		199000000,
	)
	effect := mps.AddPendingTx("tx_test", state)

	// Deltas should be 0 when no confirmed state
	if effect.DeltaX != 0 {
		t.Errorf(
			"expected deltaX 0 without confirmed state, got %d",
			effect.DeltaX,
		)
	}
	if effect.DeltaY != 0 {
		t.Errorf(
			"expected deltaY 0 without confirmed state, got %d",
			effect.DeltaY,
		)
	}
}

func TestMempoolPoolStateFeeChange(t *testing.T) {
	confirmedState := createTestPoolState(
		"pool_abc",
		"minswap",
		"tx_confirmed",
		100000000,
		200000000,
	)
	confirmedState.FeeNum = 997
	confirmedState.FeeDenom = 1000

	mps := NewMempoolPoolState("pool_abc", "minswap", confirmedState)

	// Add tx that changes fees
	newState := createTestPoolState(
		"pool_abc",
		"minswap",
		"tx_fee_change",
		100000000,
		200000000,
	)
	newState.FeeNum = 995
	newState.FeeDenom = 1000

	effect := mps.AddPendingTx("tx_fee_change", newState)

	if !effect.FeeChanged {
		t.Error("expected feeChanged to be true")
	}
	if effect.NewFeeNum != 995 {
		t.Errorf("expected newFeeNum 995, got %d", effect.NewFeeNum)
	}
	if effect.NewFeeDenom != 1000 {
		t.Errorf("expected newFeeDenom 1000, got %d", effect.NewFeeDenom)
	}
}

// ==================== MempoolStateManager Tests ====================

func TestNewMempoolStateManager(t *testing.T) {
	mgr := NewMempoolStateManager()

	if mgr == nil {
		t.Fatal("expected non-nil manager")
	}
	if mgr.PoolCount() != 0 {
		t.Errorf("expected 0 pools, got %d", mgr.PoolCount())
	}
	if mgr.TotalPendingTxs() != 0 {
		t.Errorf("expected 0 pending txs, got %d", mgr.TotalPendingTxs())
	}
}

func TestMempoolStateManagerGetOrCreatePoolState(t *testing.T) {
	mgr := NewMempoolStateManager()

	confirmedState := createTestPoolState(
		"pool_1",
		"minswap",
		"tx_confirmed",
		100000000,
		200000000,
	)

	// Create new pool state
	mps := mgr.GetOrCreatePoolState("pool_1", "minswap", confirmedState)
	if mps == nil {
		t.Fatal("expected non-nil pool state")
	}
	if mps.PoolId != "pool_1" {
		t.Errorf("expected poolId 'pool_1', got %s", mps.PoolId)
	}
	if mgr.PoolCount() != 1 {
		t.Errorf("expected 1 pool, got %d", mgr.PoolCount())
	}

	// Get existing pool state
	mps2 := mgr.GetOrCreatePoolState("pool_1", "minswap", nil)
	if mps2 != mps {
		t.Error("expected same pool state for existing pool")
	}
	if mgr.PoolCount() != 1 {
		t.Errorf("expected still 1 pool, got %d", mgr.PoolCount())
	}
}

func TestMempoolStateManagerGetPoolState(t *testing.T) {
	mgr := NewMempoolStateManager()

	// Get non-existent pool
	_, exists := mgr.GetPoolState("pool_nonexistent")
	if exists {
		t.Error("expected pool to not exist")
	}

	// Create pool and get it
	confirmedState := createTestPoolState(
		"pool_1",
		"minswap",
		"tx_1",
		100000000,
		200000000,
	)
	mgr.GetOrCreatePoolState("pool_1", "minswap", confirmedState)

	mps, exists := mgr.GetPoolState("pool_1")
	if !exists {
		t.Error("expected pool to exist")
	}
	if mps.PoolId != "pool_1" {
		t.Errorf("expected poolId 'pool_1', got %s", mps.PoolId)
	}
}

func TestMempoolStateManagerUpdateConfirmedState(t *testing.T) {
	mgr := NewMempoolStateManager()

	// Update confirmed state for non-existent pool (should create it)
	confirmedState := createTestPoolState(
		"pool_1",
		"minswap",
		"tx_1",
		100000000,
		200000000,
	)
	mgr.UpdateConfirmedState("pool_1", confirmedState)

	if mgr.PoolCount() != 1 {
		t.Errorf("expected 1 pool, got %d", mgr.PoolCount())
	}

	mps, _ := mgr.GetPoolState("pool_1")
	if mps.GetConfirmedState().TxHash != "tx_1" {
		t.Error("confirmed state should be set")
	}

	// Update confirmed state for existing pool
	newConfirmed := createTestPoolState(
		"pool_1",
		"minswap",
		"tx_2",
		101000000,
		199000000,
	)
	mgr.UpdateConfirmedState("pool_1", newConfirmed)

	if mps.GetConfirmedState().TxHash != "tx_2" {
		t.Error("confirmed state should be updated")
	}
}

func TestMempoolStateManagerAddPendingTx(t *testing.T) {
	mgr := NewMempoolStateManager()

	newState := createTestPoolState(
		"pool_1",
		"minswap",
		"tx_mempool",
		101000000,
		199000000,
	)
	effect := mgr.AddPendingTx("pool_1", "minswap", "tx_mempool", newState)

	if effect == nil {
		t.Fatal("expected non-nil effect")
	}
	if effect.TxHash != "tx_mempool" {
		t.Errorf("expected txHash 'tx_mempool', got %s", effect.TxHash)
	}
	if mgr.PoolCount() != 1 {
		t.Errorf("expected 1 pool, got %d", mgr.PoolCount())
	}
	if mgr.TotalPendingTxs() != 1 {
		t.Errorf("expected 1 pending tx, got %d", mgr.TotalPendingTxs())
	}
}

func TestMempoolStateManagerRemovePendingTx(t *testing.T) {
	mgr := NewMempoolStateManager()

	// Add pending txs to multiple pools
	state1 := createTestPoolState(
		"pool_1",
		"minswap",
		"tx_1",
		101000000,
		199000000,
	)
	state2 := createTestPoolState(
		"pool_2",
		"spectrum",
		"tx_1",
		50000000,
		100000000,
	)

	mgr.AddPendingTx("pool_1", "minswap", "tx_1", state1)
	mgr.AddPendingTx("pool_2", "spectrum", "tx_1", state2)

	if mgr.TotalPendingTxs() != 1 {
		t.Errorf("expected 1 unique pending tx, got %d", mgr.TotalPendingTxs())
	}

	// Remove pending tx (should remove from all pools)
	mgr.RemovePendingTx("tx_1")

	if mgr.TotalPendingTxs() != 0 {
		t.Errorf(
			"expected 0 pending txs after removal, got %d",
			mgr.TotalPendingTxs(),
		)
	}

	// Verify both pools have no pending txs
	mps1, _ := mgr.GetPoolState("pool_1")
	if mps1.PendingCount() != 0 {
		t.Errorf(
			"expected 0 pending txs in pool_1, got %d",
			mps1.PendingCount(),
		)
	}
	mps2, _ := mgr.GetPoolState("pool_2")
	if mps2.PendingCount() != 0 {
		t.Errorf(
			"expected 0 pending txs in pool_2, got %d",
			mps2.PendingCount(),
		)
	}
}

func TestMempoolStateManagerGetPoolsAffectedByTx(t *testing.T) {
	mgr := NewMempoolStateManager()

	// Add tx affecting multiple pools
	state1 := createTestPoolState(
		"pool_1",
		"minswap",
		"tx_multi",
		101000000,
		199000000,
	)
	state2 := createTestPoolState(
		"pool_2",
		"spectrum",
		"tx_multi",
		50000000,
		100000000,
	)
	state3 := createTestPoolState(
		"pool_3",
		"sundae",
		"tx_multi",
		75000000,
		150000000,
	)

	mgr.AddPendingTx("pool_1", "minswap", "tx_multi", state1)
	mgr.AddPendingTx("pool_2", "spectrum", "tx_multi", state2)
	mgr.AddPendingTx("pool_3", "sundae", "tx_multi", state3)

	affectedPools := mgr.GetPoolsAffectedByTx("tx_multi")
	if len(affectedPools) != 3 {
		t.Errorf("expected 3 affected pools, got %d", len(affectedPools))
	}

	// Get non-existent tx
	affectedPools = mgr.GetPoolsAffectedByTx("tx_nonexistent")
	if affectedPools != nil {
		t.Error("expected nil for non-existent tx")
	}
}

func TestMempoolStateManagerGetAllPendingTxs(t *testing.T) {
	mgr := NewMempoolStateManager()

	// Add pending txs to multiple pools
	state1a := createTestPoolState(
		"pool_1",
		"minswap",
		"tx_1a",
		101000000,
		199000000,
	)
	state1b := createTestPoolState(
		"pool_1",
		"minswap",
		"tx_1b",
		102000000,
		198000000,
	)
	state2a := createTestPoolState(
		"pool_2",
		"spectrum",
		"tx_2a",
		50000000,
		100000000,
	)

	mgr.AddPendingTx("pool_1", "minswap", "tx_1a", state1a)
	mgr.AddPendingTx("pool_1", "minswap", "tx_1b", state1b)
	mgr.AddPendingTx("pool_2", "spectrum", "tx_2a", state2a)

	allPending := mgr.GetAllPendingTxs()

	if len(allPending) != 2 {
		t.Errorf("expected 2 pools with pending txs, got %d", len(allPending))
	}

	pool1Effects, exists := allPending["pool_1"]
	if !exists {
		t.Error("expected pool_1 in results")
	}
	if len(pool1Effects) != 2 {
		t.Errorf("expected 2 pending txs for pool_1, got %d", len(pool1Effects))
	}

	pool2Effects, exists := allPending["pool_2"]
	if !exists {
		t.Error("expected pool_2 in results")
	}
	if len(pool2Effects) != 1 {
		t.Errorf("expected 1 pending tx for pool_2, got %d", len(pool2Effects))
	}
}

func TestMempoolStateManagerConcurrentAccess(t *testing.T) {
	mgr := NewMempoolStateManager()

	// Simulate concurrent access
	done := make(chan bool)

	// Writer goroutine
	go func() {
		for i := 0; i < 100; i++ {
			state := createTestPoolState(
				"pool_concurrent",
				"minswap",
				"tx_"+string(rune(i)),
				uint64(100000000+i),
				uint64(200000000-i),
			)
			mgr.AddPendingTx(
				"pool_concurrent",
				"minswap",
				"tx_"+string(rune(i)),
				state,
			)
		}
		done <- true
	}()

	// Reader goroutine
	go func() {
		for i := 0; i < 100; i++ {
			_ = mgr.PoolCount()
			_ = mgr.TotalPendingTxs()
			_ = mgr.GetAllPendingTxs()
		}
		done <- true
	}()

	// Wait for both goroutines
	<-done
	<-done

	// Should not panic and pool should exist
	if mgr.PoolCount() != 1 {
		t.Errorf("expected 1 pool, got %d", mgr.PoolCount())
	}
}

func TestMempoolPoolStateTxOrderAfterRemoval(t *testing.T) {
	confirmedState := createTestPoolState(
		"pool_abc",
		"minswap",
		"tx_confirmed",
		100000000,
		200000000,
	)

	mps := NewMempoolPoolState("pool_abc", "minswap", confirmedState)

	// Add txs in order
	for i := 0; i < 5; i++ {
		state := createTestPoolState(
			"pool_abc",
			"minswap",
			"tx_"+string(rune('a'+i)),
			uint64(100000000+i*1000000),
			uint64(200000000-i*1000000),
		)
		mps.AddPendingTx("tx_"+string(rune('a'+i)), state)
	}

	// Remove middle tx
	mps.RemovePendingTx("tx_c")

	// Verify remaining txs are still in order
	pendingTxs := mps.GetPendingTxs()
	if len(pendingTxs) != 4 {
		t.Errorf("expected 4 pending txs, got %d", len(pendingTxs))
	}

	// Should be in sequence order (a=0, b=1, d=3, e=4)
	expectedSeqs := []int{0, 1, 3, 4}
	for i, tx := range pendingTxs {
		if tx.Sequence != expectedSeqs[i] {
			t.Errorf(
				"at index %d: expected sequence %d, got %d",
				i,
				expectedSeqs[i],
				tx.Sequence,
			)
		}
	}
}
