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
	"errors"
	"strings"
	"testing"
)

func TestMainnetTargetsAreDisabledUntilVerified(t *testing.T) {
	targets := MainnetTargets()

	if targets.Network != MainnetNetwork {
		t.Fatalf("expected network %q, got %q", MainnetNetwork, targets.Network)
	}
	if targets.ExternalAPI.Enabled {
		t.Fatal("expected external API integration to be disabled by default")
	}
	if targets.ExternalAPI.BaseURL != MainnetAPIBaseURL {
		t.Fatalf(
			"expected mainnet API base URL %q, got %q",
			MainnetAPIBaseURL,
			targets.ExternalAPI.BaseURL,
		)
	}
	if targets.ExternalAPI.PriceBaseURL != MainnetPriceAPIBaseURL {
		t.Fatalf(
			"expected mainnet price API base URL %q, got %q",
			MainnetPriceAPIBaseURL,
			targets.ExternalAPI.PriceBaseURL,
		)
	}
	if targets.RuntimeReady() {
		t.Fatal("unverified Strike targets must not be runtime ready")
	}

	missing := strings.Join(targets.MissingVerification(), ",")
	for _, want := range []string{
		"verified script addresses",
		"verified intercept slot",
		"verified datum schema",
		"verified redeemer schema",
		"verified state transitions",
	} {
		if !strings.Contains(missing, want) {
			t.Fatalf("expected missing verification %q in %q", want, missing)
		}
	}
}

func TestTestnetTargetsExposeDisabledAPIConfig(t *testing.T) {
	targets := TestnetTargets()

	if targets.Network != TestnetNetwork {
		t.Fatalf("expected network %q, got %q", TestnetNetwork, targets.Network)
	}
	if targets.ExternalAPI.Enabled {
		t.Fatal("expected external API integration to be disabled by default")
	}
	if targets.ExternalAPI.BaseURL != TestnetAPIBaseURL {
		t.Fatalf(
			"expected testnet API base URL %q, got %q",
			TestnetAPIBaseURL,
			targets.ExternalAPI.BaseURL,
		)
	}
	if targets.ExternalAPI.PriceBaseURL != TestnetPriceAPIBaseURL {
		t.Fatalf(
			"expected testnet price API base URL %q, got %q",
			TestnetPriceAPIBaseURL,
			targets.ExternalAPI.PriceBaseURL,
		)
	}
	if targets.RuntimeReady() {
		t.Fatal("unverified Strike testnet targets must not be runtime ready")
	}
}

func TestTargetsForNetwork(t *testing.T) {
	targets, ok := TargetsForNetwork(MainnetNetwork)
	if !ok {
		t.Fatal("expected mainnet targets")
	}
	if targets.Network != MainnetNetwork {
		t.Fatalf("unexpected targets: %#v", targets)
	}

	_, ok = TargetsForNetwork("preprod")
	if ok {
		t.Fatal("preprod targets should not be exposed without verified config")
	}
}

func TestParserReturnsVerificationErrors(t *testing.T) {
	parser := NewParser(MainnetTargets())
	if parser.Protocol() != IntegrationName {
		t.Fatalf("expected protocol %q, got %q", IntegrationName, parser.Protocol())
	}

	parseChecks := []struct {
		name string
		err  error
	}{
		{
			name: "market datum",
			err: func() error {
				_, err := parser.ParseMarketDatum([]byte{0x01}, "tx", 0, 1)
				return err
			}(),
		},
		{
			name: "position datum",
			err: func() error {
				_, err := parser.ParsePositionDatum([]byte{0x01}, "tx", 0, 1)
				return err
			}(),
		},
		{
			name: "redeemer",
			err: func() error {
				_, err := parser.ParseRedeemer([]byte{0x01})
				return err
			}(),
		},
	}

	for _, check := range parseChecks {
		if !errors.Is(check.err, ErrUnsupported) {
			t.Fatalf("%s: expected ErrUnsupported, got %v", check.name, check.err)
		}
		if !errors.Is(check.err, ErrVerificationRequired) {
			t.Fatalf(
				"%s: expected ErrVerificationRequired, got %v",
				check.name,
				check.err,
			)
		}
		if !strings.Contains(check.err.Error(), check.name+" schema is unverified") {
			t.Fatalf("%s: unclear parser error: %v", check.name, check.err)
		}
	}
}

func TestValidateRuntimeEnablement(t *testing.T) {
	parser := NewParser(MainnetTargets())
	err := parser.ValidateRuntimeEnablement()
	if !errors.Is(err, ErrVerificationRequired) {
		t.Fatalf("expected ErrVerificationRequired, got %v", err)
	}

	targets := OnChainTargets{
		Network:                  MainnetNetwork,
		MarketStateScriptAddress: "addr_test_market",
		OrderScriptAddress:       "addr_test_order",
		PositionScriptAddress:    "addr_test_position",
		InterceptSlot:            1,
		InterceptHash:            "block_hash",
		Verification: VerificationStatus{
			ScriptAddresses:  true,
			InterceptPoint:   true,
			DatumSchema:      true,
			RedeemerSchema:   true,
			StateTransitions: true,
		},
	}
	if err := NewParser(targets).ValidateRuntimeEnablement(); err != nil {
		t.Fatalf("expected verified targets to pass validation, got %v", err)
	}
}
