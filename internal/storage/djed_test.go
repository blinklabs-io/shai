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
	"testing"

	"github.com/blinklabs-io/shai/price/djed"
	"github.com/dgraph-io/badger/v4"
	"github.com/stretchr/testify/require"
)

func TestDjedStateSaveLoad(t *testing.T) {
	db, err := badger.Open(
		badger.DefaultOptions(t.TempDir()).
			WithLoggingLevel(badger.WARNING),
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})
	store := &Storage{db: db}
	state := djed.TrackerState{
		Observations: []djed.TrackedObservation{{
			Observation: djed.Observation{
				Pair:             "ADA/USD",
				Source:           "djed",
				PriceNumerator:   17,
				PriceDenominator: 100,
				TxHash:           "tx",
			},
		}},
	}
	require.NoError(t, store.SaveDjedState("mainnet", state))
	loaded, err := store.LoadDjedState("mainnet")
	require.NoError(t, err)
	require.Equal(t, state, loaded)

	_, err = store.LoadDjedState("preview")
	require.ErrorIs(t, err, ErrDjedStateNotFound)
}
