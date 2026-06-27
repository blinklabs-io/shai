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

package strike

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewClientDisabledDoesNotCallNetwork(t *testing.T) {
	var hits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := MainnetTargets().ExternalAPI
	config.BaseURL = server.URL
	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}

	err = client.Ping(context.Background())
	if !errors.Is(err, ErrExternalAPIDisabled) {
		t.Fatalf("expected ErrExternalAPIDisabled, got %v", err)
	}
	if hits.Load() != 0 {
		t.Fatalf("disabled client made %d network requests", hits.Load())
	}
}

func TestClientPingAndServerTime(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/ping":
			w.WriteHeader(http.StatusNoContent)
		case "/v2/time":
			writeJSON(t, w, map[string]int64{"serverTime": 1700000000123})
		default:
			t.Fatalf("unexpected request path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := newEnabledTestClient(t, server.URL, server.URL+"/price")
	if err := client.Ping(context.Background()); err != nil {
		t.Fatalf("Ping returned error: %v", err)
	}

	serverTime, err := client.ServerTime(context.Background())
	if err != nil {
		t.Fatalf("ServerTime returned error: %v", err)
	}
	if serverTime.UnixMilliseconds() != 1700000000123 {
		t.Fatalf("unexpected server time: %#v", serverTime)
	}
}

func TestClientMarkPriceUsesPriceBaseURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/price/v2/markPrice" {
			t.Fatalf("unexpected request path %s", r.URL.Path)
		}
		if r.URL.Query().Get("symbol") != "BTC-USD" {
			t.Fatalf("unexpected symbol query %q", r.URL.RawQuery)
		}
		writeJSON(t, w, map[string]any{
			"data": map[string]string{
				"symbol":    "BTC-USD",
				"markPrice": "12345.67",
			},
		})
	}))
	defer server.Close()

	client := newEnabledTestClient(t, server.URL, server.URL+"/price")
	price, err := client.MarkPrice(context.Background(), "btc-usd")
	if err != nil {
		t.Fatalf("MarkPrice returned error: %v", err)
	}
	if price.Symbol != "BTC-USD" {
		t.Fatalf("unexpected symbol %q", price.Symbol)
	}
	if price.PriceString() != "12345.67" {
		t.Fatalf("unexpected mark price %q", price.PriceString())
	}
}

func TestClientAuthenticatedRequestSignsHeaders(t *testing.T) {
	privateKey := ed25519.NewKeyFromSeed(
		[]byte("01234567890123456789012345678901"),
	)
	publicKey := privateKey.Public().(ed25519.PublicKey)
	signer, err := NewEd25519Signer(publicKey, privateKey)
	if err != nil {
		t.Fatalf("NewEd25519Signer returned error: %v", err)
	}

	type orderRequest struct {
		Side   string `json:"side"`
		Symbol string `json:"symbol"`
	}
	body := orderRequest{
		Side:   "buy",
		Symbol: "BTC-USD",
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("json.Marshal returned error: %v", err)
	}
	bodyHash := sha256.Sum256(bodyBytes)
	expectedPayload := SignaturePayload(
		http.MethodPost,
		"/v2/orders?symbol=BTC-USD",
		"1700000000123",
		"nonce-1",
		hex.EncodeToString(bodyHash[:]),
	)
	expectedSignature := hex.EncodeToString(
		ed25519.Sign(privateKey, []byte(expectedPayload)),
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RequestURI() != "/v2/orders?symbol=BTC-USD" {
			t.Fatalf("unexpected request URI %s", r.URL.RequestURI())
		}
		if r.Header.Get(HeaderWalletPublicKey) != hex.EncodeToString(publicKey) {
			t.Fatalf("unexpected public key header")
		}
		if r.Header.Get(HeaderWalletSignature) != expectedSignature {
			t.Fatalf("unexpected signature header")
		}
		if r.Header.Get(HeaderWalletTimestamp) != "1700000000123" {
			t.Fatalf("unexpected timestamp header")
		}
		if r.Header.Get(HeaderWalletNonce) != "nonce-1" {
			t.Fatalf("unexpected nonce header")
		}
		writeJSON(t, w, map[string]string{"status": "accepted"})
	}))
	defer server.Close()

	config := ExternalAPIConfig{
		Enabled: true,
		BaseURL: server.URL,
	}
	client, err := NewClient(
		config,
		WithSigner(signer),
		WithClock(func() time.Time {
			return time.UnixMilli(1700000000123)
		}),
		WithNonce(func() (string, error) {
			return "nonce-1", nil
		}),
	)
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}

	var out map[string]string
	query := url.Values{"symbol": []string{"BTC-USD"}}
	err = client.DoAuthenticated(
		context.Background(),
		http.MethodPost,
		"/v2/orders",
		query,
		body,
		&out,
	)
	if err != nil {
		t.Fatalf("DoAuthenticated returned error: %v", err)
	}
	if out["status"] != "accepted" {
		t.Fatalf("unexpected response: %#v", out)
	}
}

func TestClientAuthenticatedRequestRequiresSigner(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("request should not be sent without signer")
	}))
	defer server.Close()

	client := newEnabledTestClient(t, server.URL, server.URL+"/price")
	err := client.DoAuthenticated(
		context.Background(),
		http.MethodPost,
		"/v2/orders",
		nil,
		map[string]string{"symbol": "BTC-USD"},
		nil,
	)
	if !errors.Is(err, ErrMissingSigner) {
		t.Fatalf("expected ErrMissingSigner, got %v", err)
	}
}

func TestClientReturnsAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := newEnabledTestClient(t, server.URL, server.URL+"/price")
	err := client.Ping(context.Background())
	if !errors.Is(err, ErrAPIRequestFailed) {
		t.Fatalf("expected ErrAPIRequestFailed, got %v", err)
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError, got %T", err)
	}
	if apiErr.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("unexpected status code %d", apiErr.StatusCode)
	}
	if !strings.Contains(apiErr.Body, "unavailable") {
		t.Fatalf("unexpected body %q", apiErr.Body)
	}
}

func newEnabledTestClient(
	t *testing.T,
	baseURL string,
	priceBaseURL string,
) *Client {
	t.Helper()

	config := ExternalAPIConfig{
		Enabled:      true,
		BaseURL:      baseURL,
		PriceBaseURL: priceBaseURL,
	}
	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}
	return client
}

func writeJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatalf("json.Encode returned error: %v", err)
	}
}
