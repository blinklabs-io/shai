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
	"errors"
	"strings"
	"testing"
	"time"
)

func TestParserReportsUnverifiedIntegration(t *testing.T) {
	t.Parallel()

	parser := NewParser()
	if got := parser.Protocol(); got != ProtocolName {
		t.Fatalf("unexpected protocol: got %q want %q", got, ProtocolName)
	}

	state, err := parser.ParsePoolDatum(
		nil,
		nil,
		"tx",
		0,
		0,
		time.Unix(0, 0),
	)
	if state != nil {
		t.Fatalf("expected nil state, got %#v", state)
	}
	if !errors.Is(err, ErrIntegrationUnverified) {
		t.Fatalf("expected ErrIntegrationUnverified, got %v", err)
	}

	for _, phrase := range []string{
		"script addresses",
		"intercept slot",
		"datum/redeemer schema",
		"pool ID rules",
		"reserve extraction rules",
	} {
		if !strings.Contains(err.Error(), phrase) {
			t.Fatalf("error %q missing phrase %q", err, phrase)
		}
	}
}

func TestTargetDocumentsRequiredVerification(t *testing.T) {
	t.Parallel()

	target := Target()
	if target.Protocol != ProtocolName {
		t.Fatalf("unexpected protocol: got %q want %q", target.Protocol, ProtocolName)
	}
	if target.Status != IntegrationStatusUnverified {
		t.Fatalf(
			"unexpected status: got %q want %q",
			target.Status,
			IntegrationStatusUnverified,
		)
	}
	if !target.ExternalAPIs.Optional {
		t.Fatal("external APIs must be optional")
	}
	if target.ExternalAPIs.EnabledByDefault {
		t.Fatal("external APIs must be disabled by default")
	}

	gotKeys := map[string]bool{}
	for _, item := range target.VerificationItems {
		gotKeys[item.Key] = true
		if !item.RequiredBeforeRuntime {
			t.Fatalf("verification item %q must block runtime support", item.Key)
		}
	}

	for _, key := range []string{
		"script-addresses",
		"intercept-point",
		"datum-redeemer-schema",
		"pool-id-rules",
		"reserve-extraction-rules",
	} {
		if !gotKeys[key] {
			t.Fatalf("missing verification item %q", key)
		}
	}
}

func TestVerificationChecklistReturnsCopy(t *testing.T) {
	t.Parallel()

	checklist := VerificationChecklist()
	if len(checklist) == 0 {
		t.Fatal("expected non-empty verification checklist")
	}

	checklist[0].Key = "changed"
	if VerificationChecklist()[0].Key == "changed" {
		t.Fatal("VerificationChecklist must return a defensive copy")
	}
}
