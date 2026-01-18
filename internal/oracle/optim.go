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

	"github.com/blinklabs-io/shai/internal/oracle/optim"
)

// Re-export constants for backward compatibility
const (
	OptimProtocolName      = optim.ProtocolName
	OptimBondStatusActive  = optim.BondStatusActive
	OptimBondStatusMatured = optim.BondStatusMatured
	OptimBondStatusClaimed = optim.BondStatusClaimed
	OptimInterestRateDenom = optim.InterestRateDenom
)

// Re-export types for backward compatibility
type (
	OptimBondDatum            = optim.BondDatum
	OptimAddress              = optim.Address
	OptimCredential           = optim.Credential
	OptimCredentialType       = optim.CredentialType
	OptimMaybeStakeCredential = optim.MaybeStakeCredential
	OptimStakeCredential      = optim.StakeCredential
	OptimOADADatum            = optim.OADADatum
	OptimRational             = optim.Rational
	OptimBondState            = optim.BondState
	OptimOADAState            = optim.OADAState
)

// Re-export credential type constants
const (
	OptimCredentialTypeVerificationKey = optim.CredentialTypeVerificationKey
	OptimCredentialTypeScript          = optim.CredentialTypeScript
)

// OptimParser wraps optim.Parser for backward compatibility
type OptimParser struct {
	parser *optim.Parser
}

// NewOptimParser creates a parser for Optim Finance protocol
func NewOptimParser() *OptimParser {
	return &OptimParser{parser: optim.NewParser()}
}

// Protocol returns the protocol name
func (p *OptimParser) Protocol() string {
	return p.parser.Protocol()
}

// ParseBondDatum parses an Optim bond datum and returns the bond state
func (p *OptimParser) ParseBondDatum(
	datum []byte,
	txHash string,
	txIndex uint32,
	slot uint64,
	timestamp time.Time,
) (*OptimBondState, error) {
	return p.parser.ParseBondDatum(datum, txHash, txIndex, slot, timestamp)
}

// ParseOADADatum parses an OADA datum and returns the state
func (p *OptimParser) ParseOADADatum(
	datum []byte,
	txHash string,
	txIndex uint32,
	slot uint64,
	timestamp time.Time,
) (*OptimOADAState, error) {
	return p.parser.ParseOADADatum(datum, txHash, txIndex, slot, timestamp)
}

// ParsePoolDatum implements PoolParser interface for compatibility
// Optim is a bonds/staking derivatives protocol, not an AMM, so this returns
// nil
func (p *OptimParser) ParsePoolDatum(
	datum []byte,
	txHash string,
	txIndex uint32,
	slot uint64,
	timestamp time.Time,
) (*PoolState, error) {
	// Optim doesn't have AMM pools, return nil
	return nil, nil
}

// generateOptimBondId wraps optim.GenerateBondId for backward compatibility
func generateOptimBondId(bondNFT []byte) string {
	return optim.GenerateBondId(bondNFT)
}

// GetOptimAddresses returns known Optim Finance contract addresses
func GetOptimAddresses() []string {
	return optim.GetAddresses()
}

// GetOptimBondContractAddress returns the bond contract address
func GetOptimBondContractAddress() string {
	return optim.GetBondContractAddress()
}

// GetOptimOADAContractAddress returns the OADA contract address
func GetOptimOADAContractAddress() string {
	return optim.GetOADAContractAddress()
}
