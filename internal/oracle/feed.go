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
	"log/slog"

	"github.com/blinklabs-io/adder/event"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/shai/internal/config"
	"github.com/blinklabs-io/shai/internal/indexer"
	"github.com/blinklabs-io/shai/internal/logging"
	"github.com/blinklabs-io/shai/internal/storage"
	"github.com/blinklabs-io/shai/oraclefeed"
)

const (
	feedBootstrapSlot = 188293000
	feedBootstrapHash = "79fd4b75508af079ab01bcbaa68ba1fe0ff3776a087056f33239341a8532d92d"
)

// FeedOracle maintains authenticated price-feed state from the same local
// chain-sync stream used by the DEX oracles.
type FeedOracle struct {
	idx     *indexer.Indexer
	tracker *oraclefeed.Tracker
	storage *FeedStorage
}

func NewFeedOracle(idx *indexer.Indexer) *FeedOracle {
	return &FeedOracle{
		idx:     idx,
		tracker: oraclefeed.NewTracker(),
	}
}

func (o *FeedOracle) Start() error {
	if o.idx == nil {
		return fmt.Errorf("feed oracle: nil indexer")
	}
	if err := o.tracker.ValidateConfiguration(); err != nil {
		return fmt.Errorf("feed oracle configuration: %w", err)
	}
	feedStorage, err := NewFeedStorage()
	if err != nil {
		return err
	}
	o.storage = feedStorage
	records, err := o.storage.Load()
	if err != nil {
		_ = o.storage.Close()
		return err
	}
	for _, record := range records {
		if _, matched, err := o.tracker.Apply(record.UTxO); err != nil {
			_ = o.storage.Close()
			return fmt.Errorf("restore feed UTxO: %w", err)
		} else if !matched {
			continue
		}
		if record.SpentAt != nil {
			o.tracker.ConsumeAt(
				oraclefeed.OutputRef{
					TxHash:  record.UTxO.TxHash,
					TxIndex: record.UTxO.TxIndex,
				},
				*record.SpentAt,
			)
		}
	}
	if len(records) == 0 {
		cfg := config.GetConfig()
		cursorSlot, _, cursorErr := storage.GetStorage().GetCursor()
		if cursorErr != nil {
			_ = o.storage.Close()
			return fmt.Errorf("read cursor for feed bootstrap: %w", cursorErr)
		}
		if cfg.Network == "mainnet" && cursorSlot > feedBootstrapSlot {
			if err := o.idx.SetReplayPoint(
				feedBootstrapSlot,
				feedBootstrapHash,
			); err != nil {
				_ = o.storage.Close()
				return fmt.Errorf("configure feed bootstrap replay: %w", err)
			}
		}
	}
	o.idx.AddEventFunc(o.HandleChainsyncEvent)
	logging.GetLogger().Info(
		"price feed oracle started",
		"addresses",
		o.tracker.Addresses(),
	)
	return nil
}

func (o *FeedOracle) Stop() error {
	if o.storage == nil {
		return nil
	}
	return o.storage.Close()
}

func (o *FeedOracle) Tracker() *oraclefeed.Tracker {
	return o.tracker
}

func (o *FeedOracle) HandleChainsyncEvent(evt event.Event) error {
	switch payload := evt.Payload.(type) {
	case event.TransactionEvent:
		return o.handleFeedTransaction(evt, payload)
	case event.RollbackEvent:
		o.tracker.Rollback(payload.SlotNumber)
		if err := o.storage.Rollback(payload.SlotNumber); err != nil {
			return fmt.Errorf("rollback feed storage: %w", err)
		}
	}
	return nil
}

func (o *FeedOracle) handleFeedTransaction(
	evt event.Event,
	txEvt event.TransactionEvent,
) error {
	ctx, ok := evt.Context.(event.TransactionContext)
	if !ok {
		return nil
	}
	for _, input := range transactionInputs(txEvt) {
		o.tracker.ConsumeAt(
			oraclefeed.OutputRef{
				TxHash:  input.Id().String(),
				TxIndex: input.Index(),
			},
			ctx.SlotNumber,
		)
		if err := o.storage.Spend(
			oraclefeed.OutputRef{
				TxHash:  input.Id().String(),
				TxIndex: input.Index(),
			},
			ctx.SlotNumber,
		); err != nil {
			return fmt.Errorf("persist spent feed UTxO: %w", err)
		}
	}
	for _, utxo := range producedUTXOs(txEvt, ctx.TransactionHash) {
		if err := o.handleFeedOutput(
			logging.GetLogger(),
			utxo,
			ctx,
			txEvt.BlockHash,
		); err != nil {
			return err
		}
	}
	return nil
}

func (o *FeedOracle) handleFeedOutput(
	logger *slog.Logger,
	utxo ledger.Utxo,
	ctx event.TransactionContext,
	blockHash string,
) error {
	address := utxo.Output.Address().String()
	if !o.tracker.TracksAddress(address) {
		return nil
	}
	datum := utxo.Output.Datum()
	if datum == nil {
		logger.Warn(
			"oracle feed output has no datum",
			"address", address,
			"txHash", ctx.TransactionHash,
			"txIndex", utxo.Id.Index(),
		)
		return nil
	}
	assets, err := feedAssets(utxo.Output)
	if err != nil {
		logger.Warn(
			"failed to read oracle feed assets",
			"error", err,
			"txHash", ctx.TransactionHash,
			"txIndex", utxo.Id.Index(),
		)
		return nil
	}
	feedUTxO := oraclefeed.UTxO{
		Address: address,
		Assets:  assets,
		Datum:   datum.Cbor(),
		TxHash:  ctx.TransactionHash,
		TxIndex: utxo.Id.Index(),
		Slot:    ctx.SlotNumber,
	}
	observation, _, err := o.tracker.Apply(feedUTxO)
	if err != nil {
		logger.Warn(
			"rejected oracle feed output",
			"error", err,
			"address", address,
			"txHash", ctx.TransactionHash,
			"txIndex", utxo.Id.Index(),
		)
		return nil
	}
	if err := o.storage.Save(feedUTxO); err != nil {
		return fmt.Errorf(
			"persist oracle feed output %s#%d: %w",
			ctx.TransactionHash,
			utxo.Id.Index(),
			err,
		)
	}
	logger.Info(
		"authenticated oracle feed observation",
		"source", observation.Source,
		"pair", observation.Pair,
		"price", observation.Float64(),
		"observedAt", observation.ObservedAt,
		"slot", observation.Slot,
		"blockHash", blockHash,
	)
	return nil
}

func feedAssets(output ledger.TransactionOutput) ([]oraclefeed.Asset, error) {
	multiAsset := output.Assets()
	if multiAsset == nil {
		return nil, nil
	}
	var ret []oraclefeed.Asset
	for _, policy := range multiAsset.Policies() {
		for _, name := range multiAsset.Assets(policy) {
			quantity := multiAsset.Asset(policy, name)
			if quantity == nil || quantity.Sign() < 0 || !quantity.IsUint64() {
				return nil, fmt.Errorf(
					"invalid asset quantity for %s.%x",
					policy.String(),
					name,
				)
			}
			value := quantity.Uint64()
			ret = append(ret, oraclefeed.Asset{
				PolicyID: policy.String(),
				Name:     fmt.Sprintf("%x", name),
				Quantity: value,
			})
		}
	}
	return ret, nil
}
