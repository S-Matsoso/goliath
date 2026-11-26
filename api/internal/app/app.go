// Package app wires configuration, logging, modules and the HTTP server
// together. It is the only package that knows which modules exist.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/S-Matsoso/goliath/internal/audit"
	"github.com/S-Matsoso/goliath/internal/auth"
	"github.com/S-Matsoso/goliath/internal/config"
	"github.com/S-Matsoso/goliath/internal/crypto"
	"github.com/S-Matsoso/goliath/internal/db"
	"github.com/S-Matsoso/goliath/internal/metrics"
	"github.com/S-Matsoso/goliath/internal/modules/admin"
	"github.com/S-Matsoso/goliath/internal/modules/health"
	"github.com/S-Matsoso/goliath/internal/modules/identity"
	"github.com/S-Matsoso/goliath/internal/modules/registry"
	"github.com/S-Matsoso/goliath/internal/server"
	"gorm.io/gorm"
)

// App owns the constructed application graph.
type App struct {
	cfg      config.Config
	log      *slog.Logger
	srv      *server.Server
	database *gorm.DB
}

// New builds the application: dependencies are constructed here and injected
// into the components that need them. Database schema migration completes
// before the HTTP server is constructed.
func New(cfg config.Config, log *slog.Logger) (*App, error) {
	database, err := db.Open(cfg.Database)
	if err != nil {
		return nil, fmt.Errorf("initialize database: %w", err)
	}
	if err := db.AutoMigrate(database); err != nil {
		if closeErr := db.Close(database); closeErr != nil {
			return nil, errors.Join(err, fmt.Errorf("close database after migration failure: %w", closeErr))
		}
		return nil, err
	}
	closeOnError := func(err error) (*App, error) {
		if closeErr := db.Close(database); closeErr != nil {
			return nil, errors.Join(err, fmt.Errorf("close database after startup failure: %w", closeErr))
		}
		return nil, err
	}

	auditWriter, err := audit.New(context.Background(), database, cfg.NodeID)
	if err != nil {
		return closeOnError(fmt.Errorf("initialize audit writer: %w", err))
	}
	transit, err := crypto.NewVaultTransit(cfg.Vault.Address, cfg.Vault.Token, cfg.Vault.TransitMount)
	if err != nil {
		return closeOnError(fmt.Errorf("initialize Vault Transit client: %w", err))
	}
	if err := auth.BootstrapAdmin(context.Background(), database, cfg.BootstrapAdmin.Username, cfg.BootstrapAdmin.Password); err != nil {
		return closeOnError(fmt.Errorf("initialize administrator account: %w", err))
	}
	authenticator := auth.New(database, auditWriter)

	healthService := health.NewService(cfg.AppName, database)
	healthHandler := health.NewHandler(healthService)
	healthRoutes := health.NewRoutes(healthHandler)

	collector := metrics.New()
	adminRoutes := admin.NewHandler(database, authenticator, auditWriter, collector, cfg.AppName)
	identityRoutes := identity.NewHandler(
		database,
		authenticator,
		transit,
		auditWriter,
		cfg.Vault.NIDEncryptionKey,
		cfg.Vault.NIDHMACKey,
	)
	registryRoutes := registry.NewHandler(database, authenticator, transit, auditWriter, cfg.Vault.NIDHMACKey)

	srv := server.NewWithMetrics(cfg.Server, log, collector, healthRoutes, adminRoutes, identityRoutes, registryRoutes)

	return &App{cfg: cfg, log: log, srv: srv, database: database}, nil
}

// Run starts the application and blocks until ctx is cancelled or the server
// stops, performing graceful shutdown.
func (a *App) Run(ctx context.Context) error {
	a.log.Info("starting application", "name", a.cfg.AppName, "env", a.cfg.AppEnv)
	runErr := a.srv.Run(ctx)
	if closeErr := db.Close(a.database); closeErr != nil {
		return errors.Join(runErr, fmt.Errorf("close database: %w", closeErr))
	}
	return runErr
}
