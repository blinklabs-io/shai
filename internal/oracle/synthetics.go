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
	"sync"
	"time"

	"github.com/blinklabs-io/adder/event"
	"github.com/blinklabs-io/shai/internal/config"
	"github.com/blinklabs-io/shai/internal/indexer"
	"github.com/blinklabs-io/shai/internal/logging"
)

// SyntheticsState is a parsed state object emitted by a synthetics protocol.
type SyntheticsState interface {
	Key() string
	SlotNumber() uint64
}

// SyntheticsParser parses protocol-specific synthetics datums.
type SyntheticsParser interface {
	ParseSyntheticsDatum(
		datum []byte,
		txHash string,
		txIndex uint32,
		slot uint64,
		timestamp time.Time,
	) (SyntheticsState, error)
	Protocol() string
}

// SyntheticsUpdate is emitted when a monitored synthetics state changes.
type SyntheticsUpdate struct {
	Protocol  string          `json:"protocol"`
	StateKey  string          `json:"stateKey"`
	State     SyntheticsState `json:"state"`
	BlockHash string          `json:"blockHash,omitempty"`
	TxHash    string          `json:"txHash"`
	TxIndex   uint32          `json:"txIndex"`
	Slot      uint64          `json:"slot"`
	Timestamp time.Time       `json:"timestamp"`
}

// SyntheticsOracle tracks state UTxOs for synthetics protocols.
type SyntheticsOracle struct {
	idx       *indexer.Indexer
	profile   *config.Profile
	parser    SyntheticsParser
	addresses map[string]struct{}
	states    map[string]SyntheticsState
	statesMu  sync.RWMutex
	stopChan  chan struct{}
	subs      []chan *SyntheticsUpdate
	subMu     sync.RWMutex
	stopped   bool
}

// NewSynthetics creates a synthetics oracle for the given profile and parser.
func NewSynthetics(
	idx *indexer.Indexer,
	profile *config.Profile,
	parser SyntheticsParser,
) *SyntheticsOracle {
	o := &SyntheticsOracle{
		idx:       idx,
		profile:   profile,
		parser:    parser,
		addresses: make(map[string]struct{}),
		states:    make(map[string]SyntheticsState),
		stopChan:  make(chan struct{}),
	}

	if synthCfg, ok := profile.Config.(config.SyntheticsProfileConfig); ok {
		for _, addr := range synthCfg.CDPAddresses {
			o.addresses[addr.Address] = struct{}{}
		}
		for _, addr := range synthCfg.OracleAddresses {
			o.addresses[addr.Address] = struct{}{}
		}
	}

	return o
}

// Start begins tracking synthetics state.
func (o *SyntheticsOracle) Start() error {
	o.idx.AddEventFunc(o.HandleChainsyncEvent)

	logger := logging.GetLogger()
	logger.Info(
		"Synthetics oracle started",
		"profile", o.profile.Name,
		"protocol", o.parser.Protocol(),
		"addresses", len(o.addresses),
	)

	return nil
}

// Stop stops the synthetics oracle.
func (o *SyntheticsOracle) Stop() {
	o.subMu.Lock()
	if o.stopped {
		o.subMu.Unlock()
		return
	}
	o.stopped = true
	for _, ch := range o.subs {
		close(ch)
	}
	o.subs = nil
	o.subMu.Unlock()

	close(o.stopChan)
}

// Subscribe returns a receive-only channel for synthetics state updates.
func (o *SyntheticsOracle) Subscribe() <-chan *SyntheticsUpdate {
	o.subMu.Lock()
	defer o.subMu.Unlock()

	ch := make(chan *SyntheticsUpdate, subscriberBufferSize)
	if o.stopped {
		close(ch)
		return ch
	}
	o.subs = append(o.subs, ch)
	return ch
}

// Unsubscribe removes a synthetics subscriber channel.
func (o *SyntheticsOracle) Unsubscribe(ch <-chan *SyntheticsUpdate) {
	o.subMu.Lock()
	defer o.subMu.Unlock()

	for i, sub := range o.subs {
		if (<-chan *SyntheticsUpdate)(sub) == ch {
			o.subs = append(o.subs[:i], o.subs[i+1:]...)
			close(sub)
			return
		}
	}
}

// HandleChainsyncEvent processes chain sync events.
func (o *SyntheticsOracle) HandleChainsyncEvent(evt event.Event) error {
	switch payload := evt.Payload.(type) {
	case event.TransactionEvent:
		return o.handleTransaction(evt, payload)
	case event.RollbackEvent:
		return o.handleRollback(payload)
	}
	return nil
}

func (o *SyntheticsOracle) handleTransaction(
	evt event.Event,
	txEvt event.TransactionEvent,
) error {
	logger := logging.GetLogger()
	ctx, ok := evt.Context.(event.TransactionContext)
	if !ok {
		logger.Error(
			"unexpected event context type",
			"expected", "event.TransactionContext",
			"got", fmt.Sprintf("%T", evt.Context),
		)
		return nil
	}
	if txEvt.Transaction == nil {
		return nil
	}

	for _, utxo := range txEvt.Transaction.Produced() {
		if utxo.Id == nil || utxo.Output == nil {
			continue
		}
		addr := utxo.Output.Address().String()
		if !o.isAddress(addr) {
			continue
		}

		datum := utxo.Output.Datum()
		if datum == nil {
			continue
		}

		now := time.Now()
		state, err := o.parser.ParseSyntheticsDatum(
			datum.Cbor(),
			ctx.TransactionHash,
			utxo.Id.Index(),
			ctx.SlotNumber,
			now,
		)
		if err != nil {
			logger.Debug(
				"failed to parse synthetics datum",
				"error", err,
				"protocol", o.parser.Protocol(),
				"txHash", ctx.TransactionHash,
				"txIndex", utxo.Id.Index(),
			)
			continue
		}
		if state == nil {
			continue
		}

		o.statesMu.Lock()
		o.states[state.Key()] = state
		o.statesMu.Unlock()

		update := &SyntheticsUpdate{
			Protocol:  o.parser.Protocol(),
			StateKey:  state.Key(),
			State:     state,
			BlockHash: txEvt.BlockHash,
			TxHash:    ctx.TransactionHash,
			TxIndex:   utxo.Id.Index(),
			Slot:      ctx.SlotNumber,
			Timestamp: now,
		}
		o.notifySubscribers(update)

		logger.Debug(
			"synthetics state updated",
			"stateKey", state.Key(),
			"protocol", o.parser.Protocol(),
			"slot", ctx.SlotNumber,
		)
	}

	return nil
}

func (o *SyntheticsOracle) handleRollback(evt event.RollbackEvent) error {
	o.statesMu.Lock()
	for key, state := range o.states {
		if state.SlotNumber() >= evt.SlotNumber {
			delete(o.states, key)
		}
	}
	o.statesMu.Unlock()
	return nil
}

func (o *SyntheticsOracle) isAddress(addr string) bool {
	_, ok := o.addresses[addr]
	return ok
}

func (o *SyntheticsOracle) notifySubscribers(update *SyntheticsUpdate) {
	o.subMu.RLock()
	defer o.subMu.RUnlock()

	for _, ch := range o.subs {
		updateCopy := *update
		select {
		case ch <- &updateCopy:
		default:
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- &updateCopy:
			default:
			}
		}
	}
}

// GetState returns a tracked synthetics state by key.
func (o *SyntheticsOracle) GetState(key string) (SyntheticsState, bool) {
	o.statesMu.RLock()
	defer o.statesMu.RUnlock()

	state, ok := o.states[key]
	return state, ok
}

// StateCount returns the number of tracked synthetics states.
func (o *SyntheticsOracle) StateCount() int {
	o.statesMu.RLock()
	defer o.statesMu.RUnlock()
	return len(o.states)
}
