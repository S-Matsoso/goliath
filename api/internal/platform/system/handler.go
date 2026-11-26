package system

import (
	"context"
	"net/http"
	"time"

	"github.com/S-Matsoso/goliath/internal/auth"
	"github.com/S-Matsoso/goliath/internal/db"
	"github.com/S-Matsoso/goliath/internal/metrics"
	"github.com/S-Matsoso/goliath/internal/platform/httpx"
	"github.com/S-Matsoso/goliath/internal/platform/router"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type Handler struct {
	pools     *db.Pools
	auth      *auth.OIDCAuthenticator
	collector *metrics.Collector
}

type componentStatus struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

type databasePoolMetrics struct {
	Name               string `json:"name"`
	OpenConnections    int    `json:"open_connections"`
	InUseConnections   int    `json:"in_use_connections"`
	IdleConnections    int    `json:"idle_connections"`
	MaxConnections     int    `json:"max_connections"`
	WaitCount          int64  `json:"wait_count"`
	WaitDurationMillis int64  `json:"wait_duration_ms"`
}

type adminMetricsResponse struct {
	metrics.Snapshot
	DatabasePools []databasePoolMetrics `json:"database_pools"`
}

func NewHandler(pools *db.Pools, authenticator *auth.OIDCAuthenticator, collector *metrics.Collector) *Handler {
	return &Handler{pools: pools, auth: authenticator, collector: collector}
}

func (h *Handler) Register(r *router.Router) {
	r.Handle(http.MethodGet, "/healthz", "Check service health", h.healthz)
	r.HandleProtected(
		http.MethodGet,
		"/v1/identity/system/health",
		"Read system component health",
		"identity",
		"admin",
		h.auth.Require("identity", "admin"),
		h.adminHealth,
	)
	r.HandleProtected(
		http.MethodGet,
		"/v1/identity/system/metrics",
		"Read system metrics",
		"identity",
		"admin",
		h.auth.Require("identity", "admin"),
		h.adminMetrics,
	)
}

func (h *Handler) adminMetrics(c *gin.Context) {
	pools := make([]databasePoolMetrics, 0, 7)
	for _, name := range []string{"identity", "health", "electoral", "grants", "police", "audit", "authz"} {
		database, err := h.pool(name)
		if err != nil {
			httpx.Error(c.Writer, c.Request, http.StatusServiceUnavailable, "DATABASE_UNAVAILABLE", "Database metrics are unavailable")
			return
		}
		sqlDB, err := database.DB()
		if err != nil {
			httpx.Error(c.Writer, c.Request, http.StatusServiceUnavailable, "DATABASE_UNAVAILABLE", "Database metrics are unavailable")
			return
		}
		stats := sqlDB.Stats()
		pools = append(pools, databasePoolMetrics{
			Name: name, OpenConnections: stats.OpenConnections, InUseConnections: stats.InUse,
			IdleConnections: stats.Idle, MaxConnections: stats.MaxOpenConnections,
			WaitCount: stats.WaitCount, WaitDurationMillis: stats.WaitDuration.Milliseconds(),
		})
	}
	httpx.JSON(c.Writer, http.StatusOK, adminMetricsResponse{
		Snapshot: h.collector.Snapshot(), DatabasePools: pools,
	})
}

func (h *Handler) healthz(c *gin.Context) {
	if !h.checkPools(c.Request.Context()) {
		httpx.JSON(c.Writer, http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
		return
	}
	httpx.JSON(c.Writer, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) adminHealth(c *gin.Context) {
	status := make([]componentStatus, 0, 7)
	healthy := true
	for _, name := range []string{"identity", "health", "electoral", "grants", "police", "audit", "authz"} {
		database, err := h.pool(name)
		component := componentStatus{Name: name, Status: "ok"}
		if err != nil {
			component.Status = "unavailable"
			healthy = false
		} else if sqlDB, err := database.DB(); err != nil || sqlDB.PingContext(c.Request.Context()) != nil {
			component.Status = "unavailable"
			healthy = false
		}
		status = append(status, component)
	}
	httpStatus := http.StatusOK
	if !healthy {
		httpStatus = http.StatusServiceUnavailable
	}
	httpx.JSON(c.Writer, httpStatus, map[string]any{"status": componentState(healthy), "components": status})
}

func (h *Handler) checkPools(ctx context.Context) bool {
	for _, name := range []string{"identity", "health", "electoral", "grants", "police", "audit", "authz"} {
		database, err := h.pool(name)
		if err != nil {
			return false
		}
		sqlDB, err := database.DB()
		if err != nil {
			return false
		}
		pingContext, cancel := context.WithTimeout(ctx, 2*time.Second)
		err = sqlDB.PingContext(pingContext)
		cancel()
		if err != nil {
			return false
		}
	}
	return true
}

func (h *Handler) pool(name string) (*gorm.DB, error) {
	switch name {
	case "audit":
		return h.pools.Audit, nil
	case "authz":
		return h.pools.Authz, nil
	default:
		return h.pools.For(name)
	}
}

func componentState(healthy bool) string {
	if healthy {
		return "ok"
	}
	return "degraded"
}
