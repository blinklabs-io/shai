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
	"fmt"
	"sync"
	"time"

	"github.com/blinklabs-io/adder/event"
	"github.com/blinklabs-io/shai/internal/logging"
)

// WatchType represents the type of watch
type WatchType int

const (
	// WatchTypeTxId watches for a specific transaction ID
	WatchTypeTxId WatchType = iota
	// WatchTypeUTxO watches for a specific UTxO being spent
	WatchTypeUTxO
)

// WatchCallback is called when a watch is triggered
type WatchCallback func(watchId string, evt event.Event)

// Watch represents a registered watch
type Watch struct {
	Id        string
	Type      WatchType
	Pattern   string // txHash for TxId watch, "txHash.index" for UTxO watch
	Callback  WatchCallback
	TTL       time.Duration
	CreatedAt time.Time
}

// WatchManager manages watches for transaction IDs and UTxOs
type WatchManager struct {
	sync.RWMutex
	watches   map[string]*Watch
	txIdIndex map[string][]string // txHash -> watchIds
	utxoIndex map[string][]string // "txHash.index" -> watchIds
	nextId    uint64
	stopChan  chan struct{}
	stopped   bool
}

// NewWatchManager creates a new WatchManager
func NewWatchManager() *WatchManager {
	wm := &WatchManager{
		watches:   make(map[string]*Watch),
		txIdIndex: make(map[string][]string),
		utxoIndex: make(map[string][]string),
		stopChan:  make(chan struct{}),
	}
	go wm.expirationLoop()
	return wm
}

// RegisterTxWatch registers a watch for a specific transaction ID
func (wm *WatchManager) RegisterTxWatch(
	txId string,
	ttl time.Duration,
	cb WatchCallback,
) string {
	wm.Lock()
	defer wm.Unlock()

	wm.nextId++
	watchId := fmt.Sprintf("tx-%d", wm.nextId)

	watch := &Watch{
		Id:        watchId,
		Type:      WatchTypeTxId,
		Pattern:   txId,
		Callback:  cb,
		TTL:       ttl,
		CreatedAt: time.Now(),
	}

	wm.watches[watchId] = watch
	wm.txIdIndex[txId] = append(wm.txIdIndex[txId], watchId)

	logger := logging.GetLogger()
	logger.Debug(
		"registered TX watch",
		"watchId",
		watchId,
		"txId",
		txId,
		"ttl",
		ttl,
	)

	return watchId
}

// RegisterUTxOWatch registers a watch for a specific UTxO being spent
func (wm *WatchManager) RegisterUTxOWatch(
	txId string,
	index uint32,
	ttl time.Duration,
	cb WatchCallback,
) string {
	wm.Lock()
	defer wm.Unlock()

	wm.nextId++
	watchId := fmt.Sprintf("utxo-%d", wm.nextId)
	pattern := fmt.Sprintf("%s.%d", txId, index)

	watch := &Watch{
		Id:        watchId,
		Type:      WatchTypeUTxO,
		Pattern:   pattern,
		Callback:  cb,
		TTL:       ttl,
		CreatedAt: time.Now(),
	}

	wm.watches[watchId] = watch
	wm.utxoIndex[pattern] = append(wm.utxoIndex[pattern], watchId)

	logger := logging.GetLogger()
	logger.Debug(
		"registered UTxO watch",
		"watchId", watchId,
		"txId", txId,
		"index", index,
		"ttl", ttl,
	)

	return watchId
}

// Unregister removes a watch by its ID
func (wm *WatchManager) Unregister(watchId string) {
	wm.Lock()
	defer wm.Unlock()

	wm.unregisterLocked(watchId)
}

// unregisterLocked removes a watch (caller must hold lock)
func (wm *WatchManager) unregisterLocked(watchId string) {
	watch, ok := wm.watches[watchId]
	if !ok {
		return
	}

	// Remove from index
	switch watch.Type {
	case WatchTypeTxId:
		wm.removeFromIndex(wm.txIdIndex, watch.Pattern, watchId)
	case WatchTypeUTxO:
		wm.removeFromIndex(wm.utxoIndex, watch.Pattern, watchId)
	}

	delete(wm.watches, watchId)

	logger := logging.GetLogger()
	logger.Debug("unregistered watch", "watchId", watchId)
}

// removeFromIndex removes a watchId from an index slice
func (wm *WatchManager) removeFromIndex(
	index map[string][]string,
	key string,
	watchId string,
) {
	watchIds := index[key]
	for i, id := range watchIds {
		if id == watchId {
			index[key] = append(watchIds[:i], watchIds[i+1:]...)
			break
		}
	}
	if len(index[key]) == 0 {
		delete(index, key)
	}
}

// CheckEvent checks an event against all registered watches
func (wm *WatchManager) CheckEvent(evt event.Event) {
	wm.RLock()
	defer wm.RUnlock()

	switch payload := evt.Payload.(type) {
	case event.TransactionEvent:
		ctx := evt.Context.(event.TransactionContext)
		txHash := ctx.TransactionHash

		// Check for TX ID watches
		if watchIds, ok := wm.txIdIndex[txHash]; ok {
			for _, watchId := range watchIds {
				if watch, ok := wm.watches[watchId]; ok {
					go watch.Callback(watchId, evt)
				}
			}
		}

		// Check for UTxO watches (inputs being spent)
		for _, txInput := range payload.Transaction.Consumed() {
			pattern := fmt.Sprintf("%s.%d", txInput.Id().String(), txInput.Index())
			if watchIds, ok := wm.utxoIndex[pattern]; ok {
				for _, watchId := range watchIds {
					if watch, ok := wm.watches[watchId]; ok {
						go watch.Callback(watchId, evt)
					}
				}
			}
		}
	}
}

// expireWatches removes expired watches
func (wm *WatchManager) expireWatches() {
	wm.Lock()
	defer wm.Unlock()

	now := time.Now()
	var expiredIds []string

	for watchId, watch := range wm.watches {
		if watch.TTL > 0 && now.Sub(watch.CreatedAt) > watch.TTL {
			expiredIds = append(expiredIds, watchId)
		}
	}

	for _, watchId := range expiredIds {
		wm.unregisterLocked(watchId)
		logger := logging.GetLogger()
		logger.Debug("watch expired", "watchId", watchId)
	}
}

// expirationLoop periodically checks for expired watches
func (wm *WatchManager) expirationLoop() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			wm.expireWatches()
		case <-wm.stopChan:
			return
		}
	}
}

// Stop stops the watch manager (idempotent - safe to call multiple times)
func (wm *WatchManager) Stop() {
	wm.Lock()
	defer wm.Unlock()
	if wm.stopped {
		return
	}
	wm.stopped = true
	close(wm.stopChan)
}

// WatchCount returns the number of active watches
func (wm *WatchManager) WatchCount() int {
	wm.RLock()
	defer wm.RUnlock()
	return len(wm.watches)
}
