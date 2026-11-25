package webui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/S-Matsoso/goliath/internal/platform/router"
)

func newConsoleEngine() *gin.Engine {
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	NewRoutes(NewHandler()).Register(router.New(engine))
	return engine
}

func TestIndexServesConsole(t *testing.T) {
	rec := httptest.NewRecorder()
	newConsoleEngine().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Fatalf("Content-Type = %q, want HTML", ct)
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("missing X-Content-Type-Options: nosniff")
	}

	body := rec.Body.String()
	for _, want := range []string{"Register API Console", "/health", "request-form", "/api/endpoints", "endpoints"} {
		if !strings.Contains(body, want) {
			t.Fatalf("page does not contain %q", want)
		}
	}
}

func TestIndexOnlyMatchesRoot(t *testing.T) {
	rec := httptest.NewRecorder()
	newConsoleEngine().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/assets/app.js", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d for an unknown asset", rec.Code, http.StatusNotFound)
	}
}
