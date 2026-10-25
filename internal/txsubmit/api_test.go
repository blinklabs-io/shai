package txsubmit

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSubmitTxApiClassifiesResponse(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		rejected bool
	}{
		{name: "bad request", status: http.StatusBadRequest, rejected: true},
		{name: "request timeout", status: http.StatusRequestTimeout},
		{name: "conflict", status: http.StatusConflict},
		{name: "too early", status: http.StatusTooEarly},
		{name: "rate limited", status: http.StatusTooManyRequests},
		{name: "server error", status: http.StatusInternalServerError},
		{name: "gateway timeout", status: http.StatusGatewayTimeout},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertSubmissionResponseClassification(t, test.status, test.rejected)
		})
	}
}

func assertSubmissionResponseClassification(
	t *testing.T,
	status int,
	wantRejected bool,
) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		_ *http.Request,
	) {
		response.WriteHeader(status)
	}))
	defer server.Close()

	err := submitTxApi([]byte{0x80}, server.URL)
	if err == nil {
		t.Fatal("non-accepted response returned success")
	}
	var rejectedErr *submissionRejectedError
	if got := errors.As(err, &rejectedErr); got != wantRejected {
		t.Fatalf(
			"rejection classification = %t, want %t: %v",
			got,
			wantRejected,
			err,
		)
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
