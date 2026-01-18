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
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/blinklabs-io/adder/event"
	"github.com/blinklabs-io/shai/internal/common"
	"github.com/blinklabs-io/shai/internal/config"
	"github.com/blinklabs-io/shai/internal/indexer"
	"github.com/blinklabs-io/shai/internal/logging"
	"github.com/gorilla/websocket"
)

// LendingStateType indicates the type of lending state
type LendingStateType int

const (
	LendingStateTypeMarket    LendingStateType = iota // Pool-based lending (Liqwid)
	LendingStateTypeLoan                              // Individual loan (Levvy P2P)
	LendingStateTypeLoanOffer                         // Loan offer (Levvy P2P)
	LendingStateTypePosition                          // User position (supply/borrow)
)

// LendingState represents a unified lending protocol state
// This provides a common interface for different lending protocols
type LendingState struct {
	// Identification
	StateId   string           `json:"stateId"`
	StateType LendingStateType `json:"stateType"`
	Protocol  string           `json:"protocol"`
	Network   string           `json:"network"`

	// Market/Pool State (for pool-based lending like Liqwid)
	TotalSupply      uint64  `json:"totalSupply,omitempty"`
	TotalBorrows     uint64  `json:"totalBorrows,omitempty"`
	AvailableLiq     uint64  `json:"availableLiquidity,omitempty"`
	UtilizationRate  float64 `json:"utilizationRate,omitempty"`
	InterestRate     uint64  `json:"interestRate,omitempty"`        // basis points
	CollateralFactor uint64  `json:"collateralFactor,omitempty"`    // basis points
	InterestRatePct  float64 `json:"interestRatePercent,omitempty"` // decimal

	// Asset Information
	UnderlyingAsset common.AssetClass `json:"underlyingAsset,omitempty"`
	CollateralAsset common.AssetClass `json:"collateralAsset,omitempty"`
	LpToken         common.AssetClass `json:"lpToken,omitempty"`

	// Loan State (for P2P lending like Levvy)
	LoanAmount      uint64        `json:"loanAmount,omitempty"`
	RepaymentAmount uint64        `json:"repaymentAmount,omitempty"`
	StartTime       time.Time     `json:"startTime,omitempty"`
	DueDate         time.Time     `json:"dueDate,omitempty"`
	Duration        time.Duration `json:"duration,omitempty"`
	IsOverdue       bool          `json:"isOverdue,omitempty"`
	CanLiquidate    bool          `json:"canLiquidate,omitempty"`
	LoanStatus      string        `json:"loanStatus,omitempty"` // active, repaid, liquidated

	// Participant Information
	Lender   string `json:"lender,omitempty"`
	Borrower string `json:"borrower,omitempty"`

	// Transaction Metadata
	Slot      uint64    `json:"slot"`
	TxHash    string    `json:"txHash"`
	TxIndex   uint32    `json:"txIndex"`
	Timestamp time.Time `json:"timestamp"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Key returns a unique identifier for this lending state
func (s *LendingState) Key() string {
	return fmt.Sprintf("%s:%s:%s", s.Network, s.Protocol, s.StateId)
}

// IsMarket returns true if this is a market/pool state
func (s *LendingState) IsMarket() bool {
	return s.StateType == LendingStateTypeMarket
}

// IsLoan returns true if this is an individual loan state
func (s *LendingState) IsLoan() bool {
	return s.StateType == LendingStateTypeLoan
}

// IsLoanOffer returns true if this is a loan offer state
func (s *LendingState) IsLoanOffer() bool {
	return s.StateType == LendingStateTypeLoanOffer
}

// LendingUpdate represents a lending state change event
type LendingUpdate struct {
	StateId         string           `json:"stateId"`
	StateType       LendingStateType `json:"stateType"`
	Protocol        string           `json:"protocol"`
	UtilizationRate float64          `json:"utilizationRate,omitempty"`
	InterestRate    float64          `json:"interestRate,omitempty"`
	TotalSupply     uint64           `json:"totalSupply,omitempty"`
	TotalBorrows    uint64           `json:"totalBorrows,omitempty"`
	LoanStatus      string           `json:"loanStatus,omitempty"`
	Slot            uint64           `json:"slot"`
	Timestamp       time.Time        `json:"timestamp"`
}

// NewLendingUpdate creates a LendingUpdate from a LendingState
func NewLendingUpdate(state *LendingState) *LendingUpdate {
	return &LendingUpdate{
		StateId:         state.StateId,
		StateType:       state.StateType,
		Protocol:        state.Protocol,
		UtilizationRate: state.UtilizationRate,
		InterestRate:    state.InterestRatePct,
		TotalSupply:     state.TotalSupply,
		TotalBorrows:    state.TotalBorrows,
		LoanStatus:      state.LoanStatus,
		Slot:            state.Slot,
		Timestamp:       state.Timestamp,
	}
}

// LendingParser is the interface for lending protocol parsers
// This is different from PoolParser as lending protocols have distinct structures
type LendingParser interface {
	// Protocol returns the name of the protocol
	Protocol() string

	// ParseDatum parses a lending protocol datum and returns the state
	// This is the unified entry point for all lending datum types
	ParseDatum(
		datum []byte,
		txHash string,
		txIndex uint32,
		slot uint64,
		timestamp time.Time,
	) (*LendingState, error)

	// GetAddresses returns the addresses this parser monitors
	GetAddresses() []string
}

// LendingOracle tracks lending protocol states
type LendingOracle struct {
	idx           *indexer.Indexer
	profile       *config.Profile
	parser        LendingParser
	states        map[string]*LendingState
	statesMu      sync.RWMutex
	subscribers   []chan *LendingUpdate
	subscribersMu sync.RWMutex
	addresses     []string
	storage       *LendingStorage
	stopChan      chan struct{}
	stopped       bool
	upgrader      websocket.Upgrader
	wsConns       map[*websocket.Conn]bool
	wsMu          sync.RWMutex
}

// NewLendingOracle creates a new LendingOracle instance
func NewLendingOracle(
	idx *indexer.Indexer,
	profile *config.Profile,
	parser LendingParser,
) *LendingOracle {
	o := &LendingOracle{
		idx:      idx,
		profile:  profile,
		parser:   parser,
		states:   make(map[string]*LendingState),
		stopChan: make(chan struct{}),
		wsConns:  make(map[*websocket.Conn]bool),
		upgrader: websocket.Upgrader{
			CheckOrigin: checkWebSocketOrigin,
		},
	}

	// Get addresses from parser
	o.addresses = parser.GetAddresses()

	// Also check profile config for additional addresses
	if cfg, ok := profile.Config.(config.LendingProfileConfig); ok {
		for _, addr := range cfg.MarketAddresses {
			o.addresses = append(o.addresses, addr.Address)
		}
	}

	return o
}

// Start begins tracking lending protocol states
func (o *LendingOracle) Start() error {
	logger := logging.GetLogger()

	// Initialize storage
	var err error
	o.storage, err = NewLendingStorage()
	if err != nil {
		return err
	}

	// Load persisted states
	if err := o.loadPersistedStates(); err != nil {
		logger.Warn("failed to load persisted lending states", "error", err)
	}

	// Register event handler with indexer
	o.idx.AddEventFunc(o.HandleChainsyncEvent)

	logger.Info(
		"LendingOracle started",
		"profile", o.profile.Name,
		"protocol", o.parser.Protocol(),
		"addresses", len(o.addresses),
	)

	return nil
}

// Stop stops the lending oracle (idempotent - safe to call multiple times)
func (o *LendingOracle) Stop() {
	o.statesMu.Lock()
	if o.stopped {
		o.statesMu.Unlock()
		return
	}
	o.stopped = true
	o.statesMu.Unlock()

	close(o.stopChan)

	// Close all subscriber channels
	o.subscribersMu.Lock()
	for _, ch := range o.subscribers {
		close(ch)
	}
	o.subscribers = nil
	o.subscribersMu.Unlock()

	// Close WebSocket connections
	o.wsMu.Lock()
	for conn := range o.wsConns {
		_ = conn.Close()
	}
	o.wsConns = nil
	o.wsMu.Unlock()

	// Close storage
	if o.storage != nil {
		_ = o.storage.Close()
	}
}

// HandleChainsyncEvent processes chain sync events
func (o *LendingOracle) HandleChainsyncEvent(evt event.Event) error {
	switch payload := evt.Payload.(type) {
	case event.TransactionEvent:
		return o.handleTransaction(evt, payload)
	case event.RollbackEvent:
		return o.handleRollback(payload)
	}
	return nil
}

// handleTransaction processes a transaction event
func (o *LendingOracle) handleTransaction(
	evt event.Event,
	txEvt event.TransactionEvent,
) error {
	logger := logging.GetLogger()
	ctx := evt.Context.(event.TransactionContext)

	// Check for lending UTxOs at monitored addresses
	for _, utxo := range txEvt.Transaction.Produced() {
		addr := utxo.Output.Address().String()
		if !o.isLendingAddress(addr) {
			continue
		}

		// Try to parse datum
		if utxo.Output.Datum() == nil {
			continue
		}

		// Parse the lending state using the protocol-specific parser
		timestamp := time.Now()
		state, err := o.parser.ParseDatum(
			utxo.Output.Datum().Cbor(),
			ctx.TransactionHash,
			utxo.Id.Index(),
			ctx.SlotNumber,
			timestamp,
		)
		if err != nil {
			// Not a valid lending datum for this protocol
			continue
		}

		// Set additional metadata
		state.Network = o.profile.Name
		state.UpdatedAt = time.Now()

		// Update state using scoped key (network:protocol:stateId)
		o.statesMu.Lock()
		o.states[state.Key()] = state
		o.statesMu.Unlock()

		// Persist to storage
		if o.storage != nil {
			if err := o.storage.SaveLendingState(state); err != nil {
				logger.Error(
					"failed to persist lending state",
					"error", err,
					"stateId", state.StateId,
				)
			}
		}

		// Notify subscribers
		update := NewLendingUpdate(state)
		o.notifySubscribers(update)

		logger.Debug(
			"lending state updated",
			"stateId", state.StateId,
			"protocol", state.Protocol,
			"type", state.StateType,
			"slot", state.Slot,
		)
	}

	return nil
}

// handleRollback processes a rollback event
func (o *LendingOracle) handleRollback(evt event.RollbackEvent) error {
	logger := logging.GetLogger()
	logger.Warn(
		"rollback detected in lending oracle",
		"slot", evt.SlotNumber,
		"blockHash", evt.BlockHash,
	)
	return nil
}

// isLendingAddress checks if an address is a monitored lending address
func (o *LendingOracle) isLendingAddress(addr string) bool {
	for _, lendingAddr := range o.addresses {
		if addr == lendingAddr {
			return true
		}
	}
	return false
}

// loadPersistedStates loads lending states from storage
func (o *LendingOracle) loadPersistedStates() error {
	if o.storage == nil {
		return nil
	}

	states, err := o.storage.LoadAllLendingStates()
	if err != nil {
		return err
	}

	o.statesMu.Lock()
	for _, state := range states {
		o.states[state.Key()] = state
	}
	o.statesMu.Unlock()

	logger := logging.GetLogger()
	logger.Info("loaded persisted lending states", "count", len(states))

	return nil
}

// notifySubscribers sends an update to all subscribers
func (o *LendingOracle) notifySubscribers(update *LendingUpdate) {
	o.subscribersMu.RLock()
	defer o.subscribersMu.RUnlock()

	for _, ch := range o.subscribers {
		select {
		case ch <- update:
		default:
			// Channel full, skip
		}
	}
}

// Subscribe returns a channel that receives lending updates
func (o *LendingOracle) Subscribe() <-chan *LendingUpdate {
	ch := make(chan *LendingUpdate, 100)

	o.subscribersMu.Lock()
	o.subscribers = append(o.subscribers, ch)
	o.subscribersMu.Unlock()

	return ch
}

// Unsubscribe removes a subscription channel
func (o *LendingOracle) Unsubscribe(ch <-chan *LendingUpdate) {
	o.subscribersMu.Lock()
	defer o.subscribersMu.Unlock()

	for i, sub := range o.subscribers {
		if sub == ch {
			o.subscribers = append(o.subscribers[:i], o.subscribers[i+1:]...)
			close(sub)
			break
		}
	}
}

// GetState returns a lending state by ID
// The stateId can be either the raw state ID or the scoped key (network:protocol:stateId)
func (o *LendingOracle) GetState(stateId string) (*LendingState, bool) {
	o.statesMu.RLock()
	defer o.statesMu.RUnlock()

	// First try exact match (for scoped keys)
	if state, ok := o.states[stateId]; ok {
		return state, true
	}

	// Fall back to searching by raw state ID
	for _, state := range o.states {
		if state.StateId == stateId {
			return state, true
		}
	}
	return nil, false
}

// GetAllStates returns all tracked lending states
func (o *LendingOracle) GetAllStates() []*LendingState {
	o.statesMu.RLock()
	defer o.statesMu.RUnlock()

	states := make([]*LendingState, 0, len(o.states))
	for _, state := range o.states {
		states = append(states, state)
	}
	return states
}

// GetMarkets returns all market/pool states
func (o *LendingOracle) GetMarkets() []*LendingState {
	o.statesMu.RLock()
	defer o.statesMu.RUnlock()

	markets := make([]*LendingState, 0)
	for _, state := range o.states {
		if state.IsMarket() {
			markets = append(markets, state)
		}
	}
	return markets
}

// GetLoans returns all loan states
func (o *LendingOracle) GetLoans() []*LendingState {
	o.statesMu.RLock()
	defer o.statesMu.RUnlock()

	loans := make([]*LendingState, 0)
	for _, state := range o.states {
		if state.IsLoan() {
			loans = append(loans, state)
		}
	}
	return loans
}

// GetActiveLoans returns all active loan states
func (o *LendingOracle) GetActiveLoans() []*LendingState {
	o.statesMu.RLock()
	defer o.statesMu.RUnlock()

	loans := make([]*LendingState, 0)
	for _, state := range o.states {
		if state.IsLoan() && state.LoanStatus == "active" {
			loans = append(loans, state)
		}
	}
	return loans
}

// GetOverdueLoans returns all overdue loan states
func (o *LendingOracle) GetOverdueLoans() []*LendingState {
	o.statesMu.RLock()
	defer o.statesMu.RUnlock()

	loans := make([]*LendingState, 0)
	for _, state := range o.states {
		if state.IsLoan() && state.IsOverdue {
			loans = append(loans, state)
		}
	}
	return loans
}

// StateCount returns the number of tracked states
func (o *LendingOracle) StateCount() int {
	o.statesMu.RLock()
	defer o.statesMu.RUnlock()
	return len(o.states)
}

// GetUtilization returns the utilization rate for a market
func (o *LendingOracle) GetUtilization(stateId string) (float64, bool) {
	o.statesMu.RLock()
	defer o.statesMu.RUnlock()

	if state, ok := o.states[stateId]; ok && state.IsMarket() {
		return state.UtilizationRate, true
	}
	return 0, false
}

// GetInterestRate returns the interest rate for a market
func (o *LendingOracle) GetInterestRate(stateId string) (float64, bool) {
	o.statesMu.RLock()
	defer o.statesMu.RUnlock()

	if state, ok := o.states[stateId]; ok {
		return state.InterestRatePct, true
	}
	return 0, false
}

// API Handlers

// RegisterHandlers registers HTTP handlers for lending data
func (o *LendingOracle) RegisterHandlers(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/lending/markets", o.HandleListMarkets)
	mux.HandleFunc("/api/v1/lending/markets/", o.HandleGetMarket)
	mux.HandleFunc("/api/v1/lending/loans", o.HandleListLoans)
	mux.HandleFunc("/api/v1/lending/loans/", o.HandleGetLoan)
	mux.HandleFunc("/api/v1/lending/rates", o.HandleListRates)
	mux.HandleFunc("/api/v1/lending/utilization", o.HandleListUtilization)
	mux.HandleFunc("/api/v1/lending/overdue", o.HandleListOverdueLoans)
	mux.HandleFunc("/ws/lending", o.HandleLendingStream)
}

// HandleListMarkets returns all lending markets
func (o *LendingOracle) HandleListMarkets(
	w http.ResponseWriter,
	r *http.Request,
) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	markets := o.GetMarkets()

	// Filter by protocol if specified
	protocol := r.URL.Query().Get("protocol")
	if protocol != "" {
		filtered := make([]*LendingState, 0)
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

// HandleGetMarket returns a specific market by ID
func (o *LendingOracle) HandleGetMarket(
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

	state, ok := o.GetState(marketId)
	if !ok || !state.IsMarket() {
		http.Error(w, "Market not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(state)
}

// HandleListLoans returns all loans
func (o *LendingOracle) HandleListLoans(
	w http.ResponseWriter,
	r *http.Request,
) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	loans := o.GetLoans()

	// Filter by status if specified
	status := r.URL.Query().Get("status")
	if status != "" {
		filtered := make([]*LendingState, 0)
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

// HandleGetLoan returns a specific loan by ID
func (o *LendingOracle) HandleGetLoan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	loanId := strings.TrimPrefix(r.URL.Path, "/api/v1/lending/loans/")
	if loanId == "" {
		http.Error(w, "Loan ID required", http.StatusBadRequest)
		return
	}

	state, ok := o.GetState(loanId)
	if !ok || !state.IsLoan() {
		http.Error(w, "Loan not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(state)
}

// HandleListRates returns interest rates for all markets
func (o *LendingOracle) HandleListRates(
	w http.ResponseWriter,
	r *http.Request,
) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	markets := o.GetMarkets()

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

// HandleListUtilization returns utilization rates for all markets
func (o *LendingOracle) HandleListUtilization(
	w http.ResponseWriter,
	r *http.Request,
) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	markets := o.GetMarkets()

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

// HandleListOverdueLoans returns all overdue loans
func (o *LendingOracle) HandleListOverdueLoans(
	w http.ResponseWriter,
	r *http.Request,
) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	loans := o.GetOverdueLoans()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"overdueLoans": loans,
		"count":        len(loans),
	})
}

// HandleLendingStream handles WebSocket connections for lending streaming
func (o *LendingOracle) HandleLendingStream(
	w http.ResponseWriter,
	r *http.Request,
) {
	logger := logging.GetLogger()

	conn, err := o.upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Error("WebSocket upgrade failed", "error", err)
		return
	}

	// Register connection
	o.wsMu.Lock()
	o.wsConns[conn] = true
	o.wsMu.Unlock()

	logger.Debug(
		"Lending WebSocket client connected",
		"remote",
		conn.RemoteAddr(),
	)

	// Keep connection alive and handle disconnection
	defer func() {
		o.wsMu.Lock()
		delete(o.wsConns, conn)
		o.wsMu.Unlock()
		_ = conn.Close()
		logger.Debug(
			"Lending WebSocket client disconnected",
			"remote",
			conn.RemoteAddr(),
		)
	}()

	// Start update broadcaster for this connection
	updates := o.Subscribe()
	defer o.Unsubscribe(updates)

	// Broadcast updates to this connection
	go func() {
		for update := range updates {
			if err := conn.WriteJSON(update); err != nil {
				logger.Debug(
					"failed to send lending WebSocket update",
					"error", err,
					"remote", conn.RemoteAddr(),
				)
				return
			}
		}
	}()

	// Read messages (for ping/pong and close handling)
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

// StartAPIServer starts the lending API server
func (o *LendingOracle) StartAPIServer(addr string) error {
	logger := logging.GetLogger()

	mux := http.NewServeMux()
	o.RegisterHandlers(mux)

	logger.Info("starting lending API server", "addr", addr)
	return http.ListenAndServe(addr, mux)
}
