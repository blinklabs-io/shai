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
	"strconv"
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
	orderStateSlotKeyPrefix      = "oracle_slot_order_"
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

type orderRollbackDelta map[string]*OrderState

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
		if state.Slot != record.Slot {
			return false, fmt.Errorf(
				"produced order %s has mismatched slot",
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
			if err := txn.Delete([]byte(orderStateSlotKey(
				record.Network,
				record.Protocol,
				state.Slot,
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
			if err := txn.Set(
				[]byte(orderStateSlotKey(
					record.Network,
					record.Protocol,
					state.Slot,
					state.OrderId,
				)),
				nil,
			); err != nil {
				return err
			}
		}
		if err := txn.Set([]byte(undoKey), undoData); err != nil {
			return err
		}
		if record.Slot > orderRollbackRetentionSlots {
			maxCount, maxSize := s.deleteBudget()
			if err := pruneOrderUndoRecords(
				txn,
				record.Network,
				record.Protocol,
				record.Slot-orderRollbackRetentionSlots,
				maxCount,
				maxSize,
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
	maxCount,
	maxSize int64,
) error {
	opts := badger.DefaultIteratorOptions
	prefix := []byte(orderUndoKeyPrefix + network + ":" + protocol + ":")
	opts.Prefix = prefix
	opts.PrefetchValues = false
	it := txn.NewIterator(opts)
	var (
		keys  [][]byte
		count int64
		size  int64
	)
	for it.Rewind(); it.Valid(); it.Next() {
		key := it.Item().KeyCopy(nil)
		slot, err := undoSlotFromKey(key, prefix)
		if err != nil {
			it.Close()
			return err
		}
		if slot >= beforeSlot {
			break
		}
		entry := int64(len(key)) + deleteEntryOverhead
		if count+1 > maxCount || size+entry > maxSize {
			break
		}
		count++
		size += entry
		keys = append(keys, key)
	}
	it.Close()
	for _, key := range keys {
		if err := txn.Delete(key); err != nil {
			return err
		}
	}
	return nil
}

func undoSlotFromKey(key, prefix []byte) (uint64, error) {
	slotStart := len(prefix)
	slotEnd := slotStart + 20
	if len(key) <= slotEnd || key[slotEnd] != ':' {
		return 0, fmt.Errorf("invalid undo key %q", key)
	}
	slot, err := strconv.ParseUint(string(key[slotStart:slotEnd]), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid undo key %q: %w", key, err)
	}
	return slot, nil
}

// RollbackOrderStates reverses one bounded batch of order transitions at or
// after the first invalid slot. Returned changes are already durable even when
// a later record in the batch fails; done reports that no invalid state remains.
func (s *OracleStorage) RollbackOrderStates(
	network string,
	protocol string,
	firstInvalidSlot uint64,
) (orderRollbackDelta, bool, error) {
	s.orderMu.Lock()
	defer s.orderMu.Unlock()
	changes := make(orderRollbackDelta)

	undoRecords, undoDone, err := s.loadNewestOrderRollbackSlot(
		network,
		protocol,
		firstInvalidSlot,
	)
	if err != nil {
		return changes, false, fmt.Errorf("load order rollback state: %w", err)
	}
	if !undoDone {
		for _, entry := range undoRecords {
			entryChanges, err := s.rollbackOrderUndoRecord(entry.key, entry.record)
			if err != nil {
				return changes, false, err
			}
			for orderID, state := range entryChanges {
				changes[orderID] = state
			}
		}
		return changes, false, nil
	}

	removed, done, err := s.deleteInvalidOrderStateBatch(
		network,
		protocol,
		firstInvalidSlot,
	)
	for _, orderID := range removed {
		changes[orderID] = nil
	}
	if err != nil {
		return changes, false, err
	}
	return changes, done, nil
}

type orderUndoEntry struct {
	key    string
	record orderUndoRecord
}

// loadNewestOrderRollbackSlot loads only the newest invalid slot. Cardano
// bounds the transitions within a block, so rollback memory stays bounded
// independently of the number of slots retained in the journal.
func (s *OracleStorage) loadNewestOrderRollbackSlot(
	network string,
	protocol string,
	firstInvalidSlot uint64,
) ([]orderUndoEntry, bool, error) {
	prefix := []byte(orderUndoKeyPrefix + network + ":" + protocol + ":")
	seek := append(append([]byte(nil), prefix...), 0xff)
	var entries []orderUndoEntry
	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = prefix
		opts.Reverse = true
		it := txn.NewIterator(opts)
		defer it.Close()

		var newestSlot uint64
		for it.Seek(seek); it.Valid(); it.Next() {
			item := it.Item()
			key := item.KeyCopy(nil)
			slot, err := undoSlotFromKey(key, prefix)
			if err != nil {
				return err
			}
			if slot < firstInvalidSlot {
				break
			}
			if len(entries) == 0 {
				newestSlot = slot
			} else if slot != newestSlot {
				break
			}

			var record orderUndoRecord
			if err := item.Value(func(value []byte) error {
				return json.Unmarshal(value, &record)
			}); err != nil {
				return fmt.Errorf("unmarshal order undo record %s: %w", key, err)
			}
			if orderUndoKey(record) != string(key) {
				return fmt.Errorf("order undo record %s has mismatched identity", key)
			}
			entries = append(entries, orderUndoEntry{
				key:    string(key),
				record: record,
			})
		}
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	if len(entries) == 0 {
		return nil, true, nil
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].record.TransactionIdx != entries[j].record.TransactionIdx {
			return entries[i].record.TransactionIdx >
				entries[j].record.TransactionIdx
		}
		return entries[i].record.TransactionHash >
			entries[j].record.TransactionHash
	})
	return entries, false, nil
}

func (s *OracleStorage) rollbackOrderUndoRecord(
	key string,
	record orderUndoRecord,
) (orderRollbackDelta, error) {
	changes := make(orderRollbackDelta, len(record.Produced)+len(record.Spent))
	err := s.db.Update(func(txn *badger.Txn) error {
		for _, orderId := range record.Produced {
			if err := txn.Delete([]byte(orderStateKey(
				record.Network,
				record.Protocol,
				orderId,
			))); err != nil {
				return err
			}
			if err := txn.Delete([]byte(orderStateSlotKey(
				record.Network,
				record.Protocol,
				record.Slot,
				orderId,
			))); err != nil {
				return err
			}
			changes[orderId] = nil
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
			if err := txn.Set(
				[]byte(orderStateSlotKey(
					record.Network,
					record.Protocol,
					state.Slot,
					state.OrderId,
				)),
				nil,
			); err != nil {
				return err
			}
			changes[state.OrderId] = state
		}
		return txn.Delete([]byte(key))
	})
	if err != nil {
		return nil, fmt.Errorf("persist order rollback entry %s: %w", key, err)
	}
	return changes, nil
}

func (s *OracleStorage) deleteInvalidOrderStateBatch(
	network string,
	protocol string,
	firstInvalidSlot uint64,
) ([]string, bool, error) {
	prefix := []byte(orderStateSlotKeyPrefix + network + ":" + protocol + ":")
	seek := []byte(fmt.Sprintf("%s%020d:", prefix, firstInvalidSlot))
	removed := make([]string, 0, orderRollbackDeleteBatchSize)
	done := false
	err := s.db.Update(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = prefix
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		indexKeys := make([][]byte, 0, orderRollbackDeleteBatchSize)
		orderIDs := make([]string, 0, orderRollbackDeleteBatchSize)
		for it.Seek(seek); it.Valid() && len(indexKeys) < orderRollbackDeleteBatchSize; it.Next() {
			indexKey := it.Item().KeyCopy(nil)
			orderID, err := orderIDFromStateSlotKey(indexKey, prefix)
			if err != nil {
				it.Close()
				return err
			}
			indexKeys = append(indexKeys, indexKey)
			orderIDs = append(orderIDs, orderID)
		}
		done = !it.Valid()
		it.Close()

		for i, indexKey := range indexKeys {
			orderID := orderIDs[i]
			stateKey := []byte(orderStateKey(network, protocol, orderID))
			item, err := txn.Get(stateKey)
			if err == badger.ErrKeyNotFound {
				if err := txn.Delete(indexKey); err != nil {
					return err
				}
				continue
			}
			if err != nil {
				return err
			}
			var state OrderState
			if err := item.Value(func(value []byte) error {
				return json.Unmarshal(value, &state)
			}); err != nil {
				return fmt.Errorf("unmarshal order state %s: %w", stateKey, err)
			}
			if state.Slot < firstInvalidSlot ||
				orderStateSlotKey(network, protocol, state.Slot, orderID) != string(indexKey) {
				if err := txn.Delete(indexKey); err != nil {
					return err
				}
				continue
			}
			if err := txn.Delete(stateKey); err != nil {
				return err
			}
			if err := txn.Delete(indexKey); err != nil {
				return err
			}
			removed = append(removed, orderID)
		}
		return nil
	})
	if err != nil {
		return nil, false, fmt.Errorf("delete invalid order states: %w", err)
	}
	return removed, done, nil
}

func (s *OracleStorage) ensureOrderStateIndexes(states []*OrderState) error {
	for len(states) > 0 {
		batchSize := min(orderRollbackDeleteBatchSize, len(states))
		batch := states[:batchSize]
		if err := s.db.Update(func(txn *badger.Txn) error {
			for _, state := range batch {
				if state == nil || state.Network == "" || state.Protocol == "" || state.OrderId == "" {
					return fmt.Errorf("cannot index incomplete order state")
				}
				if err := txn.Set([]byte(orderStateSlotKey(
					state.Network,
					state.Protocol,
					state.Slot,
					state.OrderId,
				)), nil); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			return fmt.Errorf("index persisted order states: %w", err)
		}
		states = states[batchSize:]
	}
	return nil
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

func orderStateSlotKey(
	network,
	protocol string,
	slot uint64,
	orderId string,
) string {
	return fmt.Sprintf(
		"%s%s:%s:%020d:%s",
		orderStateSlotKeyPrefix,
		network,
		protocol,
		slot,
		orderId,
	)
}

func orderIDFromStateSlotKey(key, prefix []byte) (string, error) {
	slotStart := len(prefix)
	orderIDStart := slotStart + 21
	if len(key) <= orderIDStart || key[orderIDStart-1] != ':' {
		return "", fmt.Errorf("invalid order state slot key %q", key)
	}
	if _, err := strconv.ParseUint(string(key[slotStart:orderIDStart-1]), 10, 64); err != nil {
		return "", fmt.Errorf("invalid order state slot key %q: %w", key, err)
	}
	return string(key[orderIDStart:]), nil
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
