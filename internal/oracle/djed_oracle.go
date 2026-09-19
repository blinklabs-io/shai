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
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/blinklabs-io/adder/event"
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/shai/common"
	"github.com/blinklabs-io/shai/internal/indexer"
	"github.com/blinklabs-io/shai/internal/logging"
	"github.com/blinklabs-io/shai/internal/storage"
	"github.com/blinklabs-io/shai/price/djed"
)

// The Cardano mainnet stability window is 3k/f = 129,600 slots. Retaining
// spent observations across that window keeps all rollback-relevant history.
const djedRollbackRetentionSlots uint64 = 129_600

// The Djed oracle NFT identity is derived once from compile-time constants.
// Deriving it per candidate output produced an error return that could only
// ever fire on a malformed constant, yet was fatal to the whole process.
var (
	djedOracleNFT       = djed.MainnetOracleAsset()
	djedOracleNFTPolicy = lcommon.NewBlake2b224(djedOracleNFT.PolicyId)
)

// djedCandidate pairs a produced output with its inline datum. Keeping the
// index and the datum together stops the two from drifting apart.
type djedCandidate struct {
	utxoIndex int
	datum     []byte
}

// DjedStateStorage persists rollback-aware Djed tracker snapshots.
type DjedStateStorage interface {
	SaveDjedState(string, djed.TrackerState) error
	LoadDjedState(string) (djed.TrackerState, error)
}

// DjedOracle adapts local chain-sync events to the authenticated Djed tracker.
type DjedOracle struct {
	idx     *indexer.Indexer
	network string
	address string
	storage DjedStateStorage

	mu      sync.RWMutex
	tracker *djed.Tracker
}

// NewDjedOracle creates a local-only Djed chain tracker.
func NewDjedOracle(
	idx *indexer.Indexer,
	network string,
	address string,
	stateStorage DjedStateStorage,
) *DjedOracle {
	return &DjedOracle{
		idx:     idx,
		network: network,
		address: address,
		storage: stateStorage,
		tracker: djed.NewTracker(),
	}
}

// Start restores persisted history and registers the chain-sync handler.
func (o *DjedOracle) Start() error {
	if o.idx == nil {
		return fmt.Errorf("djed oracle indexer is required")
	}
	if o.network != "mainnet" {
		return fmt.Errorf("djed oracle is only configured for mainnet")
	}
	if o.address != djed.MainnetOracleAddress {
		return fmt.Errorf("djed oracle address does not match mainnet deployment")
	}
	if o.storage == nil {
		return fmt.Errorf("djed oracle storage is required")
	}
	state, err := o.storage.LoadDjedState(o.network)
	if err != nil && !errors.Is(err, storage.ErrDjedStateNotFound) {
		return err
	}
	if err == nil {
		tracker, restoreErr := djed.NewTrackerFromState(state)
		if restoreErr != nil {
			// Unreadable persisted state is fatal rather than silently
			// discarded, so name the key an operator has to remove to
			// resync from the profile intercept point.
			return fmt.Errorf(
				"restore Djed tracker: %w (delete storage key %s to resync)",
				restoreErr,
				storage.DjedStateKey(o.network),
			)
		}
		o.tracker = tracker
	}
	o.idx.AddEventFunc(o.HandleChainsyncEvent)
	return nil
}

// Current returns the currently valid authenticated local observation.
func (o *DjedOracle) Current(now time.Time) (djed.Observation, error) {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.tracker.Current(now)
}

// HandleChainsyncEvent applies transactions and rollbacks atomically with
// their persisted tracker snapshot.
func (o *DjedOracle) HandleChainsyncEvent(evt event.Event) error {
	switch payload := evt.Payload.(type) {
	case event.TransactionEvent:
		ctx, ok := evt.Context.(event.TransactionContext)
		if !ok {
			logging.GetLogger().Warn(
				"djed oracle: ignoring transaction with unexpected context",
				"contextType", fmt.Sprintf("%T", evt.Context),
			)
			return nil
		}
		// evt.Timestamp is adder's wall-clock ingest time, not block time,
		// so it is not used to judge an observation's validity window.
		// Current does that at serving time.
		return o.handleTransaction(ctx, payload)
	case event.RollbackEvent:
		return o.handleRollback(payload)
	default:
		return nil
	}
}

func (o *DjedOracle) handleTransaction(
	ctx event.TransactionContext,
	txEvt event.TransactionEvent,
) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	inputRefs := make([]djed.OutputRef, 0)
	for _, input := range transactionInputs(txEvt) {
		ref := djed.OutputRef{
			TxHash:  input.Id().String(),
			TxIndex: input.Index(),
		}
		if o.tracker.Contains(ref) {
			inputRefs = append(inputRefs, ref)
		}
	}

	logger := logging.GetLogger()
	utxos := producedUTXOs(txEvt, ctx.TransactionHash)
	candidates := make([]djedCandidate, 0)
	for i, utxo := range utxos {
		output := utxo.Output
		if output.Address().String() != o.address {
			continue
		}
		if !hasDjedNFT(output.Assets()) {
			continue
		}
		datum := output.Datum()
		if datum == nil {
			logger.Warn(
				"ignoring Djed oracle output without inline datum",
				"txHash", ctx.TransactionHash,
				"txIndex", utxo.Id.Index(),
			)
			continue
		}
		candidates = append(candidates, djedCandidate{
			utxoIndex: i,
			datum:     datum.Cbor(),
		})
	}
	if len(inputRefs) == 0 && len(candidates) == 0 {
		return nil
	}

	staged, err := djed.NewTrackerFromState(o.tracker.Snapshot())
	if err != nil {
		return fmt.Errorf("stage Djed tracker: %w", err)
	}
	changed := false
	for _, ref := range inputRefs {
		if staged.ConsumeAt(ref, ctx.SlotNumber) {
			changed = true
		}
	}
	for _, candidate := range candidates {
		utxo := utxos[candidate.utxoIndex]
		ref := djed.OutputRef{
			TxHash:  ctx.TransactionHash,
			TxIndex: utxo.Id.Index(),
		}
		if staged.Contains(ref) {
			continue
		}
		output := utxo.Output
		// The parser re-checks the oracle NFT against the output's own
		// assets, so hasDjedNFT above stays a pre-filter whose failure mode
		// is a miss rather than a false accept.
		_, err = staged.Apply(
			candidate.datum,
			djed.OracleUTxO{
				Address:          output.Address().String(),
				Assets:           outputAssetAmounts(output.Assets()),
				TxHash:           ctx.TransactionHash,
				TxIndex:          utxo.Id.Index(),
				TransactionIndex: ctx.TransactionIdx,
				Slot:             ctx.SlotNumber,
				BlockHash:        txEvt.BlockHash,
			},
		)
		if err != nil {
			logger.Warn(
				"ignoring invalid Djed oracle output",
				"error", err,
				"txHash", ctx.TransactionHash,
				"txIndex", utxo.Id.Index(),
			)
			continue
		}
		changed = true
	}
	if !changed {
		return nil
	}
	if ctx.SlotNumber > djedRollbackRetentionSlots {
		staged.Prune(ctx.SlotNumber - djedRollbackRetentionSlots)
	}
	if err := o.storage.SaveDjedState(
		o.network,
		staged.Snapshot(),
	); err != nil {
		return err
	}
	o.tracker = staged
	return nil
}

func (o *DjedOracle) handleRollback(evt event.RollbackEvent) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	staged, err := djed.NewTrackerFromState(o.tracker.Snapshot())
	if err != nil {
		return fmt.Errorf("stage Djed tracker: %w", err)
	}
	changed := staged.Rollback(evt.SlotNumber)
	pruned := 0
	if evt.SlotNumber > djedRollbackRetentionSlots {
		pruned = staged.Prune(evt.SlotNumber - djedRollbackRetentionSlots)
	}
	if !changed && pruned == 0 {
		return nil
	}
	if err := o.storage.SaveDjedState(
		o.network,
		staged.Snapshot(),
	); err != nil {
		return err
	}
	o.tracker = staged
	return nil
}

// hasDjedNFT pre-filters produced outputs. MultiAsset.Asset returns a nil
// amount for an absent policy or name, so an output at the oracle address
// carrying any other native token reaches this with nil.
func hasDjedNFT(
	assets *lcommon.MultiAsset[lcommon.MultiAssetTypeOutput],
) bool {
	if assets == nil {
		return false
	}
	amount := assets.Asset(djedOracleNFTPolicy, djedOracleNFT.Name)
	return amount != nil && amount.IsInt64() && amount.Int64() == 1
}

// outputAssetAmounts converts an output's native tokens for the Djed parser.
// Amounts that do not fit a uint64 are dropped; they cannot be the oracle NFT,
// whose amount is 1.
func outputAssetAmounts(
	assets *lcommon.MultiAsset[lcommon.MultiAssetTypeOutput],
) []common.AssetAmount {
	if assets == nil {
		return nil
	}
	amounts := make([]common.AssetAmount, 0)
	for _, policy := range assets.Policies() {
		for _, name := range assets.Assets(policy) {
			amount := assets.Asset(policy, name)
			if amount == nil || !amount.IsUint64() {
				continue
			}
			amounts = append(amounts, common.AssetAmount{
				Class: common.AssetClass{
					PolicyId: policy.Bytes(),
					Name:     name,
				},
				Amount: amount.Uint64(),
			})
		}
	}
	return amounts
}
