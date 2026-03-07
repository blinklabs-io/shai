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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/blinklabs-io/shai/internal/common"
	"github.com/gorilla/websocket"
)

func TestCheckWebSocketOriginNoOrigin(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/ws/prices", nil)
	if !checkWebSocketOrigin(r) {
		t.Error("expected true for no origin header")
	}
}

func TestCheckWebSocketOriginLocalhost(t *testing.T) {
	cases := []string{
		"http://localhost",
		"http://localhost:3000",
		"http://127.0.0.1",
		"http://127.0.0.1:8080",
		"http://[::1]:8080",
		"https://localhost",
	}
	for _, origin := range cases {
		r := httptest.NewRequest(http.MethodGet, "/ws/prices", nil)
		r.Header.Set("Origin", origin)
		if !checkWebSocketOrigin(r) {
			t.Errorf("expected true for origin %s", origin)
		}
	}
}

func TestCheckWebSocketOriginSameHost(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "http://api.example.com/ws/prices", nil)
	r.Host = "api.example.com"
	r.Header.Set("Origin", "https://api.example.com")
	if !checkWebSocketOrigin(r) {
		t.Error("expected true for same-origin host")
	}
}

func TestCheckWebSocketOriginSameHostDefaultPort(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "https://api.example.com/ws/prices", nil)
	r.Host = "api.example.com:443"
	r.Header.Set("Origin", "https://api.example.com")
	if !checkWebSocketOrigin(r) {
		t.Error("expected true for same-origin default HTTPS port")
	}
}

func TestCheckWebSocketOriginMismatchedHost(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "http://api.example.com/ws/prices", nil)
	r.Host = "api.example.com"
	r.Header.Set("Origin", "https://evil.example.com")
	if checkWebSocketOrigin(r) {
		t.Error("expected false for mismatched origin host")
	}
}

func TestHandleListPools(t *testing.T) {
	api := NewOracleAPI(newTestOracleWithPools())
	mux := http.NewServeMux()
	api.RegisterHandlers(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/pools", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	var response struct {
		Pools []*PoolState `json:"pools"`
		Count int          `json:"count"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if response.Count != 2 {
		t.Fatalf("expected count 2, got %d", response.Count)
	}
}

func TestHandleListPoolsProtocolFilter(t *testing.T) {
	api := NewOracleAPI(newTestOracleWithPools())
	mux := http.NewServeMux()
	api.RegisterHandlers(mux)

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/pools?protocol=minswap-v2",
		nil,
	)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	var response struct {
		Pools []*PoolState `json:"pools"`
		Count int          `json:"count"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if response.Count != 1 {
		t.Fatalf("expected count 1, got %d", response.Count)
	}
	if len(response.Pools) != 1 || response.Pools[0].Protocol != "minswap-v2" {
		t.Fatal("expected only minswap-v2 pool in filtered response")
	}
}

func TestHandleGetPool(t *testing.T) {
	api := NewOracleAPI(newTestOracleWithPools())
	mux := http.NewServeMux()
	api.RegisterHandlers(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/pools/pool-1", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	var pool PoolState
	if err := json.Unmarshal(rr.Body.Bytes(), &pool); err != nil {
		t.Fatalf("failed to decode pool response: %v", err)
	}
	if pool.PoolId != "pool-1" {
		t.Fatalf("expected pool-1, got %s", pool.PoolId)
	}
}

func TestHandleGetPoolNotFound(t *testing.T) {
	api := NewOracleAPI(newTestOracleWithPools())
	mux := http.NewServeMux()
	api.RegisterHandlers(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/pools/missing", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", rr.Code)
	}
}

func TestHandleListPrices(t *testing.T) {
	api := NewOracleAPI(newTestOracleWithPools())
	mux := http.NewServeMux()
	api.RegisterHandlers(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/prices", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	var response struct {
		Prices []struct {
			PoolId string `json:"poolId"`
		} `json:"prices"`
		Count int `json:"count"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if response.Count != 2 {
		t.Fatalf("expected count 2, got %d", response.Count)
	}
}

func TestHandleListPoolsMultipleOracles(t *testing.T) {
	api := NewMultiOracleAPI([]*Oracle{
		newTestOracleWithPools(),
		{
			pools: map[string]*PoolState{
				"pool-3": {
					PoolId:    "pool-3",
					Protocol:  "vyfi",
					Network:   "mainnet",
					AssetX:    common.AssetAmount{Class: common.Lovelace(), Amount: 100},
					AssetY:    common.AssetAmount{Class: common.Lovelace(), Amount: 400},
					Timestamp: time.Now(),
				},
			},
			stopChan: make(chan struct{}),
		},
	})
	mux := http.NewServeMux()
	api.RegisterHandlers(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/pools", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	var response struct {
		Pools []*PoolState `json:"pools"`
		Count int          `json:"count"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if response.Count != 3 {
		t.Fatalf("expected count 3, got %d", response.Count)
	}
}

func TestHandlePriceStreamBroadcast(t *testing.T) {
	o := newTestOracleWithPools()
	defer o.Stop()
	api := NewOracleAPI(o)
	defer api.Stop()

	mux := http.NewServeMux()
	api.RegisterHandlers(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	api.startBroadcastPriceUpdates()
	waitForSubscriber(t, o)

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/prices"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to dial websocket: %v", err)
	}
	defer conn.Close()
	waitForWebSocketClients(t, api, 1)

	o.notifySubscribers(
		&PoolState{
			PoolId:    "pool-stream",
			Protocol:  "minswap-v2",
			AssetX:    common.AssetAmount{Class: common.Lovelace(), Amount: 100},
			AssetY:    common.AssetAmount{Class: common.Lovelace(), Amount: 250},
			Timestamp: time.Now(),
		},
		2.0,
	)

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var update PriceUpdate
	if err := conn.ReadJSON(&update); err != nil {
		t.Fatalf("failed to read websocket update: %v", err)
	}
	if update.PoolId != "pool-stream" {
		t.Fatalf("expected pool-stream update, got %s", update.PoolId)
	}
}

func newTestOracleWithPools() *Oracle {
	tokenA, _ := common.NewAssetClass("aa", "bb")
	tokenB, _ := common.NewAssetClass("cc", "dd")
	return &Oracle{
		pools: map[string]*PoolState{
			"pool-1": {
				PoolId:    "pool-1",
				Protocol:  "minswap-v2",
				Network:   "mainnet",
				AssetX:    common.AssetAmount{Class: common.Lovelace(), Amount: 100},
				AssetY:    common.AssetAmount{Class: tokenA, Amount: 200},
				Timestamp: time.Now(),
			},
			"pool-2": {
				PoolId:    "pool-2",
				Protocol:  "splash-v1",
				Network:   "mainnet",
				AssetX:    common.AssetAmount{Class: common.Lovelace(), Amount: 100},
				AssetY:    common.AssetAmount{Class: tokenB, Amount: 300},
				Timestamp: time.Now(),
			},
		},
		stopChan: make(chan struct{}),
	}
}

func waitForWebSocketClients(t *testing.T, api *OracleAPI, expected int) {
	t.Helper()
	timeout := time.After(2 * time.Second)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			t.Fatalf(
				"timed out waiting for websocket clients: expected %d, got %d",
				expected,
				api.WebSocketClientCount(),
			)
		case <-ticker.C:
			if api.WebSocketClientCount() >= expected {
				return
			}
		}
	}
}

func waitForSubscriber(t *testing.T, o *Oracle) {
	t.Helper()
	timeout := time.After(2 * time.Second)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			t.Fatal("timed out waiting for oracle subscription")
		case <-ticker.C:
			o.subMu.RLock()
			count := len(o.subscribers)
			o.subMu.RUnlock()
			if count > 0 {
				return
			}
		}
	}
}
