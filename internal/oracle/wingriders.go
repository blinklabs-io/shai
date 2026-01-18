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

	"github.com/blinklabs-io/shai/internal/oracle/wingriders"
)

// Re-export constants for backward compatibility
const (
	WingRidersProtocolName     = wingriders.ProtocolName
	WingRidersV2PoolScriptHash = wingriders.V2PoolScriptHash
	WingRidersV2LPTokenPolicy  = wingriders.V2LPTokenPolicy
	WingRidersFeeBasis         = wingriders.FeeBasis
)

// Re-export types for backward compatibility
type (
	WingRidersV2PoolDatum       = wingriders.V2PoolDatum
	WingRidersOptionalInt       = wingriders.OptionalInt
	WingRidersOptionalAddress   = wingriders.OptionalAddress
	WingRidersAddress           = wingriders.Address
	WingRidersCredential        = wingriders.Credential
	WingRidersStakingCredential = wingriders.StakingCredential
	WingRidersPoolVariant       = wingriders.PoolVariant
)

// WingRidersParser wraps wingriders.Parser for backward compatibility
type WingRidersParser struct {
	parser *wingriders.Parser
}

// NewWingRidersV2Parser creates a parser for WingRiders V2 pools
func NewWingRidersV2Parser() *WingRidersParser {
	return &WingRidersParser{parser: wingriders.NewV2Parser()}
}

// Protocol returns the protocol name
func (p *WingRidersParser) Protocol() string {
	return p.parser.Protocol()
}

// ParsePoolDatum parses a WingRiders pool datum
func (p *WingRidersParser) ParsePoolDatum(
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
	// Convert wingriders.PoolState to oracle.PoolState
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

// generateWingRidersPoolId wraps wingriders.GeneratePoolId for backward compatibility
func generateWingRidersPoolId(policyA, nameA, policyB, nameB []byte) string {
	return wingriders.GeneratePoolId(policyA, nameA, policyB, nameB)
}

// GetWingRidersV2PoolAddresses returns mainnet V2 pool addresses
func GetWingRidersV2PoolAddresses() []string {
	return wingriders.GetV2PoolAddresses()
}
