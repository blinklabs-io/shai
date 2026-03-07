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
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/blinklabs-io/shai/internal/logging"
	"github.com/gorilla/websocket"
)

const (
	wsReadLimit    = 512
	wsPongWait     = 60 * time.Second
	wsPingPeriod   = (wsPongWait * 9) / 10
	wsWriteTimeout = 10 * time.Second
)

// wsConn wraps a websocket.Conn with a write mutex to serialize writes
// and a sync.Once to ensure the connection is closed exactly once.
type wsConn struct {
	conn      *websocket.Conn
	mu        sync.Mutex
	closeOnce sync.Once
}

func (c *wsConn) writeJSON(v interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.conn.SetWriteDeadline(time.Now().Add(wsWriteTimeout)); err != nil {
		return err
	}
	return c.conn.WriteJSON(v)
}

func (c *wsConn) writePing() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.conn.SetWriteDeadline(time.Now().Add(wsWriteTimeout)); err != nil {
		return err
	}
	return c.conn.WriteMessage(
		websocket.PingMessage,
		nil,
	)
}

func (c *wsConn) close() {
	c.closeOnce.Do(func() {
		_ = c.conn.Close()
	})
}

// OracleAPI provides HTTP and WebSocket endpoints for oracle data
type OracleAPI struct {
	oracles       []*Oracle
	upgrader      websocket.Upgrader
	wsConns       map[*wsConn]bool
	wsMu          sync.RWMutex
	stopChan      chan struct{}
	broadcastOnce sync.Once
}

// NewOracleAPI creates a new OracleAPI instance
func NewOracleAPI(oracle *Oracle) *OracleAPI {
	return NewMultiOracleAPI([]*Oracle{oracle})
}

// NewMultiOracleAPI creates a new OracleAPI instance for multiple oracles.
func NewMultiOracleAPI(oracles []*Oracle) *OracleAPI {
	filteredOracles := make([]*Oracle, 0, len(oracles))
	for _, o := range oracles {
		if o != nil {
			filteredOracles = append(filteredOracles, o)
		}
	}
	return &OracleAPI{
		oracles:  filteredOracles,
		wsConns:  make(map[*wsConn]bool),
		stopChan: make(chan struct{}),
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

	originURL, err := url.Parse(origin)
	if err != nil || originURL.Host == "" {
		return false
	}

	originHost := originURL.Hostname()
	if originHost == "localhost" || originHost == "127.0.0.1" || originHost == "::1" {
		// Allow localhost connections for development.
		return true
	}

	host := r.Host
	if host == "" {
		host = r.URL.Host
	}
	if host == "" {
		return false
	}

	requestScheme := "http"
	if r.TLS != nil {
		requestScheme = "https"
	}
	if r.URL.Scheme != "" {
		requestScheme = r.URL.Scheme
	}

	originHostPort := normalizeHostPort(originURL.Host, originURL.Scheme)
	requestHostPort := normalizeHostPort(host, requestScheme)
	if originHostPort == "" || requestHostPort == "" {
		return false
	}
	return strings.EqualFold(originHostPort, requestHostPort)
}

func normalizeHostPort(hostPort, scheme string) string {
	host, port, err := net.SplitHostPort(hostPort)
	if err != nil {
		host = hostPort
		port = ""
	}
	host = strings.Trim(host, "[]")
	host = strings.ToLower(host)
	if host == "" {
		return ""
	}

	defaultPort := ""
	switch strings.ToLower(scheme) {
	case "http", "ws":
		defaultPort = "80"
	case "https", "wss":
		defaultPort = "443"
	}

	if port == "" || port == defaultPort {
		return host
	}
	return net.JoinHostPort(host, port)
}

// RegisterHandlers registers HTTP handlers on the given ServeMux
func (a *OracleAPI) RegisterHandlers(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/pools", a.HandleListPools)
	mux.HandleFunc("GET /api/v1/pools/{poolId}", a.HandleGetPool)
	mux.HandleFunc("GET /api/v1/prices", a.HandleListPrices)
	mux.HandleFunc("/ws/prices", a.HandlePriceStream)
	a.startBroadcastPriceUpdates()
}

// StartServer starts the HTTP server
func (a *OracleAPI) StartServer(addr string) error {
	logger := logging.GetLogger()

	mux := http.NewServeMux()
	a.RegisterHandlers(mux)

	logger.Info("starting oracle API server", "addr", addr)
	// WriteTimeout is intentionally omitted: setting it on a server that
	// also handles WebSocket connections would forcibly close long-lived
	// connections after the timeout. Write deadlines for WebSocket are
	// managed per-connection via the ping/pong mechanism.
	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadTimeout:       10 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	return server.ListenAndServe()
}

// HandleListPools returns all tracked pools
func (a *OracleAPI) HandleListPools(w http.ResponseWriter, r *http.Request) {
	pools := a.getAllPools()

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
	poolId := r.PathValue("poolId")
	if poolId == "" {
		http.Error(w, "Pool ID required", http.StatusBadRequest)
		return
	}

	pool, ok := a.getPoolState(poolId)
	if !ok {
		http.Error(w, "Pool not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(pool)
}

// HandleListPrices returns current prices for all pools
func (a *OracleAPI) HandleListPrices(w http.ResponseWriter, r *http.Request) {
	pools := a.getAllPools()

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

	raw, err := a.upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Error("WebSocket upgrade failed", "error", err)
		return
	}

	conn := &wsConn{conn: raw}

	raw.SetReadLimit(wsReadLimit)
	_ = raw.SetReadDeadline(time.Now().Add(wsPongWait))
	raw.SetPongHandler(func(string) error {
		return raw.SetReadDeadline(time.Now().Add(wsPongWait))
	})

	a.wsMu.Lock()
	a.wsConns[conn] = true
	a.wsMu.Unlock()

	logger.Debug("WebSocket client connected", "remote", raw.RemoteAddr())

	defer func() {
		a.wsMu.Lock()
		delete(a.wsConns, conn)
		a.wsMu.Unlock()
		conn.close()
		logger.Debug(
			"WebSocket client disconnected",
			"remote",
			raw.RemoteAddr(),
		)
	}()

	// Read loop handles pong responses and close frames
	for {
		_, _, err := raw.ReadMessage()
		if err != nil {
			break
		}
	}
}

func (a *OracleAPI) startBroadcastPriceUpdates() {
	a.broadcastOnce.Do(func() {
		for _, o := range a.oracles {
			go a.broadcastPriceUpdatesFromOracle(o)
		}
		go a.pingLoop()
	})
}

// Stop shuts down the API's background goroutines.
func (a *OracleAPI) Stop() {
	a.wsMu.Lock()
	for conn := range a.wsConns {
		conn.close()
		delete(a.wsConns, conn)
	}
	a.wsMu.Unlock()

	select {
	case <-a.stopChan:
	default:
		close(a.stopChan)
	}
}

func (a *OracleAPI) broadcastPriceUpdatesFromOracle(o *Oracle) {
	logger := logging.GetLogger()
	updates := o.Subscribe()
	defer o.Unsubscribe(updates)

	for {
		select {
		case <-a.stopChan:
			return
		case update, ok := <-updates:
			if !ok {
				return
			}

			a.wsMu.RLock()
			conns := make([]*wsConn, 0, len(a.wsConns))
			for conn := range a.wsConns {
				conns = append(conns, conn)
			}
			a.wsMu.RUnlock()

			var failedConns []*wsConn
			for _, conn := range conns {
				if err := conn.writeJSON(update); err != nil {
					logger.Debug(
						"failed to send WebSocket update",
						"error", err,
						"remote", conn.conn.RemoteAddr(),
					)
					failedConns = append(failedConns, conn)
				}
			}

			a.removeConns(failedConns)
		}
	}
}

func (a *OracleAPI) pingLoop() {
	ticker := time.NewTicker(wsPingPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-a.stopChan:
			return
		case <-ticker.C:
		}

		a.wsMu.RLock()
		conns := make([]*wsConn, 0, len(a.wsConns))
		for conn := range a.wsConns {
			conns = append(conns, conn)
		}
		a.wsMu.RUnlock()

		var failedConns []*wsConn
		for _, conn := range conns {
			if err := conn.writePing(); err != nil {
				failedConns = append(failedConns, conn)
			}
		}

		a.removeConns(failedConns)
	}
}

func (a *OracleAPI) removeConns(conns []*wsConn) {
	if len(conns) == 0 {
		return
	}
	a.wsMu.Lock()
	for _, conn := range conns {
		delete(a.wsConns, conn)
		conn.close()
	}
	a.wsMu.Unlock()
}

func (a *OracleAPI) getAllPools() []*PoolState {
	var merged []*PoolState
	for _, o := range a.oracles {
		merged = append(merged, o.GetAllPools()...)
	}
	return merged
}

func (a *OracleAPI) getPoolState(poolId string) (*PoolState, bool) {
	for _, o := range a.oracles {
		if pool, ok := o.GetPoolState(poolId); ok {
			return pool, true
		}
	}
	return nil, false
}

// WebSocketClientCount returns the number of connected WebSocket clients
func (a *OracleAPI) WebSocketClientCount() int {
	a.wsMu.RLock()
	defer a.wsMu.RUnlock()
	return len(a.wsConns)
}
