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
	poolStateKeyPrefix           = "oracle_pool_"
	cdpStateKeyPrefix            = "oracle_cdp_"
	orderStateKeyPrefix          = "oracle_order_"
	orderUndoKeyPrefix           = "oracle_undo_order_"
	orderRollbackDeleteBatchSize = 100
	// The Cardano mainnet stability window is 3k/f = 129,600 slots.
	orderRollbackRetentionSlots uint64 = 129_600
)

// OracleStorage handles persistence of oracle data
type OracleStorage struct {
	db      *badger.DB
	orderMu sync.Mutex
}

type orderUndoRecord struct {
	Network         string        `json:"network"`
	Protocol        string        `json:"protocol"`
	TransactionHash string        `json:"transaction_hash"`
	TransactionIdx  uint32        `json:"transaction_index"`
	Slot            uint64        `json:"slot"`
	Spent           []*OrderState `json:"spent"`
	Produced        []string      `json:"produced"`
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

// LoadAllOrderStates loads all order states from storage.
func (s *OracleStorage) LoadAllOrderStates() ([]*OrderState, error) {
	logger := logging.GetLogger()
	var states []*OrderState

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte(orderStateKeyPrefix)
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			err := item.Value(func(val []byte) error {
				var state OrderState
				if err := json.Unmarshal(val, &state); err != nil {
					logger.Warn(
						"failed to unmarshal order state",
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
		return nil, fmt.Errorf("failed to load order states: %w", err)
	}

	return states, nil
}

// LoadOrderState loads a single order state by network, protocol, and order ID.
func (s *OracleStorage) LoadOrderState(
	network,
	protocol,
	orderId string,
) (*OrderState, error) {
	key := orderStateKey(network, protocol, orderId)

	var state *OrderState
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(key))
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			state = &OrderState{}
			return json.Unmarshal(val, state)
		})
	})
	if err != nil {
		return nil, fmt.Errorf("failed to load order state: %w", err)
	}

	return state, nil
}

// ApplyOrderTransaction atomically persists a confirmed order transition and
// the predecessor states needed to undo it after a chain rollback.
func (s *OracleStorage) ApplyOrderTransaction(
	record orderUndoRecord,
	produced []*OrderState,
) (bool, error) {
	if len(record.Spent) == 0 && len(produced) == 0 {
		return false, nil
	}
	if record.Network == "" || record.Protocol == "" ||
		record.TransactionHash == "" {
		return false, fmt.Errorf("incomplete order undo record identity")
	}

	record.Produced = make([]string, 0, len(produced))
	encodedStates := make(map[string][]byte, len(produced))
	for _, state := range produced {
		if state == nil || state.OrderId == "" {
			return false, fmt.Errorf("produced order state is missing an ID")
		}
		if state.Network != record.Network ||
			state.Protocol != record.Protocol {
			return false, fmt.Errorf(
				"produced order %s has mismatched network or protocol",
				state.OrderId,
			)
		}
		data, err := json.Marshal(state)
		if err != nil {
			return false, fmt.Errorf(
				"marshal produced order %s: %w",
				state.OrderId,
				err,
			)
		}
		record.Produced = append(record.Produced, state.OrderId)
		encodedStates[state.OrderId] = data
	}
	undoData, err := json.Marshal(record)
	if err != nil {
		return false, fmt.Errorf("marshal order undo record: %w", err)
	}

	s.orderMu.Lock()
	defer s.orderMu.Unlock()
	applied := false
	undoKey := orderUndoKey(record)
	err = s.db.Update(func(txn *badger.Txn) error {
		if _, err := txn.Get([]byte(undoKey)); err == nil {
			return nil
		} else if err != badger.ErrKeyNotFound {
			return err
		}
		for _, state := range record.Spent {
			if state == nil || state.OrderId == "" {
				return fmt.Errorf("spent order state is missing an ID")
			}
			if state.Network != record.Network ||
				state.Protocol != record.Protocol {
				return fmt.Errorf(
					"spent order %s has mismatched network or protocol",
					state.OrderId,
				)
			}
			if err := txn.Delete([]byte(orderStateKey(
				record.Network,
				record.Protocol,
				state.OrderId,
			))); err != nil {
				return err
			}
		}
		for _, state := range produced {
			if err := txn.Set(
				[]byte(orderStateKey(record.Network, record.Protocol, state.OrderId)),
				encodedStates[state.OrderId],
			); err != nil {
				return err
			}
		}
		if err := txn.Set([]byte(undoKey), undoData); err != nil {
			return err
		}
		if record.Slot > orderRollbackRetentionSlots {
			if err := pruneOrderUndoRecords(
				txn,
				record.Network,
				record.Protocol,
				record.Slot-orderRollbackRetentionSlots,
			); err != nil {
				return err
			}
		}
		applied = true
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("apply order transaction: %w", err)
	}
	return applied, nil
}

func pruneOrderUndoRecords(
	txn *badger.Txn,
	network string,
	protocol string,
	beforeSlot uint64,
) error {
	opts := badger.DefaultIteratorOptions
	opts.Prefix = []byte(orderUndoKeyPrefix + network + ":" + protocol + ":")
	it := txn.NewIterator(opts)
	defer it.Close()
	for it.Rewind(); it.Valid(); it.Next() {
		item := it.Item()
		var record orderUndoRecord
		if err := item.Value(func(value []byte) error {
			return json.Unmarshal(value, &record)
		}); err != nil {
			return fmt.Errorf(
				"unmarshal order undo record %s: %w",
				item.Key(),
				err,
			)
		}
		if record.Slot < beforeSlot {
			if err := txn.Delete(item.KeyCopy(nil)); err != nil {
				return err
			}
		}
	}
	return nil
}

// RollbackOrderStates reverses journaled order transitions at or after the
// first invalid slot.
func (s *OracleStorage) RollbackOrderStates(
	network string,
	protocol string,
	firstInvalidSlot uint64,
) ([]*OrderState, error) {
	s.orderMu.Lock()
	defer s.orderMu.Unlock()

	var undoRecords []struct {
		key    string
		record orderUndoRecord
	}
	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte(
			orderUndoKeyPrefix + network + ":" + protocol + ":",
		)
		it := txn.NewIterator(opts)
		defer it.Close()
		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			key := string(item.KeyCopy(nil))
			if err := item.Value(func(value []byte) error {
				var record orderUndoRecord
				if err := json.Unmarshal(value, &record); err != nil {
					return fmt.Errorf("unmarshal order undo record %s: %w", key, err)
				}
				if record.Slot >= firstInvalidSlot {
					undoRecords = append(undoRecords, struct {
						key    string
						record orderUndoRecord
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
		return nil, fmt.Errorf("load order rollback state: %w", err)
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
	for _, entry := range undoRecords {
		if err := s.rollbackOrderUndoRecord(entry.key, entry.record); err != nil {
			return nil, err
		}
	}
	if err := s.deleteInvalidOrderStates(network, protocol, firstInvalidSlot); err != nil {
		return nil, err
	}
	states, err := s.loadOrderStates(network, protocol)
	if err != nil {
		return nil, err
	}
	sort.Slice(states, func(i, j int) bool {
		return states[i].OrderId < states[j].OrderId
	})
	return states, nil
}

func (s *OracleStorage) rollbackOrderUndoRecord(
	key string,
	record orderUndoRecord,
) error {
	err := s.db.Update(func(txn *badger.Txn) error {
		for _, orderId := range record.Produced {
			if err := txn.Delete([]byte(orderStateKey(
				record.Network,
				record.Protocol,
				orderId,
			))); err != nil {
				return err
			}
		}
		for _, state := range record.Spent {
			if state == nil || state.OrderId == "" {
				return fmt.Errorf(
					"order undo record %s has invalid spent state",
					key,
				)
			}
			data, err := json.Marshal(state)
			if err != nil {
				return fmt.Errorf(
					"marshal restored order %s: %w",
					state.OrderId,
					err,
				)
			}
			if err := txn.Set(
				[]byte(orderStateKey(record.Network, record.Protocol, state.OrderId)),
				data,
			); err != nil {
				return err
			}
		}
		return txn.Delete([]byte(key))
	})
	if err != nil {
		return fmt.Errorf("persist order rollback entry %s: %w", key, err)
	}
	return nil
}

func (s *OracleStorage) deleteInvalidOrderStates(
	network string,
	protocol string,
	firstInvalidSlot uint64,
) error {
	keys := make([]string, 0, orderRollbackDeleteBatchSize)
	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte(
			orderStateKeyPrefix + network + ":" + protocol + ":",
		)
		it := txn.NewIterator(opts)
		defer it.Close()
		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			var state OrderState
			if err := item.Value(func(value []byte) error {
				return json.Unmarshal(value, &state)
			}); err != nil {
				return fmt.Errorf(
					"unmarshal order state %s: %w",
					item.Key(),
					err,
				)
			}
			if state.Slot >= firstInvalidSlot {
				keys = append(keys, string(item.KeyCopy(nil)))
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("load invalid order states: %w", err)
	}
	for len(keys) > 0 {
		batchSize := min(orderRollbackDeleteBatchSize, len(keys))
		batch := keys[:batchSize]
		if err := s.db.Update(func(txn *badger.Txn) error {
			for _, key := range batch {
				if err := txn.Delete([]byte(key)); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			return fmt.Errorf("delete invalid order states: %w", err)
		}
		keys = keys[batchSize:]
	}
	return nil
}

func (s *OracleStorage) loadOrderStates(
	network string,
	protocol string,
) ([]*OrderState, error) {
	logger := logging.GetLogger()
	states := make([]*OrderState, 0)
	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte(
			orderStateKeyPrefix + network + ":" + protocol + ":",
		)
		it := txn.NewIterator(opts)
		defer it.Close()
		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			if err := item.Value(func(value []byte) error {
				var state OrderState
				if err := json.Unmarshal(value, &state); err != nil {
					logger.Warn(
						"failed to unmarshal order state",
						"key", string(item.Key()),
						"error", err,
					)
					return nil
				}
				states = append(states, &state)
				return nil
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("load order states after rollback: %w", err)
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
		return nil, fmt.Errorf(
			"failed to load pool states by protocol: %w",
			err,
		)
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

// orderStateKey generates the storage key for an order state.
func orderStateKey(network, protocol, orderId string) string {
	return orderStateKeyPrefix + network + ":" + protocol + ":" + orderId
}

func orderUndoKey(record orderUndoRecord) string {
	return fmt.Sprintf(
		"%s%s:%s:%020d:%s",
		orderUndoKeyPrefix,
		record.Network,
		record.Protocol,
		record.Slot,
		record.TransactionHash,
	)
}

// ParsePoolStateKey extracts network, protocol, and poolId from a pool key.
func ParsePoolStateKey(
	key string,
) (network, protocol, poolId string, err error) {
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
