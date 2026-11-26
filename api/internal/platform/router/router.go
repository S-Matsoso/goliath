// Package router mounts module routes on a Gin engine while recording them, so
// the endpoint catalog always matches what the server actually serves.
package router

import (
	"fmt"
	"net/http"
	"sort"

	"github.com/gin-gonic/gin"
)

// Route describes one mounted endpoint.
type Route struct {
	Method     string `json:"method"`
	Path       string `json:"path"`
	Summary    string `json:"summary"`
	Protected  bool   `json:"-"`
	Department string `json:"-"`
	MinRole    string `json:"-"`
}

// Router wraps a Gin engine and keeps a record of every route registered
// through it. Routes registered directly on the engine would be invisible to
// the catalog, so modules must use this type.
type Router struct {
	engine *gin.Engine
	routes []Route
}

// New returns a Router that mounts onto engine.
func New(engine *gin.Engine) *Router {
	return &Router{engine: engine}
}

// Handle mounts handlers at "METHOD path" and records the route for discovery.
// GET routes also answer HEAD, matching standard HTTP behaviour, without
// appearing twice in the catalog.
func (r *Router) Handle(method, path, summary string, handlers ...gin.HandlerFunc) {
	r.handle(method, path, summary, "", "", handlers...)
}

func (r *Router) HandleProtected(method, path, summary, department, minimumRole string, middleware gin.HandlerFunc, handler gin.HandlerFunc) {
	r.handle(method, path, summary, department, minimumRole, middleware, handler)
}

func (r *Router) handle(method, path, summary, department, minimumRole string, handlers ...gin.HandlerFunc) {
	r.engine.Handle(method, path, handlers...)
	if method == http.MethodGet {
		r.engine.Handle(http.MethodHead, path, handlers...)
	}

	protected := department != "" && minimumRole != "" && len(handlers) >= 2 && handlers[0] != nil
	r.routes = append(r.routes, Route{
		Method:     method,
		Path:       path,
		Summary:    summary,
		Protected:  protected,
		Department: department,
		MinRole:    minimumRole,
	})
}

// Routes returns a copy of the recorded routes, sorted by path then method, so
// callers cannot mutate the registry.
func (r *Router) Routes() []Route {
	routes := make([]Route, len(r.routes))
	copy(routes, r.routes)

	sort.Slice(routes, func(i, j int) bool {
		if routes[i].Path != routes[j].Path {
			return routes[i].Path < routes[j].Path
		}
		return routes[i].Method < routes[j].Method
	})
	return routes
}

func (r *Router) ValidateAuthCoverage() error {
	for _, route := range r.routes {
		if route.Path == "/healthz" || route.Path == "/api/endpoints" {
			continue
		}
		if !route.Protected {
			return fmt.Errorf("route %s %s lacks an authentication declaration", route.Method, route.Path)
		}
	}
	return nil
}
