package httpx

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestJSON(t *testing.T) {
	rec := httptest.NewRecorder()

	JSON(rec, http.StatusCreated, map[string]string{"status": "ok"})

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Fatalf("Content-Type = %q, want JSON", ct)
	}
	if want := "{\"status\":\"ok\"}\n"; rec.Body.String() != want {
		t.Fatalf("body = %q, want %q", rec.Body.String(), want)
	}
}

func TestError(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/missing", nil)
	req = req.WithContext(WithRequestID(req.Context(), "request-123"))
	rec := httptest.NewRecorder()

	Error(rec, req, http.StatusNotFound, CodeNotFound, "Resource not found")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}

	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Error.Code != CodeNotFound {
		t.Fatalf("code = %q, want %q", body.Error.Code, CodeNotFound)
	}
	if body.Error.Message != "Resource not found" {
		t.Fatalf("message = %q", body.Error.Message)
	}
	if got := rec.Body.String(); got != "{\"error\":{\"code\":\"NOT_FOUND\",\"message\":\"Resource not found\"}}\n" {
		t.Fatalf("error body = %q, want the documented error shape", got)
	}
}

func TestInternalErrorHidesDetails(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	InternalError(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}

	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Error.Code != CodeInternal {
		t.Fatalf("code = %q, want %q", body.Error.Code, CodeInternal)
	}
	if body.Error.Message != MessageInternal {
		t.Fatalf("message = %q, want the generic message", body.Error.Message)
	}
}

func TestRequestIDContext(t *testing.T) {
	if got := RequestID(context.Background()); got != "" {
		t.Fatalf("RequestID(empty) = %q, want empty", got)
	}

	ctx := WithRequestID(context.Background(), "abc")
	if got := RequestID(ctx); got != "abc" {
		t.Fatalf("RequestID(ctx) = %q, want %q", got, "abc")
	}
}
