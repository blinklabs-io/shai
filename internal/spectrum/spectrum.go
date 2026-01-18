package spectrum

import (
	"encoding/hex"
	"fmt"

	"github.com/blinklabs-io/adder/event"
	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/shai/internal/config"
	"github.com/blinklabs-io/shai/internal/indexer"
	"github.com/blinklabs-io/shai/internal/logging"
	"github.com/blinklabs-io/shai/internal/node"
	"github.com/blinklabs-io/shai/internal/storage"
	"github.com/blinklabs-io/shai/internal/txsubmit"
)

type Spectrum struct {
	idx            *indexer.Indexer
	node           *node.Node
	config         config.SpectrumProfileConfig
	name           string
	swapAddress    string
	depositAddress string
	redeemAddress  string
	poolV1Address  string
	poolV2Address  string
}

func New(
	idx *indexer.Indexer,
	node *node.Node,
	name string,
	config config.SpectrumProfileConfig,
) *Spectrum {
	s := &Spectrum{
		idx:            idx,
		node:           node,
		config:         config,
		name:           name,
		swapAddress:    scriptAddressFromHash(config.SwapHash),
		depositAddress: scriptAddressFromHash(config.DepositHash),
		redeemAddress:  scriptAddressFromHash(config.RedeemHash),
		poolV1Address:  scriptAddressFromHash(config.PoolV1Hash),
		poolV2Address:  scriptAddressFromHash(config.PoolV2Hash),
	}
	idx.AddEventFunc(s.handleChainsyncEvent)
	node.AddMempoolNewTransactionFunc(s.handleMempoolNewTransaction)
	return s
}

func (s *Spectrum) handleChainsyncEvent(evt event.Event) error {
	logger := logging.GetLogger()
	switch evt.Payload.(type) {
	case event.TransactionEvent:
		eventTx := evt.Payload.(event.TransactionEvent)
		eventCtx := evt.Context.(event.TransactionContext)
		for idx, txOutput := range eventTx.Outputs {
			if err := s.handleTransactionOutput(eventCtx.TransactionHash, idx, txOutput, false); err != nil {
				logger.Error("failure handling on-chain transaction output:", "txId", eventCtx.TransactionHash, "index", idx, "error:", err)
			}
		}
	}
	return nil
}

func (s *Spectrum) handleMempoolNewTransaction(
	mempoolTx node.TxsubmissionMempoolTransaction,
) error {
	logger := logging.GetLogger()
	tx, err := ledger.NewTransactionFromCbor(mempoolTx.Type, mempoolTx.Cbor)
	if err != nil {
		return err
	}
	for idx, txOutput := range tx.Outputs() {
		if err := s.handleTransactionOutput(tx.Hash().String(), idx, txOutput, true); err != nil {
			logger.Error(
				"failure handling mempool transaction output:",
				"txId",
				tx.Hash().String(),
				"index",
				idx,
				"error:",
				err,
			)
		}
	}
	return nil
}

func (s *Spectrum) handleTransactionOutput(
	txId string,
	txOutputIdx int,
	txOutput ledger.TransactionOutput,
	fromMempool bool,
) error {
	logger := logging.GetLogger()
	// Check for the addresses we care about
	txOutputAddress := txOutput.Address().String()
	var paymentAddr string
	if txOutput.Address().PaymentAddress() != nil {
		paymentAddr = txOutput.Address().PaymentAddress().String()
	}
	isSwap := false
	isDeposit := false
	isRedeem := false
	isPoolV1 := false
	isPoolV2 := false
	switch paymentAddr {
	case s.swapAddress:
		isSwap = true
	case s.depositAddress:
		isDeposit = true
	case s.redeemAddress:
		isRedeem = true
	case s.poolV1Address:
		isPoolV1 = true
	case s.poolV2Address:
		isPoolV2 = true
	default:
		return nil
	}
	datum := txOutput.Datum()
	if datum != nil {
		if isSwap && fromMempool {
			var swapConfig SwapConfig
			if _, err := cbor.Decode(datum.Cbor(), &swapConfig); err != nil {
				return fmt.Errorf(
					"error decoding datum: %w: cbor hex: %x",
					err,
					datum.Cbor(),
				)
			}
			// Generate wrapped version of UTxO for TX building
			swapUtxo, err := wrapTxOutput(txId, txOutputIdx, txOutput.Cbor())
			if err != nil {
				return err
			}
			// Fetch matching pool UTxO
			// We get the UTxO ID first so that we can use it later to get the address
			// that owns it
			poolUtxoId, err := storage.GetStorage().GetAssetUtxoId(
				s.name,
				swapConfig.PoolId.PolicyId,
				swapConfig.PoolId.Name,
			)
			if err != nil {
				return fmt.Errorf("no matching pool UTxO for swap: %w", err)
			}
			poolUtxo, err := storage.GetStorage().GetUtxoById(poolUtxoId)
			if err != nil {
				return err
			}
			pool, err := NewPoolFromUtxoBytes(poolUtxo)
			if err != nil {
				return err
			}
			// Get address for current pool UTxO
			poolAddr, err := storage.GetStorage().GetUtxoAddress(poolUtxoId)
			if err != nil {
				return err
			}
			// Determine which pool contract input ref to use
			var poolInputRef config.ProfileConfigInputRef
			tmpPoolAddr, _ := ledger.NewAddress(poolAddr)
			poolPaymentAddr := tmpPoolAddr.PaymentAddress().String()
			switch poolPaymentAddr {
			case s.poolV1Address:
				poolInputRef = s.config.PoolV1InputRef
			case s.poolV2Address:
				poolInputRef = s.config.PoolV2InputRef
			}
			// Build swap TX
			swapTxOpts := createSwapTxOpts{
				poolUtxoBytes:     poolUtxo[:],
				pool:              pool,
				outputPoolAddress: poolAddr,
				poolInputRef:      poolInputRef,
				swapUtxoBytes:     swapUtxo[:],
				swapConfig:        swapConfig,
			}
			txBytes, err := s.createSwapTx(swapTxOpts)
			if err != nil {
				logger.Error("failed to build transaction:", "error:", err)
			} else {
				//fmt.Printf("txBytes(%d) = %x\n", len(txBytes), txBytes)
				// Submit the TX
				txsubmit.SubmitTx(txBytes)
			}
		} else if isDeposit && fromMempool {
			var depositConfig DepositConfig
			if _, err := cbor.Decode(datum.Cbor(), &depositConfig); err != nil {
				return fmt.Errorf(
					"error decoding datum: %w: cbor hex: %x",
					err,
					datum.Cbor(),
				)
			}
		} else if isRedeem && fromMempool {
			// TODO
		} else if (isPoolV1 || isPoolV2) && !fromMempool {
			// TODO: checked TX inputs against earlier recorded UTxO ID and dump TX bytes on match
			// Write UTXO to storage
			if err := storage.GetStorage().AddUtxo(
				txOutputAddress,
				txId,
				uint32(txOutputIdx),
				txOutput.Cbor(),
			); err != nil {
				return err
			}
			var poolConfig PoolConfig
			if _, err := cbor.Decode(datum.Cbor(), &poolConfig); err != nil {
				return fmt.Errorf(
					"error decoding datum: %w: cbor hex: %x",
					err,
					datum.Cbor(),
				)
			}
			// Store pool UTXO by policy/asset
			if err := storage.GetStorage().UpdateAssetUtxo(
				s.name,
				poolConfig.Nft.PolicyId,
				poolConfig.Nft.Name,
				txId,
				uint32(txOutputIdx),
			); err != nil {
				return err
			}
			logger.Debug(
				"updated pool UTxO for asset",
				"name", s.name,
				"policyID", poolConfig.Nft.PolicyId,
				"assetName", poolConfig.Nft.Name,
				"assetNameHex", poolConfig.Nft.Name,
			)
			/*
				pool, err := NewPoolFromTransactionOutput(txOutput)
				if err != nil {
					return err
				}
				fmt.Printf("Pool (%s) prices:\n", pool.Id.Name)
				xName := string(pool.X.Class.Name)
				if pool.X.IsLovelace() {
					xName = "lovelace"
				}
				yName := string(pool.Y.Class.Name)
				if pool.Y.IsLovelace() {
					yName = "lovelace"
				}
				fmt.Printf("  - 1 %s = %f %s\n", xName, (float64(pool.Y.Amount) / float64(pool.X.Amount)), yName)
				fmt.Printf("  - 1 %s = %f %s\n", yName, (float64(pool.X.Amount) / float64(pool.Y.Amount)), xName)
			*/
		}
	}
	return nil
}

func wrapTxOutput(
	txId string,
	txOutputIdx int,
	txOutBytes []byte,
) ([]byte, error) {
	// Wrap TX output in UTxO structure to make it easier to consume later
	txIdBytes, err := hex.DecodeString(txId)
	if err != nil {
		return nil, err
	}
	// Create temp UTxO structure
	utxoTmp := []any{
		// Transaction output reference
		[]any{
			txIdBytes,
			uint32(txOutputIdx),
		},
		// Transaction output CBOR
		cbor.RawMessage(txOutBytes),
	}
	// Convert to CBOR
	cborBytes, err := cbor.Encode(&utxoTmp)
	if err != nil {
		return nil, err
	}
	return cborBytes[:], nil
}

func scriptAddressFromHash(scriptHashHex string) string {
	cfg := config.GetConfig()
	network, valid := ouroboros.NetworkByName(cfg.Network)
	if !valid {
		return ""
	}
	scriptHash, err := hex.DecodeString(scriptHashHex)
	if err != nil {
		return ""
	}
	addr, err := ledger.NewAddressFromParts(
		ledger.AddressTypeScriptNone,
		network.Id,
		scriptHash,
		nil,
	)
	if err != nil {
		return ""
	}
	return addr.String()
}

func addressFromKeys(paymentKey []byte, stakeKey []byte) string {
	cfg := config.GetConfig()
	network, valid := ouroboros.NetworkByName(cfg.Network)
	if !valid {
		return ""
	}
	addrType := ledger.AddressTypeKeyNone
	if len(stakeKey) > 0 {
		addrType = ledger.AddressTypeKeyKey
	}
	addr, err := ledger.NewAddressFromParts(
		uint8(addrType),
		network.Id,
		paymentKey,
		stakeKey,
	)
	if err != nil {
		return ""
	}
	return addr.String()
}
