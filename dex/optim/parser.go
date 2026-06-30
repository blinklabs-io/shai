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

package optim

import (
	"encoding/hex"
	"fmt"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
)

// BondState represents the parsed state of an Optim Finance liquidity bond
type BondState struct {
	BondId       string    // Unique identifier for this bond
	Protocol     string    // Protocol name (optim)
	Principal    uint64    // ADA principal locked (in lovelace)
	InterestRate uint64    // Interest rate in basis points
	Duration     uint64    // Bond duration in epochs or slots
	StartEpoch   uint64    // When bond started
	EndEpoch     uint64    // When bond matures
	StakePool    string    // Pool ID for staking (hex encoded)
	IsMatured    bool      // Whether the bond has matured
	CanClaim     bool      // Whether the bond can be claimed
	Lender       string    // Lender's credential hash (hex encoded)
	LenderIsKey  bool      // True if lender is a pubkey, false if script
	BorrowerNFT  string    // Borrower's NFT identifier (hex encoded)
	Rewards      uint64    // Accrued staking rewards
	Status       uint64    // Bond status (0=active, 1=matured, 2=claimed)
	Slot         uint64    // Slot when this state was observed
	BlockHash    string    // Block hash
	TxHash       string    // Transaction hash
	TxIndex      uint32    // Transaction output index
	Timestamp    time.Time // Block timestamp
}

// Key returns a unique key for this bond
func (b *BondState) Key() string {
	return fmt.Sprintf("optim:%s", b.BondId)
}

// StatusString returns a human-readable status
func (b *BondState) StatusString() string {
	switch b.Status {
	case BondStatusActive:
		return "active"
	case BondStatusMatured:
		return "matured"
	case BondStatusClaimed:
		return "claimed"
	default:
		return fmt.Sprintf("unknown(%d)", b.Status)
	}
}

// InterestRatePercent returns the interest rate as a percentage
func (b *BondState) InterestRatePercent() float64 {
	return float64(b.InterestRate) / float64(InterestRateDenom) * 100
}

// PrincipalADA returns the principal amount in ADA (not lovelace)
func (b *BondState) PrincipalADA() float64 {
	return float64(b.Principal) / 1000000
}

// RewardsADA returns the accrued rewards in ADA
func (b *BondState) RewardsADA() float64 {
	return float64(b.Rewards) / 1000000
}

// TotalValueADA returns principal plus rewards in ADA
func (b *BondState) TotalValueADA() float64 {
	return b.PrincipalADA() + b.RewardsADA()
}

// String returns a human-readable representation
func (b *BondState) String() string {
	bondIdShort := b.BondId
	if len(bondIdShort) > 20 {
		bondIdShort = bondIdShort[:20] + "..."
	}
	return fmt.Sprintf(
		"Bond[%s] status=%s principal=%.2f ADA rate=%.2f%% rewards=%.2f ADA",
		bondIdShort,
		b.StatusString(),
		b.PrincipalADA(),
		b.InterestRatePercent(),
		b.RewardsADA(),
	)
}

// OADAState represents the state of the OADA staking derivative
type OADAState struct {
	TotalStaked     uint64    // Total ADA staked in lovelace
	TotalOADA       uint64    // Total OADA tokens minted
	ExchangeRate    float64   // Current OADA to ADA exchange rate
	LastUpdateEpoch uint64    // Last update epoch
	Slot            uint64    // Slot when state was observed
	TxHash          string    // Transaction hash
	TxIndex         uint32    // Transaction output index
	Timestamp       time.Time // Block timestamp
}

// Key returns a unique key for OADA state
func (o *OADAState) Key() string {
	return "optim:oada"
}

// TotalStakedADA returns total staked in ADA
func (o *OADAState) TotalStakedADA() float64 {
	return float64(o.TotalStaked) / 1000000
}

// Parser implements parsing for Optim Finance protocol
type Parser struct{}

// NewParser creates a parser for Optim Finance protocol
func NewParser() *Parser {
	return &Parser{}
}

// Protocol returns the protocol name
func (p *Parser) Protocol() string {
	return ProtocolName
}

// ParseBondDatum parses an Optim bond datum and returns the bond state
func (p *Parser) ParseBondDatum(
	datum []byte,
	txHash string,
	txIndex uint32,
	slot uint64,
	timestamp time.Time,
) (*BondState, error) {
	var bondDatum BondDatum
	if _, err := cbor.Decode(datum, &bondDatum); err != nil {
		return nil, fmt.Errorf("failed to decode Optim bond datum: %w", err)
	}

	// Check if we have a valid bond NFT (indicates successful parse)
	if len(bondDatum.BondNFT) == 0 {
		return nil, nil // Not a bond datum
	}

	// Generate bond ID from the bond NFT
	bondId := GenerateBondId(bondDatum.BondNFT)

	// Extract lender credential
	lender := hex.EncodeToString(bondDatum.LenderAddress.PaymentCredential.Hash)
	lenderIsKey := bondDatum.LenderAddress.PaymentCredential.Type ==
		CredentialTypeVerificationKey

	state := &BondState{
		BondId:       bondId,
		Protocol:     p.Protocol(),
		Principal:    bondDatum.PrincipalAmount,
		InterestRate: bondDatum.InterestRate,
		Duration:     bondDatum.Duration,
		StartEpoch:   bondDatum.StartEpoch,
		EndEpoch:     bondDatum.EndEpoch,
		StakePool:    hex.EncodeToString(bondDatum.StakePool),
		IsMatured:    bondDatum.IsMatured(),
		CanClaim:     bondDatum.IsMatured() || bondDatum.IsClaimed(),
		Lender:       lender,
		LenderIsKey:  lenderIsKey,
		BorrowerNFT:  hex.EncodeToString(bondDatum.BorrowerNFT),
		Rewards:      bondDatum.AccruedRewards,
		Status:       bondDatum.Status,
		Slot:         slot,
		TxHash:       txHash,
		TxIndex:      txIndex,
		Timestamp:    timestamp,
	}

	return state, nil
}

// ParseOADADatum parses an OADA datum and returns the state
func (p *Parser) ParseOADADatum(
	datum []byte,
	txHash string,
	txIndex uint32,
	slot uint64,
	timestamp time.Time,
) (*OADAState, error) {
	var oadaDatum OADADatum
	if _, err := cbor.Decode(datum, &oadaDatum); err != nil {
		return nil, fmt.Errorf("failed to decode OADA datum: %w", err)
	}

	// Check if we have valid data
	if oadaDatum.TotalStaked == 0 && oadaDatum.TotalOADA == 0 {
		return nil, nil // Not an OADA datum or empty
	}

	state := &OADAState{
		TotalStaked:     oadaDatum.TotalStaked,
		TotalOADA:       oadaDatum.TotalOADA,
		ExchangeRate:    oadaDatum.ExchangeRateFloat(),
		LastUpdateEpoch: oadaDatum.LastUpdateEpoch,
		Slot:            slot,
		TxHash:          txHash,
		TxIndex:         txIndex,
		Timestamp:       timestamp,
	}

	return state, nil
}

// GenerateBondId generates a unique bond ID from the bond NFT
func GenerateBondId(bondNFT []byte) string {
	nftHex := hex.EncodeToString(bondNFT)
	if len(nftHex) > 32 {
		nftHex = nftHex[:32]
	}
	return fmt.Sprintf("optim_bond_%s", nftHex)
}

// GetAddresses returns known Optim Finance contract addresses
func GetAddresses() []string {
	return []string{
		// Optim Finance contract addresses (mainnet)
		// Bond and OADA contract script addresses are not publicly documented.
		// They need to be obtained from:
		// - Inspecting on-chain transactions on Cardano explorers
		// - Optim Finance GitHub (github.com/OptimFinance)
		// - Direct contact with the Optim Finance team
		//
		// Token policy IDs (verified):
		// - OADA: f6099832f9563e4cf59602b3351c3c5a8a7dda2d44575ef69b82cf8d
		// - OPTIM: e52964af4fffdb54504859875b1827b60ba679074996156461143dc1
	}
}

// GetBondContractAddress returns the bond contract address (placeholder)
func GetBondContractAddress() string {
	// Placeholder - to be filled with actual Optim bond contract address
	return ""
}

// GetOADAContractAddress returns the OADA contract address (placeholder)
func GetOADAContractAddress() string {
	// Placeholder - to be filled with actual OADA contract address
	return ""
}
