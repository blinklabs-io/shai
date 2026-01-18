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
	"strings"
	"sync"

	"github.com/blinklabs-io/shai/internal/logging"
	"github.com/gorilla/websocket"
)

// OracleAPI provides HTTP and WebSocket endpoints for oracle data
type OracleAPI struct {
	oracle   *Oracle
	upgrader websocket.Upgrader
	wsConns  map[*websocket.Conn]bool
	wsMu     sync.RWMutex
}

// NewOracleAPI creates a new OracleAPI instance
func NewOracleAPI(oracle *Oracle) *OracleAPI {
	return &OracleAPI{
		oracle:  oracle,
		wsConns: make(map[*websocket.Conn]bool),
		upgrader: websocket.Upgrader{
			CheckOrigin: checkWebSocketOrigin,
		},
	}
}

// checkWebSocketOrigin validates WebSocket connection origins.
// Allows same-origin requests and localhost connections for development.
func checkWebSocketOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true // Allow requests without Origin header (non-browser clients)
	}

	// Allow localhost connections for development
	if strings.HasPrefix(origin, "http://localhost") ||
		strings.HasPrefix(origin, "http://127.0.0.1") ||
		strings.HasPrefix(origin, "https://localhost") ||
		strings.HasPrefix(origin, "https://127.0.0.1") {
		return true
	}

	// Parse origin URL to extract host for exact comparison
	// This prevents attacks where malicious origins contain the host as substring
	// (e.g., "evil-example.com" or "example.com.attacker.com")
	originHost := extractHost(origin)
	if originHost == "" {
		return false
	}

	// Check if origin host exactly matches the request host (same-origin)
	host := r.Host
	if host == "" {
		host = r.URL.Host
	}
	// Strip port from request host for comparison if origin doesn't have port
	if !strings.Contains(originHost, ":") {
		if idx := strings.LastIndex(host, ":"); idx != -1 {
			host = host[:idx]
		}
	}
	return originHost == host
}

// extractHost extracts the host from a URL string
func extractHost(urlStr string) string {
	// Remove scheme prefix
	if idx := strings.Index(urlStr, "://"); idx != -1 {
		urlStr = urlStr[idx+3:]
	}
	// Remove path
	if idx := strings.Index(urlStr, "/"); idx != -1 {
		urlStr = urlStr[:idx]
	}
	return urlStr
}

// RegisterHandlers registers HTTP handlers on the given ServeMux
func (a *OracleAPI) RegisterHandlers(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/pools", a.HandleListPools)
	mux.HandleFunc("/api/v1/pools/", a.HandleGetPool)
	mux.HandleFunc("/api/v1/prices", a.HandleListPrices)
	mux.HandleFunc("/ws/prices", a.HandlePriceStream)
}

// StartServer starts the HTTP server
func (a *OracleAPI) StartServer(addr string) error {
	logger := logging.GetLogger()

	mux := http.NewServeMux()
	a.RegisterHandlers(mux)

	// Start WebSocket broadcaster
	go a.broadcastPriceUpdates()

	logger.Info("starting oracle API server", "addr", addr)
	return http.ListenAndServe(addr, mux)
}

// HandleListPools returns all tracked pools
func (a *OracleAPI) HandleListPools(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	pools := a.oracle.GetAllPools()

	// Filter by protocol if specified
	protocol := r.URL.Query().Get("protocol")
	if protocol != "" {
		filtered := make([]*PoolState, 0)
		for _, pool := range pools {
			if pool.Protocol == protocol {
				filtered = append(filtered, pool)
			}
		}
		pools = filtered
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"pools": pools,
		"count": len(pools),
	})
}

// HandleGetPool returns a specific pool by ID
func (a *OracleAPI) HandleGetPool(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract pool ID from path
	poolId := strings.TrimPrefix(r.URL.Path, "/api/v1/pools/")
	if poolId == "" {
		http.Error(w, "Pool ID required", http.StatusBadRequest)
		return
	}

	pool, ok := a.oracle.GetPoolState(poolId)
	if !ok {
		http.Error(w, "Pool not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(pool)
}

// HandleListPrices returns current prices for all pools
func (a *OracleAPI) HandleListPrices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	pools := a.oracle.GetAllPools()

	type PriceEntry struct {
		PoolId   string  `json:"poolId"`
		Protocol string  `json:"protocol"`
		AssetX   string  `json:"assetX"`
		AssetY   string  `json:"assetY"`
		PriceXY  float64 `json:"priceXY"`
		PriceYX  float64 `json:"priceYX"`
		ReserveX uint64  `json:"reserveX"`
		ReserveY uint64  `json:"reserveY"`
	}

	prices := make([]PriceEntry, 0, len(pools))
	for _, pool := range pools {
		prices = append(prices, PriceEntry{
			PoolId:   pool.PoolId,
			Protocol: pool.Protocol,
			AssetX:   pool.AssetX.Class.Fingerprint(),
			AssetY:   pool.AssetY.Class.Fingerprint(),
			PriceXY:  pool.PriceXY(),
			PriceYX:  pool.PriceYX(),
			ReserveX: pool.AssetX.Amount,
			ReserveY: pool.AssetY.Amount,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"prices": prices,
		"count":  len(prices),
	})
}

// HandlePriceStream handles WebSocket connections for price streaming
func (a *OracleAPI) HandlePriceStream(w http.ResponseWriter, r *http.Request) {
	logger := logging.GetLogger()

	conn, err := a.upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Error("WebSocket upgrade failed", "error", err)
		return
	}

	// Register connection
	a.wsMu.Lock()
	a.wsConns[conn] = true
	a.wsMu.Unlock()

	logger.Debug("WebSocket client connected", "remote", conn.RemoteAddr())

	// Keep connection alive and handle disconnection
	defer func() {
		a.wsMu.Lock()
		delete(a.wsConns, conn)
		a.wsMu.Unlock()
		_ = conn.Close()
		logger.Debug(
			"WebSocket client disconnected",
			"remote",
			conn.RemoteAddr(),
		)
	}()

	// Read messages (for ping/pong and close handling)
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

// broadcastPriceUpdates subscribes to price updates and broadcasts to WebSocket clients
func (a *OracleAPI) broadcastPriceUpdates() {
	logger := logging.GetLogger()
	updates := a.oracle.Subscribe()

	for update := range updates {
		var failedConns []*websocket.Conn

		a.wsMu.RLock()
		for conn := range a.wsConns {
			if err := conn.WriteJSON(update); err != nil {
				logger.Debug(
					"failed to send WebSocket update",
					"error", err,
					"remote", conn.RemoteAddr(),
				)
				failedConns = append(failedConns, conn)
			}
		}
		a.wsMu.RUnlock()

		// Remove failed connections outside of the read lock
		if len(failedConns) > 0 {
			a.wsMu.Lock()
			for _, conn := range failedConns {
				delete(a.wsConns, conn)
				_ = conn.Close()
			}
			a.wsMu.Unlock()
		}
	}
}

// WebSocketClientCount returns the number of connected WebSocket clients
func (a *OracleAPI) WebSocketClientCount() int {
	a.wsMu.RLock()
	defer a.wsMu.RUnlock()
	return len(a.wsConns)
}
