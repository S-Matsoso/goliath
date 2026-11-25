package health

import (
	"net/http"

	"github.com/S-Matsoso/goliath/internal/platform/router"
)

// Routes mounts the health module's endpoints on the server's router. It
// satisfies server.Module without importing the server package.
type Routes struct {
	handler *Handler
}

// NewRoutes returns the route set for the health module.
func NewRoutes(handler *Handler) *Routes {
	return &Routes{handler: handler}
}

// Register mounts the module's endpoints.
func (r *Routes) Register(rt *router.Router) {
	rt.Handle(http.MethodGet, "/health", "Service health check", r.handler.Health)
}
