package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/S-Matsoso/goliath/internal/platform/httpx"
)

func newRequestIDEngine(capture func(string)) *gin.Engine {
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	engine.Use(RequestID())
	engine.GET("/health", func(c *gin.Context) {
		if capture != nil {
			capture(httpx.RequestID(c.Request.Context()))
		}
		c.Status(http.StatusOK)
	})
	return engine
}

func TestRequestIDGeneratesID(t *testing.T) {
	var fromContext string
	engine := newRequestIDEngine(func(id string) { fromContext = id })

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

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
	engine := newRequestIDEngine(func(id string) { fromContext = id })

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set(HeaderRequestID, "client-request_123")

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if got := rec.Header().Get(HeaderRequestID); got != "client-request_123" {
		t.Fatalf("response header = %q, want the client ID", got)
	}
	if fromContext != "client-request_123" {
		t.Fatalf("context ID = %q, want the client ID", fromContext)
	}
}

func TestRequestIDReplacesInvalidID(t *testing.T) {
	engine := newRequestIDEngine(nil)

	for _, invalid := range []string{"bad id", "id\twith\ttabs", "semi;colon", "1234567890123456789012345678901234567"} {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		req.Header.Set(HeaderRequestID, invalid)

		rec := httptest.NewRecorder()
		engine.ServeHTTP(rec, req)

		got := rec.Header().Get(HeaderRequestID)
		if got == invalid || !validRequestID(got) {
			t.Fatalf("input %q was not replaced with a valid ID, got %q", invalid, got)
		}
	}
}
