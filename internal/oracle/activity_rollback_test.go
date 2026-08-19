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
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/blinklabs-io/adder/event"
	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/dgraph-io/badger/v4"
	"github.com/stretchr/testify/require"

	"github.com/blinklabs-io/shai/common"
	"github.com/blinklabs-io/shai/internal/config"
)

const testPoolAddress = IndigoCDPContractAddress

// TestOracleActivityRollbackKeepsRollbackSlotHistory covers the roll-backward
// that chain-sync always sends after an intersect, on every restart and
// reconnect. The rollback point is the new chain tip, so its block stays on the
// chain and is never redelivered: dropping the pool state and swap recorded at
// that slot would lose confirmed volume permanently, and would also leave the
// pool without the baseline reserves the next swap is inferred from.
func TestOracleActivityRollbackKeepsRollbackSlotHistory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "oracle")
	parser := testPoolParser{
		reserves: map[uint64][2]uint64{
			90:  {900, 2200},
			100: {1000, 2000},
			110: {1100, 1800},
		},
	}
	o, storage := newTestPoolOracle(t, parser, dir)
	require.NoError(t, o.handleTransaction(
		testPoolEvent(90),
		testPoolTxEvent(t, 90),
	))
	require.NoError(t, o.handleTransaction(
		testPoolEvent(100),
		testPoolTxEvent(t, 100),
	))
	require.NoError(t, storage.Close())

	// Restart: the persisted cursor is at slot 100, so chain-sync intersects
	// there and answers with a roll-backward to slot 100 before rolling
	// forward from slot 101.
	restarted, restartedStorage := newTestPoolOracle(t, parser, dir)
	require.NoError(t, restarted.loadPersistedStates())
	require.NoError(t, restarted.handleRollback(
		event.RollbackEvent{SlotNumber: 100, BlockHash: "block-100"},
	))

	persisted, err := restartedStorage.LoadActivityState()
	require.NoError(t, err)
	require.Len(t, persisted.Swaps, 1)
	require.Equal(t, uint64(100), persisted.Swaps[0].Slot)
	state, ok := restarted.GetPoolState("pool1")
	require.True(t, ok)
	require.NotNil(t, state)
	require.Equal(t, uint64(100), state.Slot)

	// Roll forward: the slot-110 swap is inferred against the retained
	// slot-100 reserves.
	require.NoError(t, restarted.handleTransaction(
		testPoolEvent(110),
		testPoolTxEvent(t, 110),
	))
	persisted, err = restartedStorage.LoadActivityState()
	require.NoError(t, err)
	require.Len(t, persisted.Swaps, 2)

	volume, ok, err := restarted.GetPoolVolume("pool1", 110)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, uint64(2), volume.SwapCount)
	require.Equal(t, uint64(200), volume.VolumeX)
	require.Equal(t, uint64(400), volume.VolumeY)
}

// TestOracleActivityRollbackDropsReorgedSwaps covers the other side of the
// rollback boundary: swaps after the rollback point are no longer on the chain
// and must be removed from memory and from durable storage.
func TestOracleActivityRollbackDropsReorgedSwaps(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "oracle")
	parser := testPoolParser{
		reserves: map[uint64][2]uint64{
			90:  {900, 2200},
			100: {1000, 2000},
			110: {1100, 1800},
		},
	}
	o, storage := newTestPoolOracle(t, parser, dir)
	for _, slot := range []uint64{90, 100, 110} {
		require.NoError(t, o.handleTransaction(
			testPoolEvent(slot),
			testPoolTxEvent(t, slot),
		))
	}
	persisted, err := storage.LoadActivityState()
	require.NoError(t, err)
	require.Len(t, persisted.Swaps, 2)

	require.NoError(t, o.handleRollback(
		event.RollbackEvent{SlotNumber: 100, BlockHash: "block-100"},
	))

	persisted, err = storage.LoadActivityState()
	require.NoError(t, err)
	require.Len(t, persisted.Swaps, 1)
	require.Equal(t, uint64(100), persisted.Swaps[0].Slot)
	require.Equal(t, uint64(100), persisted.LatestSlot)
	// The pool's only stored state was the reorged slot-110 one, so the pool
	// itself is invalidated.
	_, ok := o.GetPoolState("pool1")
	require.False(t, ok)
	_, err = storage.LoadPoolState(
		config.GetConfig().Network,
		parser.Protocol(),
		"pool1",
	)
	require.Error(t, err)
}

// TestOracleActivityPersistFailureIsRecoverableOnRedelivery covers a confirmed
// swap whose atomic pool-state-and-activity write fails. The failure must be
// reported to the caller, must leave no partial activity on disk, and must
// still be recorded once the transaction is redelivered.
func TestOracleActivityPersistFailureIsRecoverableOnRedelivery(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "oracle")
	parser := testPoolParser{
		reserves: map[uint64][2]uint64{
			90:  {900, 2200},
			100: {1000, 2000},
		},
	}
	o, storage := newTestPoolOracle(t, parser, dir)
	require.NoError(t, o.handleTransaction(
		testPoolEvent(90),
		testPoolTxEvent(t, 90),
	))

	// The swap at slot 100 fails its durable write.
	o.storage = closedOracleStorage(t)
	err := o.handleTransaction(testPoolEvent(100), testPoolTxEvent(t, 100))
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to persist pool")
	o.storage = storage

	// Nothing partial reached disk: the swap and the slot-100 pool state share
	// one Badger transaction.
	persisted, err := storage.LoadActivityState()
	require.NoError(t, err)
	require.Empty(t, persisted.Swaps)
	loaded, err := storage.LoadPoolState(
		config.GetConfig().Network,
		parser.Protocol(),
		"pool1",
	)
	require.NoError(t, err)
	require.NotNil(t, loaded)
	require.Equal(t, uint64(90), loaded.Slot)

	// A persist failure is fatal for the process: the handler error reaches the
	// pipeline error channel and the indexer exits. Recovery therefore happens
	// through a restart whose cursor is still behind the failed block, which
	// redelivers it after a roll-backward to slot 99.
	require.NoError(t, storage.Close())
	restarted, restartedStorage := newTestPoolOracle(t, parser, dir)
	require.NoError(t, restarted.loadPersistedStates())
	require.NoError(t, restarted.handleRollback(
		event.RollbackEvent{SlotNumber: 99, BlockHash: "block-99"},
	))
	require.NoError(t, restarted.handleTransaction(
		testPoolEvent(100),
		testPoolTxEvent(t, 100),
	))

	persisted, err = restartedStorage.LoadActivityState()
	require.NoError(t, err)
	require.Len(t, persisted.Swaps, 1)
	require.Equal(t, uint64(100), persisted.Swaps[0].AmountX)
	require.Equal(t, uint64(200), persisted.Swaps[0].AmountY)

	volume, ok, err := restarted.GetPoolVolume("pool1", 100)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, uint64(1), volume.SwapCount)
	require.Equal(t, uint64(100), volume.VolumeX)
	require.Equal(t, uint64(200), volume.VolumeY)
}

// TestOracleActivityPersistFailureVolumeSurvivesInNextTransition covers the
// restart where the failed block is not redelivered because the cursor already
// advanced past it. Net reserve turnover is still accounted for, because the
// next confirmed transition is inferred from the last durable pool state
// rather than from the discarded one.
func TestOracleActivityPersistFailureVolumeSurvivesInNextTransition(
	t *testing.T,
) {
	dir := filepath.Join(t.TempDir(), "oracle")
	parser := testPoolParser{
		reserves: map[uint64][2]uint64{
			90:  {900, 2200},
			100: {1000, 2000},
			110: {1100, 1800},
		},
	}
	o, storage := newTestPoolOracle(t, parser, dir)
	require.NoError(t, o.handleTransaction(
		testPoolEvent(90),
		testPoolTxEvent(t, 90),
	))

	o.storage = closedOracleStorage(t)
	require.Error(t, o.handleTransaction(
		testPoolEvent(100),
		testPoolTxEvent(t, 100),
	))
	o.storage = storage
	require.NoError(t, storage.Close())

	restarted, restartedStorage := newTestPoolOracle(t, parser, dir)
	require.NoError(t, restarted.loadPersistedStates())
	require.NoError(t, restarted.handleRollback(
		event.RollbackEvent{SlotNumber: 100, BlockHash: "block-100"},
	))
	require.NoError(t, restarted.handleTransaction(
		testPoolEvent(110),
		testPoolTxEvent(t, 110),
	))

	persisted, err := restartedStorage.LoadActivityState()
	require.NoError(t, err)
	require.Len(t, persisted.Swaps, 1)
	// 900 -> 1100 and 2200 -> 1800: the discarded slot-100 transition is
	// absorbed into the slot-110 transition instead of being dropped.
	require.Equal(t, uint64(200), persisted.Swaps[0].AmountX)
	require.Equal(t, uint64(400), persisted.Swaps[0].AmountY)

	volume, ok, err := restarted.GetPoolVolume("pool1", 110)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, uint64(1), volume.SwapCount)
	require.Equal(t, uint64(200), volume.VolumeX)
	require.Equal(t, uint64(400), volume.VolumeY)
}

// TestOracleActivityRedeliveredSwapIsNotDoubleCounted covers the opposite
// direction: a redelivered transaction must not add the same swap twice, in
// memory or on disk.
func TestOracleActivityRedeliveredSwapIsNotDoubleCounted(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "oracle")
	parser := testPoolParser{
		reserves: map[uint64][2]uint64{
			90:  {900, 2200},
			100: {1000, 2000},
		},
	}
	o, storage := newTestPoolOracle(t, parser, dir)
	require.NoError(t, o.handleTransaction(
		testPoolEvent(90),
		testPoolTxEvent(t, 90),
	))
	require.NoError(t, o.handleTransaction(
		testPoolEvent(100),
		testPoolTxEvent(t, 100),
	))
	require.NoError(t, o.handleTransaction(
		testPoolEvent(100),
		testPoolTxEvent(t, 100),
	))

	persisted, err := storage.LoadActivityState()
	require.NoError(t, err)
	require.Len(t, persisted.Swaps, 1)
	volume, ok, err := o.GetPoolVolume("pool1", 100)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, uint64(1), volume.SwapCount)
	require.Equal(t, uint64(100), volume.VolumeX)
	require.Equal(t, uint64(200), volume.VolumeY)

}

func newTestPoolOracle(
	t *testing.T,
	parser testPoolParser,
	dir string,
) (*Oracle, *OracleStorage) {
	t.Helper()
	profile := config.Profile{
		Name: "test-dex",
		Type: config.ProfileTypeOracle,
		Config: config.OracleProfileConfig{
			Protocol: parser.Protocol(),
			PoolAddresses: []config.ProfileConfigAddress{
				{Address: testPoolAddress},
			},
		},
	}
	o := New(nil, &profile, parser)
	storage := openActivityStorage(t, dir)
	t.Cleanup(func() {
		_ = storage.Close()
	})
	o.storage = storage
	return o, storage
}

// closedOracleStorage returns storage whose Badger handle is already closed, so
// every write fails the way a transient storage error would.
func closedOracleStorage(t *testing.T) *OracleStorage {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "broken")
	opts := badger.DefaultOptions(dir).WithLoggingLevel(badger.WARNING)
	db, err := badger.Open(opts)
	require.NoError(t, err)
	require.NoError(t, db.Close())
	return &OracleStorage{db: db}
}

func testPoolEvent(slot uint64) event.Event {
	return event.Event{
		Context: event.TransactionContext{
			TransactionHash: testPoolTxHash(slot),
			SlotNumber:      slot,
		},
	}
}

func testPoolTxEvent(t *testing.T, slot uint64) event.TransactionEvent {
	t.Helper()
	return event.TransactionEvent{
		BlockHash: fmt.Sprintf("block-%d", slot),
		Outputs: []ledger.TransactionOutput{
			newTestPoolOutput(t),
		},
	}
}

func testPoolTxHash(slot uint64) string {
	return fmt.Sprintf("%064d", slot)
}

func newTestPoolOutput(t *testing.T) ledger.TransactionOutput {
	t.Helper()
	address, err := lcommon.NewAddress(testPoolAddress)
	require.NoError(t, err)
	datum, err := cbor.Encode(uint64(1))
	require.NoError(t, err)
	outputCbor, err := cbor.Encode(&map[uint64]any{
		0: address,
		1: uint64(2_000_000),
		2: []any{
			uint64(1),
			cbor.Tag{Number: 24, Content: datum},
		},
	})
	require.NoError(t, err)
	output, err := ledger.NewTransactionOutputFromCbor(outputCbor)
	require.NoError(t, err)
	require.NotNil(t, output.Datum())
	return output
}

// testPoolParser returns a pool state whose reserves are keyed by slot, so a
// test can drive handleTransaction with an exact reserve transition.
type testPoolParser struct {
	reserves map[uint64][2]uint64
}

func (p testPoolParser) Protocol() string { return "test-dex" }

func (p testPoolParser) ParsePoolDatum(
	_ []byte,
	_ []byte,
	txHash string,
	txIndex uint32,
	slot uint64,
	timestamp time.Time,
) (*PoolState, error) {
	reserves, ok := p.reserves[slot]
	if !ok {
		return nil, fmt.Errorf("no test reserves for slot %d", slot)
	}
	return &PoolState{
		PoolId:   "pool1",
		Protocol: p.Protocol(),
		AssetX: common.AssetAmount{
			Class:  common.Lovelace(),
			Amount: reserves[0],
		},
		AssetY: common.AssetAmount{
			Class: common.AssetClass{
				PolicyId: []byte(strings.Repeat("\x01", 28)),
				Name:     []byte("TEST"),
			},
			Amount: reserves[1],
		},
		FeeNum:    997,
		FeeDenom:  1_000,
		Slot:      slot,
		TxHash:    txHash,
		TxIndex:   txIndex,
		Timestamp: timestamp,
	}, nil
}
