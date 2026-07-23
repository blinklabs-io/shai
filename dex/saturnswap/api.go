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
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	// DefaultGraphQLEndpoint is the no-auth public endpoint documented by
	// SaturnSwap for mainnet swap and pool discovery operations.
	DefaultGraphQLEndpoint = "https://api.saturnswap.io/v1/graphql/"

	defaultAPITimeout = 15 * time.Second

	maxGraphQLResponseBodySize = 1 << 20
)

var (
	ErrExternalAPIDisabled      = errors.New("saturnswap external API disabled")
	ErrInvalidExternalAPIConfig = errors.New("invalid saturnswap external API config")
	ErrGraphQL                  = errors.New("saturnswap graphql error")
)

type APIConfig struct {
	Enabled  bool
	Endpoint string
	Timeout  time.Duration
}

func DefaultAPIConfig() APIConfig {
	return APIConfig{
		Enabled:  false,
		Endpoint: DefaultGraphQLEndpoint,
		Timeout:  defaultAPITimeout,
	}
}

// Validate checks timeout and, when optional API access is enabled, endpoint
// shape.
func (cfg APIConfig) Validate() error {
	if cfg.Timeout < 0 {
		return fmt.Errorf("%w: timeout must not be negative", ErrInvalidExternalAPIConfig)
	}
	if !cfg.Enabled {
		return nil
	}
	endpoint := strings.TrimSpace(cfg.Endpoint)
	if endpoint == "" {
		endpoint = DefaultGraphQLEndpoint
	}
	parsedEndpoint, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidExternalAPIConfig, err)
	}
	if parsedEndpoint.Scheme != "http" && parsedEndpoint.Scheme != "https" {
		return fmt.Errorf(
			"%w: unsupported scheme %q",
			ErrInvalidExternalAPIConfig,
			parsedEndpoint.Scheme,
		)
	}
	if parsedEndpoint.Host == "" {
		return fmt.Errorf("%w: endpoint host is required", ErrInvalidExternalAPIConfig)
	}
	return nil
}

type ClientOption func(*Client)

func WithHTTPClient(httpClient *http.Client) ClientOption {
	return func(c *Client) {
		c.httpClient = httpClient
	}
}

// WithInsecureHTTPForTests permits cleartext HTTP endpoints on loopback hosts.
// It is intended only for local test servers; production requests must always
// use HTTPS.
func WithInsecureHTTPForTests() ClientOption {
	return func(c *Client) {
		c.allowInsecureHTTPForTests = true
	}
}

type Client struct {
	endpoint                  string
	httpClient                *http.Client
	allowInsecureHTTPForTests bool
}

func NewClient(cfg APIConfig, opts ...ClientOption) (*Client, error) {
	if !cfg.Enabled {
		return nil, ErrExternalAPIDisabled
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	endpoint := strings.TrimSpace(cfg.Endpoint)
	if endpoint == "" {
		endpoint = DefaultGraphQLEndpoint
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = defaultAPITimeout
	}
	client := &Client{
		endpoint: endpoint,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
	for _, opt := range opts {
		opt(client)
	}
	if client.httpClient == nil {
		return nil, fmt.Errorf("SaturnSwap API HTTP client is nil")
	}
	parsedEndpoint, err := url.Parse(client.endpoint)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidExternalAPIConfig, err)
	}
	if !isAllowedEndpoint(
		parsedEndpoint,
		client.allowInsecureHTTPForTests,
	) {
		return nil, fmt.Errorf(
			"%w: endpoint must use HTTPS or loopback HTTP for tests",
			ErrInvalidExternalAPIConfig,
		)
	}
	return client, nil
}

func isAllowedEndpoint(endpoint *url.URL, allowLoopbackHTTP bool) bool {
	if endpoint.Scheme == "https" {
		return true
	}
	if !allowLoopbackHTTP || endpoint.Scheme != "http" {
		return false
	}
	hostname := endpoint.Hostname()
	return strings.EqualFold(hostname, "localhost") ||
		net.ParseIP(hostname).IsLoopback()
}

func (c *Client) Endpoint() string {
	if c == nil {
		return ""
	}
	return c.endpoint
}

func (c *Client) Query(
	ctx context.Context,
	query string,
	variables any,
	out any,
) error {
	if c == nil {
		return fmt.Errorf("SaturnSwap API client is nil")
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return fmt.Errorf("GraphQL query is required")
	}
	reqBody, err := json.Marshal(graphQLRequest{
		Query:     query,
		Variables: variables,
	})
	if err != nil {
		return fmt.Errorf("failed to encode GraphQL request: %w", err)
	}
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.endpoint,
		bytes.NewReader(reqBody),
	)
	if err != nil {
		return fmt.Errorf("failed to create GraphQL request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("SaturnSwap GraphQL request failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		_, _ = io.Copy(io.Discard, resp.Body)
		return fmt.Errorf(
			"SaturnSwap GraphQL HTTP status %d: %s",
			resp.StatusCode,
			strings.TrimSpace(string(body)),
		)
	}

	body, err := io.ReadAll(
		io.LimitReader(resp.Body, maxGraphQLResponseBodySize+1),
	)
	if err != nil {
		return fmt.Errorf("failed to read GraphQL response: %w", err)
	}
	if len(body) > maxGraphQLResponseBodySize {
		return fmt.Errorf(
			"SaturnSwap GraphQL response exceeds %d bytes",
			maxGraphQLResponseBodySize,
		)
	}

	var graphResp graphQLResponse
	if err := json.Unmarshal(body, &graphResp); err != nil {
		return fmt.Errorf("failed to decode GraphQL response: %w", err)
	}
	if len(graphResp.Errors) > 0 {
		return graphQLErrors(graphResp.Errors)
	}
	if out == nil {
		return nil
	}
	if len(graphResp.Data) == 0 || bytes.Equal(graphResp.Data, []byte("null")) {
		return fmt.Errorf("GraphQL response missing data")
	}
	if err := json.Unmarshal(graphResp.Data, out); err != nil {
		return fmt.Errorf("failed to decode GraphQL data: %w", err)
	}
	return nil
}

func (c *Client) PoolsByTicker(
	ctx context.Context,
	ticker string,
) ([]Pool, error) {
	ticker = strings.TrimSpace(ticker)
	if ticker == "" {
		return nil, fmt.Errorf("ticker is required")
	}
	var out struct {
		Pools PoolConnection `json:"pools"`
	}
	if err := c.Query(
		ctx,
		poolsByTickerQuery,
		map[string]any{"ticker": ticker},
		&out,
	); err != nil {
		return nil, err
	}
	return out.Pools.Pools(), nil
}

func (c *Client) PoolByTokens(
	ctx context.Context,
	input PoolByTokensInput,
) (*Pool, error) {
	if err := validatePoolByTokensInput(input); err != nil {
		return nil, err
	}
	var out struct {
		PoolByTokens *Pool `json:"poolByTokens"`
	}
	if err := c.Query(
		ctx,
		poolByTokensQuery,
		map[string]any{"input": input},
		&out,
	); err != nil {
		return nil, err
	}
	if out.PoolByTokens == nil {
		return nil, fmt.Errorf("SaturnSwap poolByTokens returned no pool")
	}
	return out.PoolByTokens, nil
}

func validatePoolByTokensInput(input PoolByTokensInput) error {
	if input.PolicyIDOne == "" && input.AssetNameOne != "" {
		return fmt.Errorf("token one asset name requires a policy ID")
	}
	if input.PolicyIDTwo == "" && input.AssetNameTwo != "" {
		return fmt.Errorf("token two asset name requires a policy ID")
	}
	if input.PolicyIDOne == input.PolicyIDTwo &&
		input.AssetNameOne == input.AssetNameTwo {
		return fmt.Errorf("pool tokens must be different")
	}
	return nil
}

func (c *Client) CreateOrderTransaction(
	ctx context.Context,
	input CreateOrderTransactionInput,
) (*CreateOrderTransactionResult, error) {
	var out struct {
		CreateOrderTransaction CreateOrderTransactionResult `json:"createOrderTransaction"`
	}
	if err := c.Query(
		ctx,
		createOrderTransactionMutation,
		map[string]any{"input": input},
		&out,
	); err != nil {
		return nil, err
	}
	if out.CreateOrderTransaction.Error != nil {
		return &out.CreateOrderTransaction, out.CreateOrderTransaction.Error
	}
	return &out.CreateOrderTransaction, nil
}

func (c *Client) SubmitOrderTransaction(
	ctx context.Context,
	input SubmitOrderTransactionInput,
) (*SubmitOrderTransactionResult, error) {
	var out struct {
		SubmitOrderTransaction SubmitOrderTransactionResult `json:"submitOrderTransaction"`
	}
	if err := c.Query(
		ctx,
		submitOrderTransactionMutation,
		map[string]any{"input": input},
		&out,
	); err != nil {
		return nil, err
	}
	if out.SubmitOrderTransaction.Error != nil {
		return &out.SubmitOrderTransaction, out.SubmitOrderTransaction.Error
	}
	return &out.SubmitOrderTransaction, nil
}

type graphQLRequest struct {
	Query     string `json:"query"`
	Variables any    `json:"variables,omitempty"`
}

type graphQLResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []GraphQLError  `json:"errors,omitempty"`
}

type GraphQLError struct {
	Message    string         `json:"message"`
	Extensions map[string]any `json:"extensions,omitempty"`
}

type graphQLErrors []GraphQLError

func (e graphQLErrors) Error() string {
	parts := make([]string, 0, len(e))
	for _, graphErr := range e {
		if graphErr.Message != "" {
			parts = append(parts, graphErr.Message)
		}
	}
	if len(parts) == 0 {
		return "SaturnSwap GraphQL error"
	}
	return "SaturnSwap GraphQL error: " + strings.Join(parts, "; ")
}

func (e graphQLErrors) Unwrap() error {
	return ErrGraphQL
}

const poolFields = `
	id
	name
	ticker
	lp_fee_percent
	protocol_fee_percent
	is_swap_active
	is_liquidity_active
	is_verified
	token_project_one {
		id
		name
		image
		policy_id
		asset_name
		decimals
		precision
		ticker
		price
	}
	token_project_two {
		id
		name
		image
		policy_id
		asset_name
		decimals
		precision
		ticker
		price
	}
	pool_stats {
		pool_id
		price
		highest_bid
		lowest_ask
		reserve_token_one
		reserve_token_two
		tvl
		volume_1d
		volume_7d
		volume_all
	}
`

const poolsByTickerQuery = `
query SaturnPoolsByTicker($ticker: String!) {
	pools(where: { ticker: { eq: $ticker } }) {
		nodes {
` + poolFields + `
		}
		totalCount
	}
}`

const poolByTokensQuery = `
query SaturnPoolByTokens($input: GetPoolByTokensInput!) {
	poolByTokens(input: $input) {
` + poolFields + `
	}
}`

const createOrderTransactionMutation = `
mutation SaturnCreateOrder($input: CreateOrderTransactionInput!) {
	createOrderTransaction(input: $input) {
		successTransactions {
			transactionId
			hexTransaction
		}
		failTransactions {
			error {
				message
				code
			}
		}
		error {
			message
			code
		}
	}
}`

const submitOrderTransactionMutation = `
mutation SaturnSubmitOrder($input: SubmitOrderTransactionInput!) {
	submitOrderTransaction(input: $input) {
		transactionIds
		error {
			message
			code
			link
		}
	}
}`
