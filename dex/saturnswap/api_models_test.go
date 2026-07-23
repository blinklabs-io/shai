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
	"os"
	"strings"
	"testing"
)

func TestPoolFeeUnitsFromAPIResponseFixture(t *testing.T) {
	// Captured from DefaultGraphQLEndpoint on 2026-07-23. The values also
	// match the fee calculation shipped by SaturnSwap's web client.
	payload, err := os.ReadFile("testdata/pools_fee_units.json")
	if err != nil {
		t.Fatalf("read pool fixture: %v", err)
	}

	var response struct {
		Data struct {
			Pools PoolConnection `json:"pools"`
		} `json:"data"`
	}
	if err := json.Unmarshal(payload, &response); err != nil {
		t.Fatalf("unmarshal pool fixture: %v", err)
	}
	if len(response.Data.Pools.Nodes) != 2 {
		t.Fatalf(
			"pool fixture contains %d nodes, want 2",
			len(response.Data.Pools.Nodes),
		)
	}

	tests := []struct {
		name       string
		pool       Pool
		wantFeeNum uint64
	}{
		{
			name:       "0.5 percent LP fee with 30 percent protocol share",
			pool:       response.Data.Pools.Nodes[0],
			wantFeeNum: 9985,
		},
		{
			name:       "1 percent LP fee with 30 percent protocol share",
			pool:       response.Data.Pools.Nodes[1],
			wantFeeNum: 9970,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			feeNum, feeDenom, err := test.pool.EffectiveFeeParts()
			if err != nil {
				t.Fatalf("EffectiveFeeParts returned error: %v", err)
			}
			if feeNum != test.wantFeeNum || feeDenom != FeeDenom {
				t.Fatalf(
					"fee parts = %d/%d, want %d/%d",
					feeNum,
					feeDenom,
					test.wantFeeNum,
					FeeDenom,
				)
			}
		})
	}
}

func TestPoolEffectiveFeePartsOptionalProtocolFee(t *testing.T) {
	tests := []struct {
		name    string
		pool    Pool
		wantErr string
	}{
		{
			name: "omitted protocol fee defaults to zero",
			pool: Pool{
				LPFeePercent: "0.5",
			},
		},
		{
			name: "malformed protocol fee is rejected",
			pool: Pool{
				LPFeePercent:       "0.5",
				ProtocolFeePercent: "invalid",
			},
			wantErr: "protocol_fee_percent",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			feeNum, feeDenom, err := test.pool.EffectiveFeeParts()
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf(
						"EffectiveFeeParts error = %v, want error containing %q",
						err,
						test.wantErr,
					)
				}
				return
			}
			if err != nil {
				t.Fatalf("EffectiveFeeParts returned error: %v", err)
			}
			if feeNum != FeeDenom || feeDenom != FeeDenom {
				t.Fatalf(
					"fee parts = %d/%d, want %d/%d",
					feeNum,
					feeDenom,
					FeeDenom,
					FeeDenom,
				)
			}
		})
	}
}

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
