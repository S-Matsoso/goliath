package middleware

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/S-Matsoso/goliath/internal/platform/httpx"
)

func TestRecoveryConvertsPanicTo500(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(RequestID(), Recovery(log))
	engine.GET("/panic", func(c *gin.Context) {
		panic("boom: secret internal detail")
	})

	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	req.Header.Set(HeaderRequestID, "panic-test")

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}

	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Error.Code != httpx.CodeInternal {
		t.Fatalf("code = %q, want %q", body.Error.Code, httpx.CodeInternal)
	}
	if body.Error.Message != httpx.MessageInternal {
		t.Fatalf("message = %q, want the generic message", body.Error.Message)
	}
	if body.RequestID != "panic-test" {
		t.Fatalf("request_id = %q, want %q", body.RequestID, "panic-test")
	}
	if strings.Contains(rec.Body.String(), "secret") {
		t.Fatal("panic details leaked into the response body")
	}
}

func TestRecoveryPassesThroughNormalRequests(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(Recovery(log))
	engine.GET("/ok", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ok", nil))

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}
