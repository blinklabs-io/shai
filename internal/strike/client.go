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
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const defaultHTTPTimeout = 10 * time.Second

type Client struct {
	enabled      bool
	baseURL      *url.URL
	priceBaseURL *url.URL
	httpClient   *http.Client
	signer       *Ed25519Signer
	now          func() time.Time
	nonce        func() (string, error)
}

type ClientOption func(*clientOptions)

type clientOptions struct {
	httpClient *http.Client
	signer     *Ed25519Signer
	now        func() time.Time
	nonce      func() (string, error)
}

func WithHTTPClient(httpClient *http.Client) ClientOption {
	return func(options *clientOptions) {
		options.httpClient = httpClient
	}
}

func WithSigner(signer *Ed25519Signer) ClientOption {
	return func(options *clientOptions) {
		options.signer = signer
	}
}

func WithClock(now func() time.Time) ClientOption {
	return func(options *clientOptions) {
		options.now = now
	}
}

func WithNonce(nonce func() (string, error)) ClientOption {
	return func(options *clientOptions) {
		options.nonce = nonce
	}
}

func NewClient(config ExternalAPIConfig, opts ...ClientOption) (*Client, error) {
	options := clientOptions{
		httpClient: &http.Client{Timeout: defaultHTTPTimeout},
		now:        time.Now,
		nonce:      randomNonce,
	}
	for _, opt := range opts {
		opt(&options)
	}
	if options.httpClient == nil {
		return nil, fmt.Errorf("%w: HTTP client is nil", ErrInvalidExternalAPIConfig)
	}
	if options.now == nil {
		return nil, fmt.Errorf("%w: clock is nil", ErrInvalidExternalAPIConfig)
	}
	if options.nonce == nil {
		return nil, fmt.Errorf("%w: nonce generator is nil", ErrInvalidExternalAPIConfig)
	}

	client := &Client{
		enabled:    config.Enabled,
		httpClient: options.httpClient,
		signer:     options.signer,
		now:        options.now,
		nonce:      options.nonce,
	}
	if !config.Enabled {
		return client, nil
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}

	baseURL, err := parseBaseURL(config.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid base URL: %w", ErrInvalidExternalAPIConfig, err)
	}
	priceBaseURL := config.PriceBaseURL
	if priceBaseURL == "" {
		priceBaseURL = joinURLPath(baseURL, "/price").String()
	}
	parsedPriceBaseURL, err := parseBaseURL(priceBaseURL)
	if err != nil {
		return nil, fmt.Errorf(
			"%w: invalid price base URL: %w",
			ErrInvalidExternalAPIConfig,
			err,
		)
	}
	client.baseURL = baseURL
	client.priceBaseURL = parsedPriceBaseURL
	return client, nil
}

func (c *Client) Enabled() bool {
	return c != nil && c.enabled
}

func (c *Client) Ping(ctx context.Context) error {
	return c.doJSON(ctx, c.baseURL, http.MethodGet, "/v2/ping", nil, nil, nil, false)
}

type ServerTime struct {
	ServerTime int64 `json:"serverTime,omitempty"`
	Time       int64 `json:"time,omitempty"`
	Timestamp  int64 `json:"timestamp,omitempty"`
}

func (t ServerTime) UnixMilliseconds() int64 {
	switch {
	case t.ServerTime != 0:
		return t.ServerTime
	case t.Timestamp != 0:
		return t.Timestamp
	default:
		return t.Time
	}
}

func (c *Client) ServerTime(ctx context.Context) (*ServerTime, error) {
	var serverTime ServerTime
	if err := c.doJSON(
		ctx,
		c.baseURL,
		http.MethodGet,
		"/v2/time",
		nil,
		nil,
		&serverTime,
		false,
	); err != nil {
		return nil, err
	}
	return &serverTime, nil
}

type MarkPrice struct {
	Symbol          string `json:"symbol,omitempty"`
	MarkPrice       string `json:"markPrice,omitempty"`
	Price           string `json:"price,omitempty"`
	IndexPrice      string `json:"indexPrice,omitempty"`
	FundingRate     string `json:"fundingRate,omitempty"`
	NextFundingTime string `json:"nextFundingTime,omitempty"`
}

func (p MarkPrice) PriceString() string {
	if p.MarkPrice != "" {
		return p.MarkPrice
	}
	return p.Price
}

func (c *Client) MarkPrice(
	ctx context.Context,
	symbol string,
) (*MarkPrice, error) {
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	if symbol == "" {
		return nil, fmt.Errorf("%w: symbol is required", ErrInvalidExternalAPIConfig)
	}
	query := url.Values{}
	query.Set("symbol", symbol)

	var price MarkPrice
	if err := c.doJSON(
		ctx,
		c.priceBaseURL,
		http.MethodGet,
		"/v2/markPrice",
		query,
		nil,
		&price,
		false,
	); err != nil {
		return nil, err
	}
	return &price, nil
}

func (c *Client) DoAuthenticated(
	ctx context.Context,
	method string,
	path string,
	query url.Values,
	body any,
	out any,
) error {
	return c.doJSON(ctx, c.baseURL, method, path, query, body, out, true)
}

type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("%s: status %d: %s", ErrAPIRequestFailed, e.StatusCode, e.Body)
}

func (e *APIError) Is(target error) bool {
	return target == ErrAPIRequestFailed
}

func (c *Client) doJSON(
	ctx context.Context,
	baseURL *url.URL,
	method string,
	path string,
	query url.Values,
	body any,
	out any,
	authenticated bool,
) error {
	if c == nil || !c.enabled {
		return ErrExternalAPIDisabled
	}
	if baseURL == nil {
		return fmt.Errorf("%w: base URL is nil", ErrInvalidExternalAPIConfig)
	}

	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal Strike request body: %w", err)
		}
	}

	reqURL := joinURLPath(baseURL, path)
	reqURL.RawQuery = query.Encode()

	var bodyReader io.Reader
	if bodyBytes != nil {
		bodyReader = bytes.NewReader(bodyBytes)
	}
	req, err := http.NewRequestWithContext(ctx, method, reqURL.String(), bodyReader)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if bodyBytes != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if authenticated {
		timestamp := strconv.FormatInt(c.now().UnixMilli(), 10)
		nonce, err := c.nonce()
		if err != nil {
			return err
		}
		if err := c.signer.SignRequest(req, bodyBytes, timestamp, nonce); err != nil {
			return err
		}
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &APIError{
			StatusCode: resp.StatusCode,
			Body:       strings.TrimSpace(string(respBody)),
		}
	}
	if out == nil || len(bytes.TrimSpace(respBody)) == 0 {
		return nil
	}
	return decodeJSONEnvelope(respBody, out)
}

func parseBaseURL(rawURL string) (*url.URL, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("unsupported scheme %q", parsed.Scheme)
	}
	if parsed.Host == "" {
		return nil, fmt.Errorf("missing host")
	}
	return parsed, nil
}

func joinURLPath(baseURL *url.URL, path string) *url.URL {
	ret := *baseURL
	basePath := strings.TrimRight(ret.Path, "/")
	nextPath := strings.TrimLeft(path, "/")
	if basePath == "" {
		ret.Path = "/" + nextPath
	} else if nextPath == "" {
		ret.Path = basePath
	} else {
		ret.Path = basePath + "/" + nextPath
	}
	return &ret
}

func decodeJSONEnvelope(body []byte, out any) error {
	var envelope struct {
		Data   json.RawMessage `json:"data"`
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return err
	}
	switch {
	case len(bytes.TrimSpace(envelope.Data)) > 0 &&
		string(bytes.TrimSpace(envelope.Data)) != "null":
		return json.Unmarshal(envelope.Data, out)
	case len(bytes.TrimSpace(envelope.Result)) > 0 &&
		string(bytes.TrimSpace(envelope.Result)) != "null":
		return json.Unmarshal(envelope.Result, out)
	default:
		return json.Unmarshal(body, out)
	}
}

func randomNonce() (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes[:]), nil
}
