package node

import (
	"fmt"
	"net"

	"github.com/blinklabs-io/shai/internal/config"
	"github.com/blinklabs-io/shai/internal/indexer"
	"github.com/blinklabs-io/shai/internal/logging"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/gouroboros/protocol/blockfetch"
	"github.com/blinklabs-io/gouroboros/protocol/chainsync"
	"github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/blinklabs-io/gouroboros/protocol/txsubmission"
)

type Node struct {
	listener             net.Listener
	connManager          *ouroboros.ConnectionManager
	chainsyncClientState *chainsyncClientState
	chainsyncServerState map[int]*chainsyncServerState
	txsubmissionMempool  *txsubmissionMempool
}

func New(idx *indexer.Indexer) *Node {
	n := &Node{
		chainsyncServerState: make(map[int]*chainsyncServerState),
		chainsyncClientState: &chainsyncClientState{},
		txsubmissionMempool: &txsubmissionMempool{
			Transactions: make(map[string]*TxsubmissionMempoolTransaction),
		},
	}
	// Register indexer event handler
	idx.AddEventFunc(n.chainsyncClientHandleEvent)
	return n
}

func (n *Node) Start() error {
	cfg := config.GetConfig()
	logger := logging.GetLogger()
	listenAddress := fmt.Sprintf("%s:%d", cfg.ListenAddress, cfg.ListenPort)
	listener, err := net.Listen("tcp", listenAddress)
	if err != nil {
		return fmt.Errorf("failed to open listening socket: %s", err)
	}
	n.listener = listener
	n.connManager = ouroboros.NewConnectionManager(
		ouroboros.ConnectionManagerConfig{
			ErrorFunc: n.connectionManagerError,
		},
	)
	go n.acceptConnections()
	logger.Infof("listening on %s", listenAddress)
	n.txsubmissionMempool.scheduleRemoveExpired()
	return nil
}

func (n *Node) AddMempoolNewTransactionFunc(newTransactionFunc MempoolNewTransactionFunc) {
	n.txsubmissionMempool.AddNewTransactionFunc(newTransactionFunc)
}

func (n *Node) acceptConnections() {
	cfg := config.GetConfig()
	logger := logging.GetLogger()
	// Initial connection ID for tracking
	connId := 1_000_000
	for {
		// Accept connection
		conn, err := n.listener.Accept()
		if err != nil {
			logger.Errorf("accept failed: %s", err)
			continue
		}
		logger.Infof("accepted connection from %s", conn.RemoteAddr())
		// Increment connection counter
		connId++
		// Setup Ouroboros connection
		oConn, err := ouroboros.NewConnection(
			ouroboros.WithNetworkMagic(cfg.NetworkMagic),
			ouroboros.WithNodeToNode(true),
			ouroboros.WithServer(true),
			ouroboros.WithConnection(conn),
			ouroboros.WithTxSubmissionConfig(
				txsubmission.NewConfig(
					txsubmission.WithInitFunc(func() error {
						return n.txsubmissionServerInit(connId)
					}),
				),
			),
			ouroboros.WithChainSyncConfig(
				chainsync.NewConfig(
					chainsync.WithFindIntersectFunc(
						func(points []common.Point) (common.Point, chainsync.Tip, error) {
							return n.chainsyncServerFindIntersect(connId, points)
						},
					),
					chainsync.WithRequestNextFunc(
						func() error {
							return n.chainsyncServerRequestNext(connId)
						},
					),
				),
			),
			ouroboros.WithBlockFetchConfig(
				blockfetch.NewConfig(
					blockfetch.WithRequestRangeFunc(
						func(start common.Point, end common.Point) error {
							return n.blockfetchServerRequestRange(connId, start, end)
						},
					),
				),
			),
		)
		if err != nil {
			logger.Errorf("failed to setup connection: %s", err)
			continue
		}
		// Add to connection manager
		n.connManager.AddConnection(connId, oConn)
	}
}

func (n *Node) connectionManagerError(connId int, err error) {
	logger := logging.GetLogger()
	logger.Errorf("connection %d failed: %s", connId, err)
	conn := n.connManager.GetConnectionById(connId)
	if conn == nil {
		return
	}
	// Remove connection
	n.connManager.RemoveConnection(connId)
	// Clean up chainsync server state for connection
	serverState, ok := n.chainsyncServerState[connId]
	if !ok {
		return
	}
	// Unsub from chainsync updates
	if serverState.blockChan != nil {
		n.chainsyncClientState.Unsub(connId)
	}
	// Remove server state entry
	delete(n.chainsyncServerState, connId)
}
