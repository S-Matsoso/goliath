package server

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// stubModule registers a single GET route.
type stubModule struct{}

func (stubModule) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /stub", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func newTestHandler() http.Handler {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewHandler(log, stubModule{})
}

func decodeError(t *testing.T, body []byte) (code, requestID string) {
	t.Helper()
	var payload struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode body %q: %v", body, err)
	}
	return payload.Error.Code, payload.RequestID
}

func TestUnknownRouteReturnsJSON404(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/missing", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	code, requestID := decodeError(t, rec.Body.Bytes())
	if code != "NOT_FOUND" {
		t.Fatalf("code = %q, want NOT_FOUND", code)
	}
	if requestID == "" {
		t.Fatal("error response is missing request_id")
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
	code, _ := decodeError(t, rec.Body.Bytes())
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
