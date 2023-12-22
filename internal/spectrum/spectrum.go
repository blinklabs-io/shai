package spectrum

import (
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/shai/internal/indexer"
	"github.com/blinklabs-io/shai/internal/logging"
	"github.com/blinklabs-io/shai/internal/storage"

	"github.com/blinklabs-io/snek/event"
	input_chainsync "github.com/blinklabs-io/snek/input/chainsync"
)

type Spectrum struct {
	idx            *indexer.Indexer
	name           string
	swapAddress    string
	depositAddress string
}

func New(idx *indexer.Indexer, name string, swapAddress string, depositAddress string) *Spectrum {
	s := &Spectrum{
		idx:            idx,
		name:           name,
		swapAddress:    swapAddress,
		depositAddress: depositAddress,
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
			isSwap := false
			isDeposit := false
			if txOutputAddress == s.swapAddress {
				isSwap = true
			}
			if txOutputAddress == s.depositAddress {
				isDeposit = true
			}
			if isSwap || isDeposit {
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
								"error decoding TX (%s) output datum: %s",
								eventCtx.TransactionHash,
								err,
							)
							continue
						}
						// TODO: do something with swap config
						// XXX: should we only care about parsing it when it's in a TX from the mempool?
						fmt.Printf("swapConfig = %s\n", swapConfig.String())
					} else if isDeposit {
						var depositConfig DepositConfig
						if _, err := cbor.Decode(datum.Cbor(), &depositConfig); err != nil {
							logger.Warnf(
								"error decoding TX (%s) output datum: %s",
								eventCtx.TransactionHash,
								err,
							)
							continue
						}
						// TODO: do something with deposit config
						// XXX: do we actually need to do anything with it?
						fmt.Printf("depositConfig = %s\n", depositConfig.String())
					}
				}
			}
		}
	}
	return nil
}
