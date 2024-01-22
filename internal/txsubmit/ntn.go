package txsubmit

import (
	"encoding/hex"
	"sync"
	"time"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/protocol/txsubmission"
	"github.com/blinklabs-io/shai/internal/config"
	"github.com/blinklabs-io/shai/internal/logging"
)

type TxSubmitNtn struct {
	txType        uint
	txBytes       []byte
	txHash        []byte
	txHashHex     string
	sentTx        bool
	failed        bool
	doneChan      chan any
	doneChanMutex sync.Mutex
}

func NewTxSubmitNtn() *TxSubmitNtn {
	return &TxSubmitNtn{}
}

func (t *TxSubmitNtn) Submit(txRawBytes []byte, address string) {
	cfg := config.GetConfig()
	logger := logging.GetLogger()

	// Record TX bytes in global for use in handler functions
	t.txBytes = txRawBytes[:]
	t.sentTx = false

	// Determine transaction type (era)
	var err error
	t.txType, err = ledger.DetermineTransactionType(txRawBytes)
	if err != nil {
		logger.Errorf("could not parse transaction to determine type: %s", err)
		return
	}
	tx, err := ledger.NewTransactionFromCbor(t.txType, txRawBytes)
	if err != nil {
		logger.Errorf("failed to parse transaction CBOR: %s", err)
		return
	}

	// Record TX hash
	t.txHashHex = tx.Hash()
	t.txHash, err = hex.DecodeString(t.txHashHex)
	if err != nil {
		logger.Errorf("failed to decode TX hash: %s", err)
		return
	}

	// Create connection
	o, err := ouroboros.New(
		ouroboros.WithNetworkMagic(cfg.NetworkMagic),
		ouroboros.WithNodeToNode(true),
		ouroboros.WithKeepAlive(true),
		ouroboros.WithTxSubmissionConfig(
			txsubmission.NewConfig(
				txsubmission.WithRequestTxIdsFunc(t.handleRequestTxIds),
				txsubmission.WithRequestTxsFunc(t.handleRequestTxs),
			),
		),
	)
	if err != nil {
		logger.Errorf("failed to establish connection to node %s: %s", address, err)
		return
	}
	if err := o.Dial("tcp", address); err != nil {
		logger.Errorf("failed to establish connection to node %s: %s", address, err)
		return
	}

	// Capture errors
	go func() {
		err, ok := <-o.ErrorChan()
		if ok {
			t.failed = true
			logger.Errorf("failed to submit transaction %s to node %s: %s", t.txHashHex, address, err)
			t.closeDoneChan()
		}
	}()

	// Start txSubmission loop
	t.doneChan = make(chan any)
	o.TxSubmission().Client.Init()
	<-t.doneChan
	// Sleep 2s to allow time for TX to enter remote mempool before closing connection
	time.Sleep(2 * time.Second)

	if err := o.Close(); err != nil {
		logger.Errorf("failed to close connection with node %s: %s", address, err)
	}

	if !t.failed {
		logger.Infof("successfully submitted transaction %s to node %s", t.txHashHex, address)
	}
}

func (t *TxSubmitNtn) closeDoneChan() {
	t.doneChanMutex.Lock()
	defer t.doneChanMutex.Unlock()
	// Check if doneChan is already closed
	select {
	case <-t.doneChan:
		// Already closed
		return
	default:
		// Close it
		close(t.doneChan)
	}
}

func (t *TxSubmitNtn) handleRequestTxIds(
	blocking bool,
	ack uint16,
	req uint16,
) ([]txsubmission.TxIdAndSize, error) {
	if t.sentTx {
		// Terrible syncronization hack for shutdown
		t.closeDoneChan()
		time.Sleep(5 * time.Second)
		return nil, nil
	}
	ret := []txsubmission.TxIdAndSize{
		{
			TxId: txsubmission.TxId{
				EraId: uint16(t.txType),
				TxId:  [32]byte(t.txHash),
			},
			Size: uint32(len(t.txBytes)),
		},
	}
	return ret, nil
}

func (t *TxSubmitNtn) handleRequestTxs(
	txIds []txsubmission.TxId,
) ([]txsubmission.TxBody, error) {
	ret := []txsubmission.TxBody{
		{
			EraId:  uint16(t.txType),
			TxBody: t.txBytes,
		},
	}
	t.sentTx = true
	return ret, nil
}
