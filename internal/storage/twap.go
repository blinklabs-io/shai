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

	"github.com/blinklabs-io/shai/price"
	"github.com/dgraph-io/badger/v4"
)

const twapStateKeyPrefix = "price_twap_"

var ErrTWAPStateNotFound = errors.New("storage: TWAP state not found")

// SaveTWAPState atomically persists a bounded TWAP engine snapshot.
func (s *Storage) SaveTWAPState(name string, state price.TWAPState) error {
	if name == "" {
		return fmt.Errorf("storage: TWAP state name is required")
	}
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("storage: marshal TWAP state: %w", err)
	}
	if err := s.db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(twapStateKeyPrefix+name), data)
	}); err != nil {
		return fmt.Errorf("storage: save TWAP state: %w", err)
	}
	return nil
}

// LoadTWAPState loads a persisted TWAP engine snapshot.
func (s *Storage) LoadTWAPState(name string) (price.TWAPState, error) {
	if name == "" {
		return price.TWAPState{}, fmt.Errorf(
			"storage: TWAP state name is required",
		)
	}
	var state price.TWAPState
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(twapStateKeyPrefix + name))
		if err != nil {
			return err
		}
		return item.Value(func(value []byte) error {
			return json.Unmarshal(value, &state)
		})
	})
	if errors.Is(err, badger.ErrKeyNotFound) {
		return price.TWAPState{}, ErrTWAPStateNotFound
	}
	if err != nil {
		return price.TWAPState{}, fmt.Errorf(
			"storage: load TWAP state: %w",
			err,
		)
	}
	return state, nil
}
