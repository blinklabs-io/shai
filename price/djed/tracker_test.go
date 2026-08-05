// Copyright 2026 Blink Labs Software
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package djed

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTrackerTracksCurrentUnspentObservation(t *testing.T) {
	tracker := NewTracker()
	now := time.Unix(1_784_842_625, 0).UTC()
	utxo := currentMainnetUTxO(t)
	utxo.Slot = 193_104_715
	utxo.BlockHash = "current-block"
	utxo.TransactionIndex = 4

	applied, err := tracker.Apply(
		mustDecodeHex(t, currentMainnetDatum),
		utxo,
		now,
	)
	require.NoError(t, err)
	require.Equal(t, utxo.Slot, applied.Slot)
	require.Equal(t, utxo.BlockHash, applied.BlockHash)
	require.Equal(t, utxo.TransactionIndex, applied.TransactionIndex)

	current, err := tracker.Current(now)
	require.NoError(t, err)
	require.Equal(t, applied, current)

	tracker.ConsumeAt(
		OutputRef{TxHash: utxo.TxHash, TxIndex: utxo.TxIndex},
		utxo.Slot+1,
	)
	_, err = tracker.Current(now)
	require.ErrorIs(t, err, ErrNoCurrentObservation)
}

func TestTrackerRejectsExpiredCurrentObservation(t *testing.T) {
	tracker := NewTracker()
	utxo := currentMainnetUTxO(t)
	utxo.Slot = 10
	validAt := time.Unix(1_784_842_625, 0).UTC()
	_, err := tracker.Apply(
		mustDecodeHex(t, currentMainnetDatum),
		utxo,
		validAt,
	)
	require.NoError(t, err)

	current, err := tracker.Current(
		time.UnixMilli(1_784_843_516_001).UTC(),
	)
	require.ErrorIs(t, err, ErrExpired)
	require.Equal(t, Observation{}, current)
}

func TestTrackerDuplicateApplyPreservesSpend(t *testing.T) {
	tracker := NewTracker()
	now := time.Unix(1_784_842_625, 0).UTC()
	utxo := currentMainnetUTxO(t)
	utxo.Slot = 10
	data := mustDecodeHex(t, currentMainnetDatum)
	_, err := tracker.Apply(data, utxo, now)
	require.NoError(t, err)

	ref := OutputRef{TxHash: utxo.TxHash, TxIndex: utxo.TxIndex}
	tracker.ConsumeAt(ref, 20)
	_, err = tracker.Apply(data, utxo, now)
	require.NoError(t, err)
	require.ErrorIs(t, currentError(tracker, now), ErrNoCurrentObservation)
}

func TestTrackerRejectsConflictingDuplicate(t *testing.T) {
	tracker := NewTracker()
	now := time.Unix(1_784_842_625, 0).UTC()
	utxo := currentMainnetUTxO(t)
	data := mustDecodeHex(t, currentMainnetDatum)
	_, err := tracker.Apply(data, utxo, now)
	require.NoError(t, err)

	utxo.BlockHash = "conflicting-block"
	_, err = tracker.Apply(data, utxo, now)
	require.ErrorIs(t, err, ErrConflictingObservation)
}

func TestTrackerSelectsNewestCurrentlyValidObservation(t *testing.T) {
	now := time.Unix(1_784_842_625, 0).UTC()
	valid := Observation{
		TxHash:              "valid",
		Slot:                10,
		ValidFrom:           now.Add(-time.Minute),
		ValidFromInclusive:  true,
		ValidUntil:          now.Add(time.Minute),
		ValidUntilInclusive: true,
	}
	future := valid
	future.TxHash = "future"
	future.Slot = 20
	future.ValidFrom = now.Add(time.Second)
	tracker := NewTracker()
	tracker.observations[OutputRef{TxHash: valid.TxHash}] =
		trackedObservation{observation: valid}
	tracker.observations[OutputRef{TxHash: future.TxHash}] =
		trackedObservation{observation: future}

	current, err := tracker.Current(now)
	require.NoError(t, err)
	require.Equal(t, valid, current)
}

func TestTrackerUsesBlockTransactionOrder(t *testing.T) {
	tracker := NewTracker()
	now := time.Unix(1_784_842_625, 0).UTC()
	data := mustDecodeHex(t, currentMainnetDatum)
	first := currentMainnetUTxO(t)
	first.TxHash = "ffffffff"
	first.Slot = 10
	first.TransactionIndex = 1
	_, err := tracker.Apply(data, first, now)
	require.NoError(t, err)

	second := currentMainnetUTxO(t)
	second.TxHash = "00000000"
	second.Slot = 10
	second.TransactionIndex = 2
	_, err = tracker.Apply(data, second, now)
	require.NoError(t, err)

	current, err := tracker.Current(now)
	require.NoError(t, err)
	require.Equal(t, second.TxHash, current.TxHash)
	require.Equal(t, second.TransactionIndex, current.TransactionIndex)
}

func TestTrackerRollbackRestoresSpentObservation(t *testing.T) {
	tracker := NewTracker()
	now := time.Unix(1_784_842_625, 0).UTC()
	utxo := currentMainnetUTxO(t)
	utxo.Slot = 10
	applied, err := tracker.Apply(
		mustDecodeHex(t, currentMainnetDatum),
		utxo,
		now,
	)
	require.NoError(t, err)

	ref := OutputRef{TxHash: utxo.TxHash, TxIndex: utxo.TxIndex}
	tracker.ConsumeAt(ref, 20)
	require.ErrorIs(t, currentError(tracker, now), ErrNoCurrentObservation)

	tracker.Rollback(19)
	current, err := tracker.Current(now)
	require.NoError(t, err)
	require.Equal(t, applied, current)
}

func TestTrackerRollbackRemovesProducedObservation(t *testing.T) {
	tracker := NewTracker()
	now := time.Unix(1_784_842_625, 0).UTC()
	utxo := currentMainnetUTxO(t)
	utxo.Slot = 20
	_, err := tracker.Apply(
		mustDecodeHex(t, currentMainnetDatum),
		utxo,
		now,
	)
	require.NoError(t, err)

	tracker.Rollback(19)
	require.ErrorIs(t, currentError(tracker, now), ErrNoCurrentObservation)
}

func TestTrackerRollbackRetainsPointState(t *testing.T) {
	tracker := NewTracker()
	now := time.Unix(1_784_842_625, 0).UTC()
	utxo := currentMainnetUTxO(t)
	utxo.Slot = 20
	applied, err := tracker.Apply(
		mustDecodeHex(t, currentMainnetDatum),
		utxo,
		now,
	)
	require.NoError(t, err)

	tracker.Rollback(20)
	current, err := tracker.Current(now)
	require.NoError(t, err)
	require.Equal(t, applied, current)

	tracker.ConsumeAt(
		OutputRef{TxHash: utxo.TxHash, TxIndex: utxo.TxIndex},
		20,
	)
	tracker.Rollback(20)
	require.ErrorIs(t, currentError(tracker, now), ErrNoCurrentObservation)
}

func TestTrackerPrunesOnlyImmutableSpentHistory(t *testing.T) {
	tracker := NewTracker()
	now := time.Unix(1_784_842_625, 0).UTC()
	utxo := currentMainnetUTxO(t)
	utxo.Slot = 10
	_, err := tracker.Apply(
		mustDecodeHex(t, currentMainnetDatum),
		utxo,
		now,
	)
	require.NoError(t, err)
	ref := OutputRef{TxHash: utxo.TxHash, TxIndex: utxo.TxIndex}
	require.True(t, tracker.Contains(ref))
	tracker.ConsumeAt(ref, 20)

	require.Equal(t, 0, tracker.Prune(20))
	require.Len(t, tracker.observations, 1)
	require.Equal(t, 1, tracker.Prune(21))
	require.Empty(t, tracker.observations)
	require.False(t, tracker.Contains(ref))
	require.Equal(t, 0, tracker.Prune(21))
}

func TestTrackerRejectsUnauthenticatedOutput(t *testing.T) {
	tracker := NewTracker()
	utxo := currentMainnetUTxO(t)
	utxo.Assets = nil
	_, err := tracker.Apply(
		mustDecodeHex(t, currentMainnetDatum),
		utxo,
		time.Unix(1_784_842_625, 0).UTC(),
	)
	require.ErrorIs(t, err, ErrMissingNFT)
	require.ErrorIs(
		t,
		currentError(tracker, time.Now()),
		ErrNoCurrentObservation,
	)
}

func TestTrackerSnapshotRoundTripRestoresSpentOutput(t *testing.T) {
	now := time.Unix(1_784_842_625, 0).UTC()
	tracker := NewTracker()
	first := currentMainnetUTxO(t)
	first.Slot = 100
	_, err := tracker.Apply(
		mustDecodeHex(t, currentMainnetDatum),
		first,
		now,
	)
	require.NoError(t, err)
	tracker.ConsumeAt(OutputRef{
		TxHash:  first.TxHash,
		TxIndex: first.TxIndex,
	}, 101)

	restored, err := NewTrackerFromState(tracker.Snapshot())
	require.NoError(t, err)
	_, err = restored.Current(now)
	require.ErrorIs(t, err, ErrNoCurrentObservation)
	restored.Rollback(100)
	observation, err := restored.Current(now)
	require.NoError(t, err)
	require.Equal(t, first.TxHash, observation.TxHash)
}

func TestTrackerRestoreRejectsInvalidState(t *testing.T) {
	_, err := NewTrackerFromState(TrackerState{
		Observations: []TrackedObservation{{
			Observation: Observation{Pair: "ADA/EUR"},
		}},
	})
	require.ErrorIs(t, err, ErrInvalidTrackerState)
}

func currentError(tracker *Tracker, now time.Time) error {
	_, err := tracker.Current(now)
	return err
}
