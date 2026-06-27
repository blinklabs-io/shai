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
	"fmt"
	"net/url"
	"strings"
)

const (
	ProtocolName    = "strike-finance"
	ProductName     = "perpetuals"
	IntegrationName = ProtocolName + "-" + ProductName
	MainnetNetwork  = "mainnet"
	TestnetNetwork  = "testnet"

	DefaultExternalAPIEnabled = false

	MainnetAPIBaseURL      = "https://api.strikefinance.org"
	MainnetPriceAPIBaseURL = MainnetAPIBaseURL + "/price"
	TestnetAPIBaseURL      = "https://api-v2-testnet.strikefinance.org"
	TestnetPriceAPIBaseURL = TestnetAPIBaseURL + "/price"
)

// ExternalAPIConfig records optional off-chain API usage. Runtime support must
// not require it, and the default scaffold keeps it disabled.
type ExternalAPIConfig struct {
	Enabled      bool
	BaseURL      string
	PriceBaseURL string
}

// VerificationStatus tracks the on-chain facts that must be independently
// verified before any runtime profile is enabled.
type VerificationStatus struct {
	ScriptAddresses  bool
	InterceptPoint   bool
	DatumSchema      bool
	RedeemerSchema   bool
	StateTransitions bool
}

func (s VerificationStatus) Complete() bool {
	return s.ScriptAddresses &&
		s.InterceptPoint &&
		s.DatumSchema &&
		s.RedeemerSchema &&
		s.StateTransitions
}

// OnChainTargets records the candidate Strike perpetuals integration target.
// Empty address and intercept fields are intentional until verified.
type OnChainTargets struct {
	Network                  string
	MarketStateScriptAddress string
	OrderScriptAddress       string
	PositionScriptAddress    string
	InterceptSlot            uint64
	InterceptHash            string
	Verification             VerificationStatus
	ExternalAPI              ExternalAPIConfig
}

func MainnetTargets() OnChainTargets {
	return OnChainTargets{
		Network: MainnetNetwork,
		ExternalAPI: ExternalAPIConfig{
			Enabled:      DefaultExternalAPIEnabled,
			BaseURL:      MainnetAPIBaseURL,
			PriceBaseURL: MainnetPriceAPIBaseURL,
		},
	}
}

func TestnetTargets() OnChainTargets {
	return OnChainTargets{
		Network: TestnetNetwork,
		ExternalAPI: ExternalAPIConfig{
			Enabled:      DefaultExternalAPIEnabled,
			BaseURL:      TestnetAPIBaseURL,
			PriceBaseURL: TestnetPriceAPIBaseURL,
		},
	}
}

func KnownTargets() []OnChainTargets {
	return []OnChainTargets{
		MainnetTargets(),
		TestnetTargets(),
	}
}

func TargetsForNetwork(network string) (OnChainTargets, bool) {
	for _, targets := range KnownTargets() {
		if targets.Network == network {
			return targets, true
		}
	}
	return OnChainTargets{}, false
}

func (t OnChainTargets) RuntimeReady() bool {
	return len(t.MissingVerification()) == 0
}

func (t OnChainTargets) MissingVerification() []string {
	var missing []string
	if t.Network == "" {
		missing = append(missing, "network")
	}
	if t.MarketStateScriptAddress == "" ||
		t.OrderScriptAddress == "" ||
		t.PositionScriptAddress == "" ||
		!t.Verification.ScriptAddresses {
		missing = append(missing, "verified script addresses")
	}
	if t.InterceptSlot == 0 ||
		t.InterceptHash == "" ||
		!t.Verification.InterceptPoint {
		missing = append(missing, "verified intercept slot")
	}
	if !t.Verification.DatumSchema {
		missing = append(missing, "verified datum schema")
	}
	if !t.Verification.RedeemerSchema {
		missing = append(missing, "verified redeemer schema")
	}
	if !t.Verification.StateTransitions {
		missing = append(missing, "verified state transitions")
	}
	return missing
}

func (t OnChainTargets) ValidateRuntimeEnablement() error {
	if t.RuntimeReady() {
		return nil
	}
	return fmt.Errorf(
		"%w: missing %s",
		ErrVerificationRequired,
		strings.Join(t.MissingVerification(), ", "),
	)
}

func (c ExternalAPIConfig) Validate() error {
	if !c.Enabled {
		return nil
	}
	if c.BaseURL == "" {
		return fmt.Errorf("%w: base URL is required", ErrInvalidExternalAPIConfig)
	}
	if err := validateAbsoluteHTTPURL(c.BaseURL); err != nil {
		return fmt.Errorf("%w: invalid base URL: %w", ErrInvalidExternalAPIConfig, err)
	}
	if c.PriceBaseURL != "" {
		if err := validateAbsoluteHTTPURL(c.PriceBaseURL); err != nil {
			return fmt.Errorf(
				"%w: invalid price base URL: %w",
				ErrInvalidExternalAPIConfig,
				err,
			)
		}
	}
	return nil
}

func validateAbsoluteHTTPURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("unsupported scheme %q", parsed.Scheme)
	}
	if parsed.Host == "" {
		return fmt.Errorf("missing host")
	}
	return nil
}

type MarketID string
type PositionID string

type Direction string

const (
	DirectionLong  Direction = "long"
	DirectionShort Direction = "short"
)

// MarketState is the future normalized state for a Strike perpetual market.
// The datum schema is not verified, so Parser does not populate it yet.
type MarketState struct {
	MarketID          MarketID
	BaseAssetID       string
	QuoteAssetID      string
	LongOpenInterest  uint64
	ShortOpenInterest uint64
	Slot              uint64
	TxHash            string
	TxIndex           uint32
}

// PositionState is the future normalized state for an open perpetual position.
// The datum schema is not verified, so Parser does not populate it yet.
type PositionState struct {
	PositionID PositionID
	MarketID   MarketID
	Owner      string
	Direction  Direction
	Collateral uint64
	Size       uint64
	Slot       uint64
	TxHash     string
	TxIndex    uint32
}

type RedeemerAction string

const RedeemerActionUnknown RedeemerAction = "unknown"

// Redeemer is a placeholder for verified Strike perpetuals redeemer actions.
type Redeemer struct {
	Action RedeemerAction
	Raw    []byte
}
