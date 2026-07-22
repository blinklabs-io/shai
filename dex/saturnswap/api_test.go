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

package saturnswap

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewClientUsesDocumentedEndpointWhenEnabled(t *testing.T) {
	t.Parallel()

	client, err := NewClient(APIConfig{Enabled: true})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	if client.Endpoint() != DefaultGraphQLEndpoint {
		t.Fatalf(
			"unexpected endpoint: got %q want %q",
			client.Endpoint(),
			DefaultGraphQLEndpoint,
		)
	}
}

func TestPoolsByTickerAndPoolStateParsing(t *testing.T) {
	t.Parallel()

	policyID := strings.Repeat("ab", 28)
	assetName := "534e454b"

	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			req, ok := readGraphQLRequest(t, w, r)
			if !ok {
				return
			}
			if !strings.Contains(req.Query, "pools(where: { ticker: { eq: $ticker } })") {
				failTestHandler(t, w, "query missing ticker filter: %s", req.Query)
				return
			}
			vars, ok := graphqlVariables(t, w, req)
			if !ok {
				return
			}
			if vars["ticker"] != "SNEK" {
				failTestHandler(t, w, "unexpected ticker variable: %#v", vars["ticker"])
				return
			}

			writeJSON(t, w, map[string]any{
				"data": map[string]any{
					"pools": map[string]any{
						"nodes": []map[string]any{
							{
								"id":                   "pool-1",
								"name":                 "ADA x SNEK",
								"ticker":               "SNEK",
								"lp_fee_percent":       "0.3",
								"protocol_fee_percent": "0.05",
								"is_swap_active":       true,
								"is_verified":          true,
								"token_project_one": map[string]any{
									"id":       "ada",
									"name":     "Cardano",
									"ticker":   "ADA",
									"decimals": float64(6),
								},
								"token_project_two": map[string]any{
									"id":         "snek",
									"name":       "Snek",
									"ticker":     "SNEK",
									"policy_id":  policyID,
									"asset_name": assetName,
									"decimals":   float64(0),
								},
								"pool_stats": map[string]any{
									"pool_id":           "pool-1",
									"reserve_token_one": "5000000",
									"reserve_token_two": "123456789",
									"tvl":               "128456789",
								},
							},
						},
						"totalCount": float64(1),
					},
				},
			})
		},
	))
	t.Cleanup(server.Close)

	client := newEnabledTestClient(t, server.URL)
	pools, err := client.PoolsByTicker(context.Background(), "SNEK")
	if err != nil {
		t.Fatalf("PoolsByTicker failed: %v", err)
	}
	if len(pools) != 1 {
		t.Fatalf("unexpected pool count: got %d want 1", len(pools))
	}
	if pools[0].ID != "pool-1" {
		t.Fatalf("unexpected pool ID: %q", pools[0].ID)
	}

	ts := time.Unix(1700000000, 0).UTC()
	state, err := pools[0].ToPoolState(12345, ts)
	if err != nil {
		t.Fatalf("ToPoolState failed: %v", err)
	}
	if state.Protocol != ProtocolName {
		t.Fatalf("unexpected protocol: %q", state.Protocol)
	}
	if !state.AssetX.Class.IsLovelace() {
		t.Fatalf("expected ADA asset X, got %s", state.AssetX.Class.String())
	}
	if got := state.AssetY.Class.PolicyIdHex(); got != policyID {
		t.Fatalf("unexpected token policy: got %q want %q", got, policyID)
	}
	if got := state.AssetY.Class.NameHex(); got != assetName {
		t.Fatalf("unexpected token asset name: got %q want %q", got, assetName)
	}
	if state.AssetX.Amount != 5_000_000 {
		t.Fatalf("unexpected reserve X: %d", state.AssetX.Amount)
	}
	if state.AssetY.Amount != 123_456_789 {
		t.Fatalf("unexpected reserve Y: %d", state.AssetY.Amount)
	}
	if state.FeeNum != 9965 || state.FeeDenom != FeeDenom {
		t.Fatalf(
			"unexpected fee parts: got %d/%d want 9965/%d",
			state.FeeNum,
			state.FeeDenom,
			FeeDenom,
		)
	}
}

func TestPoolByTokens(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			req, ok := readGraphQLRequest(t, w, r)
			if !ok {
				return
			}
			if !strings.Contains(req.Query, "poolByTokens(input: $input)") {
				failTestHandler(t, w, "query missing poolByTokens call: %s", req.Query)
				return
			}
			vars, ok := graphqlVariables(t, w, req)
			if !ok {
				return
			}
			input, ok := vars["input"].(map[string]any)
			if !ok {
				failTestHandler(t, w, "unexpected input shape: %#v", vars["input"])
				return
			}
			if input["policyIdTwo"] != "abcd" || input["assetNameTwo"] != "746f6b656e" {
				failTestHandler(t, w, "unexpected input: %#v", input)
				return
			}

			writeJSON(t, w, map[string]any{
				"data": map[string]any{
					"poolByTokens": map[string]any{
						"id":                   "pool-by-token",
						"name":                 "ADA x TOKEN",
						"lp_fee_percent":       "0.25",
						"protocol_fee_percent": "0",
						"token_project_one": map[string]any{
							"ticker": "ADA",
						},
						"token_project_two": map[string]any{
							"ticker":     "TOKEN",
							"policy_id":  "abcd",
							"asset_name": "746f6b656e",
						},
						"pool_stats": map[string]any{
							"reserve_token_one": "1000000",
							"reserve_token_two": "2",
						},
					},
				},
			})
		},
	))
	t.Cleanup(server.Close)

	client := newEnabledTestClient(t, server.URL)
	pool, err := client.PoolByTokens(
		context.Background(),
		PoolByTokensInput{
			PolicyIDTwo:  "abcd",
			AssetNameTwo: "746f6b656e",
		},
	)
	if err != nil {
		t.Fatalf("PoolByTokens failed: %v", err)
	}
	if pool.ID != "pool-by-token" {
		t.Fatalf("unexpected pool ID: %q", pool.ID)
	}
}

func TestPoolByTokensRejectsInvalidInputBeforeRequest(t *testing.T) {
	t.Parallel()

	var requestReceived atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			requestReceived.Store(true)
			http.Error(w, "unexpected request", http.StatusInternalServerError)
		},
	))
	t.Cleanup(server.Close)

	client := newEnabledTestClient(t, server.URL)
	tests := []struct {
		name    string
		input   PoolByTokensInput
		wantErr string
	}{
		{
			name:    "empty pair",
			wantErr: "tokens must be different",
		},
		{
			name: "token one asset without policy",
			input: PoolByTokensInput{
				AssetNameOne: "746f6b656e",
			},
			wantErr: "token one asset name requires a policy ID",
		},
		{
			name: "token two asset without policy",
			input: PoolByTokensInput{
				AssetNameTwo: "746f6b656e",
			},
			wantErr: "token two asset name requires a policy ID",
		},
		{
			name: "same native token",
			input: PoolByTokensInput{
				PolicyIDOne:  "abcd",
				AssetNameOne: "746f6b656e",
				PolicyIDTwo:  "abcd",
				AssetNameTwo: "746f6b656e",
			},
			wantErr: "tokens must be different",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := client.PoolByTokens(context.Background(), test.input)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("expected %q error, got %v", test.wantErr, err)
			}
		})
	}
	if requestReceived.Load() {
		t.Fatal("invalid input caused an HTTP request")
	}
}

func TestSubmitOrderTransactionReturnsAPIError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			req, ok := readGraphQLRequest(t, w, r)
			if !ok {
				return
			}
			if !strings.Contains(req.Query, "submitOrderTransaction(input: $input)") {
				failTestHandler(
					t,
					w,
					"query missing submitOrderTransaction call: %s",
					req.Query,
				)
				return
			}

			writeJSON(t, w, map[string]any{
				"data": map[string]any{
					"submitOrderTransaction": map[string]any{
						"transactionIds": []string{},
						"error": map[string]any{
							"message": "book moved",
							"code":    "Slippage",
							"link":    "https://docs.fluxpointstudios.com/saturnswap/api-integration",
						},
					},
				},
			})
		},
	))
	t.Cleanup(server.Close)

	client := newEnabledTestClient(t, server.URL)
	result, err := client.SubmitOrderTransaction(
		context.Background(),
		SubmitOrderTransactionInput{
			PaymentAddress: "addr1user",
			SuccessTransactions: []OrderTransaction{
				{
					TransactionID:  "tx-id",
					HexTransaction: "84a400",
				},
			},
		},
	)
	if result == nil {
		t.Fatal("expected result with API error")
	}
	var apiErr *SaturnAPIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected SaturnAPIError, got %T: %v", err, err)
	}
	if apiErr.Code != "Slippage" {
		t.Fatalf("unexpected API error code: %q", apiErr.Code)
	}
}

func TestGraphQLErrorReturned(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if _, ok := readGraphQLRequest(t, w, r); !ok {
				return
			}
			writeJSON(t, w, map[string]any{
				"errors": []map[string]any{
					{"message": "schema rejected query"},
				},
			})
		},
	))
	t.Cleanup(server.Close)

	client := newEnabledTestClient(t, server.URL)
	_, err := client.PoolsByTicker(context.Background(), "SNEK")
	if err == nil {
		t.Fatal("expected GraphQL error")
	}
	if !strings.Contains(err.Error(), "schema rejected query") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHTTPErrorReturned(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if _, ok := readGraphQLRequest(t, w, r); !ok {
				return
			}
			http.Error(w, "temporarily unavailable", http.StatusServiceUnavailable)
		},
	))
	t.Cleanup(server.Close)

	client := newEnabledTestClient(t, server.URL)
	_, err := client.PoolsByTicker(context.Background(), "SNEK")
	if err == nil {
		t.Fatal("expected HTTP status error")
	}
	if !strings.Contains(err.Error(), "503") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGraphQLResponseSizeIsLimited(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if _, ok := readGraphQLRequest(t, w, r); !ok {
				return
			}
			_, _ = w.Write(bytes.Repeat(
				[]byte("x"),
				maxGraphQLResponseBodySize+1,
			))
		},
	))
	t.Cleanup(server.Close)

	client := newEnabledTestClient(t, server.URL)
	_, err := client.PoolsByTicker(context.Background(), "SNEK")
	if err == nil {
		t.Fatal("expected oversized GraphQL response error")
	}
	if !strings.Contains(err.Error(), "response exceeds") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func readGraphQLRequest(
	t *testing.T,
	w http.ResponseWriter,
	r *http.Request,
) (graphQLRequest, bool) {
	t.Helper()
	if r.Method != http.MethodPost {
		failTestHandler(t, w, "unexpected method: got %s want POST", r.Method)
		return graphQLRequest{}, false
	}
	if contentType := r.Header.Get("Content-Type"); contentType != "application/json" {
		failTestHandler(t, w, "unexpected content type: %q", contentType)
		return graphQLRequest{}, false
	}
	var req graphQLRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		failTestHandler(t, w, "failed to decode request: %v", err)
		return graphQLRequest{}, false
	}
	if strings.TrimSpace(req.Query) == "" {
		failTestHandler(t, w, "query is required")
		return graphQLRequest{}, false
	}
	if req.Variables == nil {
		req.Variables = map[string]any{}
	}
	return req, true
}

func writeJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Errorf("failed to encode response: %v", err)
	}
}

func failTestHandler(
	t *testing.T,
	w http.ResponseWriter,
	format string,
	args ...any,
) {
	t.Helper()
	t.Errorf(format, args...)
	http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
}
