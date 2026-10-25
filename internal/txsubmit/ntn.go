package txsubmit

import (
	"fmt"

	"github.com/blinklabs-io/shai/internal/logging"
)

func (t *TxSubmit) startNtn() error {
	go func() {
		logger := logging.GetLogger()
		for {
			queued, ok := <-t.transactionChan
			if !ok {
				return
			}
			if !t.node.BeginOwnedTransactionSubmission(queued.owned) {
				continue
			}
			txBytes := queued.bytes
			if err := t.node.AddOutboundTransaction(txBytes); err != nil {
				t.node.RejectOwnedTransaction(queued.owned)
				logger.Error(
					fmt.Sprintf(
						"failed to add transaction to outbound mempool: %s",
						err,
					),
				)
			} else {
				t.node.EndOwnedTransactionSubmission(queued.owned)
			}
		}
	}()
	return nil
}
