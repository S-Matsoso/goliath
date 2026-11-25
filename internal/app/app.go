// Package app wires configuration, logging, modules and the HTTP server
// together. It is the only package that knows which modules exist.
package app

import (
	"context"
	"log/slog"

	"github.com/S-Matsoso/goliath/internal/config"
	"github.com/S-Matsoso/goliath/internal/modules/health"
	"github.com/S-Matsoso/goliath/internal/server"
)

// App owns the constructed application graph.
type App struct {
	cfg config.Config
	log *slog.Logger
	srv *server.Server
}

// New builds the application: dependencies are constructed here and injected
// into the components that need them. Add future modules to this function only.
func New(cfg config.Config, log *slog.Logger) *App {
	healthService := health.NewService(cfg.AppName)
	healthHandler := health.NewHandler(healthService)
	healthRoutes := health.NewRoutes(healthHandler)

	srv := server.New(cfg.Server, log, healthRoutes)

	return &App{cfg: cfg, log: log, srv: srv}
}

// Run starts the application and blocks until ctx is cancelled or the server
// stops, performing graceful shutdown.
func (a *App) Run(ctx context.Context) error {
	a.log.Info("starting application", "name", a.cfg.AppName, "env", a.cfg.AppEnv)
	return a.srv.Run(ctx)
}
