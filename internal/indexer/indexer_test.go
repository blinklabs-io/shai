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

	input_chainsync "github.com/blinklabs-io/adder/input/chainsync"
)

func TestIndexerStopHaltsWatchManager(t *testing.T) {
	idx := New()
	if idx.Watches == nil {
		t.Fatal("expected non-nil WatchManager")
	}

	// Stop must halt the WatchManager's background expiration goroutine.
	idx.Stop()
	if !idx.Watches.stopped {
		t.Error("expected WatchManager to be stopped after Indexer.Stop()")
	}

	// Stop must be idempotent and safe to call more than once.
	idx.Stop()
}

func TestIndexerSyncStatusLogConcurrent(t *testing.T) {
	idx := New()
	defer idx.Stop()

	// Status updates arrive on the chainsync goroutine while the catch-up sync
	// log timer reschedules itself from its own goroutine. Both touch the
	// cursor/tip fields and the timer; with -race this must stay clean.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			idx.applyStatus(input_chainsync.ChainSyncStatus{
				SlotNumber:    uint64(i),
				BlockHash:     "hash",
				TipSlotNumber: uint64(i),
			})
		}
	}()
	for g := 0; g < 3; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 500; j++ {
				idx.syncStatusLog()
			}
		}()
	}
	wg.Wait()
}

func TestIndexerStopHaltsSyncLogTimer(t *testing.T) {
	idx := New()

	// Simulate a running catch-up sync timer (normally created by Start).
	fired := make(chan struct{}, 1)
	idx.syncLogTimer = time.AfterFunc(50*time.Millisecond, func() {
		fired <- struct{}{}
	})

	// Stop must halt the sync log timer so it never fires after shutdown.
	idx.Stop()

	select {
	case <-fired:
		t.Error("syncLogTimer fired after Indexer.Stop()")
	case <-time.After(150 * time.Millisecond):
		// Timer was stopped before it could fire.
	}
}
