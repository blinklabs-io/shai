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

	"github.com/blinklabs-io/shai/common"
)

func TestLendingOracleStopWithoutAPIIsIdempotent(t *testing.T) {
	o := &LendingOracle{
		stopChan: make(chan struct{}),
	}

	o.Stop()
	o.Stop()

	if o.api != nil {
		t.Fatal("expected Stop without API usage to avoid creating API")
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

func TestLendingOracleRegisterHandlersDelegatesToAPI(t *testing.T) {
	o := newTestLendingOracleWithStates(&LendingState{
		StateId:         "market-1",
		StateType:       LendingStateTypeMarket,
		Protocol:        "liqwid",
		Network:         "mainnet",
		UnderlyingAsset: common.Lovelace(),
		InterestRate:    200,
		InterestRatePct: 2.0,
	})

	mux := http.NewServeMux()
	o.RegisterHandlers(mux)

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
	if response.Count != 1 {
		t.Fatalf("expected count 1, got %d", response.Count)
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

func TestLendingOracleAPIRegistersMethodRestrictedRoutes(t *testing.T) {
	api := NewMultiLendingOracleAPI(nil)
	mux := http.NewServeMux()
	api.RegisterHandlers(mux)

	for _, path := range []string{
		"/api/v1/lending/markets",
		"/api/v1/lending/markets/market-1",
		"/api/v1/lending/loans",
		"/api/v1/lending/loans/loan-1",
		"/api/v1/lending/rates",
		"/api/v1/lending/utilization",
		"/api/v1/lending/overdue",
		"/ws/lending",
	} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, path, nil)
			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, req)
			if rr.Code != http.StatusMethodNotAllowed {
				t.Fatalf("expected status 405, got %d", rr.Code)
			}
			if allow := rr.Header().Get("Allow"); !strings.Contains(allow, "GET") {
				t.Fatalf("expected Allow header to include GET, got %q", allow)
			}
		})
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

func TestLendingRatesAndUtilizationUseScopedStateIds(t *testing.T) {
	api := NewMultiLendingOracleAPI([]*LendingOracle{
		newTestLendingOracleWithStates(&LendingState{
			StateId:         "market-1",
			StateType:       LendingStateTypeMarket,
			Protocol:        "liqwid",
			Network:         "mainnet",
			UnderlyingAsset: common.Lovelace(),
			InterestRate:    200,
			InterestRatePct: 0.02,
			UtilizationRate: 0.5,
		}),
		newTestLendingOracleWithStates(&LendingState{
			StateId:         "market-1",
			StateType:       LendingStateTypeMarket,
			Protocol:        "other-lending",
			Network:         "mainnet",
			UnderlyingAsset: common.Lovelace(),
			InterestRate:    300,
			InterestRatePct: 0.03,
			UtilizationRate: 0.6,
		}),
	})

	mux := http.NewServeMux()
	api.RegisterHandlers(mux)

	for _, tc := range []struct {
		name     string
		path     string
		field    string
		expected map[string]bool
	}{
		{
			name:  "rates",
			path:  "/api/v1/lending/rates",
			field: "rates",
			expected: map[string]bool{
				"mainnet:liqwid:market-1":        true,
				"mainnet:other-lending:market-1": true,
			},
		},
		{
			name:  "utilization",
			path:  "/api/v1/lending/utilization",
			field: "utilization",
			expected: map[string]bool{
				"mainnet:liqwid:market-1":        true,
				"mainnet:other-lending:market-1": true,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, req)
			if rr.Code != http.StatusOK {
				t.Fatalf("expected status 200, got %d", rr.Code)
			}

			var response map[string]json.RawMessage
			if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}
			var entries []struct {
				StateId string `json:"stateId"`
			}
			if err := json.Unmarshal(response[tc.field], &entries); err != nil {
				t.Fatalf("failed to decode %s entries: %v", tc.field, err)
			}
			if len(entries) != len(tc.expected) {
				t.Fatalf(
					"expected %d entries, got %d",
					len(tc.expected),
					len(entries),
				)
			}
			for _, entry := range entries {
				if !tc.expected[entry.StateId] {
					t.Fatalf("unexpected stateId %q", entry.StateId)
				}
			}
		})
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
	}
}
