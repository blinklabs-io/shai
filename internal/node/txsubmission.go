package node

import (
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/protocol/txsubmission"
	"github.com/blinklabs-io/shai/internal/logging"
)

const (
	txsubmissionMempoolExpiration       = 1 * time.Hour
	txSubmissionMempoolExpirationPeriod = 1 * time.Minute
)

type ntnTransaction struct {
	Hash string
	Type uint
	Cbor []byte
}

type txsubmissionMempool struct {
	sync.Mutex
	Transactions        map[string]*TxsubmissionMempoolTransaction
	newTransactionFuncs []MempoolNewTransactionFunc
}

type MempoolNewTransactionFunc func(TxsubmissionMempoolTransaction) error

func (t *txsubmissionMempool) AddNewTransactionFunc(
	newTransactionFunc MempoolNewTransactionFunc,
) {
	t.newTransactionFuncs = append(t.newTransactionFuncs, newTransactionFunc)
}

func (t *txsubmissionMempool) removeExpired() {
	logger := logging.GetLogger()
	t.Lock()
	defer t.Unlock()
	expiredBefore := time.Now().Add(-txsubmissionMempoolExpiration)
	for txHash, tx := range t.Transactions {
		if tx.LastSeen.Before(expiredBefore) {
			delete(t.Transactions, txHash)
			logger.Debug("removed expired transaction:", "txHash", txHash)
		}
	}
	t.scheduleRemoveExpired()
}

func (t *txsubmissionMempool) scheduleRemoveExpired() {
	_ = time.AfterFunc(txSubmissionMempoolExpirationPeriod, t.removeExpired)
}

func (t *txsubmissionMempool) addTransaction(
	tx *TxsubmissionMempoolTransaction,
) error {
	logger := logging.GetLogger()
	t.Lock()
	defer t.Unlock()
	// Update last seen for existing TX
	if mempoolTx, ok := t.Transactions[tx.Hash]; ok {
		mempoolTx.LastSeen = time.Now()
		logger.Debug("updated last seen for transaction:", "txHash", tx.Hash)
		return nil
	}
	// Add transaction record
	t.Transactions[tx.Hash] = tx
	// Call registered new transaction handlers
	for _, newTransactionFunc := range t.newTransactionFuncs {
		if err := newTransactionFunc(*tx); err != nil {
			return err
		}
	}
	logger.Debug("added transaction to mempool:", "txHash", tx.Hash)
	return nil
}

func (t *txsubmissionMempool) removeTransaction(hash string) {
	logger := logging.GetLogger()
	t.Lock()
	defer t.Unlock()
	if _, ok := t.Transactions[hash]; ok {
		delete(t.Transactions, hash)
		logger.Debug("removed transaction from mempool:", "txHash", hash)
	}
}

type TxsubmissionMempoolTransaction struct {
	Hash     string
	Type     uint
	Cbor     []byte
	LastSeen time.Time
}

func (n *Node) AddOutboundTransaction(txBytes []byte) error {
	// Determine transaction type (era)
	txType, err := ledger.DetermineTransactionType(txBytes)
	if err != nil {
		return fmt.Errorf(
			"could not parse transaction to determine type: %w",
			err,
		)
	}
	tx, err := ledger.NewTransactionFromCbor(txType, txBytes)
	if err != nil {
		return fmt.Errorf("failed to parse transaction CBOR: %w", err)
	}
	tmpTx := ntnTransaction{
		Hash: tx.Hash().String(),
		Type: txType,
		Cbor: txBytes[:],
	}
	// Re-broadcast TX to each connection's TX chan
	n.connTransactionChansMutex.Lock()
	for _, txChan := range n.connTransactionChans {
		txChan <- tmpTx
	}
	n.connTransactionChansMutex.Unlock()
	return nil
}

func (n *Node) txsubmissionServerInit(ctx txsubmission.CallbackContext) error {
	logger := logging.GetLogger()
	go func() {
		for {
			// Request available TX IDs (era and TX hash) and sizes
			txIds, err := ctx.Server.RequestTxIds(true, 10)
			if err != nil {
				logger.Error("failed to request TxIds:", "error:", err)
				return
			}
			if len(txIds) > 0 {
				// Unwrap inner TxId from TxIdAndSize
				var requestTxIds []txsubmission.TxId
				for _, txId := range txIds {
					requestTxIds = append(requestTxIds, txId.TxId)
				}
				// Request TX content for TxIds from above
				txs, err := ctx.Server.RequestTxs(requestTxIds)
				if err != nil {
					logger.Error("failed to request Txs:", "error:", err)
					return
				}
				for _, txBody := range txs {
					tx, err := ledger.NewTransactionFromCbor(
						uint(txBody.EraId),
						txBody.TxBody,
					)
					if err != nil {
						logger.Error(
							"failed to parse transaction CBOR:",
							"error:",
							err,
						)
						return
					}
					// Add transaction to mempool
					err = n.txsubmissionMempool.addTransaction(
						&TxsubmissionMempoolTransaction{
							Hash:     tx.Hash().String(),
							Type:     uint(txBody.EraId),
							Cbor:     txBody.TxBody,
							LastSeen: time.Now(),
						},
					)
					if err != nil {
						logger.Error(
							"failed to add TX to mempool:",
							"txHash",
							tx.Hash().String(),
							"error:",
							err,
						)
						return
					}
				}
			}
		}
	}()
	return nil
}

func (n *Node) txsubmissionClientRequestTxIds(
	ctx txsubmission.CallbackContext,
	blocking bool,
	ack uint16,
	req uint16,
) ([]txsubmission.TxIdAndSize, error) {
	connId := ctx.ConnectionId
	ret := []txsubmission.TxIdAndSize{}
	// Clear TX cache
	if ack > 0 {
		n.connTransactionCacheMutex.Lock()
		n.connTransactionCache[connId] = make(map[string]*ntnTransaction)
		n.connTransactionCacheMutex.Unlock()
	}
	// Get available TXs
	n.connTransactionChansMutex.Lock()
	txChan, ok := n.connTransactionChans[connId]
	n.connTransactionChansMutex.Unlock()
	// Protect against potential race condition with unexpected shutdown
	if !ok {
		return ret, nil
	}
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
		n.connTransactionCacheMutex.Lock()
		// Protect against potential race condition between this and unexpected shutdown
		if _, ok := n.connTransactionCache[connId]; ok {
			n.connTransactionCache[connId][tmpTx.Hash] = &tmpTx
		}
		n.connTransactionCacheMutex.Unlock()
	}
	return ret, nil
}

func (n *Node) txsubmissionClientRequestTxs(
	ctx txsubmission.CallbackContext,
	txIds []txsubmission.TxId,
) ([]txsubmission.TxBody, error) {
	connId := ctx.ConnectionId
	ret := []txsubmission.TxBody{}
	for _, txId := range txIds {
		txHash := hex.EncodeToString(txId.TxId[:])
		n.connTransactionCacheMutex.Lock()
		tx := n.connTransactionCache[connId][txHash]
		n.connTransactionCacheMutex.Unlock()
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
