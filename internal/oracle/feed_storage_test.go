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
	"path/filepath"
	"testing"

	"github.com/blinklabs-io/shai/oraclefeed"
	"github.com/dgraph-io/badger/v4"
	"github.com/stretchr/testify/require"
)

func TestFeedStoragePersistsSpendAndRollback(t *testing.T) {
	db, err := badger.Open(
		badger.DefaultOptions(filepath.Join(t.TempDir(), "feeds")).
			WithLoggingLevel(badger.WARNING),
	)
	require.NoError(t, err)
	storage := &FeedStorage{db: db}
	t.Cleanup(func() { require.NoError(t, storage.Close()) })

	utxo := oraclefeed.UTxO{
		Address: "addr_test",
		TxHash:  "feed",
		TxIndex: 1,
		Slot:    10,
		Datum:   []byte{1, 2, 3},
	}
	require.NoError(t, storage.Save(utxo))
	ref := oraclefeed.OutputRef{TxHash: "feed", TxIndex: 1}
	require.NoError(t, storage.Spend(ref, 20))

	records, err := storage.Load()
	require.NoError(t, err)
	require.Len(t, records, 1)
	require.NotNil(t, records[0].SpentAt)
	require.Equal(t, uint64(20), *records[0].SpentAt)
	require.Equal(t, utxo.Datum, records[0].UTxO.Datum)

	require.NoError(t, storage.Rollback(20))
	records, err = storage.Load()
	require.NoError(t, err)
	require.Len(t, records, 1)
	require.Nil(t, records[0].SpentAt)

	require.NoError(t, storage.Rollback(10))
	records, err = storage.Load()
	require.NoError(t, err)
	require.Empty(t, records)
}
