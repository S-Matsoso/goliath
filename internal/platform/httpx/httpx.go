// Package httpx contains helpers for writing JSON responses and consistent API
// errors, plus the request-ID context plumbing shared by middleware and
// handlers.
package httpx

import (
	"context"
	"encoding/json"
	"net/http"
)

// Stable machine-readable error codes returned by the API.
const (
	CodeBadRequest       = "BAD_REQUEST"
	CodeNotFound         = "NOT_FOUND"
	CodeMethodNotAllowed = "METHOD_NOT_ALLOWED"
	CodeInternal         = "INTERNAL_ERROR"
)

// MessageInternal is the only message ever sent to clients for server-side
// failures, so internal details never leak.
const MessageInternal = "An unexpected error occurred"

type errorPayload struct {
	Error     errorDetail `json:"error"`
	RequestID string      `json:"request_id"`
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type requestIDKey struct{}

// WithRequestID returns a context carrying the given request ID.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}

// RequestID returns the request ID stored in ctx, or "" when none is set.
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

// JSON writes v as a JSON response with the given status code. The payloads
// written by this API are plain structs and maps, so encoding cannot fail; if
// it somehow did, the status line is already sent and the response is truncated.
func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// Error writes a consistent JSON error response using the request ID from the
// request context, if any.
func Error(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	JSON(w, status, errorPayload{
		Error:     errorDetail{Code: code, Message: message},
		RequestID: RequestID(r.Context()),
	})
}

// InternalError writes the standard internal-server-error response without
// exposing implementation details.
func InternalError(w http.ResponseWriter, r *http.Request) {
	Error(w, r, http.StatusInternalServerError, CodeInternal, MessageInternal)
}
