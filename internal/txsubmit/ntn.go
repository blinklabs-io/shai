package txsubmit

import (
	"fmt"

	"github.com/blinklabs-io/shai/internal/logging"
)

func (t *TxSubmit) startNtn() error {
	go func() {
		logger := logging.GetLogger()
		for {
			txBytes, ok := <-t.transactionChan
			if !ok {
				return
			}
			if err := globalTxSubmit.node.AddOutboundTransaction(txBytes); err != nil {
				logger.Error(
					fmt.Sprintf(
						"failed to add transaction to outbound mempool: %s",
						err,
					),
				)
			}
		}
	}()
	return nil
}
