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
	"testing"

	"github.com/blinklabs-io/shai/internal/common"
	"github.com/gorilla/websocket"
)

func TestLendingOracleStopKeepsWebSocketMapInitialized(t *testing.T) {
	o := &LendingOracle{
		stopChan: make(chan struct{}),
		wsConns:  make(map[*websocket.Conn]bool),
	}

	o.Stop()

	if o.wsConns == nil {
		t.Fatal("expected websocket connection map to remain initialized")
	}
}

func TestLendingOracleGetStateRawIdAmbiguity(t *testing.T) {
	o := &LendingOracle{
		states: map[string]*LendingState{
			"mainnet:liqwid:duplicate": {
				StateId:  "duplicate",
				Protocol: "liqwid",
				Network:  "mainnet",
			},
			"preview:liqwid:duplicate": {
				StateId:  "duplicate",
				Protocol: "liqwid",
				Network:  "preview",
			},
			"mainnet:liqwid:unique": {
				StateId:  "unique",
				Protocol: "liqwid",
				Network:  "mainnet",
			},
		},
	}

	if _, ok := o.GetState("duplicate"); ok {
		t.Fatal("expected ambiguous raw state ID lookup to fail")
	}

	if state, ok := o.GetState("unique"); !ok || state.StateId != "unique" {
		t.Fatal("expected unique raw state ID lookup to succeed")
	}

	if state, ok := o.GetState("mainnet:liqwid:duplicate"); !ok ||
		state.Network != "mainnet" {
		t.Fatal("expected scoped state ID lookup to succeed")
	}
}

func TestMultiLendingOracleAPIRegistersRoutesForMultipleOracles(t *testing.T) {
	api := NewMultiLendingOracleAPI([]*LendingOracle{
		newTestLendingOracleWithStates(&LendingState{
			StateId:         "market-1",
			StateType:       LendingStateTypeMarket,
			Protocol:        "liqwid",
			Network:         "mainnet",
			UnderlyingAsset: common.Lovelace(),
			InterestRate:    200,
			InterestRatePct: 2.0,
		}),
		newTestLendingOracleWithStates(&LendingState{
			StateId:         "market-2",
			StateType:       LendingStateTypeMarket,
			Protocol:        "liqwid-alt",
			Network:         "mainnet",
			UnderlyingAsset: common.Lovelace(),
			InterestRate:    300,
			InterestRatePct: 3.0,
		}),
	})

	mux := http.NewServeMux()
	api.RegisterHandlers(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/lending/markets", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	var response struct {
		Markets []*LendingState `json:"markets"`
		Count   int             `json:"count"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if response.Count != 2 {
		t.Fatalf("expected count 2, got %d", response.Count)
	}
}

func TestMultiLendingOracleAPIGetMarketAcrossOracles(t *testing.T) {
	api := NewMultiLendingOracleAPI([]*LendingOracle{
		newTestLendingOracleWithStates(&LendingState{
			StateId:   "market-1",
			StateType: LendingStateTypeMarket,
			Protocol:  "liqwid",
			Network:   "mainnet",
		}),
		newTestLendingOracleWithStates(&LendingState{
			StateId:   "market-2",
			StateType: LendingStateTypeMarket,
			Protocol:  "liqwid-alt",
			Network:   "mainnet",
		}),
	})

	mux := http.NewServeMux()
	api.RegisterHandlers(mux)

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/lending/markets/mainnet:liqwid-alt:market-2",
		nil,
	)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	var state LendingState
	if err := json.Unmarshal(rr.Body.Bytes(), &state); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if state.StateId != "market-2" {
		t.Fatalf("expected market-2, got %s", state.StateId)
	}
}

func newTestLendingOracleWithStates(states ...*LendingState) *LendingOracle {
	stateMap := make(map[string]*LendingState, len(states))
	for _, state := range states {
		stateMap[state.Key()] = state
	}
	return &LendingOracle{
		states:   stateMap,
		stopChan: make(chan struct{}),
		wsConns:  make(map[*websocket.Conn]bool),
	}
}
