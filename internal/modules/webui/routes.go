package webui

import (
	"net/http"

	"github.com/S-Matsoso/goliath/internal/platform/router"
)

// Routes mounts the console page. It satisfies server.Module without importing
// the server package.
type Routes struct {
	handler *Handler
}

// NewRoutes returns the route set for the console.
func NewRoutes(handler *Handler) *Routes {
	return &Routes{handler: handler}
}

// Register mounts the console at exactly "/" so unknown paths still return the
// API's JSON 404.
func (r *Routes) Register(rt *router.Router) {
	rt.Handle(http.MethodGet, "/", "Web console", r.handler.Index)
}
