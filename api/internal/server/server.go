// Package server owns the HTTP server lifecycle. It knows how to mount modules
// but never imports their implementations.
package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/S-Matsoso/goliath/internal/config"
	"github.com/S-Matsoso/goliath/internal/metrics"
)

// Server wraps http.Server with structured logging and graceful shutdown.
type Server struct {
	httpServer      *http.Server
	log             *slog.Logger
	shutdownTimeout time.Duration
}

// New constructs the server from configuration and the modules to mount.
func New(cfg config.Server, log *slog.Logger, modules ...Module) *Server {
	return NewWithMetrics(cfg, log, nil, modules...)
}

// NewWithMetrics constructs the server and instruments each HTTP request.
func NewWithMetrics(cfg config.Server, log *slog.Logger, collector *metrics.Collector, modules ...Module) *Server {
	return &Server{
		httpServer: &http.Server{
			Addr:              cfg.Addr(),
			Handler:           NewHandlerWithMetrics(log, collector, modules...),
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       cfg.ReadTimeout,
			WriteTimeout:      cfg.WriteTimeout,
			IdleTimeout:       cfg.IdleTimeout,
		},
		log:             log,
		shutdownTimeout: cfg.ShutdownTimeout,
	}
}

// Run serves requests until ctx is cancelled or the listener fails. It then
// shuts the server down gracefully within the configured timeout.
func (s *Server) Run(ctx context.Context) error {
	serveErr := make(chan error, 1)
	go func() {
		s.log.Info("http server listening", "addr", s.httpServer.Addr)
		if err := s.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
	}()

	select {
	case <-ctx.Done():
	case err := <-serveErr:
		return fmt.Errorf("serve: %w", err)
	}

	s.log.Info("shutting down http server", "timeout", s.shutdownTimeout)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), s.shutdownTimeout)
	defer cancel()
	if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	s.log.Info("http server stopped")
	return nil
}
