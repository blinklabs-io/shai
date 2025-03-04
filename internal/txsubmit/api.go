// Copyright 2025 Blink Labs Software
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package txsubmit

import (
	"bytes"
	"errors"
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
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Add("Content-Type", "application/cbor")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %s: %w", url, err)
	}
	if resp == nil {
		return errors.New("failed with nil response")
	}
	// We have to read the entire response body and close it to prevent a memory leak
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == 202 {
		return nil
	} else {
		return fmt.Errorf("unexpected response: %s: %d: %s", url, resp.StatusCode, respBody)
	}
}
