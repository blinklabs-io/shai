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
	"sort"
	"strings"
	"sync"

	"github.com/blinklabs-io/shai/internal/config"
	"github.com/blinklabs-io/shai/internal/logging"
	"github.com/dgraph-io/badger/v4"
)

const (
	poolStateKeyPrefix    = "oracle_pool_"
	cdpStateKeyPrefix     = "oracle_cdp_"
	cdpUndoStateKeyPrefix = "oracle_undo_cdp_"
)

// OracleStorage handles persistence of oracle data
type OracleStorage struct {
	db    *badger.DB
	cdpMu sync.Mutex
}

type cdpUndoRecord struct {
	Network         string      `json:"network"`
	Protocol        string      `json:"protocol"`
	TransactionHash string      `json:"transaction_hash"`
	TransactionIdx  uint32      `json:"transaction_index"`
	Slot            uint64      `json:"slot"`
	Spent           []*CDPState `json:"spent"`
	Produced        []string    `json:"produced"`
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

// ApplyCDPTransaction atomically persists a confirmed CDP transition and the
// predecessor states needed to undo it after a chain rollback. Existing CDP
// state keys and JSON payloads are unchanged.
func (s *OracleStorage) ApplyCDPTransaction(
	record cdpUndoRecord,
	produced []*CDPState,
) (bool, error) {
	if len(record.Spent) == 0 && len(produced) == 0 {
		return false, nil
	}
	if record.Network == "" || record.Protocol == "" ||
		record.TransactionHash == "" {
		return false, fmt.Errorf("incomplete CDP undo record identity")
	}

	record.Produced = make([]string, 0, len(produced))
	encodedStates := make(map[string][]byte, len(produced))
	for _, state := range produced {
		if state == nil || state.CDPId == "" {
			return false, fmt.Errorf("produced CDP state is missing an ID")
		}
		if state.Network != record.Network || state.Protocol != record.Protocol {
			return false, fmt.Errorf(
				"produced CDP %s has mismatched network or protocol",
				state.CDPId,
			)
		}
		data, err := json.Marshal(state)
		if err != nil {
			return false, fmt.Errorf("marshal produced CDP %s: %w", state.CDPId, err)
		}
		record.Produced = append(record.Produced, state.CDPId)
		encodedStates[state.CDPId] = data
	}
	undoData, err := json.Marshal(record)
	if err != nil {
		return false, fmt.Errorf("marshal CDP undo record: %w", err)
	}

	s.cdpMu.Lock()
	defer s.cdpMu.Unlock()
	applied := false
	undoKey := cdpUndoStateKey(record)
	err = s.db.Update(func(txn *badger.Txn) error {
		if _, err := txn.Get([]byte(undoKey)); err == nil {
			return nil
		} else if err != badger.ErrKeyNotFound {
			return err
		}
		for _, state := range record.Spent {
			if state == nil || state.CDPId == "" {
				return fmt.Errorf("spent CDP state is missing an ID")
			}
			if err := txn.Delete([]byte(cdpStateKey(
				record.Network,
				record.Protocol,
				state.CDPId,
			))); err != nil {
				return err
			}
		}
		for _, state := range produced {
			if err := txn.Set(
				[]byte(cdpStateKey(record.Network, record.Protocol, state.CDPId)),
				encodedStates[state.CDPId],
			); err != nil {
				return err
			}
		}
		if err := txn.Set([]byte(undoKey), undoData); err != nil {
			return err
		}
		applied = true
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("apply CDP transaction: %w", err)
	}
	return applied, nil
}

// RollbackCDPStates reverses journaled CDP transitions at or after the first
// invalid slot. States written before journaling was introduced are still
// removed when their observation slot is rolled back.
func (s *OracleStorage) RollbackCDPStates(
	network string,
	protocol string,
	firstInvalidSlot uint64,
) ([]*CDPState, error) {
	s.cdpMu.Lock()
	defer s.cdpMu.Unlock()

	states := make(map[string]*CDPState)
	var undoRecords []struct {
		key    string
		record cdpUndoRecord
	}
	err := s.db.View(func(txn *badger.Txn) error {
		statePrefix := []byte(cdpStateKeyPrefix + network + ":" + protocol + ":")
		stateOpts := badger.DefaultIteratorOptions
		stateOpts.Prefix = statePrefix
		stateIt := txn.NewIterator(stateOpts)
		defer stateIt.Close()
		for stateIt.Rewind(); stateIt.Valid(); stateIt.Next() {
			item := stateIt.Item()
			if err := item.Value(func(value []byte) error {
				var state CDPState
				if err := json.Unmarshal(value, &state); err != nil {
					return fmt.Errorf(
						"unmarshal CDP state %s: %w",
						item.KeyCopy(nil),
						err,
					)
				}
				states[state.CDPId] = &state
				return nil
			}); err != nil {
				return err
			}
		}

		undoPrefix := []byte(cdpUndoStateKeyPrefix + network + ":" + protocol + ":")
		undoOpts := badger.DefaultIteratorOptions
		undoOpts.Prefix = undoPrefix
		undoIt := txn.NewIterator(undoOpts)
		defer undoIt.Close()
		for undoIt.Rewind(); undoIt.Valid(); undoIt.Next() {
			item := undoIt.Item()
			key := string(item.KeyCopy(nil))
			if err := item.Value(func(value []byte) error {
				var record cdpUndoRecord
				if err := json.Unmarshal(value, &record); err != nil {
					return fmt.Errorf("unmarshal CDP undo record %s: %w", key, err)
				}
				if record.Slot >= firstInvalidSlot {
					undoRecords = append(undoRecords, struct {
						key    string
						record cdpUndoRecord
					}{key: key, record: record})
				}
				return nil
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("load CDP rollback state: %w", err)
	}

	sort.Slice(undoRecords, func(i, j int) bool {
		if undoRecords[i].record.Slot != undoRecords[j].record.Slot {
			return undoRecords[i].record.Slot > undoRecords[j].record.Slot
		}
		if undoRecords[i].record.TransactionIdx !=
			undoRecords[j].record.TransactionIdx {
			return undoRecords[i].record.TransactionIdx >
				undoRecords[j].record.TransactionIdx
		}
		return undoRecords[i].record.TransactionHash >
			undoRecords[j].record.TransactionHash
	})
	toDelete := make(map[string]struct{})
	toWrite := make(map[string]*CDPState)
	journaledProduced := make(map[string]struct{})
	for _, entry := range undoRecords {
		for _, cdpId := range entry.record.Produced {
			journaledProduced[cdpId] = struct{}{}
		}
	}
	for _, entry := range undoRecords {
		for _, cdpId := range entry.record.Produced {
			delete(states, cdpId)
			delete(toWrite, cdpId)
			toDelete[cdpId] = struct{}{}
		}
		for _, state := range entry.record.Spent {
			if state == nil {
				return nil, fmt.Errorf("CDP undo record %s has nil spent state", entry.key)
			}
			states[state.CDPId] = state
			toWrite[state.CDPId] = state
			delete(toDelete, state.CDPId)
		}
	}
	for cdpId := range journaledProduced {
		if _, ok := states[cdpId]; ok {
			return nil, fmt.Errorf(
				"CDP rollback left journaled output %s: invalid undo order",
				cdpId,
			)
		}
	}
	for cdpId, state := range states {
		if state.Slot >= firstInvalidSlot {
			delete(states, cdpId)
			delete(toWrite, cdpId)
			toDelete[cdpId] = struct{}{}
		}
	}

	encodedStates := make(map[string][]byte, len(toWrite))
	for cdpId, state := range toWrite {
		data, err := json.Marshal(state)
		if err != nil {
			return nil, fmt.Errorf("marshal restored CDP %s: %w", cdpId, err)
		}
		encodedStates[cdpId] = data
	}
	err = s.db.Update(func(txn *badger.Txn) error {
		for cdpId := range toDelete {
			if err := txn.Delete([]byte(cdpStateKey(network, protocol, cdpId))); err != nil {
				return err
			}
		}
		for cdpId, data := range encodedStates {
			if err := txn.Set(
				[]byte(cdpStateKey(network, protocol, cdpId)),
				data,
			); err != nil {
				return err
			}
		}
		for _, entry := range undoRecords {
			if err := txn.Delete([]byte(entry.key)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("persist CDP rollback: %w", err)
	}

	result := make([]*CDPState, 0, len(states))
	for _, state := range states {
		result = append(result, state)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].CDPId < result[j].CDPId
	})
	return result, nil
}

// poolStateKey generates the storage key for a pool state
func poolStateKey(network, protocol, poolId string) string {
	return poolStateKeyPrefix + network + ":" + protocol + ":" + poolId
}

// cdpStateKey generates the storage key for a CDP state.
func cdpStateKey(network, protocol, cdpId string) string {
	return cdpStateKeyPrefix + network + ":" + protocol + ":" + cdpId
}

func cdpUndoStateKey(record cdpUndoRecord) string {
	return fmt.Sprintf(
		"%s%s:%s:%020d:%s",
		cdpUndoStateKeyPrefix,
		record.Network,
		record.Protocol,
		record.Slot,
		record.TransactionHash,
	)
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
