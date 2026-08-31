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
	"encoding/hex"
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
			return fmt.Errorf("restore Djed tracker: %w", restoreErr)
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
		return o.handleTransaction(evt.Timestamp, ctx, payload)
	case event.RollbackEvent:
		return o.handleRollback(payload)
	default:
		return nil
	}
}

func (o *DjedOracle) handleTransaction(
	observedAt time.Time,
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
	outputIndexes := make([]int, 0)
	outputData := make([][]byte, 0)
	for i, utxo := range utxos {
		output := utxo.Output
		if output.Address().String() != o.address {
			continue
		}
		hasNFT, err := hasDjedNFT(output.Assets())
		if err != nil {
			logger.Warn(
				"ignoring Djed oracle output with invalid asset data",
				"error", err,
				"txHash", ctx.TransactionHash,
				"txIndex", utxo.Id.Index(),
			)
			continue
		}
		if !hasNFT {
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
		outputIndexes = append(outputIndexes, i)
		outputData = append(outputData, datum.Cbor())
	}
	if len(inputRefs) == 0 && len(outputIndexes) == 0 {
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
	oracleNFT, err := djedNFT()
	if err != nil {
		return err
	}
	for candidateIndex, outputIndex := range outputIndexes {
		utxo := utxos[outputIndex]
		ref := djed.OutputRef{TxHash: ctx.TransactionHash, TxIndex: utxo.Id.Index()}
		if staged.Contains(ref) {
			continue
		}
		output := utxo.Output
		_, err = staged.Apply(
			outputData[candidateIndex],
			djed.OracleUTxO{
				Address: output.Address().String(),
				Assets: []common.AssetAmount{{
					Class:  oracleNFT,
					Amount: 1,
				}},
				TxHash:           ctx.TransactionHash,
				TxIndex:          utxo.Id.Index(),
				TransactionIndex: ctx.TransactionIdx,
				Slot:             ctx.SlotNumber,
				BlockHash:        txEvt.BlockHash,
			},
			observedAt,
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

func hasDjedNFT(
	assets *lcommon.MultiAsset[lcommon.MultiAssetTypeOutput],
) (bool, error) {
	if assets == nil {
		return false, nil
	}
	policyBytes, err := hex.DecodeString(djed.MainnetOraclePolicy)
	if err != nil {
		return false, fmt.Errorf("decode Djed NFT policy: %w", err)
	}
	name, err := hex.DecodeString(djed.OracleNFTName)
	if err != nil {
		return false, fmt.Errorf("decode Djed NFT name: %w", err)
	}
	var policy lcommon.Blake2b224
	if len(policyBytes) != len(policy) {
		return false, fmt.Errorf("invalid Djed NFT policy length")
	}
	copy(policy[:], policyBytes)
	amount := assets.Asset(policy, name)
	return amount != nil && amount.IsInt64() && amount.Int64() == 1, nil
}

func djedNFT() (common.AssetClass, error) {
	asset, err := common.NewAssetClass(
		djed.MainnetOraclePolicy,
		djed.OracleNFTName,
	)
	if err != nil {
		return common.AssetClass{}, fmt.Errorf(
			"decode Djed NFT identity: %w",
			err,
		)
	}
	return asset, nil
}
