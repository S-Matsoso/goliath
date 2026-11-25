package server

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/S-Matsoso/goliath/internal/middleware"
	"github.com/S-Matsoso/goliath/internal/platform/httpx"
)

// Module is anything that can mount its own routes. Modules implement this
// structurally, so they never import the server package.
type Module interface {
	Register(mux *http.ServeMux)
}

// NewHandler builds the root handler: module routes are mounted on a private
// ServeMux and wrapped in the standard middleware chain.
func NewHandler(log *slog.Logger, modules ...Module) http.Handler {
	mux := http.NewServeMux()
	for _, m := range modules {
		m.Register(mux)
	}

	var handler http.Handler = jsonFallback(mux)

	// Outermost first: the request ID must exist before logging or recovery
	// emit a line, and logging must wrap recovery to record panic responses.
	handler = middleware.Recovery(log)(handler)
	handler = middleware.Logging(log)(handler)
	handler = middleware.RequestID(handler)

	return handler
}

// probeMethods are the methods checked when a request matches no route, used to
// tell an unknown path apart from a method mismatch.
var probeMethods = []string{
	http.MethodGet,
	http.MethodHead,
	http.MethodPost,
	http.MethodPut,
	http.MethodPatch,
	http.MethodDelete,
	http.MethodOptions,
}

// jsonFallback serves the standard JSON error envelope for unmatched requests.
// ServeMux reports method mismatches identically to unknown paths, so the path
// is probed with the other methods to preserve correct 405 semantics.
func jsonFallback(mux *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h, pattern := mux.Handler(r); pattern != "" {
			h.ServeHTTP(w, r)
			return
		}

		if allowed := allowedMethods(mux, r); len(allowed) > 0 {
			w.Header().Set("Allow", strings.Join(allowed, ", "))
			httpx.Error(w, r, http.StatusMethodNotAllowed, httpx.CodeMethodNotAllowed, "Method not allowed")
			return
		}

		httpx.Error(w, r, http.StatusNotFound, httpx.CodeNotFound, "Resource not found")
	})
}

// allowedMethods returns the methods the mux serves for r's path, excluding
// r's own method.
func allowedMethods(mux *http.ServeMux, r *http.Request) []string {
	var allowed []string
	for _, method := range probeMethods {
		if method == r.Method {
			continue
		}
		probe := r.Clone(r.Context())
		probe.Method = method
		if _, pattern := mux.Handler(probe); pattern != "" {
			allowed = append(allowed, method)
		}
	}
	return allowed
}
