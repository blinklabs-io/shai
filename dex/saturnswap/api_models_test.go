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
	"encoding/json"
	"testing"
)

func TestOrderBookPoolUtxoUnmarshalJSON(t *testing.T) {
	payload := []byte(`{"id":"order-1","pool_utxo_type":"LIMIT_BUY_ORDER",` +
		`"price":"1.25","undocumented":{"datum":"d87980"}}`)
	wantRaw := append([]byte(nil), payload...)

	var utxo OrderBookPoolUtxo
	if err := json.Unmarshal(payload, &utxo); err != nil {
		t.Fatalf("unmarshal order-book UTxO: %v", err)
	}
	if utxo.ID != "order-1" {
		t.Fatalf("ID = %q, want %q", utxo.ID, "order-1")
	}
	if utxo.Type != PoolUtxoTypeLimitBuyOrder {
		t.Fatalf(
			"Type = %q, want %q",
			utxo.Type,
			PoolUtxoTypeLimitBuyOrder,
		)
	}
	if utxo.Price != "1.25" {
		t.Fatalf("Price = %q, want %q", utxo.Price, "1.25")
	}
	if !bytes.Equal(utxo.Raw, wantRaw) {
		t.Fatalf("Raw = %s, want %s", utxo.Raw, wantRaw)
	}

	payload[0] = '['
	if !bytes.Equal(utxo.Raw, wantRaw) {
		t.Fatalf("Raw changed with input buffer: got %s, want %s", utxo.Raw, wantRaw)
	}

	var rawFields map[string]json.RawMessage
	if err := json.Unmarshal(utxo.Raw, &rawFields); err != nil {
		t.Fatalf("unmarshal retained raw payload: %v", err)
	}
	if _, ok := rawFields["undocumented"]; !ok {
		t.Fatal("retained raw payload does not include undocumented field")
	}
}
