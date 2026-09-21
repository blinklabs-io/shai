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
	"encoding/hex"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/blinklabs-io/adder/event"
	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/blinklabs-io/shai/common"
	"github.com/blinklabs-io/shai/dex/geniusyield"
	"github.com/blinklabs-io/shai/internal/config"
)

// Mainnet order 9e0b60d328b5b2d5b2c4a5fbf33cfe72f261ed41374c61793631ebd1ac3d2fb0#0,
// in block 457297e6384df3399db554eab7996731985da20699e294370753c2573821d8f6
// at slot 115390723. The transaction bytes in testdata were read from a
// mainnet relay over block-fetch.
const (
	mainnetOrderTxHash  = "9e0b60d328b5b2d5b2c4a5fbf33cfe72f261ed41374c61793631ebd1ac3d2fb0"
	mainnetOrderSlot    = 115390723
	mainnetOrderBlock   = "457297e6384df3399db554eab7996731985da20699e294370753c2573821d8f6"
	mainnetOrderAddress = "addr1z8kllanr6dlut7t480zzytsd52l7pz4y3kcgxlfvx2ddavcxqqhquhdjsy6s2lmffy7j04j2hq9g5z3cuzkq9dkjg8yq5zwfal"
	// Output 1 of the same transaction pays the maker's own key address. It
	// must not be mistaken for an order.
	mainnetMakerAddress = "addr1qykya8hllh6mkprj7em8ps554drh5v7qup7gzw2e9d9wjlqxqqhquhdjsy6s2lmffy7j04j2hq9g5z3cuzkq9dkjg8yqpdsss9"
)

func loadMainnetOrderTx(t *testing.T) ledger.Transaction {
	t.Helper()
	raw, err := os.ReadFile("testdata/geniusyield-mainnet-order-tx.hex")
	if err != nil {
		t.Fatalf("failed to read mainnet order transaction: %v", err)
	}
	txCbor, err := hex.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatalf("failed to decode mainnet order transaction hex: %v", err)
	}
	txType, err := ledger.DetermineTransactionType(txCbor)
	if err != nil {
		t.Fatalf("failed to determine transaction type: %v", err)
	}
	tx, err := ledger.NewTransactionFromCbor(txType, txCbor)
	if err != nil {
		t.Fatalf("failed to decode mainnet order transaction: %v", err)
	}
	if got := tx.Hash().String(); got != mainnetOrderTxHash {
		t.Fatalf("testdata transaction hash = %s, want %s", got, mainnetOrderTxHash)
	}
	return tx
}

func mainnetOrderEvent(t *testing.T, tx ledger.Transaction) event.Event {
	t.Helper()
	txEvt := event.NewTransactionEventFromTx(tx, false)
	txEvt.BlockHash = mainnetOrderBlock
	return event.Event{
		Context: event.TransactionContext{
			TransactionHash: mainnetOrderTxHash,
			SlotNumber:      mainnetOrderSlot,
		},
		Payload: txEvt,
	}
}

// orderNFTName returns the order NFT token name carried by the given output's
// value. It is the order's on-chain identity, read from the output value
// rather than from the datum the parser decodes.
func orderNFTName(t *testing.T, output ledger.TransactionOutput) []byte {
	t.Helper()
	assets := output.Assets()
	if assets == nil {
		t.Fatal("order output carries no assets")
	}
	policy, err := hex.DecodeString(geniusyield.OrderNFTPolicy)
	if err != nil {
		t.Fatalf("failed to decode order NFT policy: %v", err)
	}
	policyId := lcommon.NewBlake2b224(policy)
	names := assets.Assets(policyId)
	if len(names) != 1 {
		t.Fatalf("order output carries %d order NFTs, want 1", len(names))
	}
	if qty := assets.Asset(policyId, names[0]); qty == nil ||
		qty.Cmp(big.NewInt(1)) != 0 {
		t.Fatalf("order NFT quantity = %v, want 1", qty)
	}
	return names[0]
}

// The mainnet order sits at a base address that shares the order script's
// payment credential and carries the maker's own staking credential, and its
// datum is supplied by hash in the transaction witness set rather than inline.
// Both properties are asserted here so the oracle's matching and datum
// resolution are pinned to how the chain actually stores these orders.
func TestMainnetGeniusYieldProfileTracksRealOrder(t *testing.T) {
	profile := config.Profiles["mainnet"]["geniusyield"]
	o := New(nil, &profile, NewGeniusYieldParser())
	o.storage = newTestOracleStorage(t)

	tx := loadMainnetOrderTx(t)
	orderOutput := tx.Produced()[0].Output
	if got := orderOutput.Address().String(); got != mainnetOrderAddress {
		t.Fatalf("order output address = %s, want %s", got, mainnetOrderAddress)
	}
	if _, configured := o.orderAddresses[mainnetOrderAddress]; configured {
		t.Fatal("order address must not be a configured literal address")
	}
	if orderOutput.Datum() != nil {
		t.Fatal("order output carries an inline datum; expected a datum hash")
	}
	if orderOutput.DatumHash() == nil {
		t.Fatal("order output carries neither an inline datum nor a datum hash")
	}
	if o.isOrderAddress(mainnetMakerAddress) {
		t.Fatal("maker key address must not be treated as an order address")
	}

	if err := o.HandleChainsyncEvent(mainnetOrderEvent(t, tx)); err != nil {
		t.Fatalf("HandleChainsyncEvent returned error: %v", err)
	}

	wantId := geniusyield.GenerateOrderId(orderNFTName(t, orderOutput))
	order, ok := o.GetOrderState(wantId)
	if !ok || order == nil {
		t.Fatalf("expected order %s to be tracked", wantId)
	}
	if o.OrderCount() != 1 {
		t.Fatalf("tracked orders = %d, want 1", o.OrderCount())
	}
	if order.Protocol != "geniusyield" {
		t.Errorf("Protocol = %q, want geniusyield", order.Protocol)
	}
	if order.Network != "mainnet" {
		t.Errorf("Network = %q, want mainnet", order.Network)
	}
	if order.BlockHash != mainnetOrderBlock {
		t.Errorf("BlockHash = %q, want %s", order.BlockHash, mainnetOrderBlock)
	}
	if order.TxHash != mainnetOrderTxHash || order.TxIndex != 0 {
		t.Errorf(
			"order UTxO = %s#%d, want %s#0",
			order.TxHash,
			order.TxIndex,
			mainnetOrderTxHash,
		)
	}
	if order.Slot != mainnetOrderSlot {
		t.Errorf("Slot = %d, want %d", order.Slot, mainnetOrderSlot)
	}
	// The offered asset is ADA here, and the order output also holds the fees
	// the datum records plus the order's ADA deposit, so the decoded offer and
	// fees have to fit inside the output's own lovelace value.
	locked := order.OfferedAsset.Amount +
		order.ContainedLovelaceFee +
		order.ContainedOfferedFee
	if outputLovelace := orderOutput.Amount().Uint64(); locked > outputLovelace {
		t.Errorf(
			"offer plus contained fees = %d, exceeds the %d lovelace locked in the output",
			locked,
			outputLovelace,
		)
	}
	if len(order.OfferedAsset.Class.PolicyId) != 0 {
		t.Errorf("OfferedAsset = %+v, want ADA", order.OfferedAsset.Class)
	}
	if order.PriceNum != 5000 || order.PriceDenom != 501 {
		t.Errorf("price = %d/%d, want 5000/501", order.PriceNum, order.PriceDenom)
	}
	if got, want := hex.EncodeToString(order.AskedAsset.PolicyId), "dda5fdb1002f7389b33e036b6afee82a8189becb6cba852e8b79b4fb"; got != want {
		t.Errorf("AskedAsset policy = %s, want %s", got, want)
	}

	stored, err := o.storage.LoadOrderState("mainnet", "geniusyield", wantId)
	if err != nil {
		t.Fatalf("expected order in storage: %v", err)
	}
	if stored == nil {
		t.Fatal("expected a persisted order state")
	}
	if stored.OfferedAsset.Amount != order.OfferedAsset.Amount {
		t.Errorf(
			"persisted OfferedAmount = %d, want %d",
			stored.OfferedAsset.Amount,
			order.OfferedAsset.Amount,
		)
	}
}

func TestGeniusYieldOrderSpendRemovesTrackedOrder(t *testing.T) {
	profile := config.Profiles["mainnet"]["geniusyield"]
	o := New(nil, &profile, NewGeniusYieldParser())
	o.storage = newTestOracleStorage(t)

	tx := loadMainnetOrderTx(t)
	if err := o.HandleChainsyncEvent(mainnetOrderEvent(t, tx)); err != nil {
		t.Fatalf("HandleChainsyncEvent returned error: %v", err)
	}
	orderId := geniusyield.GenerateOrderId(
		orderNFTName(t, tx.Produced()[0].Output),
	)
	if _, ok := o.GetOrderState(orderId); !ok {
		t.Fatalf("expected order %s to be tracked", orderId)
	}

	spendHash := strings.Repeat("b", 64)
	err := o.HandleChainsyncEvent(event.Event{
		Context: event.TransactionContext{
			TransactionHash: spendHash,
			SlotNumber:      mainnetOrderSlot + 1,
		},
		Payload: event.TransactionEvent{
			BlockHash: strings.Repeat("c", 64),
			Inputs: []ledger.TransactionInput{
				shelley.NewShelleyTransactionInput(mainnetOrderTxHash, 0),
			},
		},
	})
	if err != nil {
		t.Fatalf("HandleChainsyncEvent for spend returned error: %v", err)
	}
	if _, ok := o.GetOrderState(orderId); ok {
		t.Fatalf("expected spent order %s to be removed", orderId)
	}
	if _, err := o.storage.LoadOrderState(
		"mainnet",
		"geniusyield",
		orderId,
	); err == nil {
		t.Fatalf("expected spent order %s to be deleted from storage", orderId)
	}
}

func TestGeniusYieldOrderSurvivesRestartAndRollsBack(t *testing.T) {
	profile := config.Profiles["mainnet"]["geniusyield"]
	storage := newTestOracleStorage(t)
	o := New(nil, &profile, NewGeniusYieldParser())
	o.storage = storage

	tx := loadMainnetOrderTx(t)
	if err := o.HandleChainsyncEvent(mainnetOrderEvent(t, tx)); err != nil {
		t.Fatalf("HandleChainsyncEvent returned error: %v", err)
	}
	orderId := geniusyield.GenerateOrderId(
		orderNFTName(t, tx.Produced()[0].Output),
	)

	restarted := New(nil, &profile, NewGeniusYieldParser())
	restarted.storage = storage
	if err := restarted.loadPersistedStates(); err != nil {
		t.Fatalf("failed to load persisted order states: %v", err)
	}
	if _, ok := restarted.GetOrderState(orderId); !ok {
		t.Fatalf("expected order %s to survive a restart", orderId)
	}

	if err := restarted.handleRollback(event.RollbackEvent{
		SlotNumber: mainnetOrderSlot - 1,
		BlockHash:  profile.InterceptHash,
	}); err != nil {
		t.Fatalf("handleRollback returned error: %v", err)
	}
	if _, ok := restarted.GetOrderState(orderId); ok {
		t.Fatalf("expected rolled-back order %s to be removed", orderId)
	}
	if _, err := storage.LoadOrderState(
		"mainnet",
		"geniusyield",
		orderId,
	); err == nil {
		t.Fatalf("expected rolled-back order %s to be deleted from storage", orderId)
	}
	// The spent-input index must be rebuilt from storage on restart, and
	// cleared by the rollback, so a later spend of the same UTxO is a no-op.
	if err := restarted.HandleChainsyncEvent(event.Event{
		Context: event.TransactionContext{
			TransactionHash: strings.Repeat("d", 64),
			SlotNumber:      mainnetOrderSlot + 1,
		},
		Payload: event.TransactionEvent{
			Inputs: []ledger.TransactionInput{
				shelley.NewShelleyTransactionInput(mainnetOrderTxHash, 0),
			},
		},
	}); err != nil {
		t.Fatalf("HandleChainsyncEvent for spend returned error: %v", err)
	}
}

func TestOrderAPIServesTrackedOrder(t *testing.T) {
	profile := config.Profiles["mainnet"]["geniusyield"]
	o := New(nil, &profile, NewGeniusYieldParser())
	o.storage = newTestOracleStorage(t)

	tx := loadMainnetOrderTx(t)
	if err := o.HandleChainsyncEvent(mainnetOrderEvent(t, tx)); err != nil {
		t.Fatalf("HandleChainsyncEvent returned error: %v", err)
	}
	orderId := geniusyield.GenerateOrderId(
		orderNFTName(t, tx.Produced()[0].Output),
	)

	mux := http.NewServeMux()
	NewOracleAPI(o).RegisterHandlers(mux)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(
		rr,
		httptest.NewRequest(
			http.MethodGet,
			"/api/v1/orders?protocol=geniusyield",
			nil,
		),
	)
	if rr.Code != http.StatusOK {
		t.Fatalf("list orders status = %d, want 200", rr.Code)
	}
	var list struct {
		Orders []*OrderState `json:"orders"`
		Count  int           `json:"count"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &list); err != nil {
		t.Fatalf("failed to decode order list: %v", err)
	}
	if list.Count != 1 || len(list.Orders) != 1 {
		t.Fatalf("order list count = %d, want 1", list.Count)
	}
	if list.Orders[0].OrderId != orderId {
		t.Errorf("listed order = %s, want %s", list.Orders[0].OrderId, orderId)
	}

	rr = httptest.NewRecorder()
	mux.ServeHTTP(
		rr,
		httptest.NewRequest(http.MethodGet, "/api/v1/orders/"+orderId, nil),
	)
	if rr.Code != http.StatusOK {
		t.Fatalf("get order status = %d, want 200", rr.Code)
	}
	var single OrderState
	if err := json.Unmarshal(rr.Body.Bytes(), &single); err != nil {
		t.Fatalf("failed to decode order: %v", err)
	}
	if single.OrderId != orderId {
		t.Errorf("order = %s, want %s", single.OrderId, orderId)
	}

	rr = httptest.NewRecorder()
	mux.ServeHTTP(
		rr,
		httptest.NewRequest(http.MethodGet, "/api/v1/orders/gy_missing", nil),
	)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("missing order status = %d, want 404", rr.Code)
	}
}

// A tracked order's IsActive is recorded when the chain last produced it. A
// time-bounded order starts or expires with no transaction to re-observe, so
// the API re-evaluates activity at the serving time.
func TestOrderAPIReevaluatesActivityAtServingTime(t *testing.T) {
	now := time.Now()
	ended := now.Add(-time.Hour)
	starts := now.Add(time.Hour)
	o := &Oracle{
		orders: map[string]*OrderState{
			"gy_expired": {
				OrderId:      "gy_expired",
				Protocol:     "geniusyield",
				OfferedAsset: common.AssetAmount{Amount: 1_000_000},
				EndTime:      &ended,
				IsActive:     true,
			},
			"gy_pending": {
				OrderId:      "gy_pending",
				Protocol:     "geniusyield",
				OfferedAsset: common.AssetAmount{Amount: 1_000_000},
				StartTime:    &starts,
				IsActive:     true,
			},
			"gy_open": {
				OrderId:      "gy_open",
				Protocol:     "geniusyield",
				OfferedAsset: common.AssetAmount{Amount: 1_000_000},
				IsActive:     true,
			},
		},
	}

	mux := http.NewServeMux()
	NewOracleAPI(o).RegisterHandlers(mux)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(
		rr,
		httptest.NewRequest(http.MethodGet, "/api/v1/orders", nil),
	)
	if rr.Code != http.StatusOK {
		t.Fatalf("list orders status = %d, want 200", rr.Code)
	}
	var list struct {
		Orders []*OrderState `json:"orders"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &list); err != nil {
		t.Fatalf("failed to decode order list: %v", err)
	}
	want := map[string]bool{
		"gy_expired": false,
		"gy_pending": false,
		"gy_open":    true,
	}
	if len(list.Orders) != len(want) {
		t.Fatalf("listed %d orders, want %d", len(list.Orders), len(want))
	}
	for _, order := range list.Orders {
		expected, known := want[order.OrderId]
		if !known {
			t.Fatalf("unexpected order %s", order.OrderId)
		}
		if order.IsActive != expected {
			t.Errorf(
				"%s IsActive = %t, want %t",
				order.OrderId,
				order.IsActive,
				expected,
			)
		}
	}

	for orderId, expected := range want {
		rr = httptest.NewRecorder()
		mux.ServeHTTP(
			rr,
			httptest.NewRequest(http.MethodGet, "/api/v1/orders/"+orderId, nil),
		)
		if rr.Code != http.StatusOK {
			t.Fatalf("get %s status = %d, want 200", orderId, rr.Code)
		}
		var single OrderState
		if err := json.Unmarshal(rr.Body.Bytes(), &single); err != nil {
			t.Fatalf("failed to decode order %s: %v", orderId, err)
		}
		if single.IsActive != expected {
			t.Errorf(
				"%s IsActive = %t, want %t",
				orderId,
				single.IsActive,
				expected,
			)
		}
	}

	// The tracked order itself is untouched by serving it.
	tracked, ok := o.GetOrderState("gy_expired")
	if !ok || !tracked.IsActive {
		t.Error("serving an order must not rewrite its recorded IsActive")
	}
}

// newTestOrderOutput builds an output at the given address carrying an inline
// datum and no native tokens.
func newTestOrderOutput(
	t *testing.T,
	addr string,
	datum []byte,
) ledger.TransactionOutput {
	t.Helper()
	address, err := lcommon.NewAddress(addr)
	if err != nil {
		t.Fatalf("failed to parse address %s: %v", addr, err)
	}
	outputCbor, err := cbor.Encode(&map[uint64]any{
		0: address,
		1: uint64(2_000_000),
		2: []any{
			uint64(1),
			cbor.Tag{Number: 24, Content: datum},
		},
	})
	if err != nil {
		t.Fatalf("failed to encode test order output: %v", err)
	}
	output, err := ledger.NewTransactionOutputFromCbor(outputCbor)
	if err != nil {
		t.Fatalf("failed to decode test order output: %v", err)
	}
	if output.Datum() == nil {
		t.Fatal("expected decoded test order output to have an inline datum")
	}
	return output
}

// A script address can be paid to by anyone. An output parked at the order
// script's payment credential is only an order if it holds the order NFT, and
// the order key comes from the datum, so accepting one without the NFT lets a
// forged output displace the genuine order that carries it.
func TestGeniusYieldOrderWithoutItsNFTIsIgnored(t *testing.T) {
	profile := config.Profiles["mainnet"]["geniusyield"]
	o := New(nil, &profile, NewGeniusYieldParser())
	o.storage = newTestOracleStorage(t)

	tx := loadMainnetOrderTx(t)
	orderOutput := tx.Produced()[0].Output
	datumCbor := outputDatumCbor(
		orderOutput,
		witnessDatums(event.NewTransactionEventFromTx(tx, false)),
	)
	if datumCbor == nil {
		t.Fatal("failed to resolve the mainnet order datum")
	}
	if err := o.HandleChainsyncEvent(mainnetOrderEvent(t, tx)); err != nil {
		t.Fatalf("HandleChainsyncEvent returned error: %v", err)
	}
	orderId := geniusyield.GenerateOrderId(orderNFTName(t, orderOutput))
	if _, ok := o.GetOrderState(orderId); !ok {
		t.Fatalf("expected order %s to be tracked", orderId)
	}

	forgedTxHash := strings.Repeat("e", 64)
	if err := o.HandleChainsyncEvent(event.Event{
		Context: event.TransactionContext{
			TransactionHash: forgedTxHash,
			SlotNumber:      mainnetOrderSlot + 1,
		},
		Payload: event.TransactionEvent{
			BlockHash: strings.Repeat("f", 64),
			Outputs: []ledger.TransactionOutput{
				// Same order address and same datum, but the output holds
				// no order NFT.
				newTestOrderOutput(t, mainnetOrderAddress, datumCbor),
			},
		},
	}); err != nil {
		t.Fatalf("HandleChainsyncEvent for forged output returned error: %v", err)
	}

	if o.OrderCount() != 1 {
		t.Fatalf("tracked orders = %d, want 1", o.OrderCount())
	}
	tracked, ok := o.GetOrderState(orderId)
	if !ok {
		t.Fatalf("expected order %s to remain tracked", orderId)
	}
	if tracked.TxHash != mainnetOrderTxHash {
		t.Errorf(
			"order UTxO = %s, want the genuine %s",
			tracked.TxHash,
			mainnetOrderTxHash,
		)
	}
}
