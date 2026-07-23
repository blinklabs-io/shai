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
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/blinklabs-io/shai/internal/config"
	"github.com/blinklabs-io/shai/oraclefeed"
	"github.com/dgraph-io/badger/v4"
)

const feedUTxOKeyPrefix = "feed_utxo_"

type persistedFeedUTxO struct {
	UTxO    oraclefeed.UTxO `json:"utxo"`
	SpentAt *uint64         `json:"spentAt,omitempty"`
}

type FeedStorage struct {
	db *badger.DB
}

func NewFeedStorage() (*FeedStorage, error) {
	cfg := config.GetConfig()
	dbPath := filepath.Join(cfg.Storage.Directory, "oracle-feeds")
	opts := badger.DefaultOptions(dbPath).WithLoggingLevel(badger.WARNING)
	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("open feed storage: %w", err)
	}
	return &FeedStorage{db: db}, nil
}

func (s *FeedStorage) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *FeedStorage) Save(utxo oraclefeed.UTxO) error {
	record := persistedFeedUTxO{UTxO: utxo}
	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("marshal feed UTxO: %w", err)
	}
	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Set(feedUTxOKey(oraclefeed.OutputRef{
			TxHash:  utxo.TxHash,
			TxIndex: utxo.TxIndex,
		}), data)
	})
}

func (s *FeedStorage) Spend(ref oraclefeed.OutputRef, slot uint64) error {
	return s.db.Update(func(txn *badger.Txn) error {
		item, err := txn.Get(feedUTxOKey(ref))
		if errors.Is(err, badger.ErrKeyNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		var record persistedFeedUTxO
		if err := item.Value(func(value []byte) error {
			return json.Unmarshal(value, &record)
		}); err != nil {
			return err
		}
		record.SpentAt = &slot
		data, err := json.Marshal(record)
		if err != nil {
			return err
		}
		return txn.Set(feedUTxOKey(ref), data)
	})
}

func (s *FeedStorage) Rollback(slot uint64) error {
	return s.db.Update(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte(feedUTxOKeyPrefix)
		opts.PrefetchValues = true
		it := txn.NewIterator(opts)
		defer it.Close()
		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			key := item.KeyCopy(nil)
			var record persistedFeedUTxO
			if err := item.Value(func(value []byte) error {
				return json.Unmarshal(value, &record)
			}); err != nil {
				return err
			}
			if record.UTxO.Slot >= slot {
				if err := txn.Delete(key); err != nil {
					return err
				}
				continue
			}
			if record.SpentAt == nil || *record.SpentAt < slot {
				continue
			}
			record.SpentAt = nil
			data, err := json.Marshal(record)
			if err != nil {
				return err
			}
			if err := txn.Set(key, data); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *FeedStorage) Load() ([]persistedFeedUTxO, error) {
	var records []persistedFeedUTxO
	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte(feedUTxOKeyPrefix)
		it := txn.NewIterator(opts)
		defer it.Close()
		for it.Rewind(); it.Valid(); it.Next() {
			var record persistedFeedUTxO
			if err := it.Item().Value(func(value []byte) error {
				return json.Unmarshal(value, &record)
			}); err != nil {
				return err
			}
			records = append(records, record)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("load feed UTxOs: %w", err)
	}
	return records, nil
}

func feedUTxOKey(ref oraclefeed.OutputRef) []byte {
	return []byte(fmt.Sprintf("%s%s.%d", feedUTxOKeyPrefix, ref.TxHash, ref.TxIndex))
}
