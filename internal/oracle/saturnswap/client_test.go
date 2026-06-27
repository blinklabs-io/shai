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
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDefaultExternalAPIConfigDisabled(t *testing.T) {
	t.Parallel()

	config := DefaultAPIConfig()
	if config.Enabled {
		t.Fatal("default external API config must be disabled")
	}
	if config.Endpoint != DefaultGraphQLEndpoint {
		t.Fatalf(
			"unexpected endpoint: got %q want %q",
			config.Endpoint,
			DefaultGraphQLEndpoint,
		)
	}
	if err := config.Validate(); err != nil {
		t.Fatalf("disabled config should not require validation: %v", err)
	}
}

func TestClientRefusesNetworkWhenDisabled(t *testing.T) {
	t.Parallel()

	_, err := NewClient(DefaultAPIConfig())
	if !errors.Is(err, ErrExternalAPIDisabled) {
		t.Fatalf("expected ErrExternalAPIDisabled, got %v", err)
	}
}

func TestQueryPoolsByTicker(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("unexpected content-type: %s", r.Header.Get("Content-Type"))
		}

		var body graphQLRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		if !strings.Contains(body.Query, "pools") {
			t.Fatalf("query did not request pools: %s", body.Query)
		}
		variables := graphqlVariables(t, body)
		if variables["ticker"] != "SNEK" {
			t.Fatalf("unexpected ticker variable: %#v", variables["ticker"])
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"data": {
				"pools": {
					"nodes": [
						{
							"id": "pool-1",
							"ticker": "SNEK",
							"lp_fee_percent": 0.3,
							"token_project_one": {},
							"token_project_two": {
								"policy_id": "aa",
								"asset_name": "bb"
							},
							"pool_stats": {
								"reserve_token_one": "5000000",
								"reserve_token_two": 100
							}
						}
					]
				}
			}
		}`))
	}))
	defer server.Close()

	client := newEnabledTestClient(t, server.URL)
	pools, err := client.PoolsByTicker(context.Background(), "SNEK")
	if err != nil {
		t.Fatalf("PoolsByTicker failed: %v", err)
	}
	if len(pools) != 1 {
		t.Fatalf("expected one pool, got %d", len(pools))
	}
	if pools[0].ID != "pool-1" || pools[0].Ticker != "SNEK" {
		t.Fatalf("unexpected pool: %#v", pools[0])
	}
	if pools[0].LPFeePercent.String() != "0.3" {
		t.Fatalf("unexpected fee percent: %s", pools[0].LPFeePercent)
	}
	state, err := pools[0].ToPoolState(123, time.Unix(1, 0))
	if err != nil {
		t.Fatalf("ToPoolState failed: %v", err)
	}
	if state.AssetX.Amount != 5000000 || state.AssetY.Amount != 100 {
		t.Fatalf("unexpected reserves: %#v", state)
	}
}

func TestCreateAndSubmitOrderTransactions(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body graphQLRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(body.Query, "createOrderTransaction"):
			input := graphqlVariables(t, body)["input"].(map[string]interface{})
			if input["paymentAddress"] != "addr1test" {
				t.Fatalf("unexpected create input: %#v", input)
			}
			_, _ = w.Write([]byte(`{
				"data": {
					"createOrderTransaction": {
						"successTransactions": [
							{"transactionId": "draft-1", "hexTransaction": "84a1"}
						],
						"failTransactions": [],
						"error": null
					}
				}
			}`))
		case strings.Contains(body.Query, "submitOrderTransaction"):
			input := graphqlVariables(t, body)["input"].(map[string]interface{})
			if input["paymentAddress"] != "addr1test" {
				t.Fatalf("unexpected submit input: %#v", input)
			}
			_, _ = w.Write([]byte(`{
				"data": {
					"submitOrderTransaction": {
						"transactionIds": ["tx-1"],
						"error": null
					}
				}
			}`))
		default:
			t.Fatalf("unexpected query: %s", body.Query)
		}
	}))
	defer server.Close()

	client := newEnabledTestClient(t, server.URL)
	createResult, err := client.CreateOrderTransaction(
		context.Background(),
		CreateOrderTransactionInput{
			PaymentAddress: "addr1test",
			MarketOrderComponents: []MarketOrderComponent{
				{
					PoolID:          "pool-1",
					TokenAmountSell: 5,
					TokenAmountBuy:  1,
					MarketOrderType: PoolUtxoTypeMarketBuyOrder,
					Slippage:        2,
					Version:         2,
				},
			},
		},
	)
	if err != nil {
		t.Fatalf("CreateOrderTransaction failed: %v", err)
	}
	if len(createResult.SuccessTransactions) != 1 {
		t.Fatalf("unexpected create result: %#v", createResult)
	}

	submitResult, err := client.SubmitOrderTransaction(
		context.Background(),
		SubmitOrderTransactionInput{
			PaymentAddress:      "addr1test",
			SuccessTransactions: createResult.SuccessTransactions,
		},
	)
	if err != nil {
		t.Fatalf("SubmitOrderTransaction failed: %v", err)
	}
	if len(submitResult.TransactionIDs) != 1 || submitResult.TransactionIDs[0] != "tx-1" {
		t.Fatalf("unexpected submit result: %#v", submitResult)
	}
}

func TestGraphQLError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"errors":[{"message":"bad pool"}]}`))
	}))
	defer server.Close()

	client := newEnabledTestClient(t, server.URL)
	_, err := client.PoolsByTicker(context.Background(), "SNEK")
	if !errors.Is(err, ErrGraphQL) {
		t.Fatalf("expected ErrGraphQL, got %v", err)
	}
	if !strings.Contains(err.Error(), "bad pool") {
		t.Fatalf("expected graphql error message, got %v", err)
	}
}

func TestExternalAPIConfigValidation(t *testing.T) {
	t.Parallel()

	config := APIConfig{Enabled: true, Endpoint: "ftp://example.com"}
	if !errors.Is(config.Validate(), ErrInvalidExternalAPIConfig) {
		t.Fatal("expected invalid config for unsupported scheme")
	}
}

func newEnabledTestClient(t *testing.T, endpoint string) *Client {
	t.Helper()
	client, err := NewClient(APIConfig{
		Enabled:  true,
		Endpoint: endpoint,
	})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	return client
}

func graphqlVariables(
	t *testing.T,
	body graphQLRequest,
) map[string]interface{} {
	t.Helper()
	variables, ok := body.Variables.(map[string]interface{})
	if !ok {
		t.Fatalf("unexpected variables shape: %#v", body.Variables)
	}
	return variables
}
