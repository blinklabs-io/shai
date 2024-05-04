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

var globalTxSubmit = &TxSubmit{}

func Start(n *node.Node) error {
	cfg := config.GetConfig()
	globalTxSubmit.transactionChan = make(chan []byte, maxOutboundTransactions)
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
		network := ouroboros.NetworkByName(cfg.Network)
		if network == ouroboros.NetworkInvalid {
			return fmt.Errorf("unknown network: %s", cfg.Network)
		}
		cfg.Topology.Hosts = []config.TopologyConfigHost{
			{
				Address: network.PublicRootAddress,
				Port:    network.PublicRootPort,
			},
		}
		return globalTxSubmit.startNtn()
	}
}

func SubmitTx(txRawBytes []byte) {
	globalTxSubmit.transactionChan <- txRawBytes
}
