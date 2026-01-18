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

	"github.com/blinklabs-io/shai/internal/oracle/minswap"
)

// Re-export constants for backward compatibility
const (
	MinswapProtocolName     = minswap.ProtocolName
	MinswapV1PoolScriptHash = minswap.V1PoolScriptHash
	MinswapV1FactoryPolicy  = minswap.V1FactoryPolicy
	MinswapV1LPPolicy       = minswap.V1LPPolicy
	MinswapV1PoolNFTPolicy  = minswap.V1PoolNFTPolicy
	MinswapV2PoolScriptHash = minswap.V2PoolScriptHash
	MinswapV2FactoryPolicy  = minswap.V2FactoryPolicy
	MinswapV2LPPolicy       = minswap.V2LPPolicy
	MinswapFeeDenom         = minswap.FeeDenom
)

// Re-export types for backward compatibility
type (
	MinswapV1PoolDatum    = minswap.V1PoolDatum
	MinswapV2PoolDatum    = minswap.V2PoolDatum
	MinswapAsset          = minswap.Asset
	MinswapCredential     = minswap.Credential
	MinswapBaseFee        = minswap.BaseFee
	MinswapFeeSharing     = minswap.FeeSharing
	MinswapOptionalUint64 = minswap.OptionalUint64
	MinswapBool           = minswap.Bool
)

// MinswapParser wraps minswap.Parser for backward compatibility
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

// ParsePoolDatum parses a Minswap pool datum
func (p *MinswapParser) ParsePoolDatum(
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
	// Convert minswap.PoolState to oracle.PoolState
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

// generateMinswapPoolId wraps minswap.GeneratePoolId for backward compatibility
func generateMinswapPoolId(policyA, nameA, policyB, nameB []byte) string {
	return minswap.GeneratePoolId(policyA, nameA, policyB, nameB)
}

// GetMinswapV1PoolAddresses returns mainnet V1 pool addresses
func GetMinswapV1PoolAddresses() []string {
	return minswap.GetV1PoolAddresses()
}

// GetMinswapV2PoolAddresses returns mainnet V2 pool addresses
func GetMinswapV2PoolAddresses() []string {
	return minswap.GetV2PoolAddresses()
}
