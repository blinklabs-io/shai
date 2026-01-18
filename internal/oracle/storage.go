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
	"fmt"
	"strings"

	"github.com/blinklabs-io/shai/internal/config"
	"github.com/blinklabs-io/shai/internal/logging"
	"github.com/dgraph-io/badger/v4"
)

const (
	poolStateKeyPrefix = "oracle_pool_"
)

// OracleStorage handles persistence of oracle data
type OracleStorage struct {
	db *badger.DB
}

// NewOracleStorage creates a new OracleStorage instance
func NewOracleStorage() (*OracleStorage, error) {
	cfg := config.GetConfig()
	dbPath := cfg.Storage.Directory + "/oracle"

	opts := badger.DefaultOptions(dbPath).
		WithLoggingLevel(badger.WARNING)

	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to open oracle storage: %w", err)
	}

	return &OracleStorage{db: db}, nil
}

// Close closes the storage
func (s *OracleStorage) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// SavePoolState persists a pool state to storage
func (s *OracleStorage) SavePoolState(state *PoolState) error {
	key := poolStateKey(state.Network, state.Protocol, state.PoolId)

	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal pool state: %w", err)
	}

	err = s.db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(key), data)
	})
	if err != nil {
		return fmt.Errorf("failed to save pool state: %w", err)
	}

	return nil
}

// LoadPoolState loads a pool state from storage
func (s *OracleStorage) LoadPoolState(
	network, protocol, poolId string,
) (*PoolState, error) {
	key := poolStateKey(network, protocol, poolId)

	var state PoolState
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(key))
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, &state)
		})
	})
	if err != nil {
		return nil, err
	}

	return &state, nil
}

// LoadAllPoolStates loads all pool states from storage
func (s *OracleStorage) LoadAllPoolStates() ([]*PoolState, error) {
	logger := logging.GetLogger()
	var states []*PoolState

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte(poolStateKeyPrefix)
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			err := item.Value(func(val []byte) error {
				var state PoolState
				if err := json.Unmarshal(val, &state); err != nil {
					logger.Warn(
						"failed to unmarshal pool state",
						"key", string(item.Key()),
						"error", err,
					)
					return nil // Continue with other states
				}
				states = append(states, &state)
				return nil
			})
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to load pool states: %w", err)
	}

	return states, nil
}

// DeletePoolState removes a pool state from storage
func (s *OracleStorage) DeletePoolState(
	network, protocol, poolId string,
) error {
	key := poolStateKey(network, protocol, poolId)

	err := s.db.Update(func(txn *badger.Txn) error {
		return txn.Delete([]byte(key))
	})
	if err != nil {
		return fmt.Errorf("failed to delete pool state: %w", err)
	}

	return nil
}

// LoadPoolStatesByProtocol loads all pool states for a specific protocol
func (s *OracleStorage) LoadPoolStatesByProtocol(
	network, protocol string,
) ([]*PoolState, error) {
	prefix := poolStateKeyPrefix + network + ":" + protocol + ":"
	var states []*PoolState

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte(prefix)
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			err := item.Value(func(val []byte) error {
				var state PoolState
				if err := json.Unmarshal(val, &state); err != nil {
					return nil // Skip invalid entries
				}
				states = append(states, &state)
				return nil
			})
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return states, nil
}

// poolStateKey generates the storage key for a pool state
func poolStateKey(network, protocol, poolId string) string {
	return poolStateKeyPrefix + network + ":" + protocol + ":" + poolId
}

// ParsePoolStateKey extracts network, protocol, and poolId from a storage key
func ParsePoolStateKey(
	key string,
) (network, protocol, poolId string, err error) {
	if !strings.HasPrefix(key, poolStateKeyPrefix) {
		return "", "", "", fmt.Errorf("invalid pool state key prefix")
	}
	parts := strings.SplitN(
		strings.TrimPrefix(key, poolStateKeyPrefix),
		":",
		3,
	)
	if len(parts) != 3 {
		return "", "", "", fmt.Errorf("invalid pool state key format")
	}
	return parts[0], parts[1], parts[2], nil
}
