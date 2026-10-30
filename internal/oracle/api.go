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
	"container/heap"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/blinklabs-io/shai/internal/logging"
	shaiprice "github.com/blinklabs-io/shai/price"
	"github.com/gorilla/websocket"
)

const (
	wsReadLimit    = 512
	wsPongWait     = 60 * time.Second
	wsPingPeriod   = (wsPongWait * 9) / 10
	wsWriteTimeout = 10 * time.Second

	defaultOrderPageSize      = 50
	maxOrderPageSize          = 200
	maxOrderCursorLength      = 512
	maxOrderProtocolLength    = 128
	maxConcurrentOrderListOps = 4
	orderListWriteTimeout     = 10 * time.Second
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
	orderListOps  chan struct{}
}

// APIHandlerRegistrar registers API handlers on an HTTP mux.
type APIHandlerRegistrar interface {
	RegisterHandlers(*http.ServeMux)
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
			o.rebuildOrderIndex()
			filteredOracles = append(filteredOracles, o)
		}
	}
	return &OracleAPI{
		oracles:      filteredOracles,
		wsConns:      make(map[*wsConn]bool),
		stopChan:     make(chan struct{}),
		orderListOps: make(chan struct{}, maxConcurrentOrderListOps),
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
	if originHost == "localhost" || originHost == "127.0.0.1" ||
		originHost == "::1" {
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
	mux.HandleFunc(
		"GET /api/v1/pools/{poolId}/volume",
		a.HandleGetPoolVolume,
	)
	mux.HandleFunc("GET /api/v1/cdps", a.HandleListCDPs)
	mux.HandleFunc("GET /api/v1/cdps/{cdpId}", a.HandleGetCDP)
	mux.HandleFunc("GET /api/v1/orders", a.HandleListOrders)
	mux.HandleFunc("GET /api/v1/orders/{orderId}", a.HandleGetOrder)
	mux.HandleFunc("GET /api/v1/prices", a.HandleListPrices)
	mux.HandleFunc("GET /api/v1/prices/ada-usd", a.HandleADAUSDPrice)
	mux.HandleFunc("/ws/prices", a.HandlePriceStream)
	a.startBroadcastPriceUpdates()
}

// StartAPIServer starts an HTTP server with all supplied API handlers.
func StartAPIServer(
	addr string,
	registrars ...APIHandlerRegistrar,
) error {
	logger := logging.GetLogger()

	mux := http.NewServeMux()
	for _, registrar := range registrars {
		if registrar != nil {
			registrar.RegisterHandlers(mux)
		}
	}

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

// StartServer starts the HTTP server
func (a *OracleAPI) StartServer(addr string) error {
	return StartAPIServer(addr, a)
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

type apiErrorResponse struct {
	Code  string `json:"code"`
	Error string `json:"error"`
}

// HandleGetPoolVolume returns confirmed swap-shaped reserve turnover over the
// oracle's configured rolling slot window.
func (a *OracleAPI) HandleGetPoolVolume(
	w http.ResponseWriter,
	r *http.Request,
) {
	poolID := r.PathValue("poolId")
	if poolID == "" {
		a.writeJSON(w, http.StatusBadRequest, apiErrorResponse{
			Code:  "pool_id_required",
			Error: "pool ID required",
		})
		return
	}

	volume, found, available, err := a.getPoolVolume(poolID)
	switch {
	case err != nil:
		logging.GetLogger().Error(
			"failed to compute pool volume",
			"error", err,
			"poolId", poolID,
		)
		a.writeJSON(w, http.StatusInternalServerError, apiErrorResponse{
			Code:  "volume_error",
			Error: "failed to compute pool volume",
		})
	case !found:
		a.writeJSON(w, http.StatusNotFound, apiErrorResponse{
			Code:  "pool_not_found",
			Error: "pool not found",
		})
	case !available:
		a.writeJSON(w, http.StatusServiceUnavailable, apiErrorResponse{
			Code:  "volume_unavailable",
			Error: "confirmed pool volume is unavailable",
		})
	default:
		a.writeJSON(w, http.StatusOK, volume)
	}
}

// HandleListCDPs returns all tracked CDPs.
func (a *OracleAPI) HandleListCDPs(w http.ResponseWriter, r *http.Request) {
	cdps := a.getAllCDPs()

	protocol := r.URL.Query().Get("protocol")
	if protocol != "" {
		filtered := make([]*CDPState, 0)
		for _, cdp := range cdps {
			if cdp.Protocol == protocol {
				filtered = append(filtered, cdp)
			}
		}
		cdps = filtered
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"cdps":  cdps,
		"count": len(cdps),
	})
}

// HandleGetCDP returns a specific CDP by ID.
func (a *OracleAPI) HandleGetCDP(w http.ResponseWriter, r *http.Request) {
	cdpId := r.PathValue("cdpId")
	if cdpId == "" {
		http.Error(w, "CDP ID required", http.StatusBadRequest)
		return
	}

	cdp, ok := a.getCDPState(cdpId)
	if !ok {
		http.Error(w, "CDP not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(cdp)
}

type orderListResponse struct {
	Orders     []*OrderState `json:"orders"`
	Count      int           `json:"count"`
	NextCursor string        `json:"nextCursor,omitempty"`
}

type orderListCursor struct {
	filter string
	key    orderPageKey
}

type orderListHeap []orderListEntry

func (h orderListHeap) Len() int { return len(h) }
func (h orderListHeap) Less(i, j int) bool {
	return compareOrderPageKeys(h[i].key, h[j].key) > 0
}
func (h orderListHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h *orderListHeap) Push(value any) {
	*h = append(*h, value.(orderListEntry))
}
func (h *orderListHeap) Pop() any {
	old := *h
	last := old[len(old)-1]
	*h = old[:len(old)-1]
	return last
}

// HandleListOrders returns one stable page of tracked order-book orders.
func (a *OracleAPI) HandleListOrders(w http.ResponseWriter, r *http.Request) {
	controller := http.NewResponseController(w)
	if controller.SetWriteDeadline(
		time.Now().Add(orderListWriteTimeout),
	) == nil {
		defer func() { _ = controller.SetWriteDeadline(time.Time{}) }()
	}

	protocol := r.URL.Query().Get("protocol")
	if len(protocol) > maxOrderProtocolLength ||
		strings.ContainsRune(protocol, '\x00') {
		a.writeJSON(w, http.StatusBadRequest, apiErrorResponse{
			Code:  "invalid_protocol",
			Error: "protocol filter is invalid",
		})
		return
	}
	limit, err := orderPageLimit(r.URL.Query().Get("limit"))
	if err != nil {
		a.writeJSON(w, http.StatusBadRequest, apiErrorResponse{
			Code:  "invalid_limit",
			Error: err.Error(),
		})
		return
	}
	cursorValue := r.URL.Query().Get("cursor")
	cursor, err := decodeOrderListCursor(cursorValue)
	if err != nil ||
		(cursor != nil && (cursor.filter != protocol ||
			(protocol != "" && cursor.key.protocol != protocol))) {
		a.writeJSON(w, http.StatusBadRequest, apiErrorResponse{
			Code:  "invalid_cursor",
			Error: "cursor is invalid for this order list",
		})
		return
	}

	select {
	case a.orderListOps <- struct{}{}:
		defer func() { <-a.orderListOps }()
	default:
		w.Header().Set("Retry-After", "1")
		a.writeJSON(w, http.StatusServiceUnavailable, apiErrorResponse{
			Code:  "order_list_busy",
			Error: "order list is busy",
		})
		return
	}

	response, err := a.getOrderPage(r.Context(), protocol, cursor, limit)
	if err != nil {
		return
	}
	a.writeJSON(w, http.StatusOK, response)
}

// HandleGetOrder returns a specific order by ID.
func (a *OracleAPI) HandleGetOrder(w http.ResponseWriter, r *http.Request) {
	orderId := r.PathValue("orderId")
	if orderId == "" {
		http.Error(w, "order ID required", http.StatusBadRequest)
		return
	}

	order, ok := a.getOrderState(orderId)
	if !ok {
		http.Error(w, "order not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(order)
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

// HandleADAUSDPrice returns an ADA/USD estimate derived only from qualified
// locally tracked ADA/stablecoin DEX pools.
func (a *OracleAPI) HandleADAUSDPrice(
	w http.ResponseWriter,
	_ *http.Request,
) {
	result, err := shaiprice.AggregateADAUSD(
		a.getAllPools(),
		shaiprice.DefaultConfig(),
	)
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error":  err.Error(),
			"result": result,
		})
		return
	}
	_ = json.NewEncoder(w).Encode(result)
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

func (a *OracleAPI) getAllCDPs() []*CDPState {
	var merged []*CDPState
	for _, o := range a.oracles {
		merged = append(merged, o.GetAllCDPs()...)
	}
	return merged
}

func (a *OracleAPI) getOrderPage(
	ctx context.Context,
	protocol string,
	cursor *orderListCursor,
	limit int,
) (orderListResponse, error) {
	now := time.Now()
	candidates := make(orderListHeap, 0, limit+1)
	for oracleIndex, o := range a.oracles {
		var after *orderPageKey
		if cursor != nil {
			after = &cursor.key
		}
		entries, err := o.orderPage(
			ctx, protocol, after, oracleIndex, limit+1,
		)
		if err != nil {
			return orderListResponse{}, err
		}
		for _, entry := range entries {
			if candidates.Len() < limit+1 {
				heap.Push(&candidates, entry)
				continue
			}
			if compareOrderPageKeys(entry.key, candidates[0].key) < 0 {
				heap.Pop(&candidates)
				heap.Push(&candidates, entry)
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return orderListResponse{}, err
	}

	sort.Slice(candidates, func(i, j int) bool {
		return compareOrderPageKeys(candidates[i].key, candidates[j].key) < 0
	})

	hasMore := len(candidates) > limit
	if hasMore {
		candidates = candidates[:limit]
	}
	response := orderListResponse{
		Orders: make([]*OrderState, 0, len(candidates)),
		Count:  len(candidates),
	}
	for _, entry := range candidates {
		if err := ctx.Err(); err != nil {
			return orderListResponse{}, err
		}
		response.Orders = append(response.Orders, orderAt(entry.order, now))
	}
	if hasMore {
		response.NextCursor = encodeOrderListCursor(
			candidates[len(candidates)-1],
			protocol,
		)
	}
	return response, nil
}

func orderPageLimit(value string) (int, error) {
	if value == "" {
		return defaultOrderPageSize, nil
	}
	limit, err := strconv.Atoi(value)
	if err != nil || limit < 1 || limit > maxOrderPageSize {
		return 0, fmt.Errorf(
			"limit must be between 1 and %d",
			maxOrderPageSize,
		)
	}
	return limit, nil
}

func encodeOrderListCursor(entry orderListEntry, filter string) string {
	value := strings.Join([]string{
		filter,
		entry.key.protocol,
		entry.key.network,
		entry.key.orderID,
		strconv.Itoa(entry.key.oracleIndex),
	}, "\x00")
	return base64.RawURLEncoding.EncodeToString([]byte(value))
}

func decodeOrderListCursor(value string) (*orderListCursor, error) {
	if value == "" {
		return nil, nil
	}
	if len(value) > maxOrderCursorLength {
		return nil, fmt.Errorf("cursor is too long")
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, err
	}
	parts := strings.Split(string(decoded), "\x00")
	if len(parts) != 5 || parts[1] == "" || parts[3] == "" {
		return nil, fmt.Errorf("cursor has an invalid shape")
	}
	oracleIndex, err := strconv.Atoi(parts[4])
	if err != nil || oracleIndex < 0 {
		return nil, fmt.Errorf("cursor has an invalid oracle index")
	}
	return &orderListCursor{
		filter: parts[0],
		key: orderPageKey{
			orderIndexKey: orderIndexKey{
				protocol: parts[1],
				network:  parts[2],
				orderID:  parts[3],
			},
			oracleIndex: oracleIndex,
		},
	}, nil
}

func (a *OracleAPI) getOrderState(orderId string) (*OrderState, bool) {
	for _, o := range a.oracles {
		if order, ok := o.GetOrderState(orderId); ok {
			return orderAt(order, time.Now()), true
		}
	}
	return nil, false
}

// orderAt returns a copy of a tracked order with its activity re-evaluated at
// the serving time. A tracked order's IsActive is the value observed when the
// chain last produced it, and a time-bounded order starts or expires with no
// transaction to re-observe, so serving the stored value reports an expired
// order as fillable.
func orderAt(order *OrderState, now time.Time) *OrderState {
	if order == nil {
		return nil
	}
	served := *order
	served.IsActive = served.ActiveAt(now)
	return &served
}

func (a *OracleAPI) getPoolState(poolId string) (*PoolState, bool) {
	for _, o := range a.oracles {
		if pool, ok := o.GetPoolState(poolId); ok {
			return pool, true
		}
	}
	return nil, false
}

func (a *OracleAPI) getPoolVolume(
	poolID string,
) (PoolVolume, bool, bool, error) {
	for _, o := range a.oracles {
		state, found := o.GetPoolState(poolID)
		if !found || state == nil {
			continue
		}

		// Activity is advanced by all confirmed pool transitions handled by an
		// oracle, so evaluate at that oracle's newest locally tracked slot.
		atSlot := state.Slot
		for _, pool := range o.GetAllPools() {
			if pool != nil && !pool.FromMempool && pool.Slot > atSlot {
				atSlot = pool.Slot
			}
		}
		volume, available, err := o.GetPoolVolume(poolID, atSlot)
		return volume, true, available, err
	}
	return PoolVolume{}, false, false, nil
}

func (a *OracleAPI) writeJSON(
	w http.ResponseWriter,
	status int,
	value interface{},
) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		logging.GetLogger().Error(
			"failed to encode oracle API response",
			"error", err,
			"status", status,
		)
	}
}

func (a *OracleAPI) getCDPState(cdpId string) (*CDPState, bool) {
	for _, o := range a.oracles {
		if cdp, ok := o.GetCDPState(cdpId); ok {
			return cdp, true
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
