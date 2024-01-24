package txsubmit

import (
	"fmt"
	"sync"

	"github.com/blinklabs-io/shai/internal/config"

	ouroboros "github.com/blinklabs-io/gouroboros"
)

const (
	maxOutboundTransactions = 20
)

type TxSubmit struct {
	transactionChan           chan []byte
	connTransactionChans      map[int]chan ntnTransaction
	connTransactionChansMutex sync.Mutex
	connTransactionCache      map[int]map[string]*ntnTransaction
	connTransactionCacheMutex sync.Mutex
	connManager               *ouroboros.ConnectionManager
	outboundConns             map[int]*outboundConnection
	outboundConnsMutex        sync.Mutex
}

var globalTxSubmit = &TxSubmit{}

func Start() error {
	cfg := config.GetConfig()
	globalTxSubmit.transactionChan = make(chan []byte, maxOutboundTransactions)
	if len(cfg.Topology.Hosts) > 0 {
		return globalTxSubmit.startNtn(cfg.Topology.Hosts)
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
		return globalTxSubmit.startNtn(
			[]config.TopologyConfigHost{
				{
					Address: network.PublicRootAddress,
					Port:    network.PublicRootPort,
				},
			},
		)
	}
}

func SubmitTx(txRawBytes []byte) {
	globalTxSubmit.transactionChan <- txRawBytes
}
