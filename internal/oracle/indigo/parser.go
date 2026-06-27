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

package indigo

import (
	"encoding/hex"
	"fmt"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
)

// CDPState represents the parsed state of an Indigo CDP position
type CDPState struct {
	CDPId        string    // Unique identifier for this CDP
	Owner        string    // Owner's public key hash (hex) or empty if Nothing
	HasOwner     bool      // Whether the CDP has an owner set
	IAsset       string    // The iAsset identifier (hex encoded)
	MintedAmount int64     // Amount of iAsset minted
	Slot         uint64    // Slot when this state was observed
	TxHash       string    // Transaction hash
	TxIndex      uint32    // Transaction output index
	Timestamp    time.Time // Block timestamp

	// Accumulated fees information
	FeesType    int   // 0 = InterestIAssetAmount, 1 = FeesLovelacesAmount
	LastUpdated int64 // For InterestIAssetAmount: last update time
	IAssetFees  int64 // For InterestIAssetAmount: accumulated iAsset fees
	Treasury    int64 // For FeesLovelacesAmount: treasury amount
	IndyStakers int64 // For FeesLovelacesAmount: INDY stakers amount
}

// Key returns a unique key for this CDP
func (c *CDPState) Key() string {
	return fmt.Sprintf("indigo:%s", c.CDPId)
}

// IAssetName returns a human-readable name for common iAssets
func (c *CDPState) IAssetName() string {
	// Common Indigo iAsset identifiers
	switch c.IAsset {
	case hex.EncodeToString([]byte("iUSD")):
		return "iUSD"
	case hex.EncodeToString([]byte("iBTC")):
		return "iBTC"
	case hex.EncodeToString([]byte("iETH")):
		return "iETH"
	default:
		// Try to decode as ASCII if it looks like a readable name
		bytes, err := hex.DecodeString(c.IAsset)
		if err == nil && isPrintable(bytes) {
			return string(bytes)
		}
		return c.IAsset
	}
}

// isPrintable checks if all bytes are printable ASCII
func isPrintable(data []byte) bool {
	for _, b := range data {
		if b < 32 || b > 126 {
			return false
		}
	}
	return len(data) > 0
}

// Parser implements parsing for Indigo protocol CDPs
type Parser struct{}

// NewParser creates a parser for Indigo protocol
func NewParser() *Parser {
	return &Parser{}
}

// Protocol returns the protocol name
func (p *Parser) Protocol() string {
	return ProtocolName
}

// ParseCDPDatum parses an Indigo CDP datum and returns the CDP state
func (p *Parser) ParseCDPDatum(
	datum []byte,
	txHash string,
	txIndex uint32,
	slot uint64,
	timestamp time.Time,
) (*CDPState, error) {
	var cdpContent CDPContentDatum
	if _, err := cbor.Decode(datum, &cdpContent); err != nil {
		return nil, fmt.Errorf("failed to decode Indigo CDP datum: %w", err)
	}

	// Check if parsing succeeded
	if cdpContent.Inner == nil {
		return nil, nil // Not a valid CDP content datum
	}

	inner := cdpContent.Inner

	// Generate CDP ID
	cdpId := GenerateCDPId(txHash, txIndex)

	// Extract owner information
	owner := ""
	hasOwner := inner.Owner.IsJust
	if hasOwner && len(inner.Owner.Hash) > 0 {
		owner = hex.EncodeToString(inner.Owner.Hash)
	}

	state := &CDPState{
		CDPId:        cdpId,
		Owner:        owner,
		HasOwner:     hasOwner,
		IAsset:       hex.EncodeToString(inner.IAsset),
		MintedAmount: inner.MintedAmount,
		Slot:         slot,
		TxHash:       txHash,
		TxIndex:      txIndex,
		Timestamp:    timestamp,
		FeesType:     inner.AccumulatedFees.Type,
	}

	// Copy fee details based on type
	if inner.AccumulatedFees.Type == 0 {
		state.LastUpdated = inner.AccumulatedFees.LastUpdated
		state.IAssetFees = inner.AccumulatedFees.IAssetAmount
	} else {
		state.Treasury = inner.AccumulatedFees.Treasury
		state.IndyStakers = inner.AccumulatedFees.IndyStakers
	}

	return state, nil
}

// GenerateCDPId generates a unique CDP ID from transaction information
func GenerateCDPId(txHash string, txIndex uint32) string {
	if len(txHash) > 16 {
		return fmt.Sprintf("indigo_cdp_%s#%d", txHash[:16], txIndex)
	}
	return fmt.Sprintf("indigo_cdp_%s#%d", txHash, txIndex)
}

// GetAddresses returns known Indigo contract addresses
func GetAddresses() []string {
	return []string{
		CDPContractAddress,
		StabilityPoolAddress,
	}
}

// GetPolicyIDs returns known Indigo policy IDs
func GetPolicyIDs() map[string]string {
	return map[string]string{
		"iAsset": IAssetPolicyID,
		"INDY":   INDYPolicyID,
	}
}
