package spectrum

import (
	"encoding/hex"
	"fmt"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/shai/internal/config"
	"github.com/blinklabs-io/shai/internal/indexer"
	"github.com/blinklabs-io/shai/internal/logging"
	"github.com/blinklabs-io/shai/internal/storage"

	"github.com/blinklabs-io/snek/event"
	input_chainsync "github.com/blinklabs-io/snek/input/chainsync"
)

type Spectrum struct {
	idx            *indexer.Indexer
	config         config.SpectrumProfileConfig
	name           string
	swapAddress    string
	depositAddress string
	redeemAddress  string
	poolV1Address  string
	poolV2Address  string
}

func New(idx *indexer.Indexer, name string, config config.SpectrumProfileConfig) *Spectrum {
	s := &Spectrum{
		idx:            idx,
		config:         config,
		name:           name,
		swapAddress:    scriptAddressFromHash(config.SwapHash),
		depositAddress: scriptAddressFromHash(config.DepositHash),
		redeemAddress:  scriptAddressFromHash(config.RedeemHash),
		poolV1Address:  scriptAddressFromHash(config.PoolV1Hash),
		poolV2Address:  scriptAddressFromHash(config.PoolV2Hash),
	}
	idx.AddEventFunc(s.handleChainsyncEvent)
	return s
}

func (s *Spectrum) handleChainsyncEvent(evt event.Event) error {
	logger := logging.GetLogger()
	switch evt.Payload.(type) {
	case input_chainsync.TransactionEvent:
		eventTx := evt.Payload.(input_chainsync.TransactionEvent)
		eventCtx := evt.Context.(input_chainsync.TransactionContext)
		for idx, txOutput := range eventTx.Outputs {
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
				continue
			}
			// Write UTXO to storage
			if err := storage.GetStorage().AddUtxo(
				txOutputAddress,
				eventCtx.TransactionHash,
				uint32(idx),
				txOutput.Cbor(),
			); err != nil {
				return err
			}
			datum := txOutput.Datum()
			if datum != nil {
				//fmt.Printf("found transaction (%s) with datum: isSwap=%v, isDeposit=%v, isRedeem=%v, isPoolV1=%v, isPoolV2=%v\n", eventCtx.TransactionHash, isSwap, isDeposit, isRedeem, isPoolV1, isPoolV2)
				if isSwap {
					var swapConfig SwapConfig
					if _, err := cbor.Decode(datum.Cbor(), &swapConfig); err != nil {
						logger.Warnf(
							"error decoding TX (%s) output datum: %s: cbor hex: %x",
							eventCtx.TransactionHash,
							err,
							datum.Cbor(),
						)
						continue
					}
					// Get swap UTxO
					// We fetch this from storage so it's in the proper format
					swapUtxo, err := storage.GetStorage().GetUtxoById(
						fmt.Sprintf("%s.%d", eventCtx.TransactionHash, idx),
					)
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
						logger.Warnf("no matching pool UTxO for swap: %s", err)
						continue
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
					if poolPaymentAddr == s.poolV1Address {
						poolInputRef = s.config.PoolV1InputRef
					} else if poolPaymentAddr == s.poolV2Address {
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
						logger.Errorf("failed to build transaction: %s", err)
					} else if false { // TODO: remove 'if false'
						fmt.Printf("txBytes(%d) = %x\n", len(txBytes), txBytes)
					}
				} else if isDeposit {
					var depositConfig DepositConfig
					if _, err := cbor.Decode(datum.Cbor(), &depositConfig); err != nil {
						logger.Warnf(
							"error decoding TX (%s) output datum: %s: cbor hex: %x",
							eventCtx.TransactionHash,
							err,
							datum.Cbor(),
						)
						continue
					}
				} else if isRedeem {
					// TODO
				} else if isPoolV1 || isPoolV2 {
					var poolConfig PoolConfig
					if _, err := cbor.Decode(datum.Cbor(), &poolConfig); err != nil {
						logger.Warnf(
							"error decoding TX (%s) output datum: %s: cbor hex: %x",
							eventCtx.TransactionHash,
							err,
							datum.Cbor(),
						)
						continue
					}
					// Store pool UTXO by policy/asset
					if err := storage.GetStorage().UpdateAssetUtxo(
						s.name,
						poolConfig.Nft.PolicyId,
						poolConfig.Nft.Name,
						eventCtx.TransactionHash,
						uint32(idx),
					); err != nil {
						return err
					}
					logger.Debugf(
						"updated '%s' pool UTxO for asset with policy ID %x and name '%s' (%x)",
						s.name,
						poolConfig.Nft.PolicyId,
						poolConfig.Nft.Name,
						poolConfig.Nft.Name,
					)
				}
			}
		}
	}
	return nil
}

func scriptAddressFromHash(scriptHashHex string) string {
	cfg := config.GetConfig()
	network := ouroboros.NetworkByName(cfg.Network)
	if network == ouroboros.NetworkInvalid {
		return ""
	}
	scriptHash, err := hex.DecodeString(scriptHashHex)
	if err != nil {
		return ""
	}
	addr := ledger.NewAddressFromParts(
		ledger.AddressTypeScriptNone,
		network.Id,
		scriptHash,
		nil,
	)
	return addr.String()
}

func addressFromKeys(paymentKey []byte, stakeKey []byte) string {
	cfg := config.GetConfig()
	network := ouroboros.NetworkByName(cfg.Network)
	if network == ouroboros.NetworkInvalid {
		return ""
	}
	addrType := ledger.AddressTypeKeyNone
	if len(stakeKey) > 0 {
		addrType = ledger.AddressTypeKeyKey
	}
	addr := ledger.NewAddressFromParts(
		uint8(addrType),
		network.Id,
		paymentKey,
		stakeKey,
	)
	return addr.String()
}
