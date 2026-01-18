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

	"github.com/blinklabs-io/shai/internal/oracle/splash"
)

// Re-export constants for backward compatibility
const (
	SplashProtocolName     = splash.ProtocolName
	SplashV1PoolScriptHash = splash.V1PoolScriptHash
	SplashFeeDenom         = splash.FeeDenom
)

// Re-export types for backward compatibility
type (
	SplashV1PoolDatum         = splash.V1PoolDatum
	SplashAsset               = splash.Asset
	SplashBytesSingletonArray = splash.BytesSingletonArray
)

// SplashParser wraps splash.Parser for backward compatibility
type SplashParser struct {
	parser *splash.Parser
}

// NewSplashV1Parser creates a parser for Splash V1 pools
func NewSplashV1Parser() *SplashParser {
	return &SplashParser{parser: splash.NewV1Parser()}
}

// Protocol returns the protocol name
func (p *SplashParser) Protocol() string {
	return p.parser.Protocol()
}

// ParsePoolDatum parses a Splash pool datum
func (p *SplashParser) ParsePoolDatum(
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
	// Convert splash.PoolState to oracle.PoolState
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

// generateSplashPoolId wraps splash.GeneratePoolId for backward compatibility
func generateSplashPoolId(policyId, tokenName []byte) string {
	return splash.GeneratePoolId(policyId, tokenName)
}

// GetSplashV1PoolAddresses returns mainnet V1 pool addresses
func GetSplashV1PoolAddresses() []string {
	return splash.GetV1PoolAddresses()
}
