package node

import (
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

type txsubmissionMempool struct {
	sync.Mutex
	Transactions        map[string]*TxsubmissionMempoolTransaction
	newTransactionFuncs []MempoolNewTransactionFunc
}

type MempoolNewTransactionFunc func(TxsubmissionMempoolTransaction) error

func (t *txsubmissionMempool) AddNewTransactionFunc(newTransactionFunc MempoolNewTransactionFunc) {
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
			logger.Debugf("removed expired transaction %s from mempool", txHash)
		}
	}
	t.scheduleRemoveExpired()
}

func (t *txsubmissionMempool) scheduleRemoveExpired() {
	_ = time.AfterFunc(txSubmissionMempoolExpirationPeriod, t.removeExpired)
}

func (t *txsubmissionMempool) addTransaction(tx *TxsubmissionMempoolTransaction) error {
	logger := logging.GetLogger()
	t.Lock()
	defer t.Unlock()
	// Update last seen for existing TX
	if mempoolTx, ok := t.Transactions[tx.Hash]; ok {
		mempoolTx.LastSeen = time.Now()
		logger.Debugf("updated last seen for transaction %s in mempool", tx.Hash)
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
	logger.Debugf("added transaction %s to mempool", tx.Hash)
	return nil
}

func (t *txsubmissionMempool) removeTransaction(hash string) {
	logger := logging.GetLogger()
	t.Lock()
	defer t.Unlock()
	if _, ok := t.Transactions[hash]; ok {
		delete(t.Transactions, hash)
		logger.Debugf("removed transaction %s from mempool", hash)
	}
}

type TxsubmissionMempoolTransaction struct {
	Hash     string
	Type     uint
	Cbor     []byte
	LastSeen time.Time
}

func (n *Node) txsubmissionServerInit(connId int) error {
	logger := logging.GetLogger()
	conn := n.connManager.GetConnectionById(connId)
	if conn == nil {
		return fmt.Errorf("connection %d not found", connId)
	}
	txSubServer := conn.Conn.TxSubmission().Server
	go func() {
		for {
			// Request available TX IDs (era and TX hash) and sizes
			txIds, err := txSubServer.RequestTxIds(true, 10)
			if err != nil {
				logger.Errorf("failed to request TxIds: %s", err)
				return
			}
			if len(txIds) > 0 {
				// Unwrap inner TxId from TxIdAndSize
				var requestTxIds []txsubmission.TxId
				for _, txId := range txIds {
					requestTxIds = append(requestTxIds, txId.TxId)
				}
				// Request TX content for TxIds from above
				txs, err := txSubServer.RequestTxs(requestTxIds)
				if err != nil {
					logger.Errorf("failed to request Txs: %s", err)
					return
				}
				for _, txBody := range txs {
					tx, err := ledger.NewTransactionFromCbor(uint(txBody.EraId), txBody.TxBody)
					if err != nil {
						logger.Errorf("failed to parse transaction CBOR: %s", err)
						return
					}
					// Add transaction to mempool
					err = n.txsubmissionMempool.addTransaction(
						&TxsubmissionMempoolTransaction{
							Hash:     tx.Hash(),
							Type:     uint(txBody.EraId),
							Cbor:     txBody.TxBody,
							LastSeen: time.Now(),
						},
					)
					if err != nil {
						logger.Errorf("failed to add TX %s to mempool: %s", tx.Hash(), err)
						return
					}
				}
			}
		}
	}()
	return nil
}
