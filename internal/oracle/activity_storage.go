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

	// activityDeleteEntryOverhead is what Badger adds to the key length when
	// it estimates the size of one queued delete: two meta bytes and ten bytes
	// for the version appended to the key.
	activityDeleteEntryOverhead = 12
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
		maxCount, maxSize := s.activityDeleteBudget()
		return pruneActivityKeys(txn, cutoff, maxCount, maxSize)
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
			if meta.WindowSlots == 0 {
				return errors.New(
					"pool activity metadata has a zero window",
				)
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
// persisted latest slot while preserving earlier confirmed history. Deletions
// are committed in bounded batches from the newest slot down, and every batch
// rewinds the persisted latest slot in the same transaction, so an interrupted
// rollback never leaves a rolled-back swap at or below the recorded tip where a
// restart would count its volume again.
func (s *OracleStorage) RollbackActivity(slot uint64) error {
	maxCount, maxSize := s.activityDeleteBudget()
	for {
		exhausted, err := s.rollbackActivityBatch(slot, maxCount, maxSize)
		if err != nil {
			return fmt.Errorf("failed to rollback pool activity: %w", err)
		}
		if exhausted {
			return nil
		}
	}
}

// rollbackActivityBatch removes one bounded batch of rolled-back swaps, newest
// slot first, and rewinds the persisted latest slot to just below the lowest
// slot the batch removed. Both happen in one transaction, so every committed
// state keeps the invariant that no retained swap sits above the recorded
// latest slot. It reports whether the rollback is complete.
func (s *OracleStorage) rollbackActivityBatch(
	slot uint64,
	maxCount,
	maxSize int64,
) (bool, error) {
	var exhausted bool
	err := s.db.Update(func(txn *badger.Txn) error {
		keys, lowest, done, err := rolledBackActivityKeys(
			txn,
			slot,
			maxCount,
			maxSize,
		)
		if err != nil {
			return err
		}
		exhausted = done
		firstInvalid := lowest
		if done {
			firstInvalid = slot
		}
		if err := rewindActivityMeta(txn, firstInvalid); err != nil {
			return err
		}
		for _, key := range keys {
			if err := txn.Delete(key); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return false, err
	}
	return exhausted, nil
}

// rewindActivityMeta lowers the persisted latest slot to the slot before
// firstInvalid. The latest slot records observed confirmed activity, so it is
// never advanced here: a rollback point can sit ahead of it whenever the
// chain-sync cursor moved through blocks that carried no pool update, and
// claiming those slots were observed would prune retained history and reject
// volume queries for slots that are still inside the window.
func rewindActivityMeta(txn *badger.Txn, firstInvalid uint64) error {
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
	var latest uint64
	if firstInvalid > 0 {
		latest = firstInvalid - 1
	}
	if latest >= meta.LatestSlot {
		return nil
	}
	meta.LatestSlot = latest
	data, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	return txn.Set([]byte(activityMetaKey), data)
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

// activityDeleteBudget reports how many entries and how many estimated bytes
// of deletions one transaction may queue. Badger fails a transaction with
// ErrTxnTooBig once its pending entries reach maxBatchCount or their estimated
// size reaches maxBatchSize, which a long pruning gap or a deep rollback can
// otherwise exceed in a single sweep. Half of each limit is used so the pool
// state, swap, and metadata writes sharing the transaction still fit.
func (s *OracleStorage) activityDeleteBudget() (int64, int64) {
	return s.db.MaxBatchCount() / 2, s.db.MaxBatchSize() / 2
}

// pruneActivityKeys removes retained swaps below cutoff. Activity keys embed a
// zero-padded slot, so iteration is ordered by slot and stops at the first key
// inside the window instead of scanning the whole retained history on every
// pool-state write. Deletions stay inside the transaction budget; anything left
// over is pruned by later writes, which changes retention only, never the
// volume reported for a window.
func pruneActivityKeys(
	txn *badger.Txn,
	cutoff uint64,
	maxCount,
	maxSize int64,
) error {
	if cutoff == 0 {
		return nil
	}
	opts := badger.DefaultIteratorOptions
	opts.Prefix = []byte(activityKeyPrefix)
	opts.PrefetchValues = false
	it := txn.NewIterator(opts)
	var (
		keys  [][]byte
		count int64
		size  int64
	)
	for it.Rewind(); it.Valid(); it.Next() {
		key := it.Item().KeyCopy(nil)
		slot, err := activitySlotFromKey(key)
		if err != nil {
			it.Close()
			return err
		}
		if slot >= cutoff {
			break
		}
		entry := int64(len(key)) + activityDeleteEntryOverhead
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

// rolledBackActivityKeys collects the newest retained swap keys at or after
// slot, within the transaction budget. Keys of one slot are always collected
// together, so the lowest collected slot is removed completely and the caller
// can rewind the persisted latest slot to just below it in the same
// transaction. The returned flag reports that nothing at or after slot remains.
func rolledBackActivityKeys(
	txn *badger.Txn,
	slot uint64,
	maxCount,
	maxSize int64,
) ([][]byte, uint64, bool, error) {
	opts := badger.DefaultIteratorOptions
	opts.Prefix = []byte(activityKeyPrefix)
	opts.PrefetchValues = false
	opts.Reverse = true
	it := txn.NewIterator(opts)
	defer it.Close()
	var (
		keys  [][]byte
		count int64
		size  int64
	)
	lowest := slot
	// Reverse iteration has to be seeded past the last activity key, because
	// Rewind seeks to the key preceding the prefix range.
	for it.Seek(activityKeyReverseSeek()); it.Valid(); it.Next() {
		key := it.Item().KeyCopy(nil)
		keySlot, err := activitySlotFromKey(key)
		if err != nil {
			return nil, 0, false, err
		}
		if keySlot < slot {
			return keys, lowest, true, nil
		}
		entry := int64(len(key)) + activityDeleteEntryOverhead
		// The batch always takes at least the newest slot, whatever the
		// budget, so the caller is guaranteed to make progress. It stops only
		// on a slot boundary, so the lowest slot it removed is removed whole.
		if len(keys) > 0 && keySlot != lowest &&
			(count+1 > maxCount || size+entry > maxSize) {
			return keys, lowest, false, nil
		}
		count++
		size += entry
		keys = append(keys, key)
		lowest = keySlot
	}
	return keys, lowest, true, nil
}

// activityKeyReverseSeek returns a key that sorts above every activity key.
// Everything after the prefix is printable ASCII, so a trailing 0xff byte is
// above all of them and reverse iteration starts at the newest slot.
func activityKeyReverseSeek() []byte {
	return append([]byte(activityKeyPrefix), 0xff)
}
