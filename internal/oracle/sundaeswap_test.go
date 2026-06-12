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
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
)

func TestNewSundaeSwapV1Parser(t *testing.T) {
	parser := NewSundaeSwapV1Parser()
	if parser == nil {
		t.Fatal("expected non-nil parser")
	}
	if parser.Protocol() != "sundaeswap-v1" {
		t.Errorf("expected protocol 'sundaeswap-v1', got %s", parser.Protocol())
	}

	var _ PoolParser = parser
}

func TestSundaeSwapV1ParserParsePoolDatum(t *testing.T) {
	ident := make([]byte, 28)
	for i := range ident {
		ident[i] = byte(i)
	}

	tokenPolicy := make([]byte, 28)
	for i := range tokenPolicy {
		tokenPolicy[i] = 0xab
	}

	assetA := cbor.NewConstructorEncoder(0, cbor.IndefLengthList{
		[]byte{},
		[]byte{},
	})
	assetB := cbor.NewConstructorEncoder(0, cbor.IndefLengthList{
		tokenPolicy,
		[]byte("SUNDAE"),
	})
	datum := cbor.NewConstructorEncoder(0, cbor.IndefLengthList{
		ident,
		assetA,
		assetB,
		uint64(500000000),
		uint64(30),
	})
	cborData, err := cbor.Encode(&datum)
	if err != nil {
		t.Fatalf("failed to encode datum: %v", err)
	}

	utxoValue, err := buildMaryOutputCbor(
		100000000,
		tokenPolicy,
		[]byte("SUNDAE"),
		250000000,
	)
	if err != nil {
		t.Fatalf("failed to build UTxO output: %v", err)
	}

	parser := NewSundaeSwapV1Parser()
	state, err := parser.ParsePoolDatum(
		cborData,
		utxoValue,
		"abc123",
		2,
		12345,
		time.Now(),
	)
	if err != nil {
		t.Fatalf("failed to parse datum: %v", err)
	}

	if state.PoolId != "sundaeswap_000102030405060708090a0b0c0d0e0f101112131415161718191a1b" {
		t.Errorf("unexpected pool ID: %s", state.PoolId)
	}
	if state.Protocol != "sundaeswap-v1" {
		t.Errorf("expected protocol 'sundaeswap-v1', got %s", state.Protocol)
	}
	if state.Slot != 12345 {
		t.Errorf("expected slot 12345, got %d", state.Slot)
	}
	if state.TxHash != "abc123" {
		t.Errorf("expected txHash 'abc123', got %s", state.TxHash)
	}
	if state.TxIndex != 2 {
		t.Errorf("expected txIndex 2, got %d", state.TxIndex)
	}
	if state.AssetX.Amount != 100000000 {
		t.Errorf("expected assetX amount 100000000, got %d", state.AssetX.Amount)
	}
	if state.AssetY.Amount != 250000000 {
		t.Errorf("expected assetY amount 250000000, got %d", state.AssetY.Amount)
	}
	if state.FeeNum != 9970 {
		t.Errorf("expected feeNum 9970, got %d", state.FeeNum)
	}
	if state.FeeDenom != 10000 {
		t.Errorf("expected feeDenom 10000, got %d", state.FeeDenom)
	}
}
