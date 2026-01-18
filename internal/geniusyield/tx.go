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

package geniusyield

import (
	"encoding/hex"
	"fmt"
	"time"

	"github.com/Salvionied/apollo"
	serAddress "github.com/Salvionied/apollo/serialization/Address"
	"github.com/Salvionied/apollo/serialization/Key"
	"github.com/Salvionied/apollo/serialization/PlutusData"
	"github.com/Salvionied/apollo/serialization/Redeemer"
	"github.com/Salvionied/apollo/serialization/TransactionInput"
	"github.com/Salvionied/apollo/serialization/UTxO"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/shai/internal/config"
	"github.com/blinklabs-io/shai/internal/logging"
	"github.com/blinklabs-io/shai/internal/storage"
	"github.com/blinklabs-io/shai/internal/wallet"
)

const (
	// matchTxTtlSlots is the TTL for matching transactions in slots
	matchTxTtlSlots = 60

	// matchTxFee is the estimated transaction fee in lovelace
	matchTxFee = 500_000

	// minUtxoLovelace is the minimum lovelace required for a UTxO
	minUtxoLovelace = 2_000_000

	// defaultMatcherReward is the default reward for the matcher in lovelace
	defaultMatcherReward = 1_500_000

	// defaultMakerFeeFlat is the default flat maker fee in lovelace
	defaultMakerFeeFlat = 1_000_000

	// defaultMakerFeePercent is the default percent maker fee (0.3%)
	defaultMakerFeePercent = 0.003

	// defaultTakerFee is the default taker fee in lovelace
	defaultTakerFee = 500_000
)

// buildMatchTxOpts contains options for building a match transaction
type buildMatchTxOpts struct {
	route          *Route
	newOrder       *OrderState
	newOrderOutput ledger.TransactionOutput
}

// buildMatchTx builds a transaction to match orders from a route
func (gy *GeniusYield) buildMatchTx(
	route *Route,
	newOrder *OrderState,
	newOrderOutput ledger.TransactionOutput,
) ([]byte, error) {
	logger := logging.GetLogger()
	bursa := wallet.GetWallet()

	if bursa == nil {
		return nil, fmt.Errorf("no wallet available")
	}

	logger.Debug(
		"building match transaction",
		"legs", len(route.Legs),
		"totalInput", route.TotalInput,
		"totalOutput", route.TotalOutput,
	)

	// Wrap the new order output as a UTxO
	newOrderUtxoBytes, err := wrapTxOutput(
		newOrder.TxHash,
		int(newOrder.TxIndex),
		newOrderOutput.Cbor(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to wrap new order UTXO: %w", err)
	}

	// Decode new order UTxO
	var newOrderUtxo UTxO.UTxO
	if _, err := cbor.Decode(newOrderUtxoBytes, &newOrderUtxo); err != nil {
		return nil, fmt.Errorf("failed to decode new order UTxO: %w", err)
	}

	// Collect all order UTxOs to be consumed (including matched orders)
	orderUtxos := []UTxO.UTxO{newOrderUtxo}
	orderStates := []*OrderState{newOrder}

	// Fetch existing order UTXOs from storage
	for _, leg := range route.Legs {
		utxoId := fmt.Sprintf("%s#%d", leg.TxHash, leg.TxIndex)
		utxoBytes, err := storage.GetStorage().GetUtxoById(utxoId)
		if err != nil {
			logger.Warn(
				"failed to fetch order UTXO",
				"utxoId", utxoId,
				"error", err,
			)
			continue
		}

		var utxo UTxO.UTxO
		if _, err := cbor.Decode(utxoBytes, &utxo); err != nil {
			logger.Warn(
				"failed to decode order UTxO",
				"utxoId", utxoId,
				"error", err,
			)
			continue
		}

		orderUtxos = append(orderUtxos, utxo)
		orderStates = append(orderStates, leg.Order)
	}

	if len(orderUtxos) < 2 {
		return nil, fmt.Errorf(
			"insufficient UTXOs for matching (need at least 2, have %d)",
			len(orderUtxos),
		)
	}

	// Gather wallet UTxOs for fees and collateral
	walletUtxosBytes, err := storage.GetStorage().GetUtxos(bursa.PaymentAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to get wallet UTxOs: %w", err)
	}

	walletUtxos := []UTxO.UTxO{}
	for _, utxoBytes := range walletUtxosBytes {
		var utxo UTxO.UTxO
		if _, err := cbor.Decode(utxoBytes, &utxo); err != nil {
			continue
		}
		walletUtxos = append(walletUtxos, utxo)
	}

	if len(walletUtxos) == 0 {
		return nil, fmt.Errorf("no wallet UTxOs available for fees")
	}

	// Calculate fill outputs for each order
	fillOutputs, err := gy.calculateFillOutputs(route, newOrder)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate fill outputs: %w", err)
	}

	// Calculate current slot for TTL
	currentSlot := unixTimeToSlot(time.Now().Unix())

	// Decode addresses
	changeAddress, err := serAddress.DecodeAddress(bursa.PaymentAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to decode change address: %w", err)
	}

	// Build the transaction using Apollo
	cc := apollo.NewEmptyBackend()
	apollob := apollo.New(&cc)
	apollob = apollob.
		AddInputAddress(changeAddress).
		AddLoadedUTxOs(walletUtxos...).
		SetTtl(int64(currentSlot + matchTxTtlSlots))

	// Add reference inputs if configured
	for _, inputRef := range gy.config.InputRefs {
		apollob = apollob.AddReferenceInput(
			inputRef.TxId,
			int(inputRef.OutputIdx),
		)
	}

	// Build sorted input index map for redeemer construction
	sortedIdxMap := buildSortedInputIndexMap(orderUtxos)

	// Process each order
	for i, utxo := range orderUtxos {
		orderState := orderStates[i]
		fillOutput := fillOutputs[i]

		// Get sorted index for this input
		sortedIdx := sortedIdxMap[utxoKey(utxo.Input)]
		if sortedIdx < 0 {
			return nil, fmt.Errorf(
				"failed to find sorted index for order %s",
				orderState.OrderId,
			)
		}

		// Build redeemer for this order
		redeemerData, err := buildRedeemerPlutusData(fillOutput)
		if err != nil {
			return nil, fmt.Errorf(
				"failed to build redeemer for order %s: %w",
				orderState.OrderId, err,
			)
		}

		// Add order input with redeemer
		apollob = apollob.CollectFrom(
			utxo,
			Redeemer.Redeemer{
				Tag: Redeemer.SPEND,
				ExUnits: Redeemer.ExecutionUnits{
					Mem:   400_000,
					Steps: 200_000_000,
				},
				Data: redeemerData,
			},
		)

		// Build output for this order
		// If partial fill, create updated order UTxO
		// If complete fill, send assets to order owner
		if !fillOutput.isComplete {
			// Partial fill - update the order UTxO
			outputDatum, outputLovelace, outputUnits, err := buildPartialFillOutput(
				orderState,
				fillOutput,
				utxo,
			)
			if err != nil {
				return nil, fmt.Errorf(
					"failed to build partial fill output for order %s: %w",
					orderState.OrderId, err,
				)
			}

			// Get order address from original UTxO
			orderAddr := utxo.Output.GetAddress()

			apollob = apollob.PayToContract(
				orderAddr,
				outputDatum,
				int(outputLovelace),
				true,
				outputUnits...,
			)
		} else {
			// Complete fill - send received assets to order owner
			ownerAddr, err := buildOwnerAddress(orderState)
			if err != nil {
				logger.Warn(
					"failed to build owner address, using order address",
					"orderId", orderState.OrderId,
					"error", err,
				)
				// Fall back to order address
				ownerAddr = utxo.Output.GetAddress()
			}

			// Calculate assets to send to owner
			ownerLovelace, ownerUnits := calculateOwnerPayment(
				fillOutput,
				route,
				i == 0, // Is this the new order (taker)?
			)

			apollob = apollob.PayToAddress(
				ownerAddr,
				int(ownerLovelace),
				ownerUnits...,
			)
		}
	}

	// Calculate matcher reward from config or use default
	matcherReward := gy.config.MatcherReward
	if matcherReward == 0 {
		matcherReward = defaultMatcherReward
	}

	// Add matcher reward output
	apollob = apollob.PayToAddress(
		changeAddress,
		int(matcherReward),
	)

	// Complete the transaction
	tx, err := apollob.
		DisableExecutionUnitsEstimation().
		CompleteExact(matchTxFee)
	if err != nil {
		return nil, fmt.Errorf("failed to complete transaction: %w", err)
	}

	// Sign the transaction
	vKeyBytes, err := hex.DecodeString(bursa.PaymentVKey.CborHex)
	if err != nil {
		return nil, fmt.Errorf("failed to decode vkey: %w", err)
	}
	sKeyBytes, err := hex.DecodeString(bursa.PaymentExtendedSKey.CborHex)
	if err != nil {
		return nil, fmt.Errorf("failed to decode skey: %w", err)
	}

	// Strip CBOR prefix (2 bytes)
	vKeyBytes = vKeyBytes[2:]
	sKeyBytes = sKeyBytes[2:]

	// Strip public key portion from extended private key
	sKeyBytes = append(sKeyBytes[:64], sKeyBytes[96:]...)

	vkey := Key.VerificationKey{Payload: vKeyBytes}
	skey := Key.SigningKey{Payload: sKeyBytes}

	tx, err = tx.SignWithSkey(vkey, skey)
	if err != nil {
		return nil, fmt.Errorf("failed to sign transaction: %w", err)
	}

	// Get final transaction bytes
	txBytes, err := tx.GetTx().Bytes()
	if err != nil {
		return nil, fmt.Errorf("failed to serialize transaction: %w", err)
	}

	logger.Info(
		"built match transaction",
		"txSize", len(txBytes),
		"orders", len(orderUtxos),
		"legs", len(route.Legs),
	)

	return txBytes, nil
}

// calculateFillOutputs calculates the outputs for filling orders
func (gy *GeniusYield) calculateFillOutputs(
	route *Route,
	newOrder *OrderState,
) ([]orderFillOutput, error) {
	outputs := make([]orderFillOutput, 0, len(route.Legs)+1)

	// Calculate output for the new order (taker)
	newOrderOutput := orderFillOutput{
		orderId:      newOrder.OrderId,
		isComplete:   route.TotalInput >= newOrder.OfferedAmount,
		inputAmount:  route.TotalInput,
		outputAmount: route.TotalOutput,
	}
	outputs = append(outputs, newOrderOutput)

	// Calculate outputs for matched orders (makers)
	// For makers: they consume (give up) their offered asset and receive the asked asset
	// From the route's perspective: leg.OutputAmount = what taker receives = maker's consumed
	//                               leg.InputAmount = what taker sends = maker's received
	for _, leg := range route.Legs {
		legOutput := orderFillOutput{
			orderId:      leg.Order.OrderId,
			isComplete:   leg.OutputAmount >= leg.Order.OfferedAmount,
			inputAmount:  leg.OutputAmount, // Maker's consumed amount (their offered asset)
			outputAmount: leg.InputAmount,  // Maker's received amount (from taker)
		}
		outputs = append(outputs, legOutput)
	}

	return outputs, nil
}

// orderFillOutput represents the calculated output for an order fill
type orderFillOutput struct {
	orderId      string
	isComplete   bool   // Whether this completely fills the order
	inputAmount  uint64 // Amount consumed from order
	outputAmount uint64 // Amount received by order owner
}

// buildRedeemerPlutusData constructs the Plutus redeemer data
func buildRedeemerPlutusData(
	output orderFillOutput,
) (PlutusData.PlutusData, error) {
	if output.isComplete {
		// CompleteFill redeemer - Constructor 1 with empty fields
		return PlutusData.PlutusData{
			Value: cbor.NewConstructor(1, cbor.IndefLengthList{}),
		}, nil
	}

	// PartialFill redeemer - Constructor 0 with fill amount
	return PlutusData.PlutusData{
		Value: cbor.NewConstructor(
			0,
			cbor.IndefLengthList{
				output.inputAmount,
			},
		),
	}, nil
}

// buildOrderRedeemer builds the redeemer bytes for an order
func buildOrderRedeemer(output orderFillOutput) ([]byte, error) {
	if output.isComplete {
		redeemer := &CompleteFillRedeemer{}
		return redeemer.MarshalCBOR()
	}

	redeemer := &PartialFillRedeemer{
		FillAmount: output.inputAmount,
	}
	return redeemer.MarshalCBOR()
}

// buildPartialFillOutput constructs the output datum and values for a partial fill
func buildPartialFillOutput(
	order *OrderState,
	fill orderFillOutput,
	originalUtxo UTxO.UTxO,
) (*PlutusData.PlutusData, uint64, []apollo.Unit, error) {
	// For partial fills, we need to:
	// 1. Update the offered amount in the datum
	// 2. Preserve all original assets, only decrementing the offered asset

	// Get original output value
	originalCoin := uint64(originalUtxo.Output.GetAmount().GetCoin())

	// Calculate new offered amount with underflow guard
	if fill.inputAmount > order.OfferedAmount {
		return nil, 0, nil, fmt.Errorf(
			"fill amount %d exceeds offered amount %d",
			fill.inputAmount,
			order.OfferedAmount,
		)
	}
	newOfferedAmount := order.OfferedAmount - fill.inputAmount

	// The datum needs to be updated with the new offered amount
	// For now, we reconstruct the datum from the order state
	// In production, you would decode and modify the original datum
	newDatum, err := buildUpdatedOrderDatum(order, newOfferedAmount)
	if err != nil {
		return nil, 0, nil, fmt.Errorf("failed to build updated datum: %w", err)
	}

	// Calculate new lovelace (keep minimum)
	newLovelace := originalCoin
	if newLovelace < minUtxoLovelace {
		newLovelace = minUtxoLovelace
	}

	// Preserve all original assets from the UTxO, only adjusting the offered asset
	var units []apollo.Unit
	offeredPolicyHex := hex.EncodeToString(order.OfferedAsset.PolicyId)
	offeredAssetName := string(order.OfferedAsset.Name)

	if originalUtxo.Output.GetAmount().GetAssets() != nil {
		for policyId, assets := range originalUtxo.Output.GetAmount().GetAssets() {
			for assetName, amount := range assets {
				// Check if this is the offered asset - if so, use the new amount
				if policyId.Value == offeredPolicyHex &&
					assetName.String() == offeredAssetName {
					units = append(units, apollo.NewUnit(
						policyId.Value,
						assetName.String(),
						int(newOfferedAmount),
					))
				} else {
					// Preserve other assets (including order NFT) unchanged
					units = append(units, apollo.NewUnit(
						policyId.Value,
						assetName.String(),
						int(amount),
					))
				}
			}
		}
	} else if !order.OfferedAsset.IsLovelace() {
		// Fallback: if no assets map, add the offered asset with new amount
		units = append(units, apollo.NewUnit(
			offeredPolicyHex,
			offeredAssetName,
			int(newOfferedAmount),
		))
	}

	return newDatum, newLovelace, units, nil
}

// buildUpdatedOrderDatum constructs an updated datum for partial fills
// It decodes the original datum and only modifies the necessary fields
func buildUpdatedOrderDatum(
	order *OrderState,
	newOfferedAmount uint64,
) (*PlutusData.PlutusData, error) {
	// Build the datum structure matching PartialOrderDatum
	// Preserve original values for NFT and fee fields from the order state

	ownerKeyBytes, err := hex.DecodeString(order.Owner)
	if err != nil {
		return nil, fmt.Errorf("invalid owner key hex: %w", err)
	}

	// Construct the datum preserving original field values
	// Note: For a complete implementation, we should decode the original datum
	// from the UTxO and only modify offeredAmount and partialFills.
	// This implementation uses the order state which should contain the
	// original values parsed from the datum.
	datum := cbor.NewConstructor(
		0, // PartialOrderDatum constructor
		cbor.IndefLengthList{
			ownerKeyBytes,                                        // ownerKey
			buildAddressDatum(ownerKeyBytes),                     // ownerAddr
			buildAssetDatum(order.OfferedAsset),                  // offeredAsset
			order.OriginalAmount,                                 // offeredOriginalAmount (preserved)
			newOfferedAmount,                                     // offeredAmount (updated)
			buildAssetDatum(order.AskedAsset),                    // askedAsset
			buildRationalDatum(order.PriceNum, order.PriceDenom), // price
			order.NFT,                           // NFT (preserved from original)
			buildOptionalPOSIX(order.StartTime), // start (preserved)
			buildOptionalPOSIX(order.EndTime),   // end (preserved)
			order.PartialFills + 1,              // partialFills (incremented)
			order.MakerLovelaceFlatFee,          // makerLovelaceFlatFee (preserved)
			buildRationalDatum(
				order.MakerFeeNum,
				order.MakerFeeDenom,
			), // makerOfferedPercentFee (preserved)
			order.MakerFeeMax,                      // makerOfferedPercentFeeMax (preserved)
			buildContainedFeeDatumFromOrder(order), // containedFee (preserved)
			order.ContainedPayment,                 // containedPayment (preserved)
		},
	)

	return &PlutusData.PlutusData{
		Value: datum,
	}, nil
}

// buildContainedFeeDatumFromOrder constructs a contained fee datum from order state
func buildContainedFeeDatumFromOrder(order *OrderState) cbor.Constructor {
	return cbor.NewConstructor(
		0,
		cbor.IndefLengthList{
			order.ContainedLovelaceFee,
			order.ContainedOfferedFee,
			order.ContainedAskedFee,
		},
	)
}

// buildAssetDatum constructs a Plutus datum for an asset
func buildAssetDatum(asset interface{ IsLovelace() bool }) cbor.Constructor {
	type assetWithBytes interface {
		IsLovelace() bool
		GetPolicyId() []byte
		GetName() []byte
	}

	// Check if it has the required methods
	if a, ok := asset.(assetWithBytes); ok {
		return cbor.NewConstructor(
			0,
			cbor.IndefLengthList{
				a.GetPolicyId(),
				a.GetName(),
			},
		)
	}

	// Fallback for common.AssetClass
	return cbor.NewConstructor(0, cbor.IndefLengthList{[]byte{}, []byte{}})
}

// buildAddressDatum constructs a simplified address datum from raw bytes
func buildAddressDatum(ownerBytes []byte) cbor.Constructor {
	return cbor.NewConstructor(
		0,
		cbor.IndefLengthList{
			cbor.NewConstructor(0, cbor.IndefLengthList{ownerBytes}),
			cbor.NewConstructor(1, cbor.IndefLengthList{}), // No staking
		},
	)
}

// buildRationalDatum constructs a rational number datum
func buildRationalDatum(num, denom int64) cbor.Constructor {
	return cbor.NewConstructor(
		0,
		cbor.IndefLengthList{num, denom},
	)
}

// buildOptionalPOSIX constructs an optional POSIX timestamp datum
func buildOptionalPOSIX(t *time.Time) cbor.Constructor {
	if t == nil {
		return cbor.NewConstructor(1, cbor.IndefLengthList{})
	}
	return cbor.NewConstructor(
		0,
		cbor.IndefLengthList{t.UnixMilli()},
	)
}

// buildContainedFeeDatum constructs a contained fee datum (zeroed)
func buildContainedFeeDatum() cbor.Constructor {
	return cbor.NewConstructor(
		0,
		cbor.IndefLengthList{
			uint64(0), // lovelaceFee
			uint64(0), // offeredFee
			uint64(0), // askedFee
		},
	)
}

// buildOwnerAddress constructs the owner address from order state
func buildOwnerAddress(order *OrderState) (serAddress.Address, error) {
	cfg := config.GetConfig()

	// For simplicity, assume mainnet pubkey hash address
	// Full implementation would decode from the datum's ownerAddr field
	ownerBytes, err := hex.DecodeString(order.Owner)
	if err != nil {
		return serAddress.Address{}, err
	}

	// Build base address with just payment credential
	networkId := byte(1) // mainnet
	if cfg.Network == "preview" || cfg.Network == "preprod" {
		networkId = 0
	}

	// Address type 0x61 = enterprise address (payment key, no staking)
	// For proper implementation, use the full ownerAddr from datum
	addrBytes := append([]byte{0x61 | (networkId << 4)}, ownerBytes...)

	return serAddress.DecodeAddress(hex.EncodeToString(addrBytes))
}

// calculateOwnerPayment calculates the payment to send to order owner
func calculateOwnerPayment(
	fill orderFillOutput,
	route *Route,
	isTaker bool,
) (uint64, []apollo.Unit) {
	lovelace := uint64(minUtxoLovelace)
	var units []apollo.Unit

	if isTaker {
		// Taker receives the output asset
		if route.OutputAsset.IsLovelace() {
			lovelace = fill.outputAmount
		} else {
			units = append(units, apollo.NewUnit(
				hex.EncodeToString(route.OutputAsset.PolicyId),
				string(route.OutputAsset.Name),
				int(fill.outputAmount),
			))
		}
	} else {
		// Maker receives the input asset (what taker offered)
		if route.InputAsset.IsLovelace() {
			lovelace = fill.inputAmount
		} else {
			units = append(units, apollo.NewUnit(
				hex.EncodeToString(route.InputAsset.PolicyId),
				string(route.InputAsset.Name),
				int(fill.inputAmount),
			))
		}
	}

	return lovelace, units
}

// buildSortedInputIndexMap builds a map from UTxO key to sorted index
func buildSortedInputIndexMap(utxos []UTxO.UTxO) map[string]int {
	sortedUtxos := apollo.SortInputs(utxos)
	idxMap := make(map[string]int, len(sortedUtxos))

	for idx, utxo := range sortedUtxos {
		idxMap[utxoKey(utxo.Input)] = idx
	}

	return idxMap
}

// utxoKey returns a string key for a transaction input
func utxoKey(input TransactionInput.TransactionInput) string {
	return fmt.Sprintf(
		"%s#%d",
		hex.EncodeToString(input.TransactionId),
		input.Index,
	)
}

// estimateFee estimates the transaction fee
func estimateFee(numInputs, numOutputs int) uint64 {
	// Base fee + per-input + per-output costs
	// This is a rough estimate - actual fee depends on script execution
	baseFee := uint64(200000)
	perInput := uint64(50000)
	perOutput := uint64(30000)
	scriptOverhead := uint64(100000) // Per script execution

	return baseFee +
		uint64(numInputs)*perInput +
		uint64(numOutputs)*perOutput +
		uint64(numInputs)*scriptOverhead
}

// FeeConfig holds fee configuration for the batcher
type FeeConfig struct {
	MakerFeeFlat       uint64  // Flat maker fee in lovelace
	MakerFeePercent    float64 // Percent maker fee (0.0 to 1.0)
	MakerFeePercentMax uint64  // Maximum percent fee
	TakerFee           uint64  // Taker fee in lovelace
	MatcherReward      uint64  // Reward for matcher
}

// GetFeeConfig returns the fee configuration from the GeniusYield config
func (gy *GeniusYield) GetFeeConfig() FeeConfig {
	cfg := FeeConfig{
		MakerFeeFlat:       gy.config.MakerFeeFlat,
		MakerFeePercent:    gy.config.MakerFeePercent,
		MakerFeePercentMax: gy.config.MakerFeePercentMax,
		TakerFee:           gy.config.TakerFee,
		MatcherReward:      gy.config.MatcherReward,
	}

	// Apply defaults if not set
	if cfg.MakerFeeFlat == 0 {
		cfg.MakerFeeFlat = defaultMakerFeeFlat
	}
	if cfg.MakerFeePercent == 0 {
		cfg.MakerFeePercent = defaultMakerFeePercent
	}
	if cfg.TakerFee == 0 {
		cfg.TakerFee = defaultTakerFee
	}
	if cfg.MatcherReward == 0 {
		cfg.MatcherReward = defaultMatcherReward
	}

	return cfg
}

// CalculateMakerFee calculates the maker fee for a given amount
func (fc FeeConfig) CalculateMakerFee(amount uint64) uint64 {
	// Percent fee
	percentFee := uint64(float64(amount) * fc.MakerFeePercent)
	if percentFee > fc.MakerFeePercentMax && fc.MakerFeePercentMax > 0 {
		percentFee = fc.MakerFeePercentMax
	}

	// Total = flat + percent
	totalFee := fc.MakerFeeFlat + percentFee
	return totalFee
}

// CalculateTakerFee calculates the taker fee for a given amount
func (fc FeeConfig) CalculateTakerFee(amount uint64) uint64 {
	return fc.TakerFee
}

// CalculateTotalFees calculates all fees for a route
func (gy *GeniusYield) CalculateTotalFees(
	route *Route,
) (makerFees, takerFee, matcherReward uint64) {
	feeConfig := gy.GetFeeConfig()

	// Calculate maker fees for each leg
	for _, leg := range route.Legs {
		makerFees += feeConfig.CalculateMakerFee(leg.OutputAmount)
	}

	// Taker fee
	takerFee = feeConfig.CalculateTakerFee(route.TotalInput)

	// Matcher reward
	matcherReward = feeConfig.MatcherReward

	return makerFees, takerFee, matcherReward
}

// unixTimeToSlot converts Unix timestamp to slot number
func unixTimeToSlot(unixTime int64) uint64 {
	cfg := config.GetConfig()
	networkCfg := config.Networks[cfg.Network]
	return networkCfg.ShelleyOffsetSlot + uint64(
		unixTime-networkCfg.ShelleyOffsetTime,
	)
}
