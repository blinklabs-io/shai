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

package butane

import (
	"encoding/hex"
	"fmt"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/shai/common"
)

// CDPState represents the parsed state of a CDP position
type CDPState struct {
	CDPId          string
	Owner          string
	Synthetic      common.AssetClass
	MintedAmount   uint64
	StartTime      time.Time
	Slot           uint64
	TxHash         string
	TxIndex        uint32
	Timestamp      time.Time
	CollateralUtxo string // Reference to collateral UTxO
}

// Key returns a unique key for this CDP
func (c *CDPState) Key() string {
	return fmt.Sprintf("butane:%s", c.CDPId)
}

// SlotNumber returns the chain slot for this CDP state.
func (c *CDPState) SlotNumber() uint64 {
	return c.Slot
}

// Parser implements parsing for Butane protocol
type Parser struct{}

// NewParser creates a parser for Butane protocol
func NewParser() *Parser {
	return &Parser{}
}

// Protocol returns the protocol name
func (p *Parser) Protocol() string {
	return ProtocolName
}

// ParseMonoDatum parses a Butane MonoDatum and returns the CDP if present
func (p *Parser) ParseMonoDatum(
	datum []byte,
	txHash string,
	txIndex uint32,
	slot uint64,
	timestamp time.Time,
) (*CDPState, error) {
	var monoDatum MonoDatum
	if _, err := cbor.Decode(datum, &monoDatum); err != nil {
		return nil, fmt.Errorf("failed to decode Butane datum: %w", err)
	}

	// Only process CDP datums (constructor 1)
	if monoDatum.Constructor != 1 || monoDatum.CDP == nil {
		return nil, nil // Not a CDP datum
	}

	cdp := monoDatum.CDP

	// Generate CDP ID
	cdpId := GenerateCDPId(txHash, txIndex)

	// Get owner identifier
	owner := ""
	if cdp.Owner.Type == 0 && len(cdp.Owner.PubKey) > 0 {
		owner = hex.EncodeToString(cdp.Owner.PubKey)
	}

	state := &CDPState{
		CDPId:        cdpId,
		Owner:        owner,
		Synthetic:    cdp.Synthetic.ToCommonAssetClass(),
		MintedAmount: cdp.Minted,
		StartTime:    time.UnixMilli(cdp.StartTime),
		Slot:         slot,
		TxHash:       txHash,
		TxIndex:      txIndex,
		Timestamp:    timestamp,
	}

	return state, nil
}

// GenerateCDPId generates a unique CDP ID
func GenerateCDPId(txHash string, txIndex uint32) string {
	return fmt.Sprintf("butane_cdp_%s#%d", txHash, txIndex)
}

// GetCDPAddresses returns deployed mainnet Butane CDP addresses.
// Source: butaneprotocol/butane-deployments butane-v1-deployed.json.
func GetCDPAddresses() []string {
	return []string{
		CDPContractAddress,
	}
}
