package node

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/blinklabs-io/shai/internal/config"
	"github.com/blinklabs-io/shai/internal/indexer"
	"github.com/blinklabs-io/shai/internal/logging"
	"golang.org/x/sys/unix"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/gouroboros/protocol/blockfetch"
	"github.com/blinklabs-io/gouroboros/protocol/chainsync"
	"github.com/blinklabs-io/gouroboros/protocol/localtxsubmission"
	"github.com/blinklabs-io/gouroboros/protocol/peersharing"
	"github.com/blinklabs-io/gouroboros/protocol/txsubmission"
)

const (
	initialReconnectDelay   = 1 * time.Second
	maxReconnectDelay       = 128 * time.Second
	maxOutboundTransactions = 20
)

type Node struct {
	listener                  net.Listener
	listenerNtc               net.Listener
	connManager               *ouroboros.ConnectionManager
	chainsyncClientState      *chainsyncClientState
	chainsyncServerState      map[ouroboros.ConnectionId]*chainsyncServerState
	txsubmissionMempool       *txsubmissionMempool
	outboundConns             map[ouroboros.ConnectionId]outboundPeer
	outboundConnsMutex        sync.Mutex
	connTransactionChans      map[ouroboros.ConnectionId]chan ntnTransaction
	connTransactionChansMutex sync.Mutex
	connTransactionCache      map[ouroboros.ConnectionId]map[string]*ntnTransaction
	connTransactionCacheMutex sync.Mutex
}

type outboundPeer struct {
	Address        string
	ReconnectCount int
	ReconnectDelay time.Duration
}

func New(idx *indexer.Indexer) *Node {
	n := &Node{
		chainsyncServerState: make(map[ouroboros.ConnectionId]*chainsyncServerState),
		chainsyncClientState: &chainsyncClientState{},
		txsubmissionMempool: &txsubmissionMempool{
			Transactions: make(map[string]*TxsubmissionMempoolTransaction),
		},
		outboundConns:        make(map[ouroboros.ConnectionId]outboundPeer),
		connTransactionChans: make(map[ouroboros.ConnectionId]chan ntnTransaction),
		connTransactionCache: make(map[ouroboros.ConnectionId]map[string]*ntnTransaction),
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
	// NtN listener
	listenAddress := fmt.Sprintf("%s:%d", cfg.ListenAddress, cfg.ListenPort)
	listenConfig := net.ListenConfig{
		Control: socketControl,
	}
	listener, err := listenConfig.Listen(context.Background(), "tcp", listenAddress)
	if err != nil {
		return fmt.Errorf("failed to open listening socket: %s", err)
	}
	n.listener = listener
	go n.acceptConnections()
	logger.Infof("listening on %s (NtN)", listenAddress)
	// NtC listener
	listenAddressNtc := fmt.Sprintf("%s:%d", cfg.ListenAddressNtc, cfg.ListenPortNtc)
	listenConfigNtc := net.ListenConfig{
		Control: socketControl,
	}
	listenerNtc, err := listenConfigNtc.Listen(context.Background(), "tcp", listenAddressNtc)
	if err != nil {
		return fmt.Errorf("failed to open listening socket: %s", err)
	}
	n.listenerNtc = listenerNtc
	go n.acceptConnectionsNtc()
	logger.Infof("listening on %s (NtC)", listenAddressNtc)
	// Start outbound connections
	for _, host := range cfg.Topology.Hosts {
		peerAddress := net.JoinHostPort(host.Address, strconv.Itoa(int(host.Port)))
		tmpPeer := outboundPeer{Address: peerAddress}
		go func(peer outboundPeer) {
			if err := n.createOutboundConnection(peer); err != nil {
				logger.Errorf("failed to establish connection to %s: %s", peer.Address, err)
				go n.reconnectOutboundConnection(peer)
			}
		}(tmpPeer)
	}
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
		// Setup Ouroboros connection
		oConn, err := ouroboros.NewConnection(
			ouroboros.WithNetworkMagic(cfg.NetworkMagic),
			ouroboros.WithNodeToNode(true),
			ouroboros.WithServer(true),
			ouroboros.WithConnection(conn),
			// We enable peer-sharing to get our own address shared to drive incoming connections
			ouroboros.WithPeerSharing(true),
			ouroboros.WithTxSubmissionConfig(
				txsubmission.NewConfig(
					txsubmission.WithInitFunc(
						n.txsubmissionServerInit,
					),
				),
			),
			ouroboros.WithChainSyncConfig(
				chainsync.NewConfig(
					chainsync.WithFindIntersectFunc(
						n.chainsyncServerFindIntersect,
					),
					chainsync.WithRequestNextFunc(
						n.chainsyncServerRequestNext,
					),
				),
			),
			ouroboros.WithBlockFetchConfig(
				blockfetch.NewConfig(
					blockfetch.WithRequestRangeFunc(
						n.blockfetchServerRequestRange,
					),
				),
			),
			ouroboros.WithPeerSharingConfig(
				peersharing.NewConfig(
					peersharing.WithShareRequestFunc(
						n.peerSharingShareRequest,
					),
				),
			),
		)
		if err != nil {
			logger.Errorf("failed to setup connection: %s", err)
			continue
		}
		// Add to connection manager
		n.connManager.AddConnection(oConn)
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
		// Setup Ouroboros connection
		oConn, err := ouroboros.NewConnection(
			ouroboros.WithNetworkMagic(cfg.NetworkMagic),
			ouroboros.WithNodeToNode(false),
			ouroboros.WithServer(true),
			ouroboros.WithConnection(conn),
			ouroboros.WithLocalTxSubmissionConfig(
				localtxsubmission.NewConfig(
					localtxsubmission.WithSubmitTxFunc(
						func(ctx localtxsubmission.CallbackContext, tx any) error {
							return n.localTxsubmissionServerSubmitTx(
								ctx,
								// TODO: change this in the gouroboros interface
								tx.(localtxsubmission.MsgSubmitTxTransaction),
							)
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
		n.connManager.AddConnection(oConn)
	}
}

func (n *Node) createOutboundConnection(peer outboundPeer) error {
	cfg := config.GetConfig()
	logger := logging.GetLogger()
	// Setup connection to use our listening port as the source port
	// This is required for peer sharing to be useful
	clientAddr, _ := net.ResolveTCPAddr("tcp", fmt.Sprintf(":%d", cfg.ListenPort))
	dialer := net.Dialer{
		LocalAddr: clientAddr,
		Timeout:   10 * time.Second,
		Control:   socketControl,
	}
	tmpConn, err := dialer.Dial("tcp", peer.Address)
	if err != nil {
		return err
	}
	// Setup Ouroboros connection
	oConn, err := ouroboros.NewConnection(
		ouroboros.WithConnection(tmpConn),
		ouroboros.WithNetworkMagic(cfg.NetworkMagic),
		ouroboros.WithNodeToNode(true),
		ouroboros.WithFullDuplex(true),
		// We enable peer-sharing to get our own address shared to drive incoming connections
		ouroboros.WithPeerSharing(true),
		ouroboros.WithKeepAlive(true),
		ouroboros.WithTxSubmissionConfig(
			txsubmission.NewConfig(
				txsubmission.WithInitFunc(
					n.txsubmissionServerInit,
				),
				txsubmission.WithRequestTxIdsFunc(
					n.txsubmissionClientRequestTxIds,
				),
				txsubmission.WithRequestTxsFunc(
					n.txsubmissionClientRequestTxs,
				),
			),
		),
		ouroboros.WithChainSyncConfig(
			chainsync.NewConfig(
				chainsync.WithFindIntersectFunc(
					n.chainsyncServerFindIntersect,
				),
				chainsync.WithRequestNextFunc(
					n.chainsyncServerRequestNext,
				),
			),
		),
		ouroboros.WithBlockFetchConfig(
			blockfetch.NewConfig(
				blockfetch.WithRequestRangeFunc(
					n.blockfetchServerRequestRange,
				),
			),
		),
		ouroboros.WithPeerSharingConfig(
			peersharing.NewConfig(
				peersharing.WithShareRequestFunc(
					n.peerSharingShareRequest,
				),
			),
		),
	)
	if err != nil {
		return err
	}
	logger.Infof("connected to node at %s", peer.Address)
	// Add to connection manager
	n.connManager.AddConnection(oConn)
	// Add to outbound connection tracking
	n.outboundConnsMutex.Lock()
	n.outboundConns[oConn.Id()] = peer
	n.outboundConnsMutex.Unlock()
	// Add TX watcher chan
	n.connTransactionChansMutex.Lock()
	n.connTransactionChans[oConn.Id()] = make(chan ntnTransaction, maxOutboundTransactions)
	n.connTransactionChansMutex.Unlock()
	// Create TX cache
	n.connTransactionCacheMutex.Lock()
	n.connTransactionCache[oConn.Id()] = make(map[string]*ntnTransaction)
	n.connTransactionCacheMutex.Unlock()
	// Start TxSubmission loop
	oConn.TxSubmission().Client.Init()
	return nil
}

func (n *Node) reconnectOutboundConnection(peer outboundPeer) {
	logger := logging.GetLogger()
	for {
		if peer.ReconnectDelay == 0 {
			peer.ReconnectDelay = initialReconnectDelay
		} else if peer.ReconnectDelay < maxReconnectDelay {
			peer.ReconnectDelay = peer.ReconnectDelay * 2
		}
		logger.Infof("delaying %s before reconnecting to %s", peer.ReconnectDelay, peer.Address)
		time.Sleep(peer.ReconnectDelay)
		if err := n.createOutboundConnection(peer); err != nil {
			logger.Errorf("failed to establish connection to %s: %s", peer.Address, err)
			continue
		}
		return
	}
}

func (n *Node) connectionManagerConnClosed(connId ouroboros.ConnectionId, err error) {
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
	if serverState, ok := n.chainsyncServerState[connId]; ok {
		// Unsub from chainsync updates
		if serverState.blockChan != nil {
			n.chainsyncClientState.Unsub(connId)
		}
		// Remove server state entry
		delete(n.chainsyncServerState, connId)
	}
	// Clean up client transaction channel/cache
	n.outboundConnsMutex.Lock()
	if peer, ok := n.outboundConns[connId]; ok {
		// Close and remove transaction watcher channel
		n.connTransactionChansMutex.Lock()
		close(n.connTransactionChans[connId])
		delete(n.connTransactionChans, connId)
		n.connTransactionChansMutex.Unlock()
		// Remove transaction cache for connection
		n.connTransactionCacheMutex.Lock()
		delete(n.connTransactionCache, connId)
		n.connTransactionCacheMutex.Unlock()
		// Reconnect outbound connection
		go n.reconnectOutboundConnection(peer)
	}
	defer n.outboundConnsMutex.Unlock()
}

// Helper function for setting socket options on listener and outbound sockets
func socketControl(network, address string, c syscall.RawConn) error {
	var innerErr error
	err := c.Control(func(fd uintptr) {
		err := unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEADDR, 1)
		if err != nil {
			innerErr = err
			return
		}
		err = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEPORT, 1)
		if err != nil {
			innerErr = err
			return
		}
	})
	if innerErr != nil {
		return innerErr
	}
	if err != nil {
		return err
	}
	return nil
}
