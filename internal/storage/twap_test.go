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

package storage

import (
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/blinklabs-io/shai/price"
	"github.com/dgraph-io/badger/v4"
	"github.com/stretchr/testify/require"
)

func TestTWAPStateSurvivesRestartAndRollback(t *testing.T) {
	dbPath := t.TempDir()
	store := openTWAPTestStorage(t, dbPath)
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	engine, err := price.NewTWAPEngine(price.TWAPConfig{
		Window:          10 * time.Minute,
		MinCoverage:     time.Minute,
		MaxStaleness:    10 * time.Minute,
		MaxObservations: 10,
	})
	require.NoError(t, err)
	require.NoError(t, engine.Observe(
		100,
		now.Add(-2*time.Minute),
		big.NewRat(1, 5),
	))
	require.NoError(t, engine.Observe(
		101,
		now.Add(-time.Minute),
		big.NewRat(21, 100),
	))
	require.NoError(t, store.SaveTWAPState("ada-usd", engine.Snapshot()))
	require.NoError(t, store.db.Close())

	store = openTWAPTestStorage(t, dbPath)
	state, err := store.LoadTWAPState("ada-usd")
	require.NoError(t, err)
	restored, err := price.NewTWAPEngineFromState(state)
	require.NoError(t, err)
	require.Equal(t, 2, restored.Len())
	require.NoError(t, restored.Rollback(100))
	require.Equal(t, 1, restored.Len())
	require.NoError(t, store.SaveTWAPState("ada-usd", restored.Snapshot()))
}

func TestLoadTWAPStateReportsMissingAndCorruptState(t *testing.T) {
	store := openTWAPTestStorage(t, t.TempDir())
	_, err := store.LoadTWAPState("missing")
	require.ErrorIs(t, err, ErrTWAPStateNotFound)

	require.NoError(t, store.db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(twapStateKeyPrefix+"bad"), []byte("{"))
	}))
	_, err = store.LoadTWAPState("bad")
	require.Error(t, err)
	require.False(t, errors.Is(err, ErrTWAPStateNotFound))
}

func openTWAPTestStorage(t *testing.T, path string) *Storage {
	t.Helper()
	db, err := badger.Open(
		badger.DefaultOptions(path).WithLoggingLevel(badger.WARNING),
	)
	require.NoError(t, err)
	store := &Storage{db: db}
	t.Cleanup(func() {
		if !store.db.IsClosed() {
			require.NoError(t, store.db.Close())
		}
	})
	return store
}
