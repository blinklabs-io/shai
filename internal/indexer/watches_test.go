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

package indexer

import (
	"sync"
	"testing"
	"time"

	"github.com/blinklabs-io/adder/event"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
)

func TestNewWatchManager(t *testing.T) {
	wm := NewWatchManager()
	defer wm.Stop()

	if wm == nil {
		t.Fatal("expected non-nil WatchManager")
	}
	if wm.WatchCount() != 0 {
		t.Errorf("expected 0 watches, got %d", wm.WatchCount())
	}
}

func TestRegisterTxWatch(t *testing.T) {
	wm := NewWatchManager()
	defer wm.Stop()

	txId := "abc123def456"
	var called bool
	var mu sync.Mutex

	watchId := wm.RegisterTxWatch(
		txId,
		time.Hour,
		func(id string, evt event.Event) {
			mu.Lock()
			called = true
			mu.Unlock()
		},
	)

	if watchId == "" {
		t.Fatal("expected non-empty watchId")
	}
	if wm.WatchCount() != 1 {
		t.Errorf("expected 1 watch, got %d", wm.WatchCount())
	}

	mu.Lock()
	if called {
		t.Error("callback should not have been called yet")
	}
	mu.Unlock()
}

func TestRegisterUTxOWatch(t *testing.T) {
	wm := NewWatchManager()
	defer wm.Stop()

	txId := "abc123def456"
	index := uint32(0)

	watchId := wm.RegisterUTxOWatch(
		txId,
		index,
		time.Hour,
		func(id string, evt event.Event) {},
	)

	if watchId == "" {
		t.Fatal("expected non-empty watchId")
	}
	if wm.WatchCount() != 1 {
		t.Errorf("expected 1 watch, got %d", wm.WatchCount())
	}
}

func TestUnregisterWatch(t *testing.T) {
	wm := NewWatchManager()
	defer wm.Stop()

	txId := "abc123def456"
	watchId := wm.RegisterTxWatch(
		txId,
		time.Hour,
		func(id string, evt event.Event) {},
	)

	if wm.WatchCount() != 1 {
		t.Errorf("expected 1 watch, got %d", wm.WatchCount())
	}

	wm.Unregister(watchId)

	if wm.WatchCount() != 0 {
		t.Errorf("expected 0 watches after unregister, got %d", wm.WatchCount())
	}

	// Unregistering again should not panic
	wm.Unregister(watchId)
	wm.Unregister("nonexistent")
}

func TestMultipleWatches(t *testing.T) {
	wm := NewWatchManager()
	defer wm.Stop()

	// Register multiple watches
	wm.RegisterTxWatch("tx1", time.Hour, func(id string, evt event.Event) {})
	wm.RegisterTxWatch("tx2", time.Hour, func(id string, evt event.Event) {})
	wm.RegisterUTxOWatch(
		"utxo1",
		0,
		time.Hour,
		func(id string, evt event.Event) {},
	)
	wm.RegisterUTxOWatch(
		"utxo1",
		1,
		time.Hour,
		func(id string, evt event.Event) {},
	)

	if wm.WatchCount() != 4 {
		t.Errorf("expected 4 watches, got %d", wm.WatchCount())
	}
}

func TestWatchExpiration(t *testing.T) {
	wm := NewWatchManager()
	defer wm.Stop()

	// Register a watch with a very short TTL
	wm.RegisterTxWatch(
		"tx1",
		50*time.Millisecond,
		func(id string, evt event.Event) {},
	)

	if wm.WatchCount() != 1 {
		t.Errorf("expected 1 watch, got %d", wm.WatchCount())
	}

	// Wait for expiration
	time.Sleep(100 * time.Millisecond)
	wm.expireWatches()

	if wm.WatchCount() != 0 {
		t.Errorf("expected 0 watches after expiration, got %d", wm.WatchCount())
	}
}

func TestWatchNoExpiration(t *testing.T) {
	wm := NewWatchManager()
	defer wm.Stop()

	// Register a watch with TTL of 0 (no expiration)
	wm.RegisterTxWatch("tx1", 0, func(id string, evt event.Event) {})

	if wm.WatchCount() != 1 {
		t.Errorf("expected 1 watch, got %d", wm.WatchCount())
	}

	// Call expireWatches - should not remove watch with TTL 0
	wm.expireWatches()

	if wm.WatchCount() != 1 {
		t.Errorf("expected 1 watch (no expiration), got %d", wm.WatchCount())
	}
}

func TestMultipleWatchesSamePattern(t *testing.T) {
	wm := NewWatchManager()
	defer wm.Stop()

	txId := "same-tx-id"

	// Register multiple watches for the same transaction
	watchId1 := wm.RegisterTxWatch(
		txId,
		time.Hour,
		func(id string, evt event.Event) {},
	)
	watchId2 := wm.RegisterTxWatch(
		txId,
		time.Hour,
		func(id string, evt event.Event) {},
	)

	if wm.WatchCount() != 2 {
		t.Errorf("expected 2 watches, got %d", wm.WatchCount())
	}

	// Unregister one - the other must remain (independent lifecycle)
	wm.Unregister(watchId1)
	if wm.WatchCount() != 1 {
		t.Errorf("expected 1 watch after unregister, got %d", wm.WatchCount())
	}

	// Unregister the other
	wm.Unregister(watchId2)
	if wm.WatchCount() != 0 {
		t.Errorf("expected 0 watches after unregister, got %d", wm.WatchCount())
	}
}

func TestStopIdempotent(t *testing.T) {
	wm := NewWatchManager()
	// Stopping multiple times must not panic
	wm.Stop()
	wm.Stop()
}

func TestCheckEventNilTxCallbackDoesNotPanic(t *testing.T) {
	wm := NewWatchManager()
	defer wm.Stop()

	txHash := "abc123def456"
	// A watch registered with a nil callback must not crash the process when
	// a matching transaction event arrives.
	wm.RegisterTxWatch(txHash, time.Hour, nil)

	// Transaction with no consumed inputs, so only the TX ID watch can match.
	tx := mockledger.NewTransactionBuilder()
	evt := event.Event{
		Type:    "chainsync.transaction",
		Payload: event.TransactionEvent{Transaction: tx},
		Context: event.TransactionContext{TransactionHash: txHash},
	}

	wm.CheckEvent(evt)
	// Callbacks run in their own goroutines; allow them to execute so a
	// nil-callback panic would surface before the test returns.
	time.Sleep(50 * time.Millisecond)
}

func TestCheckEventFiresMatchingCallback(t *testing.T) {
	wm := NewWatchManager()
	defer wm.Stop()

	txHash := "abc123def456"
	fired := make(chan string, 1)
	watchId := wm.RegisterTxWatch(
		txHash,
		time.Hour,
		func(id string, evt event.Event) {
			fired <- id
		},
	)

	tx := mockledger.NewTransactionBuilder()
	evt := event.Event{
		Type:    "chainsync.transaction",
		Payload: event.TransactionEvent{Transaction: tx},
		Context: event.TransactionContext{TransactionHash: txHash},
	}
	wm.CheckEvent(evt)

	select {
	case id := <-fired:
		if id != watchId {
			t.Errorf("expected callback for %q, got %q", watchId, id)
		}
	case <-time.After(time.Second):
		t.Fatal("expected matching TX watch callback to fire")
	}
}

func TestCheckEventNilTransactionDoesNotPanic(t *testing.T) {
	wm := NewWatchManager()
	defer wm.Stop()

	txHash := "abc123def456"
	fired := make(chan string, 1)
	wm.RegisterTxWatch(
		txHash,
		time.Hour,
		func(id string, evt event.Event) {
			fired <- id
		},
	)

	// A TransactionEvent with a nil Transaction must not panic CheckEvent. The
	// TX ID path needs only the context, so it must still fire.
	evt := event.Event{
		Type:    "chainsync.transaction",
		Payload: event.TransactionEvent{Transaction: nil},
		Context: event.TransactionContext{TransactionHash: txHash},
	}
	wm.CheckEvent(evt)

	select {
	case <-fired:
	case <-time.After(time.Second):
		t.Fatal("expected TX watch callback to fire even with nil Transaction")
	}
}

func TestCheckEventNilUTxOCallbackDoesNotPanic(t *testing.T) {
	wm := NewWatchManager()
	defer wm.Stop()

	input, err := mockledger.NewSimpleTransactionInput([]byte("watched-utxo"), 0)
	if err != nil {
		t.Fatalf("failed to build mock input: %v", err)
	}

	// A watch registered with a nil callback must not crash the process when
	// the watched UTxO is consumed.
	wm.RegisterUTxOWatch(input.Id().String(), input.Index(), time.Hour, nil)

	// A watch with a real callback for the same UTxO must still fire, proving
	// the nil-callback guard does not suppress legitimate callbacks.
	fired := make(chan string, 1)
	watchId := wm.RegisterUTxOWatch(
		input.Id().String(),
		input.Index(),
		time.Hour,
		func(id string, evt event.Event) {
			fired <- id
		},
	)

	tx := mockledger.NewTransactionBuilder()
	tx.WithInputs(input)
	evt := event.Event{
		Type:    "chainsync.transaction",
		Payload: event.TransactionEvent{Transaction: tx},
		Context: event.TransactionContext{TransactionHash: "unrelated"},
	}

	wm.CheckEvent(evt)

	select {
	case id := <-fired:
		if id != watchId {
			t.Errorf("expected callback for %q, got %q", watchId, id)
		}
	case <-time.After(time.Second):
		t.Fatal("expected matching UTxO watch callback to fire")
	}
}
