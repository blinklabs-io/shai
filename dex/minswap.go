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

package dex

import (
	"time"

	"github.com/blinklabs-io/shai/dex/minswap"
)

// MinswapParser wraps minswap.Parser to implement oracle.PoolParser
type MinswapParser struct {
	parser *minswap.Parser
}

// NewMinswapV1Parser creates a parser for Minswap V1 pools
func NewMinswapV1Parser() *MinswapParser {
	return &MinswapParser{parser: minswap.NewV1Parser()}
}

// NewMinswapV2Parser creates a parser for Minswap V2 pools
func NewMinswapV2Parser() *MinswapParser {
	return &MinswapParser{parser: minswap.NewV2Parser()}
}

// Protocol returns the protocol name
func (p *MinswapParser) Protocol() string {
	return p.parser.Protocol()
}

// PoolAddresses returns the mainnet script addresses holding this protocol's
// pool UTxOs. Query your node for UTxOs at these addresses, then feed each
// output's datum and value CBOR to ParsePoolDatum.
func (p *MinswapParser) PoolAddresses() []string {
	return PoolAddresses(p.Protocol())
}

// ParsePoolDatum parses a Minswap pool datum
func (p *MinswapParser) ParsePoolDatum(
	datum []byte,
	utxoValue []byte,
	txHash string,
	txIndex uint32,
	slot uint64,
	timestamp time.Time,
) (*PoolState, error) {
	state, err := p.parser.ParsePoolDatum(
		datum,
		utxoValue,
		txHash,
		txIndex,
		slot,
		timestamp,
	)
	if err != nil {
		return nil, err
	}
	return &PoolState{
		PoolId:    state.PoolId,
		Protocol:  state.Protocol,
		AssetX:    state.AssetX,
		AssetY:    state.AssetY,
		FeeNum:    state.FeeNum,
		FeeDenom:  state.FeeDenom,
		Slot:      state.Slot,
		TxHash:    state.TxHash,
		TxIndex:   state.TxIndex,
		Timestamp: state.Timestamp,
	}, nil
}
