// Copyright 2025 Blink Labs Software
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
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/blinklabs-io/adder/event"
	input_chainsync "github.com/blinklabs-io/adder/input/chainsync"
	output_embedded "github.com/blinklabs-io/adder/output/embedded"
	"github.com/blinklabs-io/adder/pipeline"
	ocommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/blinklabs-io/shai/internal/config"
	"github.com/blinklabs-io/shai/internal/logging"
	"github.com/blinklabs-io/shai/internal/storage"
	"github.com/blinklabs-io/shai/internal/wallet"
)

const (
	syncStatusLogInterval = 30 * time.Second
)

type Indexer struct {
	pipeline *pipeline.Pipeline
	// mu guards the catch-up sync log timer and the cached cursor/tip fields,
	// which are touched by the chainsync status goroutine, the sync log timer
	// goroutine, and Stop.
	mu           sync.Mutex
	cursorSlot   uint64
	cursorHash   string
	tipSlot      uint64
	tipHash      string
	tipReached   bool
	syncLogTimer *time.Timer
	syncLogDone  bool
	eventFuncs   []EventFunc
	Watches      *WatchManager
}

type EventFunc func(event.Event) error

func New() *Indexer {
	return &Indexer{
		Watches: NewWatchManager(),
	}
}

func (i *Indexer) Start() error {
	cfg := config.GetConfig()
	logger := logging.GetLogger()
	// Create pipeline
	i.pipeline = pipeline.New()
	// Configure pipeline input
	inputOpts := []input_chainsync.ChainSyncOptionFunc{
		input_chainsync.WithAutoReconnect(true),
		input_chainsync.WithLogger(logging.GetLogger()),
		input_chainsync.WithStatusUpdateFunc(i.updateStatus),
		input_chainsync.WithNetwork(cfg.Network),
		input_chainsync.WithIncludeCbor(true),
	}
	if cfg.Indexer.Address != "" {
		inputOpts = append(
			inputOpts,
			input_chainsync.WithAddress(cfg.Indexer.Address),
		)
	}
	cursorSlotNumber, cursorBlockHash, err := storage.GetStorage().GetCursor()
	if err != nil {
		return err
	}
	if cursorSlotNumber > 0 {
		logger.Info(
			"found previous chainsync cursor",
			"slotNumber", cursorSlotNumber,
			"blockHash", cursorBlockHash,
		)
		hashBytes, err := hex.DecodeString(cursorBlockHash)
		if err != nil {
			return err
		}
		inputOpts = append(
			inputOpts,
			input_chainsync.WithIntersectPoints(
				[]ocommon.Point{
					{
						Hash: hashBytes,
						Slot: cursorSlotNumber,
					},
				},
			),
		)
	} else {
		// Determine intercept slot/hash from enabled profiles
		var interceptSlot uint64
		var interceptHash string
		for _, profile := range config.GetProfiles() {
			if interceptSlot == 0 || profile.InterceptSlot < interceptSlot {
				interceptSlot = profile.InterceptSlot
				interceptHash = profile.InterceptHash
			}
		}
		if interceptSlot == 0 {
			return errors.New("could not determine intercept point from profiles")
		}
		hashBytes, err := hex.DecodeString(interceptHash)
		if err != nil {
			return err
		}
		inputOpts = append(
			inputOpts,
			input_chainsync.WithIntersectPoints(
				[]ocommon.Point{
					{
						Hash: hashBytes,
						Slot: interceptSlot,
					},
				},
			),
		)
	}
	input := input_chainsync.New(
		inputOpts...,
	)
	i.pipeline.AddInput(input)
	// Configure pipeline output
	output := output_embedded.New(
		output_embedded.WithCallbackFunc(
			func(evt event.Event) error {
				// Call each registered event handler func
				for _, eventFunc := range i.eventFuncs {
					if err := eventFunc(evt); err != nil {
						return err
					}
				}
				return nil
			},
		),
	)
	i.pipeline.AddOutput(output)
	// Add our event handler
	i.AddEventFunc(i.handleEvent)
	// Start pipeline
	if err := i.pipeline.Start(); err != nil {
		logger.Error("failed to start pipeline:", "error", err)
		os.Exit(1)
	}
	// Start error handler
	go func() {
		err, ok := <-i.pipeline.ErrorChan()
		if ok {
			logger.Error("pipeline failed:", "error", err)
			os.Exit(1)
		}
	}()
	// Schedule periodic catch-up sync log messages
	i.scheduleSyncStatusLog()
	return nil
}

// Stop halts the indexer's background workers: the watch manager's expiration
// goroutine and the catch-up sync log timer. It is safe to call even if the
// indexer was never started and may be called more than once.
func (i *Indexer) Stop() {
	if i.Watches != nil {
		i.Watches.Stop()
	}
	i.mu.Lock()
	i.stopSyncLogTimerLocked()
	i.mu.Unlock()
}

// stopSyncLogTimerLocked stops the catch-up sync log timer and prevents it from
// being rescheduled. The caller must hold i.mu.
func (i *Indexer) stopSyncLogTimerLocked() {
	i.syncLogDone = true
	if i.syncLogTimer != nil {
		i.syncLogTimer.Stop()
	}
}

func (i *Indexer) AddEventFunc(eventFunc EventFunc) {
	i.eventFuncs = append(i.eventFuncs, eventFunc)
}

func (i *Indexer) handleEvent(evt event.Event) error {
	// Notify any registered watches about this event
	if i.Watches != nil {
		i.Watches.CheckEvent(evt)
	}
	switch evt.Payload.(type) {
	case event.TransactionEvent:
		eventTx := evt.Payload.(event.TransactionEvent)
		// A TransactionEvent may carry a nil Transaction; there is nothing to
		// reconcile in that case, so skip rather than dereferencing it.
		if eventTx.Transaction == nil {
			return nil
		}
		bursa := wallet.GetWallet()
		eventCtx := evt.Context.(event.TransactionContext)
		// Delete used UTXOs
		for _, txInput := range eventTx.Transaction.Consumed() {
			//logger.Debugf("UTxO %s.%d consumed in transaction %s", txInput.Id().String(), txInput.Index(), eventCtx.TransactionHash)
			if err := storage.GetStorage().RemoveUtxo(txInput.Id().String(), txInput.Index()); err != nil {
				return err
			}
		}
		// Store UTXOs for bot wallet
		for _, utxo := range eventTx.Transaction.Produced() {
			txOutputAddress := utxo.Output.Address().String()
			if txOutputAddress == bursa.PaymentAddress {
				// Write UTXO to storage
				if err := storage.GetStorage().AddUtxo(
					txOutputAddress,
					eventCtx.TransactionHash,
					utxo.Id.Index(),
					utxo.Output.Cbor(),
				); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (i *Indexer) scheduleSyncStatusLog() {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.scheduleSyncStatusLogLocked()
}

// scheduleSyncStatusLogLocked arms the catch-up sync log timer unless logging
// has been stopped. The caller must hold i.mu.
func (i *Indexer) scheduleSyncStatusLogLocked() {
	if i.syncLogDone {
		return
	}
	if i.syncLogTimer != nil {
		i.syncLogTimer.Stop()
	}
	i.syncLogTimer = time.AfterFunc(syncStatusLogInterval, i.syncStatusLog)
}

func (i *Indexer) syncStatusLog() {
	// Snapshot the cursor fields under the lock, then log and reschedule
	// without holding it. Bail out if logging has been stopped (by Stop or by
	// reaching the chain tip): a timer callback that was already in flight when
	// that happened must not emit a stale catch-up message.
	i.mu.Lock()
	if i.syncLogDone {
		i.mu.Unlock()
		return
	}
	cursorSlot := i.cursorSlot
	cursorHash := i.cursorHash
	tipSlot := i.tipSlot
	i.mu.Unlock()

	logging.GetLogger().Info(fmt.Sprintf(
		"catch-up sync in progress: at %d.%s (current tip slot is %d)",
		cursorSlot,
		cursorHash,
		tipSlot),
	)
	i.scheduleSyncStatusLog()
}

func (i *Indexer) updateStatus(status input_chainsync.ChainSyncStatus) {
	i.applyStatus(status)
	if err := storage.GetStorage().UpdateCursor(status.SlotNumber, status.BlockHash); err != nil {
		logging.GetLogger().Error("failed to update cursor:", "error", err)
	}
}

// applyStatus updates the cached cursor/tip fields from a chainsync status
// update and stops the catch-up sync log timer once the chain tip is reached.
func (i *Indexer) applyStatus(status input_chainsync.ChainSyncStatus) {
	i.mu.Lock()
	defer i.mu.Unlock()
	// Check if we've hit chain tip
	if !i.tipReached && status.TipReached {
		i.stopSyncLogTimerLocked()
		i.tipReached = true
	}
	i.cursorSlot = status.SlotNumber
	i.cursorHash = status.BlockHash
	i.tipSlot = status.TipSlotNumber
	i.tipHash = status.TipBlockHash
}
