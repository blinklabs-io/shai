package node

import (
	"fmt"
	"net"
	"sync/atomic"

	"github.com/blinklabs-io/shai/internal/config"
	"github.com/blinklabs-io/shai/internal/indexer"
	"github.com/blinklabs-io/shai/internal/logging"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/gouroboros/protocol/blockfetch"
	"github.com/blinklabs-io/gouroboros/protocol/chainsync"
	"github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/blinklabs-io/gouroboros/protocol/localtxsubmission"
	"github.com/blinklabs-io/gouroboros/protocol/txsubmission"
)

type Node struct {
	listener             net.Listener
	listenerNtc          net.Listener
	incomingConnectionId atomic.Uint64
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
	n.connManager = ouroboros.NewConnectionManager(
		ouroboros.ConnectionManagerConfig{
			ConnClosedFunc: n.connectionManagerConnClosed,
		},
	)
	// Set initial connection ID for tracking
	n.incomingConnectionId.Store(1_000_000)
	// NtN listener
	listenAddress := fmt.Sprintf("%s:%d", cfg.ListenAddress, cfg.ListenPort)
	listener, err := net.Listen("tcp", listenAddress)
	if err != nil {
		return fmt.Errorf("failed to open listening socket: %s", err)
	}
	n.listener = listener
	go n.acceptConnections()
	logger.Infof("listening on %s (NtN)", listenAddress)
	// NtC listener
	listenAddressNtc := fmt.Sprintf("%s:%d", cfg.ListenAddressNtc, cfg.ListenPortNtc)
	listenerNtc, err := net.Listen("tcp", listenAddressNtc)
	if err != nil {
		return fmt.Errorf("failed to open listening socket: %s", err)
	}
	n.listenerNtc = listenerNtc
	go n.acceptConnectionsNtc()
	logger.Infof("listening on %s (NtC)", listenAddressNtc)
	// Schedule initial mempool expired cleanup
	n.txsubmissionMempool.scheduleRemoveExpired()
	return nil
}

func (n *Node) AddMempoolNewTransactionFunc(newTransactionFunc MempoolNewTransactionFunc) {
	n.txsubmissionMempool.AddNewTransactionFunc(newTransactionFunc)
}

func (n *Node) acceptConnections() {
	cfg := config.GetConfig()
	logger := logging.GetLogger()
	for {
		// Accept connection
		conn, err := n.listener.Accept()
		if err != nil {
			logger.Errorf("accept failed: %s", err)
			continue
		}
		logger.Infof("accepted connection from %s", conn.RemoteAddr())
		// Increment connection counter
		connId := int(n.incomingConnectionId.Add(1))
		// Setup Ouroboros connection
		oConn, err := ouroboros.NewConnection(
			ouroboros.WithNetworkMagic(cfg.NetworkMagic),
			ouroboros.WithNodeToNode(true),
			ouroboros.WithServer(true),
			ouroboros.WithConnection(conn),
			ouroboros.WithTxSubmissionConfig(
				txsubmission.NewConfig(
					txsubmission.WithInitFunc(
						func(connId int) func() error {
							return func() error {
								return n.txsubmissionServerInit(connId)
							}
						}(connId),
					),
				),
			),
			ouroboros.WithChainSyncConfig(
				chainsync.NewConfig(
					chainsync.WithFindIntersectFunc(
						func(connId int) func(points []common.Point) (common.Point, chainsync.Tip, error) {
							return func(points []common.Point) (common.Point, chainsync.Tip, error) {
								return n.chainsyncServerFindIntersect(connId, points)
							}
						}(connId),
					),
					chainsync.WithRequestNextFunc(
						func(connId int) func() error {
							return func() error {
								return n.chainsyncServerRequestNext(connId)
							}
						}(connId),
					),
				),
			),
			ouroboros.WithBlockFetchConfig(
				blockfetch.NewConfig(
					blockfetch.WithRequestRangeFunc(
						func(connId int) func(start common.Point, end common.Point) error {
							return func(start common.Point, end common.Point) error {
								return n.blockfetchServerRequestRange(connId, start, end)
							}
						}(connId),
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

func (n *Node) acceptConnectionsNtc() {
	cfg := config.GetConfig()
	logger := logging.GetLogger()
	for {
		// Accept connection
		conn, err := n.listenerNtc.Accept()
		if err != nil {
			logger.Errorf("accept failed: %s", err)
			continue
		}
		logger.Infof("accepted connection from %s", conn.RemoteAddr())
		// Increment connection counter
		connId := int(n.incomingConnectionId.Add(1))
		// Setup Ouroboros connection
		oConn, err := ouroboros.NewConnection(
			ouroboros.WithNetworkMagic(cfg.NetworkMagic),
			ouroboros.WithNodeToNode(false),
			ouroboros.WithServer(true),
			ouroboros.WithConnection(conn),
			ouroboros.WithLocalTxSubmissionConfig(
				localtxsubmission.NewConfig(
					localtxsubmission.WithSubmitTxFunc(
						func(connId int) func(any) error {
							return func(tx any) error {
								return n.localTxsubmissionServerSubmitTx(
									// TODO: change this in the gouroboros interface
									tx.(localtxsubmission.MsgSubmitTxTransaction),
								)
							}
						}(connId),
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
func (n *Node) connectionManagerConnClosed(connId int, err error) {
	logger := logging.GetLogger()
	if err != nil {
		logger.Errorf("connection %d failed: %s", connId, err)
	} else {
		logger.Infof("connection %s closed", connId)
	}
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
