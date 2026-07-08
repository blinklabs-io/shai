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
	"path/filepath"
	"strings"

	"github.com/blinklabs-io/shai/internal/config"
	"github.com/blinklabs-io/shai/internal/logging"
	"github.com/dgraph-io/badger/v4"
)

const (
	poolStateKeyPrefix = "oracle_pool_"
	cdpStateKeyPrefix  = "oracle_cdp_"
)

// OracleStorage handles persistence of oracle data
type OracleStorage struct {
	db *badger.DB
}

// NewOracleStorage creates a new OracleStorage instance
func NewOracleStorage() (*OracleStorage, error) {
	cfg := config.GetConfig()
	dbPath := filepath.Join(cfg.Storage.Directory, "oracle")

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
	return s.db.Close()
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

// SaveCDPState persists a CDP state to storage.
func (s *OracleStorage) SaveCDPState(state *CDPState) error {
	key := cdpStateKey(state.Network, state.Protocol, state.CDPId)

	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal CDP state: %w", err)
	}

	err = s.db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(key), data)
	})
	if err != nil {
		return fmt.Errorf("failed to save CDP state: %w", err)
	}

	return nil
}

// LoadAllCDPStates loads all CDP states from storage.
func (s *OracleStorage) LoadAllCDPStates() ([]*CDPState, error) {
	logger := logging.GetLogger()
	var states []*CDPState

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte(cdpStateKeyPrefix)
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			err := item.Value(func(val []byte) error {
				var state CDPState
				if err := json.Unmarshal(val, &state); err != nil {
					logger.Warn(
						"failed to unmarshal CDP state",
						"key", string(item.Key()),
						"error", err,
					)
					return nil
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
		return nil, fmt.Errorf("failed to load CDP states: %w", err)
	}

	return states, nil
}

// LoadCDPState loads a single CDP state by network, protocol, and CDP ID.
func (s *OracleStorage) LoadCDPState(
	network,
	protocol,
	cdpId string,
) (*CDPState, error) {
	key := cdpStateKey(network, protocol, cdpId)

	var state *CDPState
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(key))
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			state = &CDPState{}
			return json.Unmarshal(val, state)
		})
	})
	if err != nil {
		return nil, fmt.Errorf("failed to load CDP state: %w", err)
	}

	return state, nil
}

// LoadPoolState loads a single pool state by network, protocol, and pool ID.
func (s *OracleStorage) LoadPoolState(
	network,
	protocol,
	poolId string,
) (*PoolState, error) {
	key := poolStateKey(network, protocol, poolId)

	var state *PoolState
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(key))
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			state = &PoolState{}
			return json.Unmarshal(val, state)
		})
	})
	if err != nil {
		return nil, fmt.Errorf("failed to load pool state: %w", err)
	}

	return state, nil
}

// LoadPoolStatesByProtocol loads all pool states for a specific protocol.
func (s *OracleStorage) LoadPoolStatesByProtocol(
	protocol string,
) ([]*PoolState, error) {
	logger := logging.GetLogger()
	poolStates := make([]*PoolState, 0)
	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte(poolStateKeyPrefix)
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			_, keyProtocol, _, err := ParsePoolStateKey(string(item.Key()))
			if err != nil {
				logger.Warn(
					"skipping malformed oracle pool key",
					"key", string(item.Key()),
					"error", err,
				)
				continue
			}
			if keyProtocol != protocol {
				continue
			}

			if err := item.Value(func(val []byte) error {
				var state PoolState
				if err := json.Unmarshal(val, &state); err != nil {
					logger.Warn(
						"skipping malformed oracle pool state payload",
						"key", string(item.Key()),
						"error", err,
					)
					return nil
				}
				poolStates = append(poolStates, &state)
				return nil
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to load pool states by protocol: %w", err)
	}
	return poolStates, nil
}

// DeletePoolState removes a pool state from storage
func (s *OracleStorage) DeletePoolState(state *PoolState) error {
	key := poolStateKey(state.Network, state.Protocol, state.PoolId)

	err := s.db.Update(func(txn *badger.Txn) error {
		return txn.Delete([]byte(key))
	})
	if err != nil {
		return fmt.Errorf("failed to delete pool state: %w", err)
	}

	return nil
}

// DeleteCDPState removes a CDP state from storage.
func (s *OracleStorage) DeleteCDPState(
	network,
	protocol,
	cdpId string,
) error {
	key := cdpStateKey(network, protocol, cdpId)

	err := s.db.Update(func(txn *badger.Txn) error {
		return txn.Delete([]byte(key))
	})
	if err != nil {
		return fmt.Errorf("failed to delete CDP state: %w", err)
	}

	return nil
}

// poolStateKey generates the storage key for a pool state
func poolStateKey(network, protocol, poolId string) string {
	return poolStateKeyPrefix + network + ":" + protocol + ":" + poolId
}

// cdpStateKey generates the storage key for a CDP state.
func cdpStateKey(network, protocol, cdpId string) string {
	return cdpStateKeyPrefix + network + ":" + protocol + ":" + cdpId
}

// ParsePoolStateKey extracts network, protocol, and poolId from a pool key.
func ParsePoolStateKey(key string) (network, protocol, poolId string, err error) {
	if !strings.HasPrefix(key, poolStateKeyPrefix) {
		return "", "", "", fmt.Errorf("invalid pool state key: %s", key)
	}
	trimmed := strings.TrimPrefix(key, poolStateKeyPrefix)
	parts := strings.SplitN(trimmed, ":", 3)
	if len(parts) != 3 {
		return "", "", "", fmt.Errorf("invalid pool state key: %s", key)
	}
	if parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return "", "", "", fmt.Errorf("invalid pool state key: %s", key)
	}
	return parts[0], parts[1], parts[2], nil
}

// ParseCDPStateKey extracts network, protocol, and cdpId from a CDP key.
func ParseCDPStateKey(key string) (network, protocol, cdpId string, err error) {
	if !strings.HasPrefix(key, cdpStateKeyPrefix) {
		return "", "", "", fmt.Errorf("invalid CDP state key: %s", key)
	}
	trimmed := strings.TrimPrefix(key, cdpStateKeyPrefix)
	parts := strings.SplitN(trimmed, ":", 3)
	if len(parts) != 3 {
		return "", "", "", fmt.Errorf("invalid CDP state key: %s", key)
	}
	if parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return "", "", "", fmt.Errorf("invalid CDP state key: %s", key)
	}
	return parts[0], parts[1], parts[2], nil
}
