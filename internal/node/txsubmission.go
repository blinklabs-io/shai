package node

import (
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/protocol/txsubmission"
	"github.com/blinklabs-io/shai/internal/logging"
)

const (
	txsubmissionMempoolExpiration       = 1 * time.Hour
	txSubmissionMempoolExpirationPeriod = 1 * time.Minute

	txsubmissionServerLookupAttempts = 100
	txsubmissionServerLookupInterval = 50 * time.Millisecond
)

type ntnTransaction struct {
	Hash string
	Type uint
	Cbor []byte
}

type txsubmissionMempool struct {
	sync.Mutex
	Transactions            map[string]*TxsubmissionMempoolTransaction
	newTransactionFuncs     []MempoolNewTransactionFunc
	removedTransactionFuncs []MempoolRemovedTransactionFunc
	eventQueues             map[string]*mempoolEventQueue
	addResults              map[string]*mempoolAddResult
}

type mempoolEventQueue struct {
	events   []mempoolLifecycleEvent
	draining bool
}

type mempoolLifecycleEvent struct {
	tx               TxsubmissionMempoolTransaction
	removalReason    MempoolRemovalReason
	addedCallbacks   []MempoolNewTransactionFunc
	removedCallbacks []MempoolRemovedTransactionFunc
	addResult        *mempoolAddResult
	done             chan struct{}
}

type mempoolAddResult struct {
	done       chan struct{}
	err        error
	submitting bool
}

// OwnedTransactionHandle identifies one locally admitted transaction generation.
type OwnedTransactionHandle struct {
	hash       string
	generation *mempoolAddResult
}

type MempoolNewTransactionFunc func(TxsubmissionMempoolTransaction) error

// MempoolTransactionSource identifies where a transaction entered the node.
type MempoolTransactionSource uint8

const (
	// MempoolTransactionSourceObserved is an unvalidated inbound transaction.
	MempoolTransactionSourceObserved MempoolTransactionSource = iota
	// MempoolTransactionSourceOwned is originated or accepted by this process.
	MempoolTransactionSourceOwned
)

func (s MempoolTransactionSource) callbackEligible() bool {
	return s == MempoolTransactionSourceOwned
}

// MempoolRemovalReason identifies why a transaction left the local mempool.
type MempoolRemovalReason string

const (
	// MempoolRemovalReasonConfirmed means the transaction appeared on-chain.
	MempoolRemovalReasonConfirmed MempoolRemovalReason = "confirmed"
	// MempoolRemovalReasonExpired means the transaction was not recently seen.
	MempoolRemovalReasonExpired MempoolRemovalReason = "expired"
	// MempoolRemovalReasonRejected means local submission failed.
	MempoolRemovalReasonRejected MempoolRemovalReason = "rejected"
)

// MempoolRemovedTransactionFunc receives a removed transaction and its reason.
type MempoolRemovedTransactionFunc func(
	TxsubmissionMempoolTransaction,
	MempoolRemovalReason,
)

func (t *txsubmissionMempool) AddNewTransactionFunc(
	newTransactionFunc MempoolNewTransactionFunc,
) {
	t.Lock()
	defer t.Unlock()
	t.newTransactionFuncs = append(t.newTransactionFuncs, newTransactionFunc)
}

func (t *txsubmissionMempool) AddRemovedTransactionFunc(
	removedTransactionFunc MempoolRemovedTransactionFunc,
) {
	t.Lock()
	defer t.Unlock()
	t.removedTransactionFuncs = append(
		t.removedTransactionFuncs,
		removedTransactionFunc,
	)
}

func (t *txsubmissionMempool) removeExpired() {
	t.removeExpiredAt(time.Now())
	t.scheduleRemoveExpired()
}

func (t *txsubmissionMempool) removeExpiredAt(now time.Time) {
	t.Lock()
	expiredBefore := now.Add(-txsubmissionMempoolExpiration)
	startDrains := make([]string, 0)
	for txHash, tx := range t.Transactions {
		if tx.LastSeen.Before(expiredBefore) {
			if result := t.addResults[txHash]; result != nil && result.submitting {
				continue
			}
			delete(t.Transactions, txHash)
			delete(t.addResults, txHash)
			if tx.Source.callbackEligible() {
				if t.enqueueEventLocked(mempoolLifecycleEvent{
					tx:            cloneMempoolTransaction(*tx),
					removalReason: MempoolRemovalReasonExpired,
					removedCallbacks: append(
						[]MempoolRemovedTransactionFunc(nil),
						t.removedTransactionFuncs...,
					),
				}) {
					startDrains = append(startDrains, tx.Hash)
				}
			}
		}
	}
	t.Unlock()
	for _, txHash := range startDrains {
		go t.drainEvents(txHash)
	}
}

func (t *txsubmissionMempool) scheduleRemoveExpired() {
	_ = time.AfterFunc(txSubmissionMempoolExpirationPeriod, t.removeExpired)
}

func (t *txsubmissionMempool) addTransaction(
	tx *TxsubmissionMempoolTransaction,
) error {
	_, _, err := t.addTransactionWithWait(tx, false)
	return err
}

func (t *txsubmissionMempool) addTransactionWithWait(
	tx *TxsubmissionMempoolTransaction,
	wait bool,
) (bool, *mempoolAddResult, error) {
	if !tx.Source.callbackEligible() {
		return false, nil, nil
	}
	logger := logging.GetLogger()
	t.Lock()
	// Update last seen for existing TX
	if mempoolTx, ok := t.Transactions[tx.Hash]; ok {
		mempoolTx.LastSeen = time.Now()
		result := t.addResults[tx.Hash]
		t.Unlock()
		logger.Debug("updated last seen for transaction:", "txHash", tx.Hash)
		if wait && result != nil {
			<-result.done
			return false, result, result.err
		}
		return false, result, nil
	}
	// Add transaction record
	stored := cloneMempoolTransaction(*tx)
	t.Transactions[tx.Hash] = &stored
	if t.addResults == nil {
		t.addResults = make(map[string]*mempoolAddResult)
	}
	result := &mempoolAddResult{done: make(chan struct{})}
	t.addResults[tx.Hash] = result
	startDrain := t.enqueueEventLocked(mempoolLifecycleEvent{
		tx: cloneMempoolTransaction(stored),
		addedCallbacks: append(
			[]MempoolNewTransactionFunc(nil),
			t.newTransactionFuncs...,
		),
		addResult: result,
	})
	t.Unlock()
	if startDrain {
		go t.drainEvents(tx.Hash)
	}
	if wait {
		<-result.done
		return true, result, result.err
	}
	return true, result, nil
}

func (t *txsubmissionMempool) removeTransaction(
	hash string,
	reason MempoolRemovalReason,
) {
	t.removeTransactionGeneration(hash, nil, reason)
}

func (t *txsubmissionMempool) removeTransactionGeneration(
	hash string,
	expected *mempoolAddResult,
	reason MempoolRemovalReason,
) {
	t.Lock()
	tx, ok := t.Transactions[hash]
	if !ok || (expected != nil && t.addResults[hash] != expected) {
		t.Unlock()
		return
	}
	delete(t.Transactions, hash)
	delete(t.addResults, hash)
	startDrain := false
	var done chan struct{}
	if tx.Source.callbackEligible() {
		done = make(chan struct{})
		startDrain = t.enqueueEventLocked(mempoolLifecycleEvent{
			tx:            cloneMempoolTransaction(*tx),
			removalReason: reason,
			removedCallbacks: append(
				[]MempoolRemovedTransactionFunc(nil),
				t.removedTransactionFuncs...,
			),
			done: done,
		})
	}
	t.Unlock()
	if startDrain {
		go t.drainEvents(hash)
	}
	if done != nil {
		<-done
	}
}

func cloneMempoolTransaction(
	tx TxsubmissionMempoolTransaction,
) TxsubmissionMempoolTransaction {
	tx.Cbor = append([]byte(nil), tx.Cbor...)
	return tx
}

func (t *txsubmissionMempool) enqueueEventLocked(
	event mempoolLifecycleEvent,
) bool {
	if t.eventQueues == nil {
		t.eventQueues = make(map[string]*mempoolEventQueue)
	}
	queue := t.eventQueues[event.tx.Hash]
	if queue == nil {
		queue = &mempoolEventQueue{}
		t.eventQueues[event.tx.Hash] = queue
	}
	queue.events = append(queue.events, event)
	if queue.draining {
		return false
	}
	queue.draining = true
	return true
}

func (t *txsubmissionMempool) drainEvents(txHash string) {
	for {
		t.Lock()
		queue := t.eventQueues[txHash]
		if queue == nil || len(queue.events) == 0 {
			delete(t.eventQueues, txHash)
			t.Unlock()
			return
		}
		event := queue.events[0]
		queue.events[0] = mempoolLifecycleEvent{}
		queue.events = queue.events[1:]
		t.Unlock()

		var callbackErr error
		for _, callback := range event.addedCallbacks {
			if err := callback(event.tx); err != nil {
				callbackErr = errors.Join(callbackErr, err)
				logging.GetLogger().Error(
					"mempool add callback failed",
					"txHash", event.tx.Hash,
					"error", err,
				)
			}
		}
		for _, callback := range event.removedCallbacks {
			callback(event.tx, event.removalReason)
		}
		if event.addResult != nil {
			event.addResult.err = callbackErr
			close(event.addResult.done)
		}
		if event.done != nil {
			close(event.done)
		}
		if len(event.addedCallbacks) > 0 {
			logging.GetLogger().Debug(
				"added transaction to mempool:",
				"txHash", event.tx.Hash,
			)
		}
		if len(event.removedCallbacks) == 0 {
			continue
		}
		logging.GetLogger().Debug(
			"removed transaction from mempool:",
			"txHash", event.tx.Hash,
			"reason", event.removalReason,
		)
	}
}

type TxsubmissionMempoolTransaction struct {
	Hash     string
	Type     uint
	Cbor     []byte
	LastSeen time.Time
	Source   MempoolTransactionSource
}

func newMempoolTransaction(
	txBytes []byte,
	source MempoolTransactionSource,
) (*TxsubmissionMempoolTransaction, error) {
	txType, err := ledger.DetermineTransactionType(txBytes)
	if err != nil {
		return nil, fmt.Errorf(
			"could not parse transaction to determine type: %w",
			err,
		)
	}
	tx, err := ledger.NewTransactionFromCbor(txType, txBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse transaction CBOR: %w", err)
	}
	return &TxsubmissionMempoolTransaction{
		Hash:     tx.Hash().String(),
		Type:     txType,
		Cbor:     append([]byte(nil), txBytes...),
		LastSeen: time.Now(),
		Source:   source,
	}, nil
}

// AddOwnedTransaction records an originated or accepted transaction.
func (n *Node) AddOwnedTransaction(
	txBytes []byte,
) (*OwnedTransactionHandle, bool, error) {
	tx, err := newMempoolTransaction(
		txBytes,
		MempoolTransactionSourceOwned,
	)
	if err != nil {
		return nil, false, err
	}
	added, generation, err := n.txsubmissionMempool.addTransactionWithWait(
		tx,
		true,
	)
	if err != nil {
		if added {
			n.txsubmissionMempool.removeTransactionGeneration(
				tx.Hash,
				generation,
				MempoolRemovalReasonRejected,
			)
		}
		return nil, false, err
	}
	return &OwnedTransactionHandle{
		hash:       tx.Hash,
		generation: generation,
	}, added, nil
}

// BeginOwnedTransactionSubmission pins a current generation during delivery.
func (n *Node) BeginOwnedTransactionSubmission(
	handle *OwnedTransactionHandle,
) bool {
	if handle == nil || handle.generation == nil {
		return false
	}
	n.txsubmissionMempool.Lock()
	defer n.txsubmissionMempool.Unlock()
	current := n.txsubmissionMempool.addResults[handle.hash]
	if current == nil || current != handle.generation || current.submitting {
		return false
	}
	current.submitting = true
	return true
}

// EndOwnedTransactionSubmission releases a delivered or ambiguous generation.
func (n *Node) EndOwnedTransactionSubmission(
	handle *OwnedTransactionHandle,
) {
	if handle == nil || handle.generation == nil {
		return
	}
	n.txsubmissionMempool.Lock()
	defer n.txsubmissionMempool.Unlock()
	if n.txsubmissionMempool.addResults[handle.hash] != handle.generation {
		return
	}
	handle.generation.submitting = false
	if tx := n.txsubmissionMempool.Transactions[handle.hash]; tx != nil {
		tx.LastSeen = time.Now()
	}
}

// RejectOwnedTransaction releases a definitively rejected generation.
func (n *Node) RejectOwnedTransaction(handle *OwnedTransactionHandle) {
	if handle == nil || handle.generation == nil {
		return
	}
	n.txsubmissionMempool.removeTransactionGeneration(
		handle.hash,
		handle.generation,
		MempoolRemovalReasonRejected,
	)
}

func (n *Node) AddOutboundTransaction(txBytes []byte) error {
	tx, err := newMempoolTransaction(
		txBytes,
		MempoolTransactionSourceOwned,
	)
	if err != nil {
		return err
	}
	n.connTransactionChansMutex.Lock()
	if len(n.connTransactionChans) == 0 {
		n.connTransactionChansMutex.Unlock()
		return errors.New("no outbound transaction peers available")
	}
	if _, _, err := n.txsubmissionMempool.addTransactionWithWait(
		tx,
		true,
	); err != nil {
		n.connTransactionChansMutex.Unlock()
		return err
	}
	tmpTx := ntnTransaction{
		Hash: tx.Hash,
		Type: tx.Type,
		Cbor: append([]byte(nil), tx.Cbor...),
	}
	// Re-broadcast TX to each connection's TX chan
	delivered := 0
	for _, txChan := range n.connTransactionChans {
		select {
		case txChan <- tmpTx:
			delivered++
		default:
		}
	}
	n.connTransactionChansMutex.Unlock()
	if delivered == 0 {
		return errors.New("no outbound transaction peer accepted the transaction")
	}
	return nil
}

// txsubmissionServerOnInit starts the TX pull loop for a connection whose
// peer has sent Init. OnInit only carries the connection ID, and an inbound
// connection can send Init before it has been registered with the connection
// manager, so the lookup is retried briefly.
func (n *Node) txsubmissionServerOnInit(connId ouroboros.ConnectionId) {
	logger := logging.GetLogger()
	go func() {
		var server *txsubmission.Server
		for range txsubmissionServerLookupAttempts {
			if oConn := n.connManager.GetConnectionById(connId); oConn != nil {
				server = oConn.TxSubmission().Server
				break
			}
			time.Sleep(txsubmissionServerLookupInterval)
		}
		if server == nil {
			logger.Error(
				"txsubmission connection not found",
				"connId",
				connId.String(),
			)
			return
		}
		for {
			// Request available TX IDs (era and TX hash) and sizes
			txIds, err := server.RequestTxIds(true, 10)
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
				txs, err := server.RequestTxs(requestTxIds)
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
							Source:   MempoolTransactionSourceObserved,
						},
					)
					if err != nil {
						logger.Error(
							"failed to add TX to mempool:",
							"txHash",
							tx.Hash(),
							"error:",
							err,
						)
						return
					}
				}
			}
		}
	}()
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
