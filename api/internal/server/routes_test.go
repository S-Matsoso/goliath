package server

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/S-Matsoso/goliath/internal/platform/router"
)

// stubModule registers a single GET route.
type stubModule struct{}

func (stubModule) Register(r *router.Router) {
	r.Handle(http.MethodGet, "/stub", "Stub endpoint", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
}

func newTestHandler() http.Handler {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewHandler(log, stubModule{})
}

func decodeError(t *testing.T, body []byte) string {
	t.Helper()
	var payload struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode body %q: %v", body, err)
	}
	return payload.Error.Code
}

func TestUnknownRouteReturnsJSON404(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/missing", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	code := decodeError(t, rec.Body.Bytes())
	if code != "NOT_FOUND" {
		t.Fatalf("code = %q, want NOT_FOUND", code)
	}
	if rec.Header().Get("X-Request-ID") == "" {
		t.Fatal("error response is missing the X-Request-ID header")
	}
}

func TestMethodMismatchReturnsJSON405(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/stub", nil))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
	allow := rec.Header().Get("Allow")
	if !strings.Contains(allow, http.MethodGet) {
		t.Fatalf("Allow = %q, want it to include GET", allow)
	}
	code := decodeError(t, rec.Body.Bytes())
	if code != "METHOD_NOT_ALLOWED" {
		t.Fatalf("code = %q, want METHOD_NOT_ALLOWED", code)
	}
}

func TestKnownRouteStillServed(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/stub", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestEndpointCatalogListsMountedRoutes(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/endpoints", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var payload struct {
		Endpoints []router.Route `json:"endpoints"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	found := map[string]string{}
	for _, endpoint := range payload.Endpoints {
		found[endpoint.Method+" "+endpoint.Path] = endpoint.Summary
	}
	for _, key := range []string{"GET /stub", "GET /api/endpoints"} {
		if _, ok := found[key]; !ok {
			t.Fatalf("catalog is missing %q: %+v", key, payload.Endpoints)
		}
	}
	if found["GET /stub"] == "" {
		t.Fatal("catalog route is missing its summary")
	}
	if len(payload.Endpoints) != 2 {
		t.Fatalf("catalog has %d entries, want 2 (HEAD must not be listed)", len(payload.Endpoints))
	}
}
