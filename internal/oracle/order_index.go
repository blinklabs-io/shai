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
	"cmp"
	"context"
	"strings"
)

type orderIndexKey struct {
	protocol string
	network  string
	orderID  string
}

type orderPageKey struct {
	orderIndexKey
	oracleIndex int
}

type orderListEntry struct {
	key   orderPageKey
	order *OrderState
}

type orderIndexNode struct {
	key         orderIndexKey
	left, right *orderIndexNode
	height      int
}

type orderIndexTree struct {
	root *orderIndexNode
}

type orderIndexIterator struct {
	stack []*orderIndexNode
}

func orderKey(state *OrderState) orderIndexKey {
	return orderIndexKey{
		protocol: state.Protocol,
		network:  state.Network,
		orderID:  state.OrderId,
	}
}

func compareOrderIndexKeys(left, right orderIndexKey) int {
	if result := strings.Compare(left.protocol, right.protocol); result != 0 {
		return result
	}
	if result := strings.Compare(left.network, right.network); result != 0 {
		return result
	}
	return strings.Compare(left.orderID, right.orderID)
}

func compareOrderPageKeys(left, right orderPageKey) int {
	if result := compareOrderIndexKeys(left.orderIndexKey, right.orderIndexKey); result != 0 {
		return result
	}
	return cmp.Compare(left.oracleIndex, right.oracleIndex)
}

func orderIndexHeight(node *orderIndexNode) int {
	if node == nil {
		return 0
	}
	return node.height
}

func updateOrderIndexHeight(node *orderIndexNode) {
	node.height = 1 + max(
		orderIndexHeight(node.left),
		orderIndexHeight(node.right),
	)
}

func rotateOrderIndexLeft(node *orderIndexNode) *orderIndexNode {
	right := node.right
	node.right = right.left
	right.left = node
	updateOrderIndexHeight(node)
	updateOrderIndexHeight(right)
	return right
}

func rotateOrderIndexRight(node *orderIndexNode) *orderIndexNode {
	left := node.left
	node.left = left.right
	left.right = node
	updateOrderIndexHeight(node)
	updateOrderIndexHeight(left)
	return left
}

func rebalanceOrderIndex(node *orderIndexNode) *orderIndexNode {
	updateOrderIndexHeight(node)
	balance := orderIndexHeight(node.left) - orderIndexHeight(node.right)
	switch {
	case balance > 1:
		left := node.left
		if orderIndexHeight(left.left) < orderIndexHeight(left.right) {
			node.left = rotateOrderIndexLeft(left)
		}
		return rotateOrderIndexRight(node)
	case balance < -1:
		right := node.right
		if orderIndexHeight(right.right) < orderIndexHeight(right.left) {
			node.right = rotateOrderIndexRight(right)
		}
		return rotateOrderIndexLeft(node)
	default:
		return node
	}
}

func insertOrderIndexNode(
	node *orderIndexNode,
	key orderIndexKey,
) *orderIndexNode {
	if node == nil {
		return &orderIndexNode{key: key, height: 1}
	}
	switch compareOrderIndexKeys(key, node.key) {
	case -1:
		node.left = insertOrderIndexNode(node.left, key)
	case 1:
		node.right = insertOrderIndexNode(node.right, key)
	default:
		return node
	}
	return rebalanceOrderIndex(node)
}

func deleteOrderIndexNode(
	node *orderIndexNode,
	key orderIndexKey,
) *orderIndexNode {
	if node == nil {
		return nil
	}
	switch compareOrderIndexKeys(key, node.key) {
	case -1:
		node.left = deleteOrderIndexNode(node.left, key)
	case 1:
		node.right = deleteOrderIndexNode(node.right, key)
	default:
		if node.left == nil {
			return node.right
		}
		if node.right == nil {
			return node.left
		}
		successor := node.right
		for successor.left != nil {
			successor = successor.left
		}
		node.key = successor.key
		node.right = deleteOrderIndexNode(node.right, successor.key)
	}
	return rebalanceOrderIndex(node)
}

func (tree *orderIndexTree) insert(key orderIndexKey) {
	tree.root = insertOrderIndexNode(tree.root, key)
}

func (tree *orderIndexTree) delete(key orderIndexKey) {
	tree.root = deleteOrderIndexNode(tree.root, key)
}

func (tree *orderIndexTree) iterator(
	protocol string,
	after *orderPageKey,
	oracleIndex int,
) orderIndexIterator {
	iterator := orderIndexIterator{}
	for node := tree.root; node != nil; {
		include := true
		switch {
		case after != nil:
			include = compareOrderPageKeys(orderPageKey{
				orderIndexKey: node.key,
				oracleIndex:   oracleIndex,
			}, *after) > 0
		case protocol != "":
			include = node.key.protocol >= protocol
		}
		if include {
			iterator.stack = append(iterator.stack, node)
			node = node.left
		} else {
			node = node.right
		}
	}
	return iterator
}

func (iterator *orderIndexIterator) next() (orderIndexKey, bool) {
	if len(iterator.stack) == 0 {
		return orderIndexKey{}, false
	}
	last := len(iterator.stack) - 1
	node := iterator.stack[last]
	iterator.stack = iterator.stack[:last]
	for child := node.right; child != nil; child = child.left {
		iterator.stack = append(iterator.stack, child)
	}
	return node.key, true
}

func (o *Oracle) rebuildOrderIndex() {
	o.ordersMu.Lock()
	defer o.ordersMu.Unlock()
	o.rebuildOrderIndexLocked()
}

func (o *Oracle) rebuildOrderIndexLocked() {
	o.orderIndex = orderIndexTree{}
	for _, state := range o.orders {
		if state != nil {
			o.orderIndex.insert(orderKey(state))
		}
	}
}

func (o *Oracle) putOrderLocked(state *OrderState) {
	if state == nil {
		return
	}
	key := orderKey(state)
	if previous, ok := o.orders[state.OrderId]; ok && previous != nil {
		previousKey := orderKey(previous)
		if compareOrderIndexKeys(previousKey, key) == 0 {
			o.orders[state.OrderId] = state
			o.orderIndex.insert(key)
			return
		}
		o.orderIndex.delete(previousKey)
	}
	o.orders[state.OrderId] = state
	o.orderIndex.insert(key)
}

func (o *Oracle) deleteOrderLocked(orderID string) {
	state, ok := o.orders[orderID]
	if !ok {
		return
	}
	if state != nil {
		o.orderIndex.delete(orderKey(state))
	}
	delete(o.orders, orderID)
}

func (o *Oracle) orderPage(
	ctx context.Context,
	protocol string,
	after *orderPageKey,
	oracleIndex int,
	limit int,
) ([]orderListEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	o.ordersMu.RLock()
	defer o.ordersMu.RUnlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	iterator := o.orderIndex.iterator(protocol, after, oracleIndex)
	entries := make([]orderListEntry, 0, limit)
	for len(entries) < limit {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		key, ok := iterator.next()
		if !ok {
			break
		}
		if protocol != "" && key.protocol != protocol {
			break
		}
		state := o.orders[key.orderID]
		if state == nil {
			continue
		}
		entries = append(entries, orderListEntry{
			key: orderPageKey{
				orderIndexKey: key,
				oracleIndex:   oracleIndex,
			},
			order: state,
		})
	}
	return entries, nil
}
