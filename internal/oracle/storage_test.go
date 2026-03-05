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
	"path/filepath"
	"testing"

	"github.com/blinklabs-io/shai/internal/common"
	"github.com/dgraph-io/badger/v4"
)

func newTestOracleStorage(t *testing.T) *OracleStorage {
	t.Helper()

	dir := filepath.Join(t.TempDir(), "oracle")
	opts := badger.DefaultOptions(dir).WithLoggingLevel(badger.WARNING)
	db, err := badger.Open(opts)
	if err != nil {
		t.Fatalf("failed to open test badger DB: %v", err)
	}

	storage := &OracleStorage{db: db}
	t.Cleanup(func() {
		if err := storage.Close(); err != nil {
			t.Fatalf("failed to close test badger DB: %v", err)
		}
	})
	return storage
}

func TestOracleStorageLoadPoolState(t *testing.T) {
	storage := newTestOracleStorage(t)

	state := &PoolState{
		Network:  "mainnet",
		Protocol: "minswap-v2",
		PoolId:   "pool1",
		AssetX:   common.AssetAmount{Amount: 1000},
		AssetY:   common.AssetAmount{Amount: 2000},
	}
	if err := storage.SavePoolState(state); err != nil {
		t.Fatalf("failed to save pool state: %v", err)
	}

	loaded, err := storage.LoadPoolState("mainnet", "minswap-v2", "pool1")
	if err != nil {
		t.Fatalf("failed to load pool state: %v", err)
	}
	if loaded == nil {
		t.Fatalf(
			"expected loaded pool state for mainnet/minswap-v2/pool1, got nil",
		)
	}
	if loaded.PoolId != state.PoolId {
		t.Fatalf("expected pool ID %s, got %s", state.PoolId, loaded.PoolId)
	}
	if loaded.AssetX.Amount != state.AssetX.Amount {
		t.Fatalf(
			"expected assetX amount %d, got %d",
			state.AssetX.Amount,
			loaded.AssetX.Amount,
		)
	}
}

func TestOracleStorageLoadPoolStatesByProtocol(t *testing.T) {
	storage := newTestOracleStorage(t)

	states := []*PoolState{
		{
			Network:  "mainnet",
			Protocol: "minswap-v2",
			PoolId:   "pool1",
		},
		{
			Network:  "mainnet",
			Protocol: "splash-v1",
			PoolId:   "pool2",
		},
		{
			Network:  "mainnet",
			Protocol: "minswap-v2",
			PoolId:   "pool3",
		},
	}

	for _, state := range states {
		if err := storage.SavePoolState(state); err != nil {
			t.Fatalf("failed to save state %s: %v", state.PoolId, err)
		}
	}

	filtered, err := storage.LoadPoolStatesByProtocol("minswap-v2")
	if err != nil {
		t.Fatalf("failed to load filtered states: %v", err)
	}
	if len(filtered) != 2 {
		t.Fatalf("expected 2 filtered states, got %d", len(filtered))
	}
	for _, state := range filtered {
		if state.Protocol != "minswap-v2" {
			t.Fatalf("expected protocol minswap-v2, got %s", state.Protocol)
		}
	}
}

func TestParsePoolStateKey(t *testing.T) {
	network, protocol, poolId, err := ParsePoolStateKey(
		"oracle_pool_mainnet:minswap-v2:pool1",
	)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if network != "mainnet" {
		t.Fatalf("expected network mainnet, got %s", network)
	}
	if protocol != "minswap-v2" {
		t.Fatalf("expected protocol minswap-v2, got %s", protocol)
	}
	if poolId != "pool1" {
		t.Fatalf("expected pool ID pool1, got %s", poolId)
	}

	_, _, _, err = ParsePoolStateKey("bad-key")
	if err == nil {
		t.Fatal("expected parse error for invalid key")
	}

	_, _, _, err = ParsePoolStateKey("foo:bar:baz")
	if err == nil {
		t.Fatal("expected parse error for key missing oracle prefix")
	}

	_, _, _, err = ParsePoolStateKey("oracle_pool_mainnet::pool1")
	if err == nil {
		t.Fatal("expected parse error for key with empty protocol")
	}
}

func TestOracleStorageLoadPoolStatesByProtocolSkipsMalformedEntries(t *testing.T) {
	storage := newTestOracleStorage(t)

	goodState := &PoolState{
		Network:  "mainnet",
		Protocol: "minswap-v2",
		PoolId:   "pool1",
	}
	if err := storage.SavePoolState(goodState); err != nil {
		t.Fatalf("failed to save good pool state: %v", err)
	}

	err := storage.db.Update(func(txn *badger.Txn) error {
		// Invalid key: empty protocol segment
		if err := txn.Set(
			[]byte("oracle_pool_mainnet::pool-bad-key"),
			[]byte(`{"poolId":"pool-bad-key","protocol":"minswap-v2"}`),
		); err != nil {
			return err
		}
		// Invalid value for target protocol
		return txn.Set(
			[]byte("oracle_pool_mainnet:minswap-v2:pool-bad-value"),
			[]byte("not-json"),
		)
	})
	if err != nil {
		t.Fatalf("failed to seed malformed rows: %v", err)
	}

	filtered, err := storage.LoadPoolStatesByProtocol("minswap-v2")
	if err != nil {
		t.Fatalf("expected malformed rows to be skipped, got error: %v", err)
	}
	if len(filtered) != 1 {
		t.Fatalf("expected 1 valid state, got %d", len(filtered))
	}
	if filtered[0].PoolId != "pool1" {
		t.Fatalf("expected pool1, got %s", filtered[0].PoolId)
	}
}
