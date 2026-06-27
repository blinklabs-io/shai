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
	"testing"

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
