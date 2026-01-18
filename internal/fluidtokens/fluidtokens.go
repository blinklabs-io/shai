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

package fluidtokens

import (
	"fmt"
	"sync"
	"time"

	"github.com/blinklabs-io/adder/event"
	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/shai/internal/config"
	"github.com/blinklabs-io/shai/internal/indexer"
	"github.com/blinklabs-io/shai/internal/logging"
	"github.com/blinklabs-io/shai/internal/storage"
	"github.com/blinklabs-io/shai/internal/txsubmit"
	"github.com/blinklabs-io/shai/internal/wallet"
)

// FluidTokens handles the FluidTokens liquidation protocol
type FluidTokens struct {
	idx               *indexer.Indexer
	profile           *config.Profile
	contractAddresses []string
	rentals           map[string]*TrackedRental
	rentalsMu         sync.RWMutex
	checkTicker       *time.Ticker
	stopChan          chan struct{}
	stopped           bool
}

// TrackedRental represents a rental position being tracked
type TrackedRental struct {
	TxHash   string
	TxIndex  uint32
	Datum    *RentDatum
	Output   ledger.TransactionOutput
	LastSeen time.Time
}

// New creates a new FluidTokens handler
func New(idx *indexer.Indexer, profile *config.Profile) *FluidTokens {
	ft := &FluidTokens{
		idx:      idx,
		profile:  profile,
		rentals:  make(map[string]*TrackedRental),
		stopChan: make(chan struct{}),
	}

	// Build list of contract addresses from profile config
	if ftConfig, ok := profile.Config.(config.FluidTokensProfileConfig); ok {
		for _, addr := range ftConfig.Addresses {
			ft.contractAddresses = append(ft.contractAddresses, addr.Address)
		}
	}

	return ft
}

// Start begins monitoring for liquidation opportunities
func (ft *FluidTokens) Start() error {
	logger := logging.GetLogger()

	// Register event handler with indexer
	ft.idx.AddEventFunc(ft.HandleChainsyncEvent)

	// Start periodic check for expired rentals
	ft.checkTicker = time.NewTicker(30 * time.Second)
	go ft.checkExpiredRentals()

	logger.Info(
		"FluidTokens liquidator started",
		"profile", ft.profile.Name,
		"addresses", len(ft.contractAddresses),
	)

	return nil
}

// Stop stops the FluidTokens handler (idempotent - safe to call multiple times)
func (ft *FluidTokens) Stop() {
	ft.rentalsMu.Lock()
	if ft.stopped {
		ft.rentalsMu.Unlock()
		return
	}
	ft.stopped = true
	ft.rentalsMu.Unlock()

	if ft.checkTicker != nil {
		ft.checkTicker.Stop()
	}
	close(ft.stopChan)
}

// HandleChainsyncEvent processes chain sync events
func (ft *FluidTokens) HandleChainsyncEvent(evt event.Event) error {
	switch payload := evt.Payload.(type) {
	case event.TransactionEvent:
		return ft.handleTransaction(evt, payload)
	case event.RollbackEvent:
		return ft.handleRollback(payload)
	}
	return nil
}

// handleTransaction processes a transaction event
func (ft *FluidTokens) handleTransaction(
	evt event.Event,
	txEvt event.TransactionEvent,
) error {
	logger := logging.GetLogger()
	ctx := evt.Context.(event.TransactionContext)

	// Check for consumed rentals (remove from tracking)
	for _, input := range txEvt.Transaction.Consumed() {
		key := utxoKey(input.Id().String(), input.Index())
		ft.rentalsMu.Lock()
		if _, exists := ft.rentals[key]; exists {
			delete(ft.rentals, key)
			// Also remove from persistent storage
			if err := storage.GetStorage().RemoveUtxo(
				input.Id().String(),
				input.Index(),
			); err != nil {
				logger.Error(
					"failed to remove rental UTxO from storage",
					"error", err,
					"txHash", input.Id().String(),
					"index", input.Index(),
				)
			}
			logger.Debug(
				"rental UTxO consumed",
				"txHash", input.Id().String(),
				"index", input.Index(),
			)
		}
		ft.rentalsMu.Unlock()
	}

	// Check for new rentals at contract addresses
	for _, utxo := range txEvt.Transaction.Produced() {
		addr := utxo.Output.Address().String()
		if !ft.isContractAddress(addr) {
			continue
		}

		// Try to parse datum
		if utxo.Output.Datum() == nil {
			continue
		}

		var datum RentDatum
		if _, err := cbor.Decode(
			utxo.Output.Datum().Cbor(),
			&datum,
		); err != nil {
			// Not a valid rent datum, skip
			continue
		}

		// Track this rental
		key := utxoKey(ctx.TransactionHash, utxo.Id.Index())
		rental := &TrackedRental{
			TxHash:   ctx.TransactionHash,
			TxIndex:  utxo.Id.Index(),
			Datum:    &datum,
			Output:   utxo.Output,
			LastSeen: time.Now(),
		}

		ft.rentalsMu.Lock()
		ft.rentals[key] = rental
		ft.rentalsMu.Unlock()

		// Also store in persistent storage for recovery
		if err := storage.GetStorage().AddUtxo(
			addr,
			ctx.TransactionHash,
			utxo.Id.Index(),
			utxo.Output.Cbor(),
		); err != nil {
			logger.Error(
				"failed to store rental UTxO",
				"error", err,
				"txHash", ctx.TransactionHash,
				"index", utxo.Id.Index(),
			)
		}

		logger.Debug(
			"tracking new rental",
			"txHash", ctx.TransactionHash,
			"index", utxo.Id.Index(),
			"owner", datum.OwnerPaymentCred.Hash,
			"deadline", datum.Deadline(),
		)
	}

	return nil
}

// handleRollback processes a rollback event
func (ft *FluidTokens) handleRollback(evt event.RollbackEvent) error {
	logger := logging.GetLogger()
	logger.Warn(
		"rollback detected",
		"slot", evt.SlotNumber,
		"blockHash", evt.BlockHash,
	)

	// Remove any rentals that were seen at or after the rollback point
	// These UTxOs may no longer exist on the canonical chain
	ft.rentalsMu.Lock()
	var toRemove []string
	for key, rental := range ft.rentals {
		// Check if the rental was created from a transaction at or after rollback slot
		// Since we don't store the slot, use LastSeen as approximation
		// This is conservative - some valid rentals may be removed and re-added
		if rental.LastSeen.After(time.Now().Add(-30 * time.Second)) {
			toRemove = append(toRemove, key)
		}
	}
	for _, key := range toRemove {
		delete(ft.rentals, key)
		logger.Debug("removed rental due to rollback", "key", key)
	}
	ft.rentalsMu.Unlock()

	return nil
}

// checkExpiredRentals periodically checks for rentals that can be liquidated
func (ft *FluidTokens) checkExpiredRentals() {
	for {
		select {
		case <-ft.checkTicker.C:
			ft.processExpiredRentals()
		case <-ft.stopChan:
			return
		}
	}
}

// processExpiredRentals finds and processes expired rentals
func (ft *FluidTokens) processExpiredRentals() {
	logger := logging.GetLogger()

	ft.rentalsMu.RLock()
	var expired []*TrackedRental
	for _, rental := range ft.rentals {
		if rental.Datum.CanBeReturned() {
			expired = append(expired, rental)
		}
	}
	ft.rentalsMu.RUnlock()

	if len(expired) == 0 {
		return
	}

	logger.Info(
		"found expired rentals",
		"count", len(expired),
	)

	// Process up to 10 at a time (like the reference implementation)
	limit := 10
	if len(expired) < limit {
		limit = len(expired)
	}

	for i := 0; i < limit; i++ {
		rental := expired[i]
		if err := ft.liquidateRental(rental, uint64(i)); err != nil {
			logger.Error(
				"failed to liquidate rental",
				"error", err,
				"txHash", rental.TxHash,
				"index", rental.TxIndex,
			)
		}
	}
}

// liquidateRental builds and submits a liquidation transaction
func (ft *FluidTokens) liquidateRental(
	rental *TrackedRental,
	batchIndex uint64,
) error {
	logger := logging.GetLogger()
	bursa := wallet.GetWallet()

	logger.Info(
		"liquidating rental",
		"txHash", rental.TxHash,
		"index", rental.TxIndex,
		"owner", rental.Datum.OwnerPaymentCred.Hash,
		"deadline", rental.Datum.Deadline(),
	)

	// Build liquidation transaction
	txBytes, err := BuildReturnTx(BuildReturnTxOptions{
		RentalTxHash:  rental.TxHash,
		RentalTxIndex: rental.TxIndex,
		RentalOutput:  rental.Output,
		RentDatum:     rental.Datum,
		BatchIndex:    batchIndex,
		Profile:       ft.profile,
		ChangeAddress: bursa.PaymentAddress,
	})
	if err != nil {
		return err
	}

	// Submit the transaction
	txsubmit.SubmitTx(txBytes)

	logger.Info(
		"submitted liquidation transaction",
		"rentalTxHash", rental.TxHash,
		"index", rental.TxIndex,
	)

	return nil
}

// isContractAddress checks if an address is a monitored contract address
func (ft *FluidTokens) isContractAddress(addr string) bool {
	for _, contractAddr := range ft.contractAddresses {
		if addr == contractAddr {
			return true
		}
	}
	return false
}

// utxoKey creates a unique key for a UTxO
func utxoKey(txHash string, index uint32) string {
	return fmt.Sprintf("%s.%d", txHash, index)
}

// RentalCount returns the number of tracked rentals
func (ft *FluidTokens) RentalCount() int {
	ft.rentalsMu.RLock()
	defer ft.rentalsMu.RUnlock()
	return len(ft.rentals)
}

// GetExpiredRentals returns all rentals that can be liquidated
func (ft *FluidTokens) GetExpiredRentals() []*TrackedRental {
	ft.rentalsMu.RLock()
	defer ft.rentalsMu.RUnlock()

	var expired []*TrackedRental
	for _, rental := range ft.rentals {
		if rental.Datum.CanBeReturned() {
			expired = append(expired, rental)
		}
	}
	return expired
}
