package txsubmit

import (
	"bytes"
	"fmt"
	"io"
	"net/http"

	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/shai/internal/logging"
)

func (t *TxSubmit) startApi(url string) error {
	go func() {
		logger := logging.GetLogger()
		for {
			txBytes, ok := <-t.transactionChan
			if !ok {
				return
			}
			// Determine transaction type (era)
			txType, err := ledger.DetermineTransactionType(txBytes)
			if err != nil {
				logger.Error("could not parse transaction to determine type:", "error:", err)
				return
			}
			tx, err := ledger.NewTransactionFromCbor(txType, txBytes)
			if err != nil {
				logger.Error("failed to parse transaction CBOR:", "error:", err)
				return
			}
			// Submit transaction
			if err := submitTxApi(txBytes, url); err != nil {
				logger.Error("failed to submit transaction via API:", "txHash", tx.Hash(), "error:", err)
			} else {
				logger.Info("successfully submitted transaction via API", "txHash", tx.Hash())
			}
		}
	}()
	return nil
}

func submitTxApi(txRawBytes []byte, url string) error {
	reqBody := bytes.NewBuffer(txRawBytes)
	req, err := http.NewRequest(http.MethodPost, url, reqBody)
	if err != nil {
		return fmt.Errorf("failed to create request: %s", err)
	}
	req.Header.Add("Content-Type", "application/cbor")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %s: %s", url, err)
	}
	if resp == nil {
		return fmt.Errorf("failed with nil response")
	}
	// We have to read the entire response body and close it to prevent a memory leak
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %s", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == 202 {
		return nil
	} else {
		return fmt.Errorf("unexpected response: %s: %d: %s", url, resp.StatusCode, respBody)
	}
}
