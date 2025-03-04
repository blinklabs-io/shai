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
	"time"

	"github.com/blinklabs-io/shai/internal/config"
	"github.com/blinklabs-io/shai/internal/logging"
	"github.com/blinklabs-io/shai/internal/storage"
	"github.com/blinklabs-io/shai/internal/wallet"

	"github.com/blinklabs-io/adder/event"
	input_chainsync "github.com/blinklabs-io/adder/input/chainsync"
	output_embedded "github.com/blinklabs-io/adder/output/embedded"
	"github.com/blinklabs-io/adder/pipeline"
	ocommon "github.com/blinklabs-io/gouroboros/protocol/common"
)

const (
	syncStatusLogInterval = 30 * time.Second
)

type Indexer struct {
	pipeline     *pipeline.Pipeline
	cursorSlot   uint64
	cursorHash   string
	tipSlot      uint64
	tipHash      string
	tipReached   bool
	syncLogTimer *time.Timer
	//lastBlockData any
	eventFuncs []EventFunc
}

type EventFunc func(event.Event) error

func New() *Indexer {
	return &Indexer{}
}

func (i *Indexer) Start() error {
	cfg := config.GetConfig()
	logger := logging.GetLogger()
	// Create pipeline
	i.pipeline = pipeline.New()
	// Configure pipeline input
	inputOpts := []input_chainsync.ChainSyncOptionFunc{
		input_chainsync.WithBulkMode(true),
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
				// TODO: run these in parallel
				// Call each registered event handler func
				for _, eventFunc := range i.eventFuncs {
					if err := eventFunc(evt); err != nil {
						fmt.Printf("err = %s\n", err)
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

func (i *Indexer) AddEventFunc(eventFunc EventFunc) {
	i.eventFuncs = append(i.eventFuncs, eventFunc)
}

func (i *Indexer) handleEvent(evt event.Event) error {
	//logger := logging.GetLogger()
	switch evt.Payload.(type) {
	case input_chainsync.TransactionEvent:
		bursa := wallet.GetWallet()
		eventTx := evt.Payload.(input_chainsync.TransactionEvent)
		eventCtx := evt.Context.(input_chainsync.TransactionContext)
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
	i.syncLogTimer = time.AfterFunc(syncStatusLogInterval, i.syncStatusLog)
}

func (i *Indexer) syncStatusLog() {
	logger := logging.GetLogger()
	logger.Info(fmt.Sprintf(
		"catch-up sync in progress: at %d.%s (current tip slot is %d)",
		i.cursorSlot,
		i.cursorHash,
		i.tipSlot),
	)
	i.scheduleSyncStatusLog()
}

func (i *Indexer) updateStatus(status input_chainsync.ChainSyncStatus) {
	logger := logging.GetLogger()
	// Check if we've hit chain tip
	if !i.tipReached && status.TipReached {
		if i.syncLogTimer != nil {
			i.syncLogTimer.Stop()
		}
		i.tipReached = true
	}
	i.cursorSlot = status.SlotNumber
	i.cursorHash = status.BlockHash
	i.tipSlot = status.TipSlotNumber
	i.tipHash = status.TipBlockHash
	if err := storage.GetStorage().UpdateCursor(status.SlotNumber, status.BlockHash); err != nil {
		logger.Error("failed to update cursor:", "error", err)
	}
}
