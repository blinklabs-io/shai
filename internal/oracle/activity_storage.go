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
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/dgraph-io/badger/v4"
)

const (
	activityKeyPrefix = "oracle_activity_swap_"
	activityMetaKey   = "oracle_activity_meta"
)

type activityMetadata struct {
	WindowSlots uint64 `json:"windowSlots"`
	LatestSlot  uint64 `json:"latestSlot"`
}

// SavePoolStateAndActivity atomically persists the latest pool state, the
// optional confirmed swap, and rolling activity metadata.
func (s *OracleStorage) SavePoolStateAndActivity(
	state *PoolState,
	swap *SwapTransition,
	windowSlots uint64,
) error {
	if state == nil {
		return errors.New("failed to save pool activity: nil pool state")
	}
	if windowSlots == 0 {
		return errors.New("failed to save pool activity: zero window")
	}
	poolData, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal pool state: %w", err)
	}
	var swapData []byte
	var swapKey string
	if swap != nil {
		if swap.Network != state.Network ||
			swap.Protocol != state.Protocol ||
			swap.PoolID != state.PoolId ||
			swap.Slot != state.Slot {
			return errors.New(
				"failed to save pool activity: swap does not match pool state",
			)
		}
		swapData, err = json.Marshal(swap)
		if err != nil {
			return fmt.Errorf("failed to marshal pool activity: %w", err)
		}
		swapKey = poolActivityKey(*swap)
	}

	err = s.db.Update(func(txn *badger.Txn) error {
		meta := activityMetadata{
			WindowSlots: windowSlots,
			LatestSlot:  state.Slot,
		}
		item, err := txn.Get([]byte(activityMetaKey))
		if err == nil {
			if err := item.Value(func(value []byte) error {
				return json.Unmarshal(value, &meta)
			}); err != nil {
				return err
			}
			if meta.WindowSlots != windowSlots {
				return fmt.Errorf(
					"persisted activity window is %d, requested %d",
					meta.WindowSlots,
					windowSlots,
				)
			}
			if state.Slot > meta.LatestSlot {
				meta.LatestSlot = state.Slot
			}
		} else if !errors.Is(err, badger.ErrKeyNotFound) {
			return err
		}
		metaData, err := json.Marshal(meta)
		if err != nil {
			return err
		}
		if err := txn.Set(
			[]byte(poolStateKey(state.Network, state.Protocol, state.PoolId)),
			poolData,
		); err != nil {
			return err
		}
		if swap != nil {
			if err := txn.Set([]byte(swapKey), swapData); err != nil {
				return err
			}
		}
		if err := txn.Set([]byte(activityMetaKey), metaData); err != nil {
			return err
		}
		var cutoff uint64
		if meta.LatestSlot > windowSlots {
			cutoff = meta.LatestSlot - windowSlots
		}
		return deleteActivityKeys(txn, func(slot uint64) bool {
			return slot < cutoff
		})
	})
	if err != nil {
		return fmt.Errorf("failed to save pool state and activity: %w", err)
	}
	return nil
}

// LoadActivityState loads all retained confirmed swaps and tracker metadata.
func (s *OracleStorage) LoadActivityState() (ActivityState, error) {
	var state ActivityState
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(activityMetaKey))
		if errors.Is(err, badger.ErrKeyNotFound) {
			opts := badger.DefaultIteratorOptions
			opts.Prefix = []byte(activityKeyPrefix)
			it := txn.NewIterator(opts)
			defer it.Close()
			it.Rewind()
			if it.Valid() {
				return errors.New(
					"pool activity swaps exist without metadata",
				)
			}
			return nil
		}
		if err != nil {
			return err
		}
		if err := item.Value(func(value []byte) error {
			var meta activityMetadata
			if err := json.Unmarshal(value, &meta); err != nil {
				return err
			}
			state.WindowSlots = meta.WindowSlots
			state.LatestSlot = meta.LatestSlot
			return nil
		}); err != nil {
			return err
		}

		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte(activityKeyPrefix)
		it := txn.NewIterator(opts)
		defer it.Close()
		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			if _, err := activitySlotFromKey(item.Key()); err != nil {
				return err
			}
			if err := item.Value(func(value []byte) error {
				var swap SwapTransition
				if err := json.Unmarshal(value, &swap); err != nil {
					return err
				}
				state.Swaps = append(state.Swaps, swap)
				return nil
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return ActivityState{}, fmt.Errorf(
			"failed to load pool activity: %w",
			err,
		)
	}
	return state, nil
}

// RollbackActivity removes swaps at or after the rollback slot and rewinds the
// persisted latest slot while preserving earlier confirmed history.
func (s *OracleStorage) RollbackActivity(slot uint64) error {
	err := s.db.Update(func(txn *badger.Txn) error {
		if err := deleteActivityKeys(txn, func(activitySlot uint64) bool {
			return activitySlot >= slot
		}); err != nil {
			return err
		}

		item, err := txn.Get([]byte(activityMetaKey))
		if errors.Is(err, badger.ErrKeyNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		var meta activityMetadata
		if err := item.Value(func(value []byte) error {
			return json.Unmarshal(value, &meta)
		}); err != nil {
			return err
		}
		meta.LatestSlot = 0
		if slot > 0 {
			meta.LatestSlot = slot - 1
		}
		data, err := json.Marshal(meta)
		if err != nil {
			return err
		}
		return txn.Set([]byte(activityMetaKey), data)
	})
	if err != nil {
		return fmt.Errorf("failed to rollback pool activity: %w", err)
	}
	return nil
}

func poolActivityKey(swap SwapTransition) string {
	identity := sha256.Sum256([]byte(
		swap.Network + "\x00" + swap.Protocol + "\x00" + swap.PoolID,
	))
	return fmt.Sprintf(
		"%s%020d:%s:%d:%x",
		activityKeyPrefix,
		swap.Slot,
		swap.TxHash,
		swap.TxIndex,
		identity,
	)
}

func activitySlotFromKey(key []byte) (uint64, error) {
	value := strings.TrimPrefix(string(key), activityKeyPrefix)
	slotText, _, ok := strings.Cut(value, ":")
	if !ok || len(slotText) != 20 {
		return 0, fmt.Errorf("invalid pool activity key %q", key)
	}
	slot, err := strconv.ParseUint(slotText, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid pool activity key %q: %w", key, err)
	}
	return slot, nil
}

func deleteActivityKeys(
	txn *badger.Txn,
	shouldDelete func(uint64) bool,
) error {
	opts := badger.DefaultIteratorOptions
	opts.Prefix = []byte(activityKeyPrefix)
	it := txn.NewIterator(opts)
	var keys [][]byte
	for it.Rewind(); it.Valid(); it.Next() {
		key := it.Item().KeyCopy(nil)
		slot, err := activitySlotFromKey(key)
		if err != nil {
			it.Close()
			return err
		}
		if shouldDelete(slot) {
			keys = append(keys, key)
		}
	}
	it.Close()
	for _, key := range keys {
		if err := txn.Delete(key); err != nil {
			return err
		}
	}
	return nil
}
