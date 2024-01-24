package txsubmit

import (
	"encoding/hex"
	"fmt"
	"time"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/protocol/txsubmission"
	"github.com/blinklabs-io/shai/internal/config"
	"github.com/blinklabs-io/shai/internal/logging"
)

const (
	initialReconnectDelay = 1 * time.Second
	maxReconnectDelay     = 128 * time.Second
)

type ntnTransaction struct {
	Hash string
	Type uint
	Cbor []byte
}

type outboundConnection struct {
	Address        string
	ReconnectCount int
	ReconnectDelay time.Duration
}

func (t *TxSubmit) startNtn(topologyHosts []config.TopologyConfigHost) error {
	logger := logging.GetLogger()
	t.connTransactionChans = make(map[int]chan ntnTransaction)
	t.connTransactionCache = make(map[int]map[string]*ntnTransaction)
	t.outboundConns = make(map[int]*outboundConnection)
	t.connManager = ouroboros.NewConnectionManager(
		ouroboros.ConnectionManagerConfig{
			ErrorFunc: t.connectionManagerError,
		},
	)
	// Start outbound connections
	for idx, host := range topologyHosts {
		go func(connId int, address string) {
			if err := t.createOutboundConnection(connId, address); err != nil {
				logger.Errorf("failed to establish connection to %s: %s", address, err)
				go t.reconnectOutboundConnection(connId)
			}
		}(idx, fmt.Sprintf("%s:%d", host.Address, host.Port))
	}
	go t.handleTransactionNtn()
	return nil
}

func (t *TxSubmit) handleTransactionNtn() {
	logger := logging.GetLogger()
	for {
		txBytes, ok := <-t.transactionChan
		if !ok {
			return
		}
		// Determine transaction type (era)
		txType, err := ledger.DetermineTransactionType(txBytes)
		if err != nil {
			logger.Errorf("could not parse transaction to determine type: %s", err)
			return
		}
		tx, err := ledger.NewTransactionFromCbor(txType, txBytes)
		if err != nil {
			logger.Errorf("failed to parse transaction CBOR: %s", err)
			return
		}
		tmpTx := ntnTransaction{
			Hash: tx.Hash(),
			Type: txType,
			Cbor: txBytes[:],
		}
		// Re-broadcast TX to each connection's TX chan
		t.connTransactionChansMutex.Lock()
		for _, txChan := range t.connTransactionChans {
			txChan <- tmpTx
		}
		t.connTransactionChansMutex.Unlock()
		logger.Infof("submitted transaction %s via NtN TxSubmission", tx.Hash())
	}
}

func (t *TxSubmit) createOutboundConnection(connId int, address string) error {
	cfg := config.GetConfig()
	logger := logging.GetLogger()
	// Add to outbound connection tracking
	t.outboundConnsMutex.Lock()
	if _, ok := t.outboundConns[connId]; !ok {
		t.outboundConns[connId] = &outboundConnection{
			Address: address,
		}
	}
	t.outboundConnsMutex.Unlock()
	// Setup Ouroboros connection
	oConn, err := ouroboros.NewConnection(
		ouroboros.WithNetworkMagic(cfg.NetworkMagic),
		ouroboros.WithNodeToNode(true),
		ouroboros.WithKeepAlive(true),
		ouroboros.WithTxSubmissionConfig(
			txsubmission.NewConfig(
				txsubmission.WithRequestTxIdsFunc(func(
					blocking bool,
					ack uint16,
					req uint16,
				) ([]txsubmission.TxIdAndSize, error) {
					return t.txsubmissionClientRequestTxIds(connId, blocking, ack, req)
				}),
				txsubmission.WithRequestTxsFunc(func(
					txIds []txsubmission.TxId,
				) ([]txsubmission.TxBody, error) {
					return t.txsubmissionClientRequestTxs(connId, txIds)
				}),
			),
		),
	)
	if err != nil {
		return err
	}
	// Establish connection
	if err := oConn.Dial("tcp", address); err != nil {
		return err
	}
	logger.Infof("connected to node at %s", address)
	// Add to connection manager
	t.connManager.AddConnection(connId, oConn)
	// Add TX watcher chan
	t.connTransactionChansMutex.Lock()
	t.connTransactionChans[connId] = make(chan ntnTransaction, maxOutboundTransactions)
	t.connTransactionChansMutex.Unlock()
	// Create TX cache
	t.connTransactionCacheMutex.Lock()
	t.connTransactionCache[connId] = make(map[string]*ntnTransaction)
	t.connTransactionCacheMutex.Unlock()
	// Start TxSubmission loop
	oConn.TxSubmission().Client.Init()
	return nil
}

func (t *TxSubmit) reconnectOutboundConnection(connId int) {
	logger := logging.GetLogger()
	outboundConn := t.outboundConns[connId]
	for {
		if outboundConn.ReconnectDelay == 0 {
			outboundConn.ReconnectDelay = initialReconnectDelay
		} else if outboundConn.ReconnectDelay < maxReconnectDelay {
			outboundConn.ReconnectDelay = outboundConn.ReconnectDelay * 2
		}
		logger.Infof("delaying %s before reconnecting to %s", outboundConn.ReconnectDelay, outboundConn.Address)
		time.Sleep(outboundConn.ReconnectDelay)
		if err := t.createOutboundConnection(connId, outboundConn.Address); err != nil {
			logger.Errorf("failed to establish connection to %s: %s", outboundConn.Address, err)
			continue
		}
		return
	}
}

func (t *TxSubmit) connectionManagerError(connId int, err error) {
	logger := logging.GetLogger()
	logger.Errorf("connection %d failed: %s", connId, err)
	conn := t.connManager.GetConnectionById(connId)
	if conn == nil {
		return
	}
	// Remove connection
	t.connManager.RemoveConnection(connId)
	// Close and remove transaction watcher channel
	t.connTransactionChansMutex.Lock()
	close(t.connTransactionChans[connId])
	delete(t.connTransactionChans, connId)
	t.connTransactionChansMutex.Unlock()
	// Remove transaction cache for connection
	t.connTransactionCacheMutex.Lock()
	delete(t.connTransactionCache, connId)
	t.connTransactionCacheMutex.Unlock()
	// Reconnect if it was an outbound connection
	if _, ok := t.outboundConns[connId]; ok {
		go t.reconnectOutboundConnection(connId)
	}
	time.Sleep(1 * time.Second)
}

func (t *TxSubmit) txsubmissionClientRequestTxIds(
	connId int,
	blocking bool,
	ack uint16,
	req uint16,
) ([]txsubmission.TxIdAndSize, error) {
	ret := []txsubmission.TxIdAndSize{}
	// Clear TX cache
	if ack > 0 {
		t.connTransactionCacheMutex.Lock()
		t.connTransactionCache[connId] = make(map[string]*ntnTransaction)
		t.connTransactionCacheMutex.Unlock()
	}
	// Get available TXs
	t.connTransactionChansMutex.Lock()
	txChan := t.connTransactionChans[connId]
	t.connTransactionChansMutex.Unlock()
	var tmpTxs []ntnTransaction
	doneWaiting := false
	for {
		if blocking && len(tmpTxs) == 0 {
			// Wait until we see a TX
			tmpTx, ok := <-txChan
			if !ok {
				break
			}
			tmpTxs = append(tmpTxs, tmpTx)
		} else {
			// Return immediately if no TX is available
			select {
			case tmpTx, ok := <-txChan:
				if !ok {
					doneWaiting = true
					break
				}
				tmpTxs = append(tmpTxs, tmpTx)
			default:
				doneWaiting = true
			}
			if doneWaiting {
				break
			}
		}
	}
	for _, tmpTx := range tmpTxs {
		tmpTx := tmpTx
		// Add to return value
		txHashBytes, err := hex.DecodeString(tmpTx.Hash)
		if err != nil {
			return nil, err
		}
		ret = append(
			ret,
			txsubmission.TxIdAndSize{
				TxId: txsubmission.TxId{
					EraId: uint16(tmpTx.Type),
					TxId:  [32]byte(txHashBytes),
				},
				Size: uint32(len(tmpTx.Cbor)),
			},
		)
		// Add to transaction cache
		t.connTransactionCacheMutex.Lock()
		t.connTransactionCache[connId][tmpTx.Hash] = &tmpTx
		t.connTransactionCacheMutex.Unlock()
	}
	return ret, nil
}

func (t *TxSubmit) txsubmissionClientRequestTxs(
	connId int,
	txIds []txsubmission.TxId,
) ([]txsubmission.TxBody, error) {
	ret := []txsubmission.TxBody{}
	for _, txId := range txIds {
		txHash := hex.EncodeToString(txId.TxId[:])
		t.connTransactionCacheMutex.Lock()
		tx := t.connTransactionCache[connId][txHash]
		t.connTransactionCacheMutex.Unlock()
		if tx != nil {
			ret = append(
				ret,
				txsubmission.TxBody{
					EraId:  uint16(tx.Type),
					TxBody: tx.Cbor,
				},
			)
		}
	}
	return ret, nil
}
