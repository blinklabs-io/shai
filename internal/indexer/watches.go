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
	"runtime/debug"
	"sync"
	"time"

	"github.com/blinklabs-io/adder/event"
	"github.com/blinklabs-io/shai/internal/logging"
)

const (
	// watchExpirationInterval is how often the background loop scans for
	// expired watches.
	watchExpirationInterval = 10 * time.Second
)

// WatchType represents the type of watch
type WatchType int

const (
	// WatchTypeTxId watches for a specific transaction ID appearing on-chain
	WatchTypeTxId WatchType = iota
	// WatchTypeUTxO watches for a specific UTxO being spent (consumed) on-chain
	WatchTypeUTxO
)

// WatchCallback is called when a watch is triggered. It receives the id of
// the watch that fired and the event that triggered it.
type WatchCallback func(watchId string, evt event.Event)

// Watch represents a registered watch
type Watch struct {
	Id   string
	Type WatchType
	// Pattern is the lookup key: the txHash for a WatchTypeTxId watch, or
	// "txHash#index" for a WatchTypeUTxO watch.
	Pattern   string
	Callback  WatchCallback
	TTL       time.Duration
	CreatedAt time.Time
}

// WatchManager manages watches for transaction IDs and UTxOs. It provides
// O(1) indexed lookups for incoming events and TTL-based expiration via a
// background loop.
type WatchManager struct {
	sync.RWMutex
	watches   map[string]*Watch
	txIdIndex map[string][]string // txHash -> watchIds
	utxoIndex map[string][]string // "txHash#index" -> watchIds
	nextId    uint64
	stopChan  chan struct{}
	stopped   bool
}

// utxoPattern builds the lookup key used for UTxO watches.
func utxoPattern(txId string, index uint32) string {
	return fmt.Sprintf("%s#%d", txId, index)
}

// NewWatchManager creates a new WatchManager and starts its background
// expiration loop.
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

// RegisterTxWatch registers a watch for a specific transaction ID. A TTL of
// 0 means the watch never expires. It returns the generated watch id.
func (wm *WatchManager) RegisterTxWatch(
	txId string,
	ttl time.Duration,
	callback WatchCallback,
) string {
	wm.Lock()
	defer wm.Unlock()

	if wm.stopped {
		return ""
	}

	wm.nextId++
	watchId := fmt.Sprintf("tx-%d", wm.nextId)

	watch := &Watch{
		Id:        watchId,
		Type:      WatchTypeTxId,
		Pattern:   txId,
		Callback:  callback,
		TTL:       ttl,
		CreatedAt: time.Now(),
	}

	wm.watches[watchId] = watch
	wm.txIdIndex[txId] = append(wm.txIdIndex[txId], watchId)

	logging.GetLogger().Debug(
		"registered TX watch",
		"watchId", watchId,
		"txId", txId,
		"ttl", ttl,
	)

	return watchId
}

// RegisterUTxOWatch registers a watch for a specific UTxO (txId#index) being
// consumed. A TTL of 0 means the watch never expires. It returns the
// generated watch id.
func (wm *WatchManager) RegisterUTxOWatch(
	txId string,
	index uint32,
	ttl time.Duration,
	callback WatchCallback,
) string {
	wm.Lock()
	defer wm.Unlock()

	if wm.stopped {
		return ""
	}

	wm.nextId++
	watchId := fmt.Sprintf("utxo-%d", wm.nextId)
	pattern := utxoPattern(txId, index)

	watch := &Watch{
		Id:        watchId,
		Type:      WatchTypeUTxO,
		Pattern:   pattern,
		Callback:  callback,
		TTL:       ttl,
		CreatedAt: time.Now(),
	}

	wm.watches[watchId] = watch
	wm.utxoIndex[pattern] = append(wm.utxoIndex[pattern], watchId)

	logging.GetLogger().Debug(
		"registered UTxO watch",
		"watchId", watchId,
		"txId", txId,
		"index", index,
		"ttl", ttl,
	)

	return watchId
}

// Unregister removes a watch by its id. It is safe to call on an unknown id
// or to call more than once for the same id.
func (wm *WatchManager) Unregister(watchId string) {
	wm.Lock()
	defer wm.Unlock()
	wm.unregisterLocked(watchId)
}

// unregisterLocked removes a watch. The caller must hold the write lock.
func (wm *WatchManager) unregisterLocked(watchId string) {
	watch, ok := wm.watches[watchId]
	if !ok {
		return
	}

	switch watch.Type {
	case WatchTypeTxId:
		wm.removeFromIndex(wm.txIdIndex, watch.Pattern, watchId)
	case WatchTypeUTxO:
		wm.removeFromIndex(wm.utxoIndex, watch.Pattern, watchId)
	}

	delete(wm.watches, watchId)

	logging.GetLogger().Debug("unregistered watch", "watchId", watchId)
}

// removeFromIndex removes a single watchId from the slice stored under key,
// deleting the key entirely when no watches remain.
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

// CheckEvent matches an incoming event against all registered watches and
// fires the callbacks of any that match. Callbacks are invoked in their own
// goroutines so a slow callback cannot block the event loop or hold the lock.
func (wm *WatchManager) CheckEvent(evt event.Event) {
	wm.RLock()
	defer wm.RUnlock()

	// Once stopped, deliver no further callbacks. The watch tables are also
	// cleared on Stop, but gating here prevents an in-flight event from firing
	// a callback after the caller has torn the manager down.
	if wm.stopped {
		return
	}

	payload, ok := evt.Payload.(event.TransactionEvent)
	if !ok {
		return
	}

	// TX ID watches keyed on the transaction hash from the context.
	if ctx, ok := evt.Context.(event.TransactionContext); ok {
		wm.fireWatches(wm.txIdIndex[ctx.TransactionHash], evt)
	}

	// UTxO watches keyed on each consumed input (txHash#index). A nil
	// Transaction has no inputs to match, so skip the lookup rather than
	// dereferencing it.
	if payload.Transaction != nil {
		for _, txInput := range payload.Transaction.Consumed() {
			pattern := utxoPattern(txInput.Id().String(), txInput.Index())
			wm.fireWatches(wm.utxoIndex[pattern], evt)
		}
	}
}

// fireWatches invokes the callback for each of the given watch ids, each in
// its own goroutine so a slow callback cannot block the event loop or hold the
// lock. Watches with a nil callback are skipped so a registration that omitted
// a callback cannot panic the process. A panic from within a callback is
// recovered and logged so a misbehaving watch cannot crash the process. The
// caller must hold the lock.
func (wm *WatchManager) fireWatches(watchIds []string, evt event.Event) {
	for _, watchId := range watchIds {
		watch, ok := wm.watches[watchId]
		if !ok || watch.Callback == nil {
			continue
		}
		go func(id string, callback WatchCallback) {
			defer func() {
				if r := recover(); r != nil {
					logging.GetLogger().Error(
						"watch callback panicked",
						"watchId", id,
						"panic", r,
						"stack", string(debug.Stack()),
					)
				}
			}()
			callback(id, evt)
		}(watchId, watch.Callback)
	}
}

// expireWatches removes any watch whose TTL has elapsed. A TTL of 0 means the
// watch never expires.
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
		logging.GetLogger().Debug("watch expired", "watchId", watchId)
	}
}

// expirationLoop periodically scans for expired watches until Stop is called.
func (wm *WatchManager) expirationLoop() {
	ticker := time.NewTicker(watchExpirationInterval)
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

// Stop halts the background expiration loop and clears all registered watches.
// After Stop, CheckEvent delivers no further callbacks and Register* calls are
// rejected. It is idempotent and safe to call multiple times.
func (wm *WatchManager) Stop() {
	wm.Lock()
	defer wm.Unlock()
	if wm.stopped {
		return
	}
	wm.stopped = true
	close(wm.stopChan)
	wm.watches = make(map[string]*Watch)
	wm.txIdIndex = make(map[string][]string)
	wm.utxoIndex = make(map[string][]string)
}

// WatchCount returns the number of active watches.
func (wm *WatchManager) WatchCount() int {
	wm.RLock()
	defer wm.RUnlock()
	return len(wm.watches)
}
