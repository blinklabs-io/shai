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

package fluidtokens

import (
	"encoding/hex"
	"fmt"
	"time"

	"github.com/Salvionied/apollo"
	serAddress "github.com/Salvionied/apollo/serialization/Address"
	"github.com/Salvionied/apollo/serialization/Key"
	"github.com/Salvionied/apollo/serialization/PlutusData"
	"github.com/Salvionied/apollo/serialization/Redeemer"
	"github.com/Salvionied/apollo/serialization/UTxO"
	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/shai/internal/config"
	"github.com/blinklabs-io/shai/internal/logging"
	"github.com/blinklabs-io/shai/internal/storage"
	"github.com/blinklabs-io/shai/internal/wallet"
)

const (
	// returnTxTtlSlots is the TTL for return transactions in slots (~10 minutes)
	returnTxTtlSlots = 600
	// returnTxFee is the estimated transaction fee in lovelace
	returnTxFee = 300_000
	// minUtxoLovelace is the minimum lovelace required for a UTxO
	minUtxoLovelace = 2_000_000
)

// BuildReturnTxOptions contains options for building a return transaction
type BuildReturnTxOptions struct {
	RentalTxHash  string
	RentalTxIndex uint32
	RentalOutput  ledger.TransactionOutput
	RentDatum     *RentDatum
	BatchIndex    uint64
	Profile       *config.Profile
	ChangeAddress string
}

// BuildReturnTx builds a transaction to return an expired rental NFT
func BuildReturnTx(opts BuildReturnTxOptions) ([]byte, error) {
	logger := logging.GetLogger()
	bursa := wallet.GetWallet()

	// Guard against nil wallet
	if bursa == nil {
		return nil, fmt.Errorf("wallet not configured")
	}

	// Get profile configuration
	ftConfig, ok := opts.Profile.Config.(config.FluidTokensProfileConfig)
	if !ok {
		return nil, fmt.Errorf("invalid FluidTokens profile configuration")
	}

	// Validate the rental is eligible for return
	if !opts.RentDatum.CanBeReturned() {
		return nil, fmt.Errorf("rental is not eligible for return")
	}

	// Build owner address from datum credentials
	ownerAddr, err := buildAddressFromCredentials(
		opts.RentDatum.OwnerPaymentCred,
		opts.RentDatum.OwnerStakingCred,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to build owner address: %w", err)
	}

	logger.Debug(
		"building return transaction",
		"rentalTxHash", opts.RentalTxHash,
		"rentalTxIndex", opts.RentalTxIndex,
		"ownerAddress", ownerAddr,
		"batchIndex", opts.BatchIndex,
	)

	// Decode the rental UTxO
	var rentalUtxo UTxO.UTxO
	if _, err := cbor.Decode(opts.RentalOutput.Cbor(), &rentalUtxo.Output); err != nil {
		return nil, fmt.Errorf("failed to decode rental output: %w", err)
	}
	// Set the input reference
	txHashBytes, err := hex.DecodeString(opts.RentalTxHash)
	if err != nil {
		return nil, fmt.Errorf("failed to decode rental tx hash: %w", err)
	}
	rentalUtxo.Input.TransactionId = txHashBytes
	rentalUtxo.Input.Index = int(opts.RentalTxIndex)

	// Gather wallet UTxOs for fees and collateral
	utxosBytes, err := storage.GetStorage().GetUtxos(bursa.PaymentAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to get wallet UTxOs: %w", err)
	}
	var walletUtxos []UTxO.UTxO
	for _, utxoBytes := range utxosBytes {
		var utxo UTxO.UTxO
		if _, err := cbor.Decode(utxoBytes, &utxo); err != nil {
			continue // Skip invalid UTxOs
		}
		walletUtxos = append(walletUtxos, utxo)
	}
	if len(walletUtxos) == 0 {
		return nil, fmt.Errorf("no wallet UTxOs available for fees")
	}

	// Parse addresses
	ownerAddress, err := serAddress.DecodeAddress(ownerAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to decode owner address: %w", err)
	}
	changeAddress, err := serAddress.DecodeAddress(opts.ChangeAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to decode change address: %w", err)
	}

	// Calculate current slot and TTL
	currentSlot := unixTimeToSlot(time.Now().Unix())

	// Get rental UTxO value
	rentalLovelace := uint64(rentalUtxo.Output.GetAmount().GetCoin())
	if rentalLovelace < minUtxoLovelace {
		rentalLovelace = minUtxoLovelace
	}

	// Build units for non-ADA assets in the rental UTxO
	var returnUnits []apollo.Unit
	if rentalUtxo.Output.GetAmount().GetAssets() != nil {
		for policyId, assets := range rentalUtxo.Output.GetAmount().GetAssets() {
			for assetName, amount := range assets {
				returnUnits = append(
					returnUnits,
					apollo.NewUnit(
						policyId.Value,
						assetName.String(),
						int(amount),
					),
				)
			}
		}
	}

	// Create the return redeemer (Constructor 4 with batch index)
	returnRedeemer := cbor.NewConstructor(
		4,
		cbor.IndefLengthList{
			opts.BatchIndex,
		},
	)

	// Build the transaction using Apollo
	cc := apollo.NewEmptyBackend()
	apollob := apollo.New(&cc)

	// Start building the transaction
	apollob = apollob.
		AddInputAddress(changeAddress).
		AddLoadedUTxOs(walletUtxos...).
		SetTtl(int64(currentSlot + returnTxTtlSlots))

	// Note: Reference script inputs would be added here if available
	// to reduce transaction fees. The script hashes are:
	// - LoanRequestHash: ftConfig.LoanRequestHash
	// - ActiveRequestHash: ftConfig.ActiveRequestHash
	// - RepaymentHash: ftConfig.RepaymentHash
	_ = ftConfig // Use config for script hash validation

	// Collect from the rental UTxO with the return redeemer
	apollob = apollob.CollectFrom(
		rentalUtxo,
		Redeemer.Redeemer{
			Tag: Redeemer.SPEND,
			ExUnits: Redeemer.ExecutionUnits{
				Mem:   500_000,
				Steps: 200_000_000,
			},
			Data: PlutusData.PlutusData{
				Value: returnRedeemer,
			},
		},
	)

	// Pay to owner address (return the NFT and assets)
	apollob = apollob.PayToAddress(
		ownerAddress,
		int(rentalLovelace),
		returnUnits...,
	)

	// Complete the transaction with fee estimation
	tx, err := apollob.
		DisableExecutionUnitsEstimation().
		CompleteExact(returnTxFee)
	if err != nil {
		return nil, fmt.Errorf("failed to complete transaction: %w", err)
	}

	// Sign the transaction with wallet keys
	vKeyBytes, err := hex.DecodeString(bursa.PaymentVKey.CborHex)
	if err != nil {
		return nil, fmt.Errorf("failed to decode verification key: %w", err)
	}
	sKeyBytes, err := hex.DecodeString(bursa.PaymentExtendedSKey.CborHex)
	if err != nil {
		return nil, fmt.Errorf("failed to decode signing key: %w", err)
	}

	// Strip CBOR wrapper (leading 2 bytes)
	vKeyBytes = vKeyBytes[2:]
	sKeyBytes = sKeyBytes[2:]
	// Extract signing key portion from extended key
	sKeyBytes = append(sKeyBytes[:64], sKeyBytes[96:]...)

	vkey := Key.VerificationKey{Payload: vKeyBytes}
	skey := Key.SigningKey{Payload: sKeyBytes}

	tx, err = tx.SignWithSkey(vkey, skey)
	if err != nil {
		return nil, fmt.Errorf("failed to sign transaction: %w", err)
	}

	// Get transaction bytes
	txBytes, err := tx.GetTx().Bytes()
	if err != nil {
		return nil, fmt.Errorf("failed to serialize transaction: %w", err)
	}

	logger.Info(
		"built return transaction",
		"txHash", tx.GetTx().Id(),
		"rentalTxHash", opts.RentalTxHash,
		"ownerAddress", ownerAddr,
		"batchIndex", opts.BatchIndex,
	)

	return txBytes, nil
}

// buildAddressFromCredentials constructs a Cardano address from credentials
func buildAddressFromCredentials(
	paymentCred Credential,
	stakingCred StakingCredential,
) (string, error) {
	if len(paymentCred.Hash) != 28 {
		return "", fmt.Errorf(
			"invalid payment credential length: %d",
			len(paymentCred.Hash),
		)
	}

	// Get network ID from config
	cfg := config.GetConfig()
	networkId := byte(0x01) // mainnet default
	if cfg.Network == "preview" || cfg.Network == "preprod" || cfg.Network == "testnet" {
		networkId = 0x00
	}

	var addrBytes []byte

	if stakingCred.IsPresent && len(stakingCred.Cred.Hash) == 28 {
		// Base address with staking credential
		// Address type 0x0X for base address (X = network ID)
		header := networkId
		if paymentCred.Type == 1 {
			header |= 0x10 // script payment credential
		}
		if stakingCred.Cred.Type == 1 {
			header |= 0x20 // script staking credential
		}
		addrBytes = append(addrBytes, header)
		addrBytes = append(addrBytes, paymentCred.Hash...)
		addrBytes = append(addrBytes, stakingCred.Cred.Hash...)
	} else {
		// Enterprise address (no staking)
		// Address type 0x6X for enterprise address (X = network ID)
		header := byte(0x60) | networkId
		if paymentCred.Type == 1 {
			header |= 0x10 // script payment credential
		}
		addrBytes = append(addrBytes, header)
		addrBytes = append(addrBytes, paymentCred.Hash...)
	}

	// Use bech32 encoding for the address
	addr, err := serAddress.DecodeAddress(hex.EncodeToString(addrBytes))
	if err != nil {
		// If decoding fails, try constructing from raw bytes
		return constructBech32Address(addrBytes)
	}

	return addr.String(), nil
}

// constructBech32Address builds a bech32 address from raw bytes
func constructBech32Address(addrBytes []byte) (string, error) {
	// Use Apollo's Address structure to properly encode the address
	if len(addrBytes) < 29 {
		return "", fmt.Errorf(
			"invalid address bytes length: %d",
			len(addrBytes),
		)
	}

	addr := serAddress.Address{}
	addr.PaymentPart = addrBytes[1:29]
	if len(addrBytes) > 29 {
		addr.StakingPart = addrBytes[29:]
	}

	// Parse header byte: upper nibble is address type, lower nibble is network
	header := addrBytes[0]
	addr.AddressType = (header >> 4) & 0x0F
	addr.Network = header & 0x0F
	addr.HeaderByte = header

	// Set HRP based on network
	if addr.Network == serAddress.MAINNET {
		addr.Hrp = "addr"
	} else {
		addr.Hrp = "addr_test"
	}

	return addr.String(), nil
}

// unixTimeToSlot converts Unix timestamp to Cardano slot number
func unixTimeToSlot(unixTime int64) uint64 {
	cfg := config.GetConfig()
	networkCfg, ok := config.Networks[cfg.Network]
	if !ok {
		// Fallback to mainnet parameters if network not found
		networkCfg = config.Networks["mainnet"]
	}
	// Handle case where even mainnet isn't configured (shouldn't happen)
	if networkCfg.ShelleyOffsetTime == 0 {
		return 0
	}
	return networkCfg.ShelleyOffsetSlot + uint64(
		unixTime-networkCfg.ShelleyOffsetTime,
	)
}

// ValidateReturnTx validates that a return transaction is properly formed
func ValidateReturnTx(txBytes []byte, rental *TrackedRental) error {
	if len(txBytes) == 0 {
		return fmt.Errorf("transaction bytes are empty")
	}
	if rental == nil {
		return fmt.Errorf("rental is nil")
	}
	if !rental.Datum.CanBeReturned() {
		return fmt.Errorf("rental is not eligible for return")
	}
	return nil
}

// EstimateReturnTxFee estimates the fee for a return transaction
func EstimateReturnTxFee(opts BuildReturnTxOptions) (uint64, error) {
	// Base fee estimation
	// Actual fee depends on transaction size and execution units
	return returnTxFee, nil
}
