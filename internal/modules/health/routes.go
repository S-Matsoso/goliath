package health

import "net/http"

// Routes mounts the health module's endpoints on the server's mux. It satisfies
// server.Module without importing the server package.
type Routes struct {
	handler *Handler
}

// NewRoutes returns the route set for the health module.
func NewRoutes(handler *Handler) *Routes {
	return &Routes{handler: handler}
}

// Register mounts the module's endpoints.
func (r *Routes) Register(mux *http.ServeMux) {
	mux.Handle("GET /health", http.HandlerFunc(r.handler.Health))
}
