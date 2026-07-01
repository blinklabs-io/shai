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
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/blinklabs-io/adder/event"
	"github.com/blinklabs-io/shai/internal/common"
	"github.com/blinklabs-io/shai/internal/config"
	"github.com/blinklabs-io/shai/internal/indexer"
	"github.com/blinklabs-io/shai/internal/logging"
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
	api           *LendingOracleAPI
	apiMu         sync.Mutex
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
	}

	// Get addresses from parser
	o.addresses = parser.GetAddresses()

	// Also check profile config for additional addresses
	if cfg, ok := profile.Config.(config.LendingProfileConfig); ok {
		for _, addr := range cfg.MarketAddresses {
			o.addresses = append(o.addresses, addr.Address)
		}
		for _, addr := range cfg.OracleAddresses {
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

	o.stopAPI()

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
	ctx, ok := evt.Context.(event.TransactionContext)
	if !ok {
		logger.Error(
			"unexpected event context type",
			"expected", "event.TransactionContext",
			"got", fmt.Sprintf("%T", evt.Context),
		)
		return nil
	}
	cfg := config.GetConfig()

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
		if state == nil {
			continue
		}

		// Set additional metadata
		state.Network = cfg.Network
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

	// Fall back to searching by raw state ID. If multiple scoped states share
	// the same raw ID, the unscoped lookup is ambiguous and should not return
	// an arbitrary map iteration result.
	var matched *LendingState
	for _, state := range o.states {
		if state.StateId == stateId {
			if matched != nil {
				return nil, false
			}
			matched = state
		}
	}
	if matched != nil {
		return matched, true
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

// RegisterHandlers registers HTTP handlers for lending data
func (o *LendingOracle) RegisterHandlers(mux *http.ServeMux) {
	o.lendingAPI().RegisterHandlers(mux)
}

// HandleListMarkets returns all lending markets
func (o *LendingOracle) HandleListMarkets(
	w http.ResponseWriter,
	r *http.Request,
) {
	o.lendingAPI().HandleListMarkets(w, r)
}

// HandleGetMarket returns a specific market by ID
func (o *LendingOracle) HandleGetMarket(
	w http.ResponseWriter,
	r *http.Request,
) {
	o.lendingAPI().HandleGetMarket(w, r)
}

// HandleListLoans returns all loans
func (o *LendingOracle) HandleListLoans(
	w http.ResponseWriter,
	r *http.Request,
) {
	o.lendingAPI().HandleListLoans(w, r)
}

// HandleGetLoan returns a specific loan by ID
func (o *LendingOracle) HandleGetLoan(w http.ResponseWriter, r *http.Request) {
	o.lendingAPI().HandleGetLoan(w, r)
}

// HandleListRates returns interest rates for all markets
func (o *LendingOracle) HandleListRates(
	w http.ResponseWriter,
	r *http.Request,
) {
	o.lendingAPI().HandleListRates(w, r)
}

// HandleListUtilization returns utilization rates for all markets
func (o *LendingOracle) HandleListUtilization(
	w http.ResponseWriter,
	r *http.Request,
) {
	o.lendingAPI().HandleListUtilization(w, r)
}

// HandleListOverdueLoans returns all overdue loans
func (o *LendingOracle) HandleListOverdueLoans(
	w http.ResponseWriter,
	r *http.Request,
) {
	o.lendingAPI().HandleListOverdueLoans(w, r)
}

// HandleLendingStream handles WebSocket connections for lending streaming
func (o *LendingOracle) HandleLendingStream(
	w http.ResponseWriter,
	r *http.Request,
) {
	o.lendingAPI().HandleLendingStream(w, r)
}

// StartAPIServer starts the lending API server
func (o *LendingOracle) StartAPIServer(addr string) error {
	return StartAPIServer(addr, o.lendingAPI())
}

func (o *LendingOracle) lendingAPI() *LendingOracleAPI {
	o.apiMu.Lock()
	defer o.apiMu.Unlock()
	if o.api == nil {
		o.api = NewLendingOracleAPI(o)
	}
	return o.api
}

func (o *LendingOracle) stopAPI() {
	o.apiMu.Lock()
	api := o.api
	o.apiMu.Unlock()
	if api != nil {
		api.Stop()
	}
}
