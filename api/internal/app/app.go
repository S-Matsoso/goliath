// Package app wires configuration, department-scoped databases, handlers and
// the HTTP server together.
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
	"github.com/S-Matsoso/goliath/internal/dept/electoral"
	"github.com/S-Matsoso/goliath/internal/dept/grants"
	"github.com/S-Matsoso/goliath/internal/dept/health"
	"github.com/S-Matsoso/goliath/internal/dept/identity"
	"github.com/S-Matsoso/goliath/internal/dept/police"
	"github.com/S-Matsoso/goliath/internal/metrics"
	"github.com/S-Matsoso/goliath/internal/platform/auditlog"
	"github.com/S-Matsoso/goliath/internal/platform/router"
	"github.com/S-Matsoso/goliath/internal/platform/system"
	"github.com/S-Matsoso/goliath/internal/server"
	"github.com/S-Matsoso/goliath/internal/web"
)

type App struct {
	cfg   config.Config
	log   *slog.Logger
	srv   *server.Server
	pools *db.Pools
}

func New(cfg config.Config, log *slog.Logger) (*App, error) {
	pools, err := db.OpenPools(cfg.DepartmentDSNs, cfg.AuditDSN, cfg.AuthzDSN)
	if err != nil {
		return nil, fmt.Errorf("initialize department database pools: %w", err)
	}
	closeOnError := func(err error) (*App, error) {
		if closeErr := pools.Close(); closeErr != nil {
			return nil, errors.Join(err, fmt.Errorf("close database pools after startup failure: %w", closeErr))
		}
		return nil, err
	}

	auditWriter, err := audit.New(context.Background(), pools.Audit, cfg.NodeID)
	if err != nil {
		return closeOnError(fmt.Errorf("initialize audit writer: %w", err))
	}
	transit, err := crypto.NewVaultTransit(cfg.Vault.Address, cfg.Vault.Token, cfg.Vault.TransitMount)
	if err != nil {
		return closeOnError(fmt.Errorf("initialize Vault Transit client: %w", err))
	}
	authenticator, err := auth.NewOIDCAuthenticator(cfg.OIDC, cfg.DevHeaderAuth, pools.Authz, auditWriter)
	if err != nil {
		return closeOnError(fmt.Errorf("initialize OIDC authentication: %w", err))
	}

	identityRoutes, err := identity.NewHandler(
		pools, authenticator, transit, auditWriter,
		cfg.Vault.NIDEncryptionKey, cfg.Vault.NIDHMACKey,
	)
	if err != nil {
		return closeOnError(fmt.Errorf("initialize identity module: %w", err))
	}
	healthRoutes, err := health.NewHandler(
		pools, authenticator, transit, auditWriter,
		cfg.Vault.NIDEncryptionKey, cfg.Vault.NIDHMACKey,
	)
	if err != nil {
		return closeOnError(fmt.Errorf("initialize health module: %w", err))
	}
	electoralRoutes, err := electoral.NewHandler(
		pools, authenticator, transit, auditWriter, cfg.Vault.NIDHMACKey,
	)
	if err != nil {
		return closeOnError(fmt.Errorf("initialize electoral module: %w", err))
	}
	grantsRoutes, err := grants.NewHandler(
		pools, authenticator, transit, auditWriter, cfg.Vault.NIDHMACKey,
	)
	if err != nil {
		return closeOnError(fmt.Errorf("initialize grants module: %w", err))
	}
	policeRoutes, err := police.NewHandler(
		pools, authenticator, transit, auditWriter,
		cfg.Vault.NIDEncryptionKey, cfg.Vault.NIDHMACKey,
	)
	if err != nil {
		return closeOnError(fmt.Errorf("initialize police module: %w", err))
	}

	static, err := web.Handler()
	if err != nil {
		return closeOnError(fmt.Errorf("initialize embedded frontend: %w", err))
	}
	collector := metrics.New()
	profileRoutes := profileRoutes{authenticator: authenticator}
	auditRoutes := auditlog.NewHandler(pools, authenticator)
	systemRoutes := system.NewHandler(pools, authenticator, collector)
	srv, err := server.NewSecureWithMetrics(
		cfg.Server, log, collector, static,
		profileRoutes, systemRoutes,
		identityRoutes, healthRoutes, electoralRoutes, grantsRoutes, policeRoutes, auditRoutes,
	)
	if err != nil {
		return closeOnError(fmt.Errorf("initialize secure HTTP server: %w", err))
	}

	return &App{cfg: cfg, log: log, srv: srv, pools: pools}, nil
}

func (a *App) Run(ctx context.Context) error {
	a.log.Info("starting application", "name", a.cfg.AppName, "env", a.cfg.AppEnv)
	runErr := a.srv.Run(ctx)
	if closeErr := a.pools.Close(); closeErr != nil {
		return errors.Join(runErr, fmt.Errorf("close database pools: %w", closeErr))
	}
	return runErr
}

type profileRoutes struct {
	authenticator *auth.OIDCAuthenticator
}

func (r profileRoutes) Register(routes *router.Router) {
	auth.RegisterProfile(routes, r.authenticator)
}
