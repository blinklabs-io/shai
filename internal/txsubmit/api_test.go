package txsubmit

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSubmitTxApiClassifiesExplicitRejection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		_ *http.Request,
	) {
		response.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	err := submitTxApi([]byte{0x80}, server.URL)
	var rejectedErr *submissionRejectedError
	if !errors.As(err, &rejectedErr) {
		t.Fatalf("explicit rejection was not classified: %v", err)
	}
}

func TestSubmitTxApiKeepsTransportFailureAmbiguous(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		_ *http.Request,
	) {
		hijacker := response.(http.Hijacker)
		conn, _, err := hijacker.Hijack()
		if err != nil {
			t.Errorf("hijack connection: %v", err)
			return
		}
		_ = conn.Close()
	}))
	defer server.Close()

	err := submitTxApi([]byte{0x80}, server.URL)
	if err == nil {
		t.Fatal("transport failure returned success")
	}
	var rejectedErr *submissionRejectedError
	if errors.As(err, &rejectedErr) {
		t.Fatalf("transport failure was classified as rejection: %v", err)
	}
}
