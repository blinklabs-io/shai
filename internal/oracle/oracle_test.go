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
	"fmt"
	"testing"
	"time"

	"github.com/blinklabs-io/shai/internal/common"
)

func TestOracleSubscribeNotifyUnsubscribe(t *testing.T) {
	o := &Oracle{
		pools:    make(map[string]*PoolState),
		stopChan: make(chan struct{}),
	}

	ch := o.Subscribe()
	if len(o.subscribers) != 1 {
		t.Fatalf("expected 1 subscriber, got %d", len(o.subscribers))
	}

	state := &PoolState{
		PoolId:    "pool-1",
		Protocol:  "test",
		AssetX:    common.AssetAmount{Amount: 100},
		AssetY:    common.AssetAmount{Amount: 250},
		Timestamp: time.Now(),
	}
	o.notifySubscribers(state, 2.0)

	select {
	case update := <-ch:
		if update.PoolId != "pool-1" {
			t.Fatalf("expected pool-1 update, got %s", update.PoolId)
		}
		if update.PrevPriceXY != 2.0 {
			t.Fatalf("expected prev price 2.0, got %f", update.PrevPriceXY)
		}
	default:
		t.Fatal("expected update on subscriber channel")
	}

	o.Unsubscribe(ch)
	if len(o.subscribers) != 0 {
		t.Fatalf("expected 0 subscribers, got %d", len(o.subscribers))
	}

	_, ok := <-ch
	if ok {
		t.Fatal("expected unsubscribed channel to be closed")
	}
}

func TestOracleStopIdempotentAndClosesSubscribers(t *testing.T) {
	o := &Oracle{
		pools:    make(map[string]*PoolState),
		stopChan: make(chan struct{}),
	}
	ch1 := o.Subscribe()
	ch2 := o.Subscribe()

	o.Stop()
	o.Stop()

	_, ok := <-ch1
	if ok {
		t.Fatal("expected subscriber channel 1 to be closed")
	}
	_, ok = <-ch2
	if ok {
		t.Fatal("expected subscriber channel 2 to be closed")
	}

	// Subscribe after stop should return an already-closed channel.
	ch3 := o.Subscribe()
	_, ok = <-ch3
	if ok {
		t.Fatal("expected subscriber channel 3 to be closed")
	}
}

func TestOracleFullBufferDropsOldest(t *testing.T) {
	o := &Oracle{
		pools:    make(map[string]*PoolState),
		stopChan: make(chan struct{}),
	}
	ch := o.Subscribe()

	const totalUpdates = subscriberBufferSize + 50
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 1; i <= totalUpdates; i++ {
			state := &PoolState{
				PoolId:    fmt.Sprintf("pool-%03d", i),
				Protocol:  "test",
				AssetX:    common.AssetAmount{Amount: 100},
				AssetY:    common.AssetAmount{Amount: 200},
				Timestamp: time.Now(),
			}
			o.notifySubscribers(state, float64(i))
		}
	}()

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("notifySubscribers blocked while subscriber channel was full")
	}

	if drops := o.DroppedNotifications(); drops == 0 {
		t.Fatal("expected dropped notifications when subscriber channel overflows")
	}

	if got := len(ch); got != subscriberBufferSize {
		t.Fatalf(
			"expected full subscriber buffer (%d), got %d",
			subscriberBufferSize,
			got,
		)
	}

	firstExpected := totalUpdates - subscriberBufferSize + 1
	for i := 0; i < subscriberBufferSize; i++ {
		update := <-ch
		wantIndex := firstExpected + i
		wantPoolID := fmt.Sprintf("pool-%03d", wantIndex)
		if update.PoolId != wantPoolID {
			t.Fatalf("expected pool id %s, got %s", wantPoolID, update.PoolId)
		}
		if update.PrevPriceXY != float64(wantIndex) {
			t.Fatalf(
				"expected prev price %.0f, got %.0f",
				float64(wantIndex),
				update.PrevPriceXY,
			)
		}
	}
}

func TestOracleNotifySubscribersSendsDistinctUpdateInstances(t *testing.T) {
	o := &Oracle{
		pools:    make(map[string]*PoolState),
		stopChan: make(chan struct{}),
	}
	ch1 := o.Subscribe()
	ch2 := o.Subscribe()

	state := &PoolState{
		PoolId:    "pool-1",
		Protocol:  "test",
		AssetX:    common.AssetAmount{Amount: 100},
		AssetY:    common.AssetAmount{Amount: 250},
		Timestamp: time.Now(),
	}

	o.notifySubscribers(state, 2.0)

	update1 := <-ch1
	update2 := <-ch2
	if update1 == update2 {
		t.Fatal("expected each subscriber to receive a distinct *PriceUpdate")
	}
}

func TestOracleNotifySubscribersDropPathUsesDistinctInstances(t *testing.T) {
	o := &Oracle{
		pools:    make(map[string]*PoolState),
		stopChan: make(chan struct{}),
	}
	chFull := o.Subscribe()
	chNormal := o.Subscribe()

	// Fill first subscriber buffer to force drop/retry path.
	fullChan := o.subscribers[0]
	for i := 0; i < subscriberBufferSize; i++ {
		fullChan <- &PriceUpdate{PoolId: fmt.Sprintf("old-%d", i)}
	}

	state := &PoolState{
		PoolId:    "pool-drop-path",
		Protocol:  "test",
		AssetX:    common.AssetAmount{Amount: 100},
		AssetY:    common.AssetAmount{Amount: 300},
		Timestamp: time.Now(),
	}
	o.notifySubscribers(state, 2.5)

	updateNormal := <-chNormal

	var updateFull *PriceUpdate
	for i := 0; i < subscriberBufferSize; i++ {
		updateFull = <-chFull
	}

	if updateFull == nil {
		t.Fatal("expected update on full subscriber channel")
	}
	if updateFull.PoolId != "pool-drop-path" {
		t.Fatalf("expected pool-drop-path, got %s", updateFull.PoolId)
	}
	if updateNormal == nil {
		t.Fatal("expected update on normal subscriber channel")
	}
	if updateNormal.PoolId != "pool-drop-path" {
		t.Fatalf("expected pool-drop-path, got %s", updateNormal.PoolId)
	}
	if updateFull == updateNormal {
		t.Fatal("expected distinct *PriceUpdate instances across subscribers")
	}

	// Mutating one must not affect the other.
	updateFull.PoolId = "mutated"
	if updateNormal.PoolId != "pool-drop-path" {
		t.Fatal("expected subscriber updates to be independent copies")
	}
}
