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

	"github.com/blinklabs-io/shai/internal/common"
	"github.com/blinklabs-io/shai/internal/oracle/vyfi"
)

// Re-export constants for backward compatibility
const (
	VyFiProtocolName   = vyfi.ProtocolName
	VyFiPoolScriptHash = vyfi.PoolScriptHash
	VyFiLPTokenPolicy  = vyfi.LPTokenPolicy
)

// Re-export types for backward compatibility
type (
	VyFiPoolDatum    = vyfi.PoolDatum
	VyFiOrderDatum   = vyfi.OrderDatum
	VyFiOrderDetails = vyfi.OrderDetails
	VyFiOrderType    = vyfi.OrderType
)

// Re-export order type constants
const (
	VyFiOrderTypeAddLiquidity    = vyfi.OrderTypeAddLiquidity
	VyFiOrderTypeRemoveLiquidity = vyfi.OrderTypeRemoveLiquidity
	VyFiOrderTypeServe           = vyfi.OrderTypeServe
	VyFiOrderTypeTradeAToB       = vyfi.OrderTypeTradeAToB
	VyFiOrderTypeTradeBToA       = vyfi.OrderTypeTradeBToA
	VyFiOrderTypeZapInA          = vyfi.OrderTypeZapInA
	VyFiOrderTypeZapInB          = vyfi.OrderTypeZapInB
)

// VyFiParser wraps vyfi.Parser for backward compatibility
type VyFiParser struct {
	parser *vyfi.Parser
}

// NewVyFiParser creates a parser for VyFi pools
func NewVyFiParser() *VyFiParser {
	return &VyFiParser{parser: vyfi.NewParser()}
}

// Protocol returns the protocol name
func (p *VyFiParser) Protocol() string {
	return p.parser.Protocol()
}

// ParsePoolDatum parses a VyFi pool datum
func (p *VyFiParser) ParsePoolDatum(
	datum []byte,
	txHash string,
	txIndex uint32,
	slot uint64,
	timestamp time.Time,
	assetA common.AssetClass,
	assetB common.AssetClass,
	poolNFT string,
) (*PoolState, error) {
	state, err := p.parser.ParsePoolDatum(
		datum,
		txHash,
		txIndex,
		slot,
		timestamp,
		assetA,
		assetB,
		poolNFT,
	)
	if err != nil {
		return nil, err
	}
	// Convert vyfi.PoolState to oracle.PoolState
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

// ParsePoolDatumSimple parses just the pool datum without asset context
func (p *VyFiParser) ParsePoolDatumSimple(
	datum []byte,
) (*VyFiPoolDatum, error) {
	return p.parser.ParsePoolDatumSimple(datum)
}

// generateVyFiPoolId wraps vyfi.GeneratePoolId for backward compatibility
func generateVyFiPoolId(
	poolNFT string,
	assetA common.AssetClass,
	assetB common.AssetClass,
) string {
	return vyfi.GeneratePoolId(poolNFT, assetA, assetB)
}

// GetVyFiPoolAddresses returns mainnet pool addresses
func GetVyFiPoolAddresses() []string {
	return vyfi.GetPoolAddresses()
}
