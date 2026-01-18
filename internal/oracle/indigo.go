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

	"github.com/blinklabs-io/shai/internal/oracle/indigo"
)

// Re-export constants for backward compatibility
const (
	IndigoProtocolName       = indigo.ProtocolName
	IndigoCDPContractAddress = indigo.CDPContractAddress
)

// Re-export types for backward compatibility
type (
	IndigoCDPContentDatum = indigo.CDPContentDatum
	IndigoCDPInner        = indigo.CDPInner
	IndigoMaybePubKeyHash = indigo.MaybePubKeyHash
	IndigoAccumulatedFees = indigo.AccumulatedFees
	IndigoCDPState        = indigo.CDPState
)

// IndigoParser wraps indigo.Parser for backward compatibility
type IndigoParser struct {
	parser *indigo.Parser
}

// NewIndigoParser creates a parser for Indigo protocol
func NewIndigoParser() *IndigoParser {
	return &IndigoParser{parser: indigo.NewParser()}
}

// Protocol returns the protocol name
func (p *IndigoParser) Protocol() string {
	return p.parser.Protocol()
}

// ParseCDPDatum parses an Indigo CDP datum and returns the CDP state
func (p *IndigoParser) ParseCDPDatum(
	datum []byte,
	txHash string,
	txIndex uint32,
	slot uint64,
	timestamp time.Time,
) (*IndigoCDPState, error) {
	return p.parser.ParseCDPDatum(datum, txHash, txIndex, slot, timestamp)
}

// ParsePoolDatum implements PoolParser interface for compatibility
// Indigo is a synthetics/CDP protocol, not an AMM, so this returns nil
func (p *IndigoParser) ParsePoolDatum(
	datum []byte,
	txHash string,
	txIndex uint32,
	slot uint64,
	timestamp time.Time,
) (*PoolState, error) {
	// Indigo doesn't have AMM pools, return nil
	return nil, nil
}

// generateIndigoCDPId wraps indigo.GenerateCDPId for backward compatibility
func generateIndigoCDPId(txHash string, txIndex uint32) string {
	return indigo.GenerateCDPId(txHash, txIndex)
}

// GetIndigoAddresses returns known Indigo contract addresses
func GetIndigoAddresses() []string {
	return indigo.GetAddresses()
}
