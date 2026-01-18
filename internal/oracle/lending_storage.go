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
	lendingStateKeyPrefix = "lending_state_"
)

// LendingStorage handles persistence of lending oracle data
type LendingStorage struct {
	db *badger.DB
}

// NewLendingStorage creates a new LendingStorage instance
func NewLendingStorage() (*LendingStorage, error) {
	cfg := config.GetConfig()
	dbPath := cfg.Storage.Directory + "/lending"

	opts := badger.DefaultOptions(dbPath).
		WithLoggingLevel(badger.WARNING)

	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to open lending storage: %w", err)
	}

	return &LendingStorage{db: db}, nil
}

// Close closes the storage
func (s *LendingStorage) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// SaveLendingState persists a lending state to storage
func (s *LendingStorage) SaveLendingState(state *LendingState) error {
	key := lendingStateKey(state.Network, state.Protocol, state.StateId)

	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal lending state: %w", err)
	}

	err = s.db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(key), data)
	})
	if err != nil {
		return fmt.Errorf("failed to save lending state: %w", err)
	}

	return nil
}

// LoadLendingState loads a lending state from storage
func (s *LendingStorage) LoadLendingState(
	network, protocol, stateId string,
) (*LendingState, error) {
	key := lendingStateKey(network, protocol, stateId)

	var state LendingState
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

// LoadAllLendingStates loads all lending states from storage
func (s *LendingStorage) LoadAllLendingStates() ([]*LendingState, error) {
	logger := logging.GetLogger()
	var states []*LendingState

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte(lendingStateKeyPrefix)
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			err := item.Value(func(val []byte) error {
				var state LendingState
				if err := json.Unmarshal(val, &state); err != nil {
					logger.Warn(
						"failed to unmarshal lending state",
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
		return nil, fmt.Errorf("failed to load lending states: %w", err)
	}

	return states, nil
}

// DeleteLendingState removes a lending state from storage
func (s *LendingStorage) DeleteLendingState(
	network, protocol, stateId string,
) error {
	key := lendingStateKey(network, protocol, stateId)

	err := s.db.Update(func(txn *badger.Txn) error {
		return txn.Delete([]byte(key))
	})
	if err != nil {
		return fmt.Errorf("failed to delete lending state: %w", err)
	}

	return nil
}

// LoadLendingStatesByProtocol loads all states for a specific protocol
func (s *LendingStorage) LoadLendingStatesByProtocol(
	network, protocol string,
) ([]*LendingState, error) {
	prefix := lendingStateKeyPrefix + network + ":" + protocol + ":"
	var states []*LendingState

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte(prefix)
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			err := item.Value(func(val []byte) error {
				var state LendingState
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

// LoadLendingStatesByType loads all states of a specific type
func (s *LendingStorage) LoadLendingStatesByType(
	stateType LendingStateType,
) ([]*LendingState, error) {
	var states []*LendingState

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte(lendingStateKeyPrefix)
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			err := item.Value(func(val []byte) error {
				var state LendingState
				if err := json.Unmarshal(val, &state); err != nil {
					return nil // Skip invalid entries
				}
				if state.StateType == stateType {
					states = append(states, &state)
				}
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

// lendingStateKey generates the storage key for a lending state
func lendingStateKey(network, protocol, stateId string) string {
	return lendingStateKeyPrefix + network + ":" + protocol + ":" + stateId
}

// ParseLendingStateKey extracts network, protocol, and stateId from a key
func ParseLendingStateKey(
	key string,
) (network, protocol, stateId string, err error) {
	if !strings.HasPrefix(key, lendingStateKeyPrefix) {
		return "", "", "", fmt.Errorf("invalid lending state key prefix")
	}
	parts := strings.SplitN(
		strings.TrimPrefix(key, lendingStateKeyPrefix),
		":",
		3,
	)
	if len(parts) != 3 {
		return "", "", "", fmt.Errorf("invalid lending state key format")
	}
	return parts[0], parts[1], parts[2], nil
}
