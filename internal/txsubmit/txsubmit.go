package txsubmit

import (
	"fmt"

	"github.com/blinklabs-io/shai/internal/config"
	"github.com/blinklabs-io/shai/internal/node"

	ouroboros "github.com/blinklabs-io/gouroboros"
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
	globalTxSubmit = &TxSubmit{
		transactionChan: make(chan []byte, maxOutboundTransactions),
		node:            n,
	}
	if len(cfg.Topology.Hosts) > 0 {
		return globalTxSubmit.startNtn()
	} else if cfg.Submit.Url != "" {
		return globalTxSubmit.startApi(cfg.Submit.Url)
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
		return globalTxSubmit.startNtn()
	}
}

func SubmitTx(txRawBytes []byte) {
	globalTxSubmit.transactionChan <- txRawBytes
}
