package geniusyield

import (
	"encoding/hex"
	"math/big"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/blinklabs-io/shai/common"
	dexgy "github.com/blinklabs-io/shai/dex/geniusyield"
	"github.com/blinklabs-io/shai/internal/node"
)

func TestOwnedMempoolOutputIsNotActionableBeforeConfirmation(t *testing.T) {
	gy := newLifecycleTestGeniusYield()
	txBytes, orderAddress, policy := lifecycleTestOrderTransaction(t)
	gy.orderAddresses = []string{orderAddress}
	gy.config.OrderNFTPolicy = hex.EncodeToString(policy)
	tx, err := ledger.NewTransactionFromCbor(babbage.EraIdBabbage, txBytes)
	if err != nil {
		t.Fatalf("decode test transaction: %v", err)
	}
	err = gy.handleMempoolNewTransaction(node.TxsubmissionMempoolTransaction{
		Hash:   tx.Hash().String(),
		Type:   babbage.EraIdBabbage,
		Cbor:   txBytes,
		Source: node.MempoolTransactionSourceOwned,
	})
	if err != nil {
		t.Fatalf("handle owned transaction: %v", err)
	}
	if got := gy.sor.GetOrderCount(); got != 0 {
		t.Fatalf("unconfirmed output entered the order book: %d", got)
	}
	if _, ok := gy.pendingTransactions[tx.Hash().String()]; !ok {
		t.Fatal("owned transaction inputs were not reserved")
	}
}

func TestPendingSpendersReferenceCountReservations(t *testing.T) {
	gy := newLifecycleTestGeniusYield()
	input := lifecycleTestInput(0x11, 2)
	chainOrder := lifecycleTestOrder(
		"gy_chain",
		input.Id().String(),
		input.Index(),
	)
	gy.chainOrders[transactionInputRef(input)] = chainOrder
	gy.syncOrderLocked(chainOrder.state.OrderId)

	if !gy.reservePendingTransaction("pending-a", []ledger.TransactionInput{input}) {
		t.Fatal("first pending transaction was not registered")
	}
	if !gy.reservePendingTransaction("pending-b", []ledger.TransactionInput{input}) {
		t.Fatal("second pending transaction was not registered")
	}
	if gy.sor.GetOrderCount() != 0 {
		t.Fatal("reserved chain order remained available")
	}

	gy.handleMempoolRemovedTransaction(
		node.TxsubmissionMempoolTransaction{Hash: "pending-a"},
		node.MempoolRemovalReasonExpired,
	)
	if gy.sor.GetOrderCount() != 0 {
		t.Fatal("order returned while a competing spender remained pending")
	}

	gy.handleMempoolRemovedTransaction(
		node.TxsubmissionMempoolTransaction{Hash: "pending-b"},
		node.MempoolRemovalReasonExpired,
	)
	if got := gy.sor.GetOrderCount(); got != 1 {
		t.Fatalf("expected chain order restored after final expiry, got %d", got)
	}
}

func TestPeerMempoolTransactionsCannotChangeOrders(t *testing.T) {
	gy := newLifecycleTestGeniusYield()
	input := lifecycleTestInput(0x19, 1)
	order := lifecycleTestOrder(
		"gy_peer_boundary",
		input.Id().String(),
		input.Index(),
	)
	gy.chainOrders[transactionInputRef(input)] = order
	gy.syncOrderLocked(order.state.OrderId)
	peerTx := node.TxsubmissionMempoolTransaction{
		Hash:   "peer-claimed-order",
		Cbor:   []byte("unvalidated peer body"),
		Source: node.MempoolTransactionSourceObserved,
	}
	if err := gy.handleMempoolNewTransaction(peerTx); err != nil {
		t.Fatalf("peer transaction crossed trust boundary: %v", err)
	}
	if err := gy.handleMempoolNewTransaction(peerTx); err != nil {
		t.Fatalf("repeated peer transaction crossed trust boundary: %v", err)
	}
	if len(gy.pendingTransactions) != 0 {
		t.Fatal("peer transaction created actionable order state")
	}
	if got := gy.sor.GetOrderCount(); got != 1 {
		t.Fatalf("peer transaction suppressed chain order, got %d orders", got)
	}
}

func TestConfirmationReconcilesOrder(t *testing.T) {
	gy := newLifecycleTestGeniusYield()
	input := lifecycleTestInput(0x22, 0)
	oldOrder := lifecycleTestOrder(
		"gy_partial",
		input.Id().String(),
		input.Index(),
	)
	gy.chainOrders[transactionInputRef(input)] = oldOrder
	gy.syncOrderLocked(oldOrder.state.OrderId)

	const txHash = "confirmed"
	gy.reservePendingTransaction(txHash, []ledger.TransactionInput{input})
	newOrder := lifecycleTestOrder(oldOrder.state.OrderId, txHash, 1)
	gy.handleMempoolRemovedTransaction(
		node.TxsubmissionMempoolTransaction{Hash: txHash},
		node.MempoolRemovalReasonConfirmed,
	)
	pending := gy.pendingTransactions[txHash]
	if pending == nil || !pending.confirming {
		t.Fatal("confirmation removal released pending state before chain event")
	}

	gy.applyChainTransaction(
		txHash,
		[]ledger.TransactionInput{input},
		[]*trackedOrder{newOrder},
	)
	if got := visibleLifecycleOrder(t, gy, oldOrder.state.OrderId); got != newOrder.state {
		t.Fatal("confirmed order did not replace consumed chain order")
	}
	if _, ok := gy.pendingTransactions[txHash]; ok {
		t.Fatal("confirmed transaction remained pending")
	}

}

func TestConfirmedOrderMustRemainUnreservedBeforeMatching(t *testing.T) {
	gy := newLifecycleTestGeniusYield()
	order := lifecycleTestOrder("gy_confirmed", "confirmed", 0)
	ref := orderRef(order.state.TxHash, order.state.TxIndex)
	gy.chainOrders[ref] = order
	if !gy.confirmedOrderEligible(order, time.Now()) {
		t.Fatal("current unreserved order was not eligible")
	}
	gy.reservedBy[ref] = map[string]struct{}{"spender": {}}
	if gy.confirmedOrderEligible(order, time.Now()) {
		t.Fatal("reserved confirmed order remained eligible")
	}
	delete(gy.reservedBy, ref)
	gy.chainOrders[ref] = lifecycleTestOrder("replacement", "confirmed", 0)
	if gy.confirmedOrderEligible(order, time.Now()) {
		t.Fatal("replaced confirmed order remained eligible")
	}
}

func TestReleasedReservationDoesNotRestoreExpiredOrder(t *testing.T) {
	gy := newLifecycleTestGeniusYield()
	input := lifecycleTestInput(0x23, 0)
	order := lifecycleTestOrder("gy_expired", input.Id().String(), input.Index())
	future := time.Now().Add(time.Hour)
	order.state.EndTime = &future
	gy.chainOrders[transactionInputRef(input)] = order
	gy.syncOrderLocked(order.state.OrderId)
	if got := gy.sor.GetOrderCount(); got != 1 {
		t.Fatalf("expected active order, got %d", got)
	}

	gy.reservePendingTransaction("pending", []ledger.TransactionInput{input})
	past := time.Now().Add(-time.Second)
	order.state.EndTime = &past
	gy.handleMempoolRemovedTransaction(
		node.TxsubmissionMempoolTransaction{Hash: "pending"},
		node.MempoolRemovalReasonExpired,
	)
	if got := gy.sor.GetOrderCount(); got != 0 {
		t.Fatalf("expired order was restored after reservation release: %d", got)
	}
}

func TestFutureOrderBecomesRoutableAfterStart(t *testing.T) {
	gy := newLifecycleTestGeniusYield()
	input := lifecycleTestInput(0x24, 0)
	order := lifecycleTestOrder("gy_future", input.Id().String(), input.Index())
	start := time.Now().Add(time.Hour)
	order.state.StartTime = &start
	gy.chainOrders[transactionInputRef(input)] = order
	gy.syncOrderLocked(order.state.OrderId)
	if got := gy.sor.GetOrderCount(); got != 1 {
		t.Fatalf("future order was not retained: %d", got)
	}
	_, err := gy.sor.FindRoute(
		common.Lovelace(),
		order.state.OfferedAsset,
		1_000_000,
		10_000,
	)
	if err == nil {
		t.Fatal("future order was routable before its start")
	}
	past := time.Now().Add(-time.Second)
	order.state.StartTime = &past
	route, err := gy.sor.FindRoute(
		common.Lovelace(),
		order.state.OfferedAsset,
		1_000_000,
		10_000,
	)
	if err != nil {
		t.Fatalf("future order did not become routable: %v", err)
	}
	if len(route.Legs) != 1 || route.Legs[0].Order != order.state {
		t.Fatalf("unexpected route after order start: %+v", route.Legs)
	}
}

func TestRouteRejectsIneligibleOrderAtExecution(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name   string
		mutate func(*dexgy.OrderState)
	}{
		{
			name: "inactive",
			mutate: func(order *dexgy.OrderState) {
				order.IsActive = false
			},
		},
		{
			name: "future",
			mutate: func(order *dexgy.OrderState) {
				start := now.Add(time.Minute)
				order.StartTime = &start
			},
		},
		{
			name: "expired",
			mutate: func(order *dexgy.OrderState) {
				end := now.Add(-time.Minute)
				order.EndTime = &end
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			order := lifecycleTestOrder("ineligible", "tx", 0)
			tt.mutate(order.state)
			route := &Route{Legs: []RouteLeg{{Order: order.state}}}
			if routeEligible(route, now) {
				t.Fatal("route with ineligible order was executable")
			}
		})
	}
}

func newLifecycleTestGeniusYield() *GeniusYield {
	return &GeniusYield{
		sor:                 NewSmartOrderRouter(),
		chainOrders:         make(map[string]*trackedOrder),
		pendingTransactions: make(map[string]*pendingTransaction),
		reservedBy:          make(map[string]map[string]struct{}),
	}
}

func lifecycleTestInput(hashByte byte, index uint32) ledger.TransactionInput {
	var txHash lcommon.Blake2b256
	txHash[0] = hashByte
	return ledger.ShelleyTransactionInput{
		TxId:        txHash,
		OutputIndex: index,
	}
}

func lifecycleTestOrder(orderID, txHash string, txIndex uint32) *trackedOrder {
	return &trackedOrder{state: &dexgy.OrderState{
		OrderId: orderID,
		OfferedAsset: common.AssetClass{
			PolicyId: []byte{0x44},
			Name:     []byte("TOKEN"),
		},
		OfferedAmount: 1_000_000,
		AskedAsset:    common.Lovelace(),
		Price:         1,
		PriceNum:      1,
		PriceDenom:    1,
		IsActive:      true,
		TxHash:        txHash,
		TxIndex:       txIndex,
		Timestamp:     time.Now(),
	}}
}

func lifecycleTestOrderTransaction(t *testing.T) ([]byte, string, []byte) {
	t.Helper()
	address, err := lcommon.NewAddress(
		"addr_test1wrtt4xm4p84vse3g3l6swtf2rqs943t0w39ustwdszxt3lsyrt40u",
	)
	if err != nil {
		t.Fatalf("parse test address: %v", err)
	}
	policy := make([]byte, lcommon.Blake2b224Size)
	policy[0] = 0x42
	nft := []byte("ORDER")
	none := cbor.NewConstructorEncoder(1, []any{})
	credential := cbor.NewConstructorEncoder(0, []any{make([]byte, 28)})
	ownerAddress := cbor.NewConstructorEncoder(
		0,
		[]any{credential, none},
	)
	lovelace := cbor.NewConstructorEncoder(0, []any{[]byte{}, []byte{}})
	rational := cbor.NewConstructorEncoder(0, []any{int64(1), int64(1)})
	containedFee := cbor.NewConstructorEncoder(
		0,
		[]any{uint64(0), uint64(0), uint64(0)},
	)
	datumBytes, err := cbor.Encode(cbor.NewConstructorEncoder(0, []any{
		make([]byte, 28), ownerAddress, lovelace,
		uint64(2_000_000), uint64(2_000_000), lovelace,
		rational, nft, none, none, uint64(0), uint64(0),
		rational, uint64(0), containedFee, uint64(0),
	}))
	if err != nil {
		t.Fatalf("encode order datum: %v", err)
	}
	var datumOption babbage.BabbageTransactionOutputDatumOption
	if _, err := cbor.Decode(
		mustEncodeLifecycleTest(t, []any{
			babbage.DatumOptionTypeData,
			cbor.Tag{Number: 24, Content: datumBytes},
		}),
		&datumOption,
	); err != nil {
		t.Fatalf("decode datum option: %v", err)
	}
	assets := lcommon.NewMultiAsset[lcommon.MultiAssetTypeOutput](
		map[lcommon.Blake2b224]map[cbor.ByteString]*big.Int{
			lcommon.NewBlake2b224(policy): {
				cbor.NewByteString(nft): big.NewInt(1),
			},
		},
	)
	tx := &babbage.BabbageTransaction{
		Body: babbage.BabbageTransactionBody{
			TxInputs: shelley.NewShelleyTransactionInputSet(
				[]shelley.ShelleyTransactionInput{{}},
			),
			TxOutputs: []babbage.BabbageTransactionOutput{{
				OutputAddress: address,
				OutputAmount: mary.MaryTransactionOutputValue{
					Amount: 2_000_000,
					Assets: &assets,
				},
				DatumOption: &datumOption,
			}},
		},
		TxIsValid: true,
	}
	txBytes, err := tx.MarshalCBOR()
	if err != nil {
		t.Fatalf("encode test transaction: %v", err)
	}
	return txBytes, address.String(), policy
}

func mustEncodeLifecycleTest(t *testing.T, value any) []byte {
	t.Helper()
	data, err := cbor.Encode(value)
	if err != nil {
		t.Fatalf("encode test value: %v", err)
	}
	return data
}

func visibleLifecycleOrder(
	t *testing.T,
	gy *GeniusYield,
	orderID string,
) *dexgy.OrderState {
	t.Helper()
	for _, pair := range gy.sor.GetAllPairs() {
		book := gy.sor.GetOrderBook(pair)
		if book == nil {
			continue
		}
		for _, entry := range append(book.snapshotBids(), book.snapshotAsks()...) {
			if entry.Order.OrderId == orderID {
				return entry.Order
			}
		}
	}
	t.Fatalf("order %q is not visible", orderID)
	return nil
}
