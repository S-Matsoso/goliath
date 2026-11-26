// Package server assembles the HTTP server: module routes, middleware and the
// server lifecycle. It never imports a feature module.
package server

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/S-Matsoso/goliath/internal/metrics"
	"github.com/S-Matsoso/goliath/internal/middleware"
	"github.com/S-Matsoso/goliath/internal/platform/httpx"
	"github.com/S-Matsoso/goliath/internal/platform/router"
)

// Module is anything that can mount its own routes. Modules implement this
// structurally, so they never import the server package.
type Module interface {
	Register(*router.Router)
}

// endpointCatalog is the discovery payload consumed by the web console.
type endpointCatalog struct {
	Endpoints []router.Route `json:"endpoints"`
}

// NewHandler builds the root handler: module routes are mounted on a private
// Gin engine and wrapped in the standard middleware chain.
func NewHandler(log *slog.Logger, modules ...Module) http.Handler {
	return NewHandlerWithMetrics(log, nil, modules...)
}

// NewHandlerWithMetrics adds process request metrics to the standard middleware chain.
func NewHandlerWithMetrics(log *slog.Logger, collector *metrics.Collector, modules ...Module) http.Handler {
	// Request logging and recovery come from this project's middleware, so
	// Gin's debug output would only duplicate noise.
	gin.SetMode(gin.ReleaseMode)

	engine := gin.New()
	engine.HandleMethodNotAllowed = true

	engine.Use(middleware.RequestID())
	if collector != nil {
		engine.Use(collector.Middleware())
	}
	engine.Use(middleware.Logging(log))
	engine.Use(middleware.Recovery(log))

	routes := router.New(engine)
	for _, m := range modules {
		m.Register(routes)
	}

	// Served by the server rather than a module: it describes every mounted
	// route, including the module-provided ones.
	routes.Handle(http.MethodGet, "/api/endpoints", "List available endpoints", func(c *gin.Context) {
		c.JSON(http.StatusOK, endpointCatalog{Endpoints: routes.Routes()})
	})

	engine.NoRoute(func(c *gin.Context) {
		httpx.Error(c.Writer, c.Request, http.StatusNotFound, httpx.CodeNotFound, "Resource not found")
	})

	engine.NoMethod(func(c *gin.Context) {
		// Gin has already set the Allow header from the registered methods.
		httpx.Error(c.Writer, c.Request, http.StatusMethodNotAllowed, httpx.CodeMethodNotAllowed, "Method not allowed")
	})

	return engine
}
