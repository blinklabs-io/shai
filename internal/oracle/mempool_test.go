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
	"math"
	"testing"

	"github.com/blinklabs-io/shai/common"
)

func TestMempoolPoolState_AddPendingTx(t *testing.T) {
	confirmed := &PoolState{
		PoolId:   "pool1",
		Protocol: "test",
		AssetX:   common.AssetAmount{Amount: 1000},
		AssetY:   common.AssetAmount{Amount: 2000},
	}
	mps := NewMempoolPoolState("pool1", "test", confirmed)
	newState := &PoolState{
		PoolId:   "pool1",
		Protocol: "test",
		TxHash:   "tx1",
		AssetX:   common.AssetAmount{Amount: 1100},
		AssetY:   common.AssetAmount{Amount: 1900},
	}
	effect := mps.AddPendingTx("tx1", newState)
	if effect == nil {
		t.Fatal("expected non-nil effect")
	}
	if effect.DeltaX != 100 {
		t.Errorf("expected deltaX=100, got %d", effect.DeltaX)
	}
	if effect.DeltaY != -100 {
		t.Errorf("expected deltaY=-100, got %d", effect.DeltaY)
	}
	if mps.PendingCount() != 1 {
		t.Errorf("expected 1 pending, got %d", mps.PendingCount())
	}
}

func TestMempoolPoolState_DuplicateTx(t *testing.T) {
	mps := NewMempoolPoolState("pool1", "test", &PoolState{})
	state := &PoolState{TxHash: "tx1"}
	effect1 := mps.AddPendingTx("tx1", state)
	effect2 := mps.AddPendingTx("tx1", state)
	if effect1 == nil || effect2 == nil {
		t.Fatal("expected non-nil effects")
	}
	if effect1 == effect2 {
		t.Error("expected duplicate tx result to be returned as a copy")
	}
	if effect1.TxHash != effect2.TxHash ||
		effect1.Timestamp != effect2.Timestamp ||
		effect1.DeltaX != effect2.DeltaX ||
		effect1.DeltaY != effect2.DeltaY {
		t.Error("expected duplicate tx to return the existing effect values")
	}
	if mps.PendingCount() != 1 {
		t.Errorf("expected 1 pending, got %d", mps.PendingCount())
	}
}

func TestMempoolPoolState_AddPendingTxNilState(t *testing.T) {
	mps := NewMempoolPoolState("pool1", "test", &PoolState{})
	if effect := mps.AddPendingTx("tx1", nil); effect != nil {
		t.Fatalf("expected nil effect for nil state, got %#v", effect)
	}
	if mps.PendingCount() != 0 {
		t.Fatalf("expected no pending tx for nil state, got %d", mps.PendingCount())
	}
}

func TestMempoolStateManager_AddPendingTxNilStateDoesNotCreatePool(
	t *testing.T,
) {
	mgr := NewMempoolStateManager()
	if effect := mgr.AddPendingTx("pool1", "test", "tx1", nil); effect != nil {
		t.Fatalf("expected nil effect for nil state, got %#v", effect)
	}
	if mgr.PoolCount() != 0 {
		t.Fatalf("expected no tracked pool for nil state, got %d", mgr.PoolCount())
	}
}

func TestMempoolPoolState_RemovePendingTx(t *testing.T) {
	mps := NewMempoolPoolState("pool1", "test", &PoolState{})
	mps.AddPendingTx("tx1", &PoolState{TxHash: "tx1"})
	mps.AddPendingTx("tx2", &PoolState{TxHash: "tx2"})
	mps.RemovePendingTx("tx1")
	if mps.PendingCount() != 1 {
		t.Errorf("expected 1 pending after remove, got %d", mps.PendingCount())
	}
	mps.RemovePendingTx("nonexistent")
}

func TestMempoolPoolState_SetConfirmedState(t *testing.T) {
	confirmed := &PoolState{TxHash: "confirmed1"}
	mps := NewMempoolPoolState("pool1", "test", confirmed)
	mps.AddPendingTx("tx1", &PoolState{TxHash: "tx1"})
	newConfirmed := &PoolState{TxHash: "tx1"}
	mps.SetConfirmedState(newConfirmed)
	if mps.PendingCount() != 0 {
		t.Errorf("expected 0 pending after confirm, got %d", mps.PendingCount())
	}
	confirmedState := mps.GetConfirmedState()
	if confirmedState == nil {
		t.Fatal("expected confirmed state")
	}
	if confirmedState == newConfirmed {
		t.Error("expected confirmed state to be returned as a copy")
	}
	if confirmedState.TxHash != newConfirmed.TxHash {
		t.Error("confirmed state not updated")
	}
}

func TestMempoolPoolState_GetConfirmedStateReturnsCopy(t *testing.T) {
	confirmed := &PoolState{
		TxHash: "confirmed1",
		AssetX: common.AssetAmount{
			Class:  common.AssetClass{PolicyId: []byte{0x01}},
			Amount: 1000,
		},
		AssetY: common.AssetAmount{
			Class:  common.AssetClass{Name: []byte{0x02}},
			Amount: 2000,
		},
	}
	mps := NewMempoolPoolState("pool1", "test", confirmed)

	confirmed.AssetX.Amount = 9999
	confirmed.AssetX.Class.PolicyId[0] = 0xff

	got := mps.GetConfirmedState()
	if got == nil {
		t.Fatal("expected confirmed state")
	}
	if got.AssetX.Amount != 1000 {
		t.Fatalf("expected stored amount 1000, got %d", got.AssetX.Amount)
	}
	if got.AssetX.Class.PolicyId[0] != 0x01 {
		t.Fatalf(
			"expected stored policy id byte 0x01, got 0x%02x",
			got.AssetX.Class.PolicyId[0],
		)
	}

	got.AssetX.Amount = 42
	got.AssetX.Class.PolicyId[0] = 0xee
	gotAgain := mps.GetConfirmedState()
	if gotAgain.AssetX.Amount != 1000 {
		t.Fatalf(
			"expected returned mutation not to change stored amount, got %d",
			gotAgain.AssetX.Amount,
		)
	}
	if gotAgain.AssetX.Class.PolicyId[0] != 0x01 {
		t.Fatalf(
			"expected returned mutation not to change stored policy id, got 0x%02x",
			gotAgain.AssetX.Class.PolicyId[0],
		)
	}
}

func TestMempoolPoolState_SetConfirmedStateRebasesPendingDeltas(t *testing.T) {
	confirmed := &PoolState{
		TxHash: "confirmed0",
		AssetX: common.AssetAmount{Amount: 1000},
		AssetY: common.AssetAmount{Amount: 2000},
	}
	mps := NewMempoolPoolState("pool1", "test", confirmed)

	// A pending tx whose resulting pool reserves would be X=1100, Y=1900.
	pending := &PoolState{
		TxHash: "txA",
		AssetX: common.AssetAmount{Amount: 1100},
		AssetY: common.AssetAmount{Amount: 1900},
	}
	effect := mps.AddPendingTx("txA", pending)
	if effect == nil {
		t.Fatal("expected pending effect")
	}
	if effect.DeltaX != 100 || effect.DeltaY != -100 {
		t.Fatalf(
			"baseline deltas wrong: got DeltaX=%d DeltaY=%d",
			effect.DeltaX,
			effect.DeltaY,
		)
	}
	priceBefore := effect.ResultingPriceXY

	// A *different* transaction confirms, moving the pool to X=1050, Y=1950.
	newConfirmed := &PoolState{
		TxHash: "confirmedB",
		AssetX: common.AssetAmount{Amount: 1050},
		AssetY: common.AssetAmount{Amount: 1950},
	}
	mps.SetConfirmedState(newConfirmed)

	// txA is not the confirmed tx, so it must remain pending.
	if mps.PendingCount() != 1 {
		t.Fatalf("expected txA to remain pending, got %d", mps.PendingCount())
	}

	got := mps.GetPendingTxs()[0]
	// Deltas must now be relative to the *new* confirmed state:
	// 1100-1050 = 50, 1900-1950 = -50.
	if got.DeltaX != 50 {
		t.Errorf("expected rebased DeltaX=50, got %d", got.DeltaX)
	}
	if got.DeltaY != -50 {
		t.Errorf("expected rebased DeltaY=-50, got %d", got.DeltaY)
	}
	// The tx's resulting reserves are fixed, so its absolute resulting price
	// must be unchanged by a confirmed-state update.
	if got.ResultingPriceXY != priceBefore {
		t.Errorf(
			"expected ResultingPriceXY unchanged (%v), got %v",
			priceBefore,
			got.ResultingPriceXY,
		)
	}
	// Sanity: new confirmed reserves + rebased delta == original resulting
	// reserves (X=1100).
	if int64(newConfirmed.AssetX.Amount)+got.DeltaX != 1100 {
		t.Errorf("rebased DeltaX inconsistent with resulting reserves")
	}
}

func TestMempoolPoolState_AddPendingTxNoOverflowNilConfirmed(t *testing.T) {
	// With no confirmed state, the delta is the absolute reserve. A reserve
	// above math.MaxInt64 must not sign-flip via a uint64->int64 cast; it
	// saturates to MaxInt64 instead.
	mps := NewMempoolPoolState("pool1", "test", nil)
	newState := &PoolState{
		TxHash: "tx1",
		AssetX: common.AssetAmount{Amount: math.MaxUint64},
		AssetY: common.AssetAmount{Amount: uint64(math.MaxInt64) + 1},
	}
	effect := mps.AddPendingTx("tx1", newState)
	if effect == nil {
		t.Fatal("expected non-nil effect")
	}
	if effect.DeltaX != math.MaxInt64 {
		t.Errorf("expected DeltaX saturated to MaxInt64, got %d", effect.DeltaX)
	}
	if effect.DeltaY != math.MaxInt64 {
		t.Errorf("expected DeltaY saturated to MaxInt64, got %d", effect.DeltaY)
	}
}

func TestMempoolPoolState_AddPendingTxNoOverflowLargeDelta(t *testing.T) {
	confirmed := &PoolState{
		AssetX: common.AssetAmount{Amount: 0},
		AssetY: common.AssetAmount{Amount: math.MaxUint64},
	}
	mps := NewMempoolPoolState("pool1", "test", confirmed)
	newState := &PoolState{
		TxHash: "tx1",
		// X grows from 0 to MaxUint64: a positive delta beyond int64 range.
		AssetX: common.AssetAmount{Amount: math.MaxUint64},
		// Y drains from MaxUint64 to 0: a negative delta beyond int64 range.
		AssetY: common.AssetAmount{Amount: 0},
	}
	effect := mps.AddPendingTx("tx1", newState)
	if effect == nil {
		t.Fatal("expected non-nil effect")
	}
	if effect.DeltaX != math.MaxInt64 {
		t.Errorf(
			"expected DeltaX saturated to MaxInt64 (positive), got %d",
			effect.DeltaX,
		)
	}
	if effect.DeltaY != math.MinInt64 {
		t.Errorf(
			"expected DeltaY saturated to MinInt64 (negative), got %d",
			effect.DeltaY,
		)
	}
}

func TestMempoolPoolState_SetConfirmedStateNoOverflowOnLargeShift(
	t *testing.T,
) {
	confirmed := &PoolState{
		TxHash: "c0",
		AssetX: common.AssetAmount{Amount: math.MaxUint64},
		AssetY: common.AssetAmount{Amount: 1000},
	}
	mps := NewMempoolPoolState("pool1", "test", confirmed)
	// A pending tx that leaves X unchanged (delta 0) relative to confirmed.
	mps.AddPendingTx("txA", &PoolState{
		TxHash: "txA",
		AssetX: common.AssetAmount{Amount: math.MaxUint64},
		AssetY: common.AssetAmount{Amount: 1000},
	})

	// A different tx confirms, draining X from MaxUint64 down to 0. The shift
	// (old - new) is a large positive value beyond int64 range. The rebased
	// delta must move positive, never sign-flip negative.
	mps.SetConfirmedState(&PoolState{
		TxHash: "cB",
		AssetX: common.AssetAmount{Amount: 0},
		AssetY: common.AssetAmount{Amount: 1000},
	})

	got := mps.GetPendingTxs()
	if len(got) != 1 {
		t.Fatalf("expected 1 pending tx, got %d", len(got))
	}
	if got[0].DeltaX <= 0 {
		t.Errorf(
			"expected positive rebased DeltaX after large shift, got %d",
			got[0].DeltaX,
		)
	}
}

func TestMempoolTxEffect_PriceImpact(t *testing.T) {
	effect := &MempoolTxEffect{ResultingPriceXY: 1.1}
	impact := effect.PriceImpact(1.0)
	if impact < 9.9 || impact > 10.1 {
		t.Errorf("expected ~10%% impact, got %.2f%%", impact)
	}
	if impact = effect.PriceImpact(0); impact != 0 {
		t.Errorf("expected 0 impact for zero price, got %.2f", impact)
	}
}

func TestMempoolPoolState_GetPendingTxsReturnsCopies(t *testing.T) {
	mps := NewMempoolPoolState("pool1", "test", &PoolState{})
	effect := mps.AddPendingTx("tx1", &PoolState{TxHash: "tx1"})
	if effect == nil {
		t.Fatal("expected effect")
	}
	effect.DeltaX = 12345

	txs := mps.GetPendingTxs()
	if len(txs) != 1 {
		t.Fatalf("expected 1 pending tx, got %d", len(txs))
	}
	if txs[0].DeltaX == 12345 {
		t.Fatal("AddPendingTx returned mutable internal effect")
	}

	txs[0].DeltaX = 999
	txsAgain := mps.GetPendingTxs()
	if len(txsAgain) != 1 {
		t.Fatalf("expected 1 pending tx, got %d", len(txsAgain))
	}
	if txsAgain[0].DeltaX == 999 {
		t.Fatal("GetPendingTxs returned mutable internal effect")
	}
}

func TestMempoolStateManager_Basic(t *testing.T) {
	mgr := NewMempoolStateManager()
	confirmed := &PoolState{
		PoolId:   "pool1",
		Protocol: "test",
		AssetX:   common.AssetAmount{Amount: 1000},
		AssetY:   common.AssetAmount{Amount: 2000},
	}
	ps := mgr.GetOrCreatePoolState("pool1", "test", confirmed)
	if ps == nil {
		t.Fatal("expected non-nil pool state")
	}
	if mgr.PoolCount() != 1 {
		t.Errorf("expected 1 pool, got %d", mgr.PoolCount())
	}
	newState := &PoolState{
		PoolId:   "pool1",
		Protocol: "test",
		TxHash:   "tx1",
		AssetX:   common.AssetAmount{Amount: 1100},
		AssetY:   common.AssetAmount{Amount: 1900},
	}
	mgr.AddPendingTx("pool1", "test", "tx1", newState)
	if mgr.TotalPendingTxs() != 1 {
		t.Errorf("expected 1 pending tx, got %d", mgr.TotalPendingTxs())
	}
	affected := mgr.GetPoolsAffectedByTx("tx1")
	if len(affected) != 1 || affected[0] != "pool1" {
		t.Errorf("expected [pool1], got %v", affected)
	}
	mgr.RemovePendingTx("tx1")
	if mgr.TotalPendingTxs() != 0 {
		t.Errorf("expected 0 pending txs, got %d", mgr.TotalPendingTxs())
	}
}

func TestMempoolStateManager_GetPendingTxOrder(t *testing.T) {
	mgr := NewMempoolStateManager()
	_ = mgr.GetOrCreatePoolState("pool1", "test", &PoolState{})
	mgr.AddPendingTx("pool1", "test", "tx-a", &PoolState{TxHash: "tx-a"})
	mgr.AddPendingTx("pool1", "test", "tx-b", &PoolState{TxHash: "tx-b"})
	mgr.AddPendingTx("pool1", "test", "tx-c", &PoolState{TxHash: "tx-c"})
	ps, ok := mgr.GetPoolState("pool1")
	if !ok || ps == nil {
		t.Fatal("expected pool1 to be tracked")
	}
	txs := ps.GetPendingTxs()
	if len(txs) != 3 {
		t.Fatalf("expected 3 pending txs, got %d", len(txs))
	}
	if txs[0].TxHash != "tx-a" || txs[1].TxHash != "tx-b" ||
		txs[2].TxHash != "tx-c" {
		t.Errorf(
			"expected order [tx-a, tx-b, tx-c], got [%s, %s, %s]",
			txs[0].TxHash,
			txs[1].TxHash,
			txs[2].TxHash,
		)
	}
}
