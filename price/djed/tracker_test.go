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

	applied, err := tracker.Apply(
		mustDecodeHex(t, currentMainnetDatum),
		utxo,
		now,
	)
	require.NoError(t, err)
	require.Equal(t, utxo.Slot, applied.Slot)
	require.Equal(t, utxo.BlockHash, applied.BlockHash)

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
	require.Equal(t, utxo.TxHash, current.TxHash)
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

	tracker.Rollback(20)
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

	tracker.Rollback(20)
	require.ErrorIs(t, currentError(tracker, now), ErrNoCurrentObservation)
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

func currentError(tracker *Tracker, now time.Time) error {
	_, err := tracker.Current(now)
	return err
}
