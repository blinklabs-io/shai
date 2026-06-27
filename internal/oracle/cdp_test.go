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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/blinklabs-io/adder/event"
	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/blinklabs-io/shai/internal/config"
)

func TestOracleIndigoCDPUpdateDeletesSpentState(t *testing.T) {
	o := newTestIndigoOracle(t)
	oldHash := strings.Repeat("a", 64)
	newHash := strings.Repeat("b", 64)
	oldID := generateIndigoCDPId(oldHash, 0)
	oldState := &CDPState{
		CDPId:    oldID,
		Network:  "mainnet",
		Protocol: IndigoProtocolName,
		Slot:     10,
	}
	o.cdps[oldID] = oldState
	if err := o.storage.SaveCDPState(oldState); err != nil {
		t.Fatalf("failed to save old CDP state: %v", err)
	}

	err := o.handleTransaction(
		event.Event{
			Context: event.TransactionContext{
				TransactionHash: newHash,
				SlotNumber:      20,
			},
		},
		event.TransactionEvent{
			BlockHash: "block-20",
			Inputs: []ledger.TransactionInput{
				shelley.NewShelleyTransactionInput(oldHash, 0),
			},
			Outputs: []ledger.TransactionOutput{
				newTestCDPOutput(t, "iUSD", 42_000_000),
			},
		},
	)
	if err != nil {
		t.Fatalf("handleTransaction returned error: %v", err)
	}

	if _, ok := o.GetCDPState(oldID); ok {
		t.Fatalf("expected spent CDP %s to be removed", oldID)
	}
	if _, err := o.storage.LoadCDPState("mainnet", IndigoProtocolName, oldID); err == nil {
		t.Fatalf("expected spent CDP %s to be deleted from storage", oldID)
	}

	newID := generateIndigoCDPId(newHash, 0)
	newState, ok := o.GetCDPState(newID)
	if !ok || newState == nil {
		t.Fatalf("expected replacement CDP %s to be tracked", newID)
	}
	if newState.MintedAmount != 42_000_000 {
		t.Fatalf("expected minted amount 42000000, got %d", newState.MintedAmount)
	}
	if newState.BlockHash != "block-20" {
		t.Fatalf("expected block hash block-20, got %s", newState.BlockHash)
	}
	if _, err := o.storage.LoadCDPState("mainnet", IndigoProtocolName, newID); err != nil {
		t.Fatalf("expected replacement CDP in storage: %v", err)
	}

	api := NewOracleAPI(o)
	defer api.Stop()
	mux := http.NewServeMux()
	api.RegisterHandlers(mux)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cdps", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}
	var response struct {
		CDPs  []*CDPState `json:"cdps"`
		Count int         `json:"count"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to decode CDP list: %v", err)
	}
	if response.Count != 1 {
		t.Fatalf("expected one CDP in API response, got %d", response.Count)
	}
	if response.CDPs[0].CDPId != newID {
		t.Fatalf("expected API to return %s, got %s", newID, response.CDPs[0].CDPId)
	}
}

func TestOracleIndigoCDPCloseDeletesSpentState(t *testing.T) {
	o := newTestIndigoOracle(t)
	oldHash := strings.Repeat("c", 64)
	oldID := generateIndigoCDPId(oldHash, 1)
	oldState := &CDPState{
		CDPId:    oldID,
		Network:  "mainnet",
		Protocol: IndigoProtocolName,
		Slot:     10,
	}
	o.cdps[oldID] = oldState
	if err := o.storage.SaveCDPState(oldState); err != nil {
		t.Fatalf("failed to save old CDP state: %v", err)
	}

	err := o.handleTransaction(
		event.Event{
			Context: event.TransactionContext{
				TransactionHash: strings.Repeat("d", 64),
				SlotNumber:      20,
			},
		},
		event.TransactionEvent{
			BlockHash: "block-20",
			Inputs: []ledger.TransactionInput{
				shelley.NewShelleyTransactionInput(oldHash, 1),
			},
		},
	)
	if err != nil {
		t.Fatalf("handleTransaction returned error: %v", err)
	}

	if o.CDPCount() != 0 {
		t.Fatalf("expected no tracked CDPs, got %d", o.CDPCount())
	}
	if _, err := o.storage.LoadCDPState("mainnet", IndigoProtocolName, oldID); err == nil {
		t.Fatalf("expected closed CDP %s to be deleted from storage", oldID)
	}
}

func TestOracleStorageCDPStateSaveLoadDelete(t *testing.T) {
	storage := newTestOracleStorage(t)
	state := &CDPState{
		CDPId:        "indigo_cdp_abc#0",
		Network:      "mainnet",
		Protocol:     IndigoProtocolName,
		IAsset:       "69555344",
		MintedAmount: 1_000_000,
	}
	if err := storage.SaveCDPState(state); err != nil {
		t.Fatalf("failed to save CDP state: %v", err)
	}

	loaded, err := storage.LoadCDPState("mainnet", IndigoProtocolName, state.CDPId)
	if err != nil {
		t.Fatalf("failed to load CDP state: %v", err)
	}
	if loaded == nil {
		t.Fatalf("expected loaded CDP state")
	}
	if loaded.CDPId != state.CDPId {
		t.Fatalf("expected CDP ID %s, got %s", state.CDPId, loaded.CDPId)
	}

	all, err := storage.LoadAllCDPStates()
	if err != nil {
		t.Fatalf("failed to load all CDP states: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 CDP state, got %d", len(all))
	}

	if err := storage.DeleteCDPState("mainnet", IndigoProtocolName, state.CDPId); err != nil {
		t.Fatalf("failed to delete CDP state: %v", err)
	}
	all, err = storage.LoadAllCDPStates()
	if err != nil {
		t.Fatalf("failed to load all CDP states after delete: %v", err)
	}
	if len(all) != 0 {
		t.Fatalf("expected 0 CDP states after delete, got %d", len(all))
	}
}

func newTestIndigoOracle(t *testing.T) *Oracle {
	t.Helper()
	profile := config.Profile{
		Name: "indigo",
		Type: config.ProfileTypeSynthetics,
		Config: config.SyntheticsProfileConfig{
			Protocol: IndigoProtocolName,
			CDPAddresses: []config.ProfileConfigAddress{
				{Address: IndigoCDPContractAddress},
			},
		},
	}
	o := New(nil, &profile, NewIndigoParser())
	o.storage = newTestOracleStorage(t)
	return o
}

func newTestCDPOutput(
	t *testing.T,
	iAsset string,
	mintedAmount int64,
) ledger.TransactionOutput {
	t.Helper()
	address, err := lcommon.NewAddress(IndigoCDPContractAddress)
	if err != nil {
		t.Fatalf("failed to parse Indigo CDP address: %v", err)
	}
	outputCbor, err := cbor.Encode(&map[uint64]any{
		0: address,
		1: uint64(2_000_000),
		2: []any{
			uint64(1),
			cbor.Tag{
				Number:  24,
				Content: testIndigoCDPDatum(t, iAsset, mintedAmount),
			},
		},
	})
	if err != nil {
		t.Fatalf("failed to encode test CDP output: %v", err)
	}
	output, err := ledger.NewTransactionOutputFromCbor(outputCbor)
	if err != nil {
		t.Fatalf("failed to decode test CDP output: %v", err)
	}
	if output.Datum() == nil {
		t.Fatal("expected decoded test CDP output to have inline datum")
	}
	return output
}

func testIndigoCDPDatum(
	t *testing.T,
	iAsset string,
	mintedAmount int64,
) []byte {
	t.Helper()
	owner := cbor.NewConstructorEncoder(1, cbor.IndefLengthList{})
	accumulatedFees := cbor.NewConstructorEncoder(0, cbor.IndefLengthList{
		int64(1),
		int64(0),
	})
	inner := cbor.NewConstructorEncoder(0, cbor.IndefLengthList{
		owner,
		[]byte(iAsset),
		mintedAmount,
		accumulatedFees,
	})
	outer := cbor.NewConstructorEncoder(0, cbor.IndefLengthList{inner})
	cborData, err := cbor.Encode(&outer)
	if err != nil {
		t.Fatalf("failed to encode Indigo CDP datum: %v", err)
	}
	return cborData
}
