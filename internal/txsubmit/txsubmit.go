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
	transactionChan chan []byte
}

var globalTxSubmit *TxSubmit

func Start(n *node.Node) error {
	cfg := config.GetConfig()
	txSubmit := &TxSubmit{
		transactionChan: make(chan []byte, maxOutboundTransactions),
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

func SubmitTx(txRawBytes []byte) {
	if globalTxSubmit == nil {
		panic("txsubmit: SubmitTx called before Start")
	}
	globalTxSubmit.transactionChan <- txRawBytes
}
