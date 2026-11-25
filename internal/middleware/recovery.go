package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/S-Matsoso/goliath/internal/platform/httpx"
)

// Recovery converts handler panics into a logged 500 response with the standard
// error envelope, keeping the process alive.
func Recovery(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				rec := recover()
				if rec == nil {
					return
				}
				log.Error("panic recovered",
					"panic", rec,
					"method", r.Method,
					"path", r.URL.Path,
					"request_id", httpx.RequestID(r.Context()),
					"stack", string(debug.Stack()),
				)
				httpx.InternalError(w, r)
			}()

			next.ServeHTTP(w, r)
		})
	}
}
