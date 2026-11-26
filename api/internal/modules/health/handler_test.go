package health

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/S-Matsoso/goliath/internal/platform/router"
)

func newHealthEngine() (*gin.Engine, *router.Router) {
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	engine.HandleMethodNotAllowed = true
	routes := router.New(engine)
	NewRoutes(NewHandler(NewService("register-api"))).Register(routes)

	return engine, routes
}

func TestHealthRoute(t *testing.T) {
	engine, routes := newHealthEngine()

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Fatalf("Content-Type = %q, want JSON", ct)
	}

	var got Status
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	want := Status{Status: "ok", Service: "register-api"}
	if got != want {
		t.Fatalf("body = %+v, want %+v", got, want)
	}

	registered := routes.Routes()
	if len(registered) != 1 {
		t.Fatalf("registered %d routes, want 1", len(registered))
	}
	if registered[0].Method != http.MethodGet || registered[0].Path != "/health" || registered[0].Summary == "" {
		t.Fatalf("route metadata = %+v", registered[0])
	}
}

func TestHealthRouteMethodNotAllowed(t *testing.T) {
	engine, _ := newHealthEngine()

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/health", nil))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
	if allow := rec.Header().Get("Allow"); allow != "GET, HEAD" {
		t.Fatalf("Allow = %q, want %q", allow, "GET, HEAD")
	}
}
