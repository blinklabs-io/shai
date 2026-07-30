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

package storage

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/blinklabs-io/shai/price/djed"
	"github.com/dgraph-io/badger/v4"
)

const djedStateKeyPrefix = "price_djed_"

var ErrDjedStateNotFound = errors.New("storage: Djed state not found")

// SaveDjedState atomically persists retained Djed rollback history.
func (s *Storage) SaveDjedState(
	network string,
	state djed.TrackerState,
) error {
	if network == "" {
		return fmt.Errorf("storage: Djed network is required")
	}
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("storage: marshal Djed state: %w", err)
	}
	if err := s.db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(djedStateKeyPrefix+network), data)
	}); err != nil {
		return fmt.Errorf("storage: save Djed state: %w", err)
	}
	return nil
}

// LoadDjedState loads retained Djed rollback history.
func (s *Storage) LoadDjedState(network string) (djed.TrackerState, error) {
	if network == "" {
		return djed.TrackerState{}, fmt.Errorf(
			"storage: Djed network is required",
		)
	}
	var state djed.TrackerState
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(djedStateKeyPrefix + network))
		if err != nil {
			return err
		}
		return item.Value(func(value []byte) error {
			return json.Unmarshal(value, &state)
		})
	})
	if errors.Is(err, badger.ErrKeyNotFound) {
		return djed.TrackerState{}, ErrDjedStateNotFound
	}
	if err != nil {
		return djed.TrackerState{}, fmt.Errorf(
			"storage: load Djed state: %w",
			err,
		)
	}
	return state, nil
}
