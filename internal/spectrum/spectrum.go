package spectrum

import (
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/shai/internal/indexer"
	"github.com/blinklabs-io/shai/internal/logging"
	"github.com/blinklabs-io/shai/internal/storage"

	"github.com/blinklabs-io/snek/event"
	input_chainsync "github.com/blinklabs-io/snek/input/chainsync"
)

type Spectrum struct {
	idx                 *indexer.Indexer
	name                string
	swapAddress         string
	swapStakeAddress    string
	depositAddress      string
	depositStakeAddress string
	poolAddress         string
	poolStakeAddress    string
}

func New(idx *indexer.Indexer, name string, swapAddress string, depositAddress string, poolAddress string) *Spectrum {
	var swapStakeAddress, depositStakeAddress, poolStakeAddress string
	tmpSwapAddress, _ := ledger.NewAddress(swapAddress)
	if tmpSwapAddress.StakeAddress() != nil {
		swapStakeAddress = tmpSwapAddress.StakeAddress().String()
	}
	tmpDepositAddress, _ := ledger.NewAddress(depositAddress)
	if tmpDepositAddress.StakeAddress() != nil {
		depositStakeAddress = tmpDepositAddress.StakeAddress().String()
	}
	tmpPoolAddress, _ := ledger.NewAddress(poolAddress)
	if tmpPoolAddress.StakeAddress() != nil {
		poolStakeAddress = tmpPoolAddress.StakeAddress().String()
	}
	s := &Spectrum{
		idx:                 idx,
		name:                name,
		swapAddress:         swapAddress,
		swapStakeAddress:    swapStakeAddress,
		depositAddress:      depositAddress,
		depositStakeAddress: depositStakeAddress,
		poolAddress:         poolAddress,
		poolStakeAddress:    poolStakeAddress,
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
			var txOutputStakeAddress string
			if txOutput.Address().StakeAddress() != nil {
				txOutputStakeAddress = txOutput.Address().StakeAddress().String()
			}
			isSwap := false
			isDeposit := false
			isPool := false
			if txOutputAddress == s.swapAddress || (txOutputStakeAddress != "" && txOutputStakeAddress == s.swapStakeAddress) {
				isSwap = true
			}
			if txOutputAddress == s.depositAddress || (txOutputStakeAddress != "" && txOutputStakeAddress == s.depositStakeAddress) {
				isDeposit = true
			}
			if txOutputAddress == s.poolAddress || (txOutputStakeAddress != "" && txOutputStakeAddress == s.poolStakeAddress) {
				isPool = true
			}
			if isSwap || isDeposit || isPool {
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
						// Fetch matching pool UTxO
						poolUtxo, err := storage.GetStorage().GetAssetUtxo(
							s.name,
							swapConfig.PoolId.PolicyId,
							swapConfig.PoolId.Name,
						)
						if err != nil {
							return err
						}
						// TODO: do something useful with this
						fmt.Printf("poolUtxo(%d) = %x\n", len(poolUtxo), poolUtxo)
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
						/*
							// Store deposit UTXO by policy/asset
							if err := storage.GetStorage().UpdateAssetUtxo(
								s.name,
								depositConfig.PoolId.PolicyId,
								depositConfig.PoolId.Name,
								eventCtx.TransactionHash,
								uint32(idx),
							); err != nil {
								return err
							}
							logger.Debugf(
								"updated '%s' deposit UTxO for asset with policy ID %x and name '%s' (%x)",
								s.name,
								depositConfig.PoolId.PolicyId,
								depositConfig.PoolId.Name,
								depositConfig.PoolId.Name,
							)
						*/
					} else if isPool {
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
	}
	return nil
}
