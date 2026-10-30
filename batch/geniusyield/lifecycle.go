package geniusyield

import (
	"fmt"
	"sort"
	"time"

	"github.com/blinklabs-io/gouroboros/ledger"
	dexgy "github.com/blinklabs-io/shai/dex/geniusyield"
	"github.com/blinklabs-io/shai/internal/node"
)

type trackedOrder struct {
	state      *dexgy.OrderState
	address    string
	outputCbor []byte
}

type pendingTransaction struct {
	inputs     map[string]struct{}
	confirming bool
}

func orderRef(txHash string, txIndex uint32) string {
	return fmt.Sprintf("%s.%d", txHash, txIndex)
}

func transactionInputRef(input ledger.TransactionInput) string {
	return orderRef(input.Id().String(), input.Index())
}

func (gy *GeniusYield) reservePendingTransaction(
	txHash string,
	inputs []ledger.TransactionInput,
) bool {
	gy.mu.Lock()
	defer gy.mu.Unlock()

	if _, ok := gy.pendingTransactions[txHash]; ok {
		return false
	}
	pending := &pendingTransaction{
		inputs: make(map[string]struct{}),
	}
	affected := make(map[string]struct{})
	for _, input := range inputs {
		ref := transactionInputRef(input)
		if _, ok := pending.inputs[ref]; ok {
			continue
		}
		pending.inputs[ref] = struct{}{}
		spenders := gy.reservedBy[ref]
		if spenders == nil {
			spenders = make(map[string]struct{})
			gy.reservedBy[ref] = spenders
		}
		spenders[txHash] = struct{}{}
		gy.addAffectedOrderLocked(affected, ref)
	}
	gy.pendingTransactions[txHash] = pending
	gy.syncAffectedOrdersLocked(affected)
	return true
}

func (gy *GeniusYield) handleMempoolRemovedTransaction(
	tx node.TxsubmissionMempoolTransaction,
	reason node.MempoolRemovalReason,
) {
	gy.mu.Lock()
	defer gy.mu.Unlock()

	pending := gy.pendingTransactions[tx.Hash]
	if pending == nil {
		return
	}
	if reason == node.MempoolRemovalReasonConfirmed {
		pending.confirming = true
		return
	}
	affected := make(map[string]struct{})
	gy.dropPendingTransactionLocked(tx.Hash, affected)
	gy.syncAffectedOrdersLocked(affected)
}

func (gy *GeniusYield) dropPendingTransactionLocked(
	txHash string,
	affected map[string]struct{},
) {
	pending := gy.pendingTransactions[txHash]
	if pending == nil {
		return
	}
	for ref := range pending.inputs {
		spenders := gy.reservedBy[ref]
		delete(spenders, txHash)
		if len(spenders) == 0 {
			delete(gy.reservedBy, ref)
		}
		gy.addAffectedOrderLocked(affected, ref)
	}
	delete(gy.pendingTransactions, txHash)
}

func (gy *GeniusYield) applyChainTransaction(
	txHash string,
	inputs []ledger.TransactionInput,
	outputs []*trackedOrder,
) {
	gy.mu.Lock()
	defer gy.mu.Unlock()

	affected := make(map[string]struct{})
	pendingToDrop := map[string]struct{}{txHash: {}}
	for _, input := range inputs {
		ref := transactionInputRef(input)
		for pendingHash := range gy.reservedBy[ref] {
			pendingToDrop[pendingHash] = struct{}{}
		}
	}
	for pendingHash := range pendingToDrop {
		gy.dropPendingTransactionLocked(pendingHash, affected)
	}

	for _, input := range inputs {
		ref := transactionInputRef(input)
		if order := gy.chainOrders[ref]; order != nil {
			affected[order.state.OrderId] = struct{}{}
			delete(gy.chainOrders, ref)
		}
	}
	for _, order := range outputs {
		ref := orderRef(order.state.TxHash, order.state.TxIndex)
		if old := gy.chainOrders[ref]; old != nil {
			affected[old.state.OrderId] = struct{}{}
		}
		gy.chainOrders[ref] = order
		affected[order.state.OrderId] = struct{}{}
	}
	gy.syncAffectedOrdersLocked(affected)
}

func (gy *GeniusYield) addAffectedOrderLocked(
	affected map[string]struct{},
	ref string,
) {
	if order := gy.chainOrders[ref]; order != nil {
		affected[order.state.OrderId] = struct{}{}
	}
}

func (gy *GeniusYield) syncAffectedOrdersLocked(
	affected map[string]struct{},
) {
	for orderID := range affected {
		gy.syncOrderLocked(orderID)
	}
}

func (gy *GeniusYield) syncOrderLocked(orderID string) {
	gy.sor.RemoveOrder(orderID)
	order := gy.visibleOrderLocked(orderID)
	if order == nil || order.state == nil {
		return
	}
	now := time.Now()
	if !order.state.IsActive && order.state.OfferedAmount > 0 &&
		order.state.StartTime != nil && order.state.StartTime.After(now) &&
		(order.state.EndTime == nil || order.state.EndTime.After(now)) {
		order.state.IsActive = true
	}
	if orderVisible(order.state, now) {
		gy.sor.AddOrder(order.state)
	}
}

func orderVisible(order *dexgy.OrderState, now time.Time) bool {
	return order != nil && order.IsActive && order.OfferedAmount > 0 &&
		(order.EndTime == nil || order.EndTime.After(now))
}

func orderEligible(order *dexgy.OrderState, now time.Time) bool {
	if !orderVisible(order, now) {
		return false
	}
	if order.StartTime != nil && order.StartTime.After(now) {
		return false
	}
	return true
}

func routeEligible(route *Route, now time.Time) bool {
	if route == nil || len(route.Legs) == 0 {
		return false
	}
	for _, leg := range route.Legs {
		if !orderEligible(leg.Order, now) {
			return false
		}
	}
	return true
}

func (gy *GeniusYield) visibleOrderLocked(orderID string) *trackedOrder {
	refs := make([]string, 0)
	for ref, order := range gy.chainOrders {
		if order.state.OrderId == orderID && len(gy.reservedBy[ref]) == 0 {
			refs = append(refs, ref)
		}
	}
	if len(refs) == 0 {
		return nil
	}
	sort.Strings(refs)
	return gy.chainOrders[refs[0]]
}
