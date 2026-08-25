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
	"time"

	"github.com/blinklabs-io/shai/dex/butane"
)

// ButaneParser adapts Butane CDP datums to the shared oracle CDP model.
type ButaneParser struct {
	parser *butane.Parser
}

var _ CDPParser = (*ButaneParser)(nil)

// NewButaneParser creates a parser for Butane protocol
func NewButaneParser() *ButaneParser {
	return &ButaneParser{parser: butane.NewParser()}
}

// Protocol returns the protocol name
func (p *ButaneParser) Protocol() string {
	return p.parser.Protocol()
}

// ParseCDPDatum parses a Butane CDP datum for the shared CDP lifecycle.
func (p *ButaneParser) ParseCDPDatum(
	datum []byte,
	txHash string,
	txIndex uint32,
	slot uint64,
	timestamp time.Time,
) (*CDPState, error) {
	state, err := p.parser.ParseMonoDatum(
		datum,
		txHash,
		txIndex,
		slot,
		timestamp,
	)
	if err != nil || state == nil {
		return nil, err
	}
	return &CDPState{
		CDPId:        state.CDPId,
		Owner:        state.Owner,
		HasOwner:     state.Owner != "",
		IAsset:       state.Synthetic.Fingerprint(),
		MintedAmount: state.MintedAmount,
		Slot:         state.Slot,
		TxHash:       state.TxHash,
		TxIndex:      state.TxIndex,
		Timestamp:    state.Timestamp,
	}, nil
}

// CDPIdForOutput returns the Butane CDP state key for a UTxO.
func (p *ButaneParser) CDPIdForOutput(txHash string, txIndex uint32) string {
	return butane.GenerateCDPId(txHash, txIndex)
}

// ParsePoolDatum implements PoolParser interface for compatibility
// Butane is a synthetics protocol, not an AMM, so this returns nil
func (p *ButaneParser) ParsePoolDatum(
	datum []byte,
	utxoValue []byte,
	txHash string,
	txIndex uint32,
	slot uint64,
	timestamp time.Time,
) (*PoolState, error) {
	// Butane doesn't have AMM pools, return nil
	return nil, nil
}
