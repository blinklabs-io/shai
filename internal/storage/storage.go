// Copyright 2025 Blink Labs Software
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
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/shai/internal/config"
	"github.com/blinklabs-io/shai/internal/logging"
	"github.com/dgraph-io/badger/v4"
)

const (
	chainsyncCursorKey = "chainsync_cursor"
	fingerprintKey     = "config_fingerprint"
)

type Storage struct {
	db *badger.DB
}

var globalStorage = &Storage{}

func (s *Storage) Load() error {
	cfg := config.GetConfig()
	badgerOpts := badger.DefaultOptions(cfg.Storage.Directory).
		WithLogger(NewBadgerLogger()).
		// The default INFO logging is a bit verbose
		WithLoggingLevel(badger.WARNING)
	db, err := badger.Open(badgerOpts)
	// TODO: setup automatic GC for Badger
	if err != nil {
		return err
	}
	s.db = db
	//defer db.Close()
	if err := s.compareFingerprint(); err != nil {
		return err
	}
	return nil
}

func (s *Storage) compareFingerprint() error {
	cfg := config.GetConfig()
	fingerprint := fmt.Sprintf(
		"network=%s,profiles=%s",
		cfg.Network,
		strings.Join(cfg.Profiles, ","),
	)
	err := s.db.Update(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(fingerprintKey))
		if err != nil {
			if errors.Is(err, badger.ErrKeyNotFound) {
				if err := txn.Set([]byte(fingerprintKey), []byte(fingerprint)); err != nil {
					return err
				}
				return nil
			} else {
				return err
			}
		}
		err = item.Value(func(v []byte) error {
			if string(v) != fingerprint {
				return fmt.Errorf(
					"config fingerprint in DB doesn't match current config: %s",
					v,
				)
			}
			return nil
		})
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

func (s *Storage) UpdateCursor(slotNumber uint64, blockHash string) error {
	err := s.db.Update(func(txn *badger.Txn) error {
		val := fmt.Sprintf("%d,%s", slotNumber, blockHash)
		if err := txn.Set([]byte(chainsyncCursorKey), []byte(val)); err != nil {
			return err
		}
		return nil
	})
	return err
}

func (s *Storage) GetCursor() (uint64, string, error) {
	var slotNumber uint64
	var blockHash string
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(chainsyncCursorKey))
		if err != nil {
			return err
		}
		err = item.Value(func(v []byte) error {
			var err error
			cursorParts := strings.Split(string(v), ",")
			slotNumber, err = strconv.ParseUint(cursorParts[0], 10, 64)
			if err != nil {
				return err
			}
			blockHash = cursorParts[1]
			return nil
		})
		if err != nil {
			return err
		}
		return nil
	})
	if errors.Is(err, badger.ErrKeyNotFound) {
		return 0, "", nil
	}
	return slotNumber, blockHash, err
}

func (s *Storage) AddUtxo(
	address string,
	txId string,
	txOutIdx uint32,
	txOutBytes []byte,
) error {
	logger := logging.GetLogger()
	utxoId := fmt.Sprintf("%s.%d", txId, txOutIdx)
	logger.Debug("adding UTxO to storage", "utxoId", utxoId)
	utxoKey := "utxo_" + utxoId
	utxoAddressKey := utxoKey + "_address"
	addressKey := "address_" + address
	err := s.db.Update(func(txn *badger.Txn) error {
		// Wrap TX output in UTxO structure to make it easier to consume later
		txIdBytes, err := hex.DecodeString(txId)
		if err != nil {
			return err
		}
		// Create temp UTxO structure
		utxoTmp := []any{
			// Transaction output reference
			[]any{
				txIdBytes,
				uint32(txOutIdx),
			},
			// Transaction output CBOR
			cbor.RawMessage(txOutBytes),
		}
		// Convert to CBOR
		cborBytes, err := cbor.Encode(&utxoTmp)
		if err != nil {
			return err
		}
		if err := txn.Set([]byte(utxoKey), cborBytes); err != nil {
			return err
		}
		// Set address for UTxO
		if err := txn.Set([]byte(utxoAddressKey), []byte(address)); err != nil {
			return err
		}
		// Update UTxOs for address
		var oldVal []byte
		addressItem, err := txn.Get([]byte(addressKey))
		if err != nil {
			if !errors.Is(err, badger.ErrKeyNotFound) {
				return err
			}
		} else {
			err = addressItem.Value(func(val []byte) error {
				oldVal = append([]byte{}, val...)
				return nil
			})
			if err != nil {
				return err
			}
		}
		var newVal string
		if len(oldVal) == 0 {
			newVal = utxoId
		} else {
			newVal = fmt.Sprintf("%s,%s", oldVal, utxoId)
		}
		if err := txn.Set([]byte(addressKey), []byte(newVal)); err != nil {
			return err
		}
		return nil
	})
	return err
}

func (s *Storage) RemoveUtxo(
	txId string,
	utxoIdx uint32,
) error {
	logger := logging.GetLogger()
	utxoId := fmt.Sprintf("%s.%d", txId, utxoIdx)
	utxoKey := "utxo_" + utxoId
	utxoAddressKey := utxoKey + "_address"
	err := s.db.Update(func(txn *badger.Txn) error {
		// Lookup current address for UTxO
		// This also allows us to shortcut the rest if we don't have the UTxO in storage at all
		utxoAddressItem, err := txn.Get([]byte(utxoAddressKey))
		if err != nil {
			if errors.Is(err, badger.ErrKeyNotFound) {
				return nil
			}
			return err
		}
		logger.Debug("removing UTxO from storage", "utxoId", utxoId)
		err = utxoAddressItem.Value(func(addressVal []byte) error {
			// Delete UTxO key
			if err := txn.Delete([]byte(utxoKey)); err != nil {
				return fmt.Errorf("failed to delete UTxO key: %w", err)
			}
			// Get UTxO list for address
			addressKey := fmt.Sprintf("address_%s", addressVal)
			addressItem, err := txn.Get([]byte(addressKey))
			if err != nil {
				if errors.Is(err, badger.ErrKeyNotFound) {
					return nil
				}
				return fmt.Errorf("failed to lookup UTxO address: %w", err)
			}
			err = addressItem.Value(func(utxosVal []byte) error {
				// Remove UTxO from list
				var newUtxos []string
				utxoItems := strings.Split(string(utxosVal), ",")
				for _, utxoItem := range utxoItems {
					if utxoItem != utxoId {
						newUtxos = append(newUtxos, utxoItem)
					}
				}
				newVal := strings.Join(newUtxos, ",")
				if err := txn.Set([]byte(addressKey), []byte(newVal)); err != nil {
					return err
				}
				return nil
			})
			if err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			return err
		}
		// Delete UTxO address key
		if err := txn.Delete([]byte(utxoAddressKey)); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

func (s *Storage) GetUtxos(address string) ([][]byte, error) {
	ret := [][]byte{}
	// Get list of UTxO IDs for address
	addressKey := "address_" + address
	var utxoIds []string
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(addressKey))
		if err != nil {
			return err
		}
		err = item.Value(func(v []byte) error {
			utxoIds = strings.Split(string(v), ",")
			return nil
		})
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	// Retrieve UTxOs
	for _, utxoId := range utxoIds {
		tmpUtxo, err := s.GetUtxoById(utxoId)
		if err != nil {
			return nil, err
		}
		ret = append(ret, tmpUtxo)
	}
	return ret, nil
}

func (s *Storage) GetUtxoById(utxoId string) ([]byte, error) {
	var ret []byte
	key := "utxo_" + utxoId
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(key))
		if err != nil {
			return err
		}
		err = item.Value(func(v []byte) error {
			ret = append([]byte{}, v...)
			return nil
		})
		return err
	})
	return ret, err
}

func (s *Storage) GetUtxoAddress(utxoId string) (string, error) {
	var ret []byte
	utxoKey := "utxo_" + utxoId
	utxoAddressKey := utxoKey + "_address"
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(utxoAddressKey))
		if err != nil {
			return err
		}
		err = item.Value(func(v []byte) error {
			ret = append([]byte{}, v...)
			return nil
		})
		return err
	})
	return string(ret), err
}

func (s *Storage) UpdateAssetUtxo(
	keyPrefix string,
	policyId []byte,
	assetName []byte,
	txId string,
	txOutIdx uint32,
) error {
	err := s.db.Update(func(txn *badger.Txn) error {
		key := fmt.Sprintf(
			"%s_asset_%s_%s",
			keyPrefix,
			hex.EncodeToString(policyId),
			hex.EncodeToString(assetName),
		)
		utxoId := fmt.Sprintf("%s.%d", txId, txOutIdx)
		if err := txn.Set([]byte(key), []byte(utxoId)); err != nil {
			return err
		}
		return nil
	})
	return err
}

func (s *Storage) GetAssetUtxoId(
	keyPrefix string,
	policyId []byte,
	assetName []byte,
) (string, error) {
	var utxoId []byte
	err := s.db.View(func(txn *badger.Txn) error {
		key := fmt.Sprintf(
			"%s_asset_%s_%s",
			keyPrefix,
			hex.EncodeToString(policyId),
			hex.EncodeToString(assetName),
		)
		item, err := txn.Get([]byte(key))
		if err != nil {
			return err
		}
		err = item.Value(func(v []byte) error {
			utxoId = append([]byte{}, v...)
			return nil
		})
		return err
	})
	if err != nil {
		if errors.Is(err, badger.ErrKeyNotFound) {
			return "", fmt.Errorf(
				"no UTxO found for asset with policy ID %x and name '%s' (%x)",
				policyId,
				assetName,
				assetName,
			)
		} else {
			return "", err
		}
	}
	return string(utxoId), err
}

func (s *Storage) GetAssetUtxo(
	keyPrefix string,
	policyId []byte,
	assetName []byte,
) ([]byte, error) {
	utxoId, err := s.GetAssetUtxoId(keyPrefix, policyId, assetName)
	if err != nil {
		return nil, err
	}
	return s.GetUtxoById(string(utxoId))
}

func GetStorage() *Storage {
	return globalStorage
}

// BadgerLogger is a wrapper type to give our logger the expected interface
type BadgerLogger struct {
	logger *slog.Logger
}

func NewBadgerLogger() *BadgerLogger {
	return &BadgerLogger{
		logger: logging.GetLogger(),
	}
}

func (b *BadgerLogger) Infof(msg string, args ...any) {
	b.logger.Error(msg, args...)
}

func (b *BadgerLogger) Warningf(msg string, args ...any) {
	b.logger.Warn(msg, args...)
}

func (b *BadgerLogger) Debugf(msg string, args ...any) {
	b.logger.Debug(msg, args...)
}

func (b *BadgerLogger) Errorf(msg string, args ...any) {
	b.logger.Error(msg, args...)
}
