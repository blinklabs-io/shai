package node

import (
	"time"

	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/protocol/localtxsubmission"
	"github.com/blinklabs-io/shai/internal/logging"
)

func (n *Node) localTxsubmissionServerSubmitTx(msgSubmitTxTransaction localtxsubmission.MsgSubmitTxTransaction) error {
	logger := logging.GetLogger()
	txEraId := uint(msgSubmitTxTransaction.EraId)
	txBytes := msgSubmitTxTransaction.Raw.Content.([]byte)
	tx, err := ledger.NewTransactionFromCbor(txEraId, txBytes)
	if err != nil {
		logger.Errorf("failed to parse transaction CBOR: %s", err)
		// XXX: do we want to return the error to the submitter?
		return nil
	}
	// Add transaction to mempool
	err = n.txsubmissionMempool.addTransaction(
		&TxsubmissionMempoolTransaction{
			Hash:     tx.Hash(),
			Type:     txEraId,
			Cbor:     txBytes,
			LastSeen: time.Now(),
		},
	)
	if err != nil {
		logger.Errorf("failed to add TX %s to mempool: %s", tx.Hash(), err)
		// XXX: do we want to return the error to the submitter?
		return nil
	}
	return nil
}
