package node

import (
	"errors"
	"fmt"

	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/protocol/localtxsubmission"
)

func (n *Node) localTxsubmissionServerSubmitTx(
	_ localtxsubmission.CallbackContext,
	msgSubmitTxTransaction localtxsubmission.MsgSubmitTxTransaction,
) error {
	txEraId := uint(msgSubmitTxTransaction.EraId)
	txBytes, ok := msgSubmitTxTransaction.Raw.Content.([]byte)
	if !ok {
		return errors.New("invalid transaction payload")
	}
	_, err := ledger.NewTransactionFromCbor(txEraId, txBytes)
	if err != nil {
		return fmt.Errorf("invalid transaction CBOR: %w", err)
	}
	return errors.New("local transaction submission is not supported")
}
