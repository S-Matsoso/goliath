// Package server owns the HTTP server lifecycle. It knows how to mount modules
// but never imports their implementations.
package server

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/S-Matsoso/goliath/internal/config"
	"github.com/S-Matsoso/goliath/internal/metrics"
)

// Server wraps http.Server with structured logging and graceful shutdown.
type Server struct {
	httpServer      *http.Server
	log             *slog.Logger
	shutdownTimeout time.Duration
	tlsEnabled      bool
}

// New constructs the server from configuration and the modules to mount.
func New(cfg config.Server, log *slog.Logger, modules ...Module) *Server {
	return NewWithMetrics(cfg, log, nil, modules...)
}

// NewWithMetrics constructs the server and instruments each HTTP request.
func NewWithMetrics(cfg config.Server, log *slog.Logger, collector *metrics.Collector, modules ...Module) *Server {
	server, _ := newServer(cfg, log, collector, nil, false, modules...)
	return server
}

func NewSecureWithMetrics(cfg config.Server, log *slog.Logger, collector *metrics.Collector, static http.Handler, modules ...Module) (*Server, error) {
	return newServer(cfg, log, collector, static, true, modules...)
}

func newServer(cfg config.Server, log *slog.Logger, collector *metrics.Collector, static http.Handler, secure bool, modules ...Module) (*Server, error) {
	var handler http.Handler
	var err error
	if secure {
		handler, err = NewSecureHandlerWithMetrics(log, collector, static, modules...)
	} else {
		handler = NewHandlerWithMetrics(log, collector, modules...)
	}
	if err != nil {
		return nil, err
	}
	tlsConfig, err := serverTLSConfig(cfg)
	if err != nil {
		return nil, err
	}
	return &Server{
		httpServer: &http.Server{
			Addr:              cfg.Addr(),
			Handler:           handler,
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       cfg.ReadTimeout,
			WriteTimeout:      cfg.WriteTimeout,
			IdleTimeout:       cfg.IdleTimeout,
			TLSConfig:         tlsConfig,
		},
		log:             log,
		shutdownTimeout: cfg.ShutdownTimeout,
		tlsEnabled:      cfg.TLSCert != "",
	}, nil
}

// Run serves requests until ctx is cancelled or the listener fails. It then
// shuts the server down gracefully within the configured timeout.
func (s *Server) Run(ctx context.Context) error {
	serveErr := make(chan error, 1)
	go func() {
		s.log.Info("http server listening", "addr", s.httpServer.Addr)
		var err error
		if s.tlsEnabled {
			err = s.httpServer.ListenAndServeTLS("", "")
		} else {
			err = s.httpServer.ListenAndServe()
		}
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
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

func serverTLSConfig(cfg config.Server) (*tls.Config, error) {
	if cfg.TLSCert == "" && cfg.TLSKey == "" {
		return nil, nil
	}
	certificate, err := tls.LoadX509KeyPair(cfg.TLSCert, cfg.TLSKey)
	if err != nil {
		return nil, fmt.Errorf("load server TLS certificate: %w", err)
	}
	result := &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{certificate},
	}
	if cfg.TLSClientCA != "" {
		file, err := os.Open(cfg.TLSClientCA)
		if err != nil {
			return nil, fmt.Errorf("open client CA file: %w", err)
		}
		defer file.Close()
		caPEM, err := io.ReadAll(io.LimitReader(file, 1<<20))
		if err != nil {
			return nil, fmt.Errorf("read client CA file: %w", err)
		}
		clientCAs := x509.NewCertPool()
		if !clientCAs.AppendCertsFromPEM(caPEM) {
			return nil, errors.New("client CA file contains no valid certificates")
		}
		result.ClientCAs = clientCAs
		result.ClientAuth = tls.RequireAndVerifyClientCert
	}
	return result, nil
}
