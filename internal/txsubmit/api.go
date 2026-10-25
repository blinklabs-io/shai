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
	"time"

	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/shai/internal/logging"
)

const (
	submitTxRequestTimeout  = 30 * time.Second
	maxSubmitTxResponseSize = 64 << 10
)

func (t *TxSubmit) startApi(url string) error {
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
			// Determine transaction type (era)
			txType, err := ledger.DetermineTransactionType(txBytes)
			if err != nil {
				logger.Error(
					"could not parse transaction to determine type:",
					"error:",
					err,
				)
				return
			}
			tx, err := ledger.NewTransactionFromCbor(txType, txBytes)
			if err != nil {
				logger.Error("failed to parse transaction CBOR:", "error:", err)
				return
			}
			// Submit transaction
			if err := submitTxApi(txBytes, url); err != nil {
				var rejectedErr *submissionRejectedError
				if errors.As(err, &rejectedErr) {
					t.node.RejectOwnedTransaction(queued.owned)
				} else {
					t.node.EndOwnedTransactionSubmission(queued.owned)
				}
				logger.Error(
					"failed to submit transaction via API:",
					"txHash",
					tx.Hash(),
					"error:",
					err,
				)
			} else {
				t.node.EndOwnedTransactionSubmission(queued.owned)
				logger.Info("successfully submitted transaction via API", "txHash", tx.Hash())
			}
		}
	}()
	return nil
}

type submissionRejectedError struct {
	url        string
	statusCode int
	body       []byte
}

func (e *submissionRejectedError) Error() string {
	return fmt.Sprintf(
		"unexpected response: %s: %d: %s",
		e.url,
		e.statusCode,
		e.body,
	)
}

func submitTxApi(txRawBytes []byte, url string) error {
	reqBody := bytes.NewBuffer(txRawBytes)
	req, err := http.NewRequest(http.MethodPost, url, reqBody)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Add("Content-Type", "application/cbor")
	client := &http.Client{Timeout: submitTxRequestTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %s: %w", url, err)
	}
	if resp == nil {
		return errors.New("failed with nil response")
	}
	defer resp.Body.Close() //nolint:errcheck
	respBody, err := io.ReadAll(
		io.LimitReader(resp.Body, maxSubmitTxResponseSize+1),
	)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}
	if len(respBody) > maxSubmitTxResponseSize {
		respBody = []byte(fmt.Sprintf(
			"response body exceeds %d bytes",
			maxSubmitTxResponseSize,
		))
	}
	if resp.StatusCode == 202 {
		return nil
	}
	if !isDefinitiveSubmissionRejection(resp.StatusCode) {
		return fmt.Errorf(
			"ambiguous response: %s: %d: %s",
			url,
			resp.StatusCode,
			respBody,
		)
	}
	return &submissionRejectedError{
		url:        url,
		statusCode: resp.StatusCode,
		body:       respBody,
	}
}

func isDefinitiveSubmissionRejection(statusCode int) bool {
	switch statusCode {
	case http.StatusBadRequest,
		http.StatusUnauthorized,
		http.StatusPaymentRequired,
		http.StatusForbidden,
		http.StatusNotFound,
		http.StatusMethodNotAllowed,
		http.StatusNotAcceptable,
		http.StatusProxyAuthRequired,
		http.StatusGone,
		http.StatusLengthRequired,
		http.StatusPreconditionFailed,
		http.StatusRequestEntityTooLarge,
		http.StatusRequestURITooLong,
		http.StatusUnsupportedMediaType,
		http.StatusRequestedRangeNotSatisfiable,
		http.StatusExpectationFailed,
		http.StatusMisdirectedRequest,
		http.StatusUnprocessableEntity,
		http.StatusUpgradeRequired,
		http.StatusPreconditionRequired,
		http.StatusRequestHeaderFieldsTooLarge,
		http.StatusUnavailableForLegalReasons:
		return true
	default:
		return false
	}
}
