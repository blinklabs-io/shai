package txsubmit

import (
	"bytes"
	"fmt"
	"io"
	"net/http"

	"github.com/blinklabs-io/shai/internal/config"
)

func submitTxApi(txRawBytes []byte) error {
	cfg := config.GetConfig()
	reqBody := bytes.NewBuffer(txRawBytes)
	req, err := http.NewRequest(http.MethodPost, cfg.Submit.Url, reqBody)
	if err != nil {
		return fmt.Errorf("failed to create request: %s", err)
	}
	req.Header.Add("Content-Type", "application/cbor")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf(
			"failed to send request: %s: %s",
			cfg.Submit.Url,
			err,
		)
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
		return fmt.Errorf("failed to submit TX to API: %s: %d: %s", cfg.Submit.Url, resp.StatusCode, respBody)
	}
}
