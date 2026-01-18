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

	"github.com/blinklabs-io/shai/internal/oracle/sundaeswap"
)

// Re-export constants for backward compatibility
const (
	SundaeSwapProtocolName     = sundaeswap.ProtocolName
	SundaeSwapV1PoolScriptHash = sundaeswap.V1PoolScriptHash
	SundaeSwapV3PoolScriptHash = sundaeswap.V3PoolScriptHash
	SundaeSwapFeeDenom         = sundaeswap.FeeDenom
	SundaeSwapV1DefaultFee     = sundaeswap.V1DefaultFee
)

// Re-export types for backward compatibility
type (
	SundaeSwapV1PoolDatum      = sundaeswap.V1PoolDatum
	SundaeSwapV3PoolDatum      = sundaeswap.V3PoolDatum
	SundaeSwapAssetPair        = sundaeswap.AssetPair
	SundaeSwapAsset            = sundaeswap.Asset
	SundaeSwapOptionalMultisig = sundaeswap.OptionalMultisig
)

// SundaeSwapParser wraps sundaeswap.Parser for backward compatibility
type SundaeSwapParser struct {
	parser *sundaeswap.Parser
}

// NewSundaeSwapV1Parser creates a parser for SundaeSwap V1 pools
func NewSundaeSwapV1Parser() *SundaeSwapParser {
	return &SundaeSwapParser{parser: sundaeswap.NewV1Parser()}
}

// NewSundaeSwapV3Parser creates a parser for SundaeSwap V3 pools
func NewSundaeSwapV3Parser() *SundaeSwapParser {
	return &SundaeSwapParser{parser: sundaeswap.NewV3Parser()}
}

// Protocol returns the protocol name
func (p *SundaeSwapParser) Protocol() string {
	return p.parser.Protocol()
}

// ParsePoolDatum parses a SundaeSwap pool datum
func (p *SundaeSwapParser) ParsePoolDatum(
	datum []byte,
	txHash string,
	txIndex uint32,
	slot uint64,
	timestamp time.Time,
) (*PoolState, error) {
	state, err := p.parser.ParsePoolDatum(
		datum,
		txHash,
		txIndex,
		slot,
		timestamp,
	)
	if err != nil {
		return nil, err
	}
	// Convert sundaeswap.PoolState to oracle.PoolState
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

// generateSundaeSwapPoolId wraps sundaeswap.GeneratePoolId for backward compatibility
func generateSundaeSwapPoolId(identifier []byte) string {
	return sundaeswap.GeneratePoolId(identifier)
}

// GetSundaeSwapV1PoolAddresses returns mainnet V1 pool addresses
func GetSundaeSwapV1PoolAddresses() []string {
	return sundaeswap.GetV1PoolAddresses()
}

// GetSundaeSwapV3PoolAddresses returns mainnet V3 pool addresses
func GetSundaeSwapV3PoolAddresses() []string {
	return sundaeswap.GetV3PoolAddresses()
}
