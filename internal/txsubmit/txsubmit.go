package txsubmit

import (
	"fmt"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/shai/internal/config"
	"github.com/blinklabs-io/shai/internal/node"
)

const (
	maxOutboundTransactions = 20
)

type TxSubmit struct {
	node            *node.Node
	transactionChan chan queuedTransaction
}

type queuedTransaction struct {
	bytes []byte
	owned *node.OwnedTransactionHandle
}

var globalTxSubmit *TxSubmit

func Start(n *node.Node) error {
	cfg := config.GetConfig()
	txSubmit := &TxSubmit{
		transactionChan: make(chan queuedTransaction, maxOutboundTransactions),
		node:            n,
	}
	var err error
	if len(cfg.Topology.Hosts) > 0 {
		err = txSubmit.startNtn()
	} else if cfg.Submit.Url != "" {
		err = txSubmit.startApi(cfg.Submit.Url)
		/*
			} else if cfg.Submit.SocketPath != "" {
				return submitTxNtC(txRawBytes)
		*/
	} else {
		// Populate address info from indexer network
		network, valid := ouroboros.NetworkByName(cfg.Network)
		if !valid {
			return fmt.Errorf("unknown network: %s", cfg.Network)
		}
		cfg.Topology.Hosts = []config.TopologyConfigHost{
			{
				// TODO - how many bootstrap peers are there?
				Address: network.BootstrapPeers[0].Address,
				Port:    network.BootstrapPeers[0].Port,
			},
		}
		err = txSubmit.startNtn()
	}
	if err != nil {
		return err
	}
	globalTxSubmit = txSubmit
	return nil
}

func IsStarted() bool {
	return globalTxSubmit != nil && globalTxSubmit.transactionChan != nil
}

func SubmitTx(txRawBytes []byte) error {
	if globalTxSubmit == nil {
		return fmt.Errorf("txsubmit is not started")
	}
	owned, added, err := globalTxSubmit.node.AddOwnedTransaction(txRawBytes)
	if err != nil {
		return fmt.Errorf("failed to track submitted transaction: %w", err)
	}
	if !added {
		return nil
	}
	globalTxSubmit.transactionChan <- queuedTransaction{
		bytes: append([]byte(nil), txRawBytes...),
		owned: owned,
	}
	return nil
}
