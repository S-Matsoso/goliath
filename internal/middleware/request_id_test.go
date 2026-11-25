package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/S-Matsoso/goliath/internal/platform/httpx"
)

func TestRequestIDGeneratesID(t *testing.T) {
	var fromContext string
	handler := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fromContext = httpx.RequestID(r.Context())
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

	header := rec.Header().Get(HeaderRequestID)
	if header == "" {
		t.Fatal("response is missing X-Request-ID")
	}
	if len(header) != 32 {
		t.Fatalf("generated ID length = %d, want 32 hex characters", len(header))
	}
	if fromContext != header {
		t.Fatalf("context ID = %q, want header %q", fromContext, header)
	}
}

func TestRequestIDReusesClientID(t *testing.T) {
	var fromContext string
	handler := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fromContext = httpx.RequestID(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set(HeaderRequestID, "client-request_123")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get(HeaderRequestID); got != "client-request_123" {
		t.Fatalf("response header = %q, want the client ID", got)
	}
	if fromContext != "client-request_123" {
		t.Fatalf("context ID = %q, want the client ID", fromContext)
	}
}

func TestRequestIDReplacesInvalidID(t *testing.T) {
	handler := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	for _, invalid := range []string{"bad id", "id\twith\ttabs", "semi;colon"} {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		req.Header.Set(HeaderRequestID, invalid)

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		got := rec.Header().Get(HeaderRequestID)
		if got == invalid || !validRequestID(got) {
			t.Fatalf("input %q was not replaced with a valid ID, got %q", invalid, got)
		}
	}
}
