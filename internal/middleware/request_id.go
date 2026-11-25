// Package middleware contains reusable, composable HTTP middleware.
package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strconv"
	"time"

	"github.com/S-Matsoso/goliath/internal/platform/httpx"
)

// HeaderRequestID is the request/response header carrying the correlation ID.
const HeaderRequestID = "X-Request-ID"

const maxRequestIDLen = 128

// RequestID assigns every request a correlation ID, exposes it on the response
// header and stores it in the request context for handlers and error responses.
// A caller-supplied ID is reused when it is well formed.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(HeaderRequestID)
		if !validRequestID(id) {
			id = newRequestID()
		}
		w.Header().Set(HeaderRequestID, id)
		next.ServeHTTP(w, r.WithContext(httpx.WithRequestID(r.Context(), id)))
	})
}

// validRequestID accepts short, header-safe IDs (letters, digits, '-' and '_').
func validRequestID(id string) bool {
	if id == "" || len(id) > maxRequestIDLen {
		return false
	}
	for _, c := range id {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-', c == '_':
		default:
			return false
		}
	}
	return true
}

func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand failure is exceptionally rare; a timestamp still gives a
		// usable correlation value rather than an empty header.
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(b[:])
}
