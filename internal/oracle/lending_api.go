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

// LendingOracleAPI provides HTTP and WebSocket endpoints for multiple lending
// oracle instances.
type LendingOracleAPI struct {
	oracles  []*LendingOracle
	upgrader websocket.Upgrader
	wsConns  map[*websocket.Conn]bool
	wsMu     sync.RWMutex
}

// NewLendingOracleAPI creates a lending API instance for one oracle.
func NewLendingOracleAPI(oracle *LendingOracle) *LendingOracleAPI {
	return NewMultiLendingOracleAPI([]*LendingOracle{oracle})
}

// NewMultiLendingOracleAPI creates a lending API instance for multiple oracles.
func NewMultiLendingOracleAPI(oracles []*LendingOracle) *LendingOracleAPI {
	filteredOracles := make([]*LendingOracle, 0, len(oracles))
	for _, o := range oracles {
		if o != nil {
			filteredOracles = append(filteredOracles, o)
		}
	}
	return &LendingOracleAPI{
		oracles: filteredOracles,
		wsConns: make(map[*websocket.Conn]bool),
		upgrader: websocket.Upgrader{
			CheckOrigin: checkWebSocketOrigin,
		},
	}
}

// RegisterHandlers registers HTTP handlers for lending data.
func (a *LendingOracleAPI) RegisterHandlers(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/lending/markets", a.HandleListMarkets)
	mux.HandleFunc("/api/v1/lending/markets/", a.HandleGetMarket)
	mux.HandleFunc("/api/v1/lending/loans", a.HandleListLoans)
	mux.HandleFunc("/api/v1/lending/loans/", a.HandleGetLoan)
	mux.HandleFunc("/api/v1/lending/rates", a.HandleListRates)
	mux.HandleFunc("/api/v1/lending/utilization", a.HandleListUtilization)
	mux.HandleFunc("/api/v1/lending/overdue", a.HandleListOverdueLoans)
	mux.HandleFunc("/ws/lending", a.HandleLendingStream)
}

// HandleListMarkets returns all lending markets.
func (a *LendingOracleAPI) HandleListMarkets(
	w http.ResponseWriter,
	r *http.Request,
) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	markets := a.getMarkets()

	protocol := r.URL.Query().Get("protocol")
	if protocol != "" {
		filtered := make([]*LendingState, 0, len(markets))
		for _, market := range markets {
			if market.Protocol == protocol {
				filtered = append(filtered, market)
			}
		}
		markets = filtered
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"markets": markets,
		"count":   len(markets),
	})
}

// HandleGetMarket returns a specific market by ID.
func (a *LendingOracleAPI) HandleGetMarket(
	w http.ResponseWriter,
	r *http.Request,
) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	marketId := strings.TrimPrefix(r.URL.Path, "/api/v1/lending/markets/")
	if marketId == "" {
		http.Error(w, "Market ID required", http.StatusBadRequest)
		return
	}

	state, ok := a.getState(marketId)
	if !ok || !state.IsMarket() {
		http.Error(w, "Market not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(state)
}

// HandleListLoans returns all loans.
func (a *LendingOracleAPI) HandleListLoans(
	w http.ResponseWriter,
	r *http.Request,
) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	loans := a.getLoans()

	status := r.URL.Query().Get("status")
	if status != "" {
		filtered := make([]*LendingState, 0, len(loans))
		for _, loan := range loans {
			if loan.LoanStatus == status {
				filtered = append(filtered, loan)
			}
		}
		loans = filtered
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"loans": loans,
		"count": len(loans),
	})
}

// HandleGetLoan returns a specific loan by ID.
func (a *LendingOracleAPI) HandleGetLoan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	loanId := strings.TrimPrefix(r.URL.Path, "/api/v1/lending/loans/")
	if loanId == "" {
		http.Error(w, "Loan ID required", http.StatusBadRequest)
		return
	}

	state, ok := a.getState(loanId)
	if !ok || !state.IsLoan() {
		http.Error(w, "Loan not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(state)
}

// HandleListRates returns interest rates for all markets.
func (a *LendingOracleAPI) HandleListRates(
	w http.ResponseWriter,
	r *http.Request,
) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	markets := a.getMarkets()

	type RateEntry struct {
		StateId      string  `json:"stateId"`
		Protocol     string  `json:"protocol"`
		Asset        string  `json:"asset"`
		InterestRate float64 `json:"interestRate"`
		BasisPoints  uint64  `json:"basisPoints"`
	}

	rates := make([]RateEntry, 0, len(markets))
	for _, market := range markets {
		rates = append(rates, RateEntry{
			StateId:      market.StateId,
			Protocol:     market.Protocol,
			Asset:        market.UnderlyingAsset.Fingerprint(),
			InterestRate: market.InterestRatePct,
			BasisPoints:  market.InterestRate,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"rates": rates,
		"count": len(rates),
	})
}

// HandleListUtilization returns utilization rates for all markets.
func (a *LendingOracleAPI) HandleListUtilization(
	w http.ResponseWriter,
	r *http.Request,
) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	markets := a.getMarkets()

	type UtilEntry struct {
		StateId         string  `json:"stateId"`
		Protocol        string  `json:"protocol"`
		Asset           string  `json:"asset"`
		UtilizationRate float64 `json:"utilizationRate"`
		TotalSupply     uint64  `json:"totalSupply"`
		TotalBorrows    uint64  `json:"totalBorrows"`
		Available       uint64  `json:"availableLiquidity"`
	}

	utils := make([]UtilEntry, 0, len(markets))
	for _, market := range markets {
		utils = append(utils, UtilEntry{
			StateId:         market.StateId,
			Protocol:        market.Protocol,
			Asset:           market.UnderlyingAsset.Fingerprint(),
			UtilizationRate: market.UtilizationRate,
			TotalSupply:     market.TotalSupply,
			TotalBorrows:    market.TotalBorrows,
			Available:       market.AvailableLiq,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"utilization": utils,
		"count":       len(utils),
	})
}

// HandleListOverdueLoans returns all overdue loans.
func (a *LendingOracleAPI) HandleListOverdueLoans(
	w http.ResponseWriter,
	r *http.Request,
) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	loans := a.getOverdueLoans()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"overdueLoans": loans,
		"count":        len(loans),
	})
}

// HandleLendingStream handles WebSocket connections for lending streaming.
func (a *LendingOracleAPI) HandleLendingStream(
	w http.ResponseWriter,
	r *http.Request,
) {
	logger := logging.GetLogger()

	conn, err := a.upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Error("WebSocket upgrade failed", "error", err)
		return
	}

	a.wsMu.Lock()
	a.wsConns[conn] = true
	a.wsMu.Unlock()

	logger.Debug(
		"Lending WebSocket client connected",
		"remote",
		conn.RemoteAddr(),
	)

	type subscription struct {
		oracle  *LendingOracle
		updates <-chan *LendingUpdate
	}

	done := make(chan struct{})
	subs := make([]subscription, 0, len(a.oracles))
	for _, o := range a.oracles {
		subs = append(subs, subscription{
			oracle:  o,
			updates: o.Subscribe(),
		})
	}

	defer func() {
		close(done)
		for _, sub := range subs {
			sub.oracle.Unsubscribe(sub.updates)
		}
		a.wsMu.Lock()
		delete(a.wsConns, conn)
		a.wsMu.Unlock()
		_ = conn.Close()
		logger.Debug(
			"Lending WebSocket client disconnected",
			"remote",
			conn.RemoteAddr(),
		)
	}()

	var writeMu sync.Mutex
	for _, sub := range subs {
		go func(updates <-chan *LendingUpdate) {
			for {
				select {
				case update, ok := <-updates:
					if !ok {
						return
					}
					writeMu.Lock()
					err := conn.WriteJSON(update)
					writeMu.Unlock()
					if err != nil {
						logger.Debug(
							"failed to send lending WebSocket update",
							"error", err,
							"remote", conn.RemoteAddr(),
						)
						_ = conn.Close()
						return
					}
				case <-done:
					return
				}
			}
		}(sub.updates)
	}

	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

// Stop closes active lending WebSocket connections.
func (a *LendingOracleAPI) Stop() {
	a.wsMu.Lock()
	defer a.wsMu.Unlock()
	for conn := range a.wsConns {
		_ = conn.Close()
		delete(a.wsConns, conn)
	}
}

// WebSocketClientCount returns the number of connected WebSocket clients.
func (a *LendingOracleAPI) WebSocketClientCount() int {
	a.wsMu.RLock()
	defer a.wsMu.RUnlock()
	return len(a.wsConns)
}

func (a *LendingOracleAPI) getMarkets() []*LendingState {
	var merged []*LendingState
	for _, o := range a.oracles {
		merged = append(merged, o.GetMarkets()...)
	}
	return merged
}

func (a *LendingOracleAPI) getLoans() []*LendingState {
	var merged []*LendingState
	for _, o := range a.oracles {
		merged = append(merged, o.GetLoans()...)
	}
	return merged
}

func (a *LendingOracleAPI) getOverdueLoans() []*LendingState {
	var merged []*LendingState
	for _, o := range a.oracles {
		merged = append(merged, o.GetOverdueLoans()...)
	}
	return merged
}

func (a *LendingOracleAPI) getState(stateId string) (*LendingState, bool) {
	for _, o := range a.oracles {
		o.statesMu.RLock()
		state, ok := o.states[stateId]
		o.statesMu.RUnlock()
		if ok {
			return state, true
		}
	}

	var matched *LendingState
	for _, o := range a.oracles {
		o.statesMu.RLock()
		for _, state := range o.states {
			if state.StateId == stateId {
				if matched != nil {
					o.statesMu.RUnlock()
					return nil, false
				}
				matched = state
			}
		}
		o.statesMu.RUnlock()
	}
	if matched != nil {
		return matched, true
	}
	return nil, false
}
