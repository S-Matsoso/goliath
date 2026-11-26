package auditlog

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/S-Matsoso/goliath/internal/auth"
	"github.com/S-Matsoso/goliath/internal/db"
	"github.com/S-Matsoso/goliath/internal/platform/api"
	"github.com/S-Matsoso/goliath/internal/platform/httpx"
	"github.com/S-Matsoso/goliath/internal/platform/router"
	"github.com/gin-gonic/gin"
)

var departments = []string{"identity", "health", "electoral", "grants", "police"}

type Handler struct {
	pools *db.Pools
	auth  *auth.OIDCAuthenticator
}

type row struct {
	Sequence uint64    `gorm:"column:seq"`
	Occurred time.Time `gorm:"column:occurred_at"`
	Official *uint64   `gorm:"column:official_id"`
	Action   string    `gorm:"column:action"`
	Purpose  *string   `gorm:"column:purpose"`
	Outcome  string    `gorm:"column:outcome"`
}

type entry struct {
	Sequence   uint64  `json:"sequence"`
	OccurredAt string  `json:"occurred_at"`
	OfficialID *uint64 `json:"official_id,omitempty"`
	Action     string  `json:"action"`
	Purpose    *string `json:"purpose,omitempty"`
	Outcome    string  `json:"outcome"`
}

func NewHandler(pools *db.Pools, authenticator *auth.OIDCAuthenticator) *Handler {
	return &Handler{pools: pools, auth: authenticator}
}

func (h *Handler) Register(r *router.Router) {
	for _, department := range departments {
		code := department
		r.HandleProtected(
			http.MethodGet,
			"/v1/"+code+"/audit",
			"Read own department audit events",
			code,
			"auditor",
			h.auth.Require(code, "auditor"),
			func(c *gin.Context) { h.list(c, code) },
		)
	}
}

func (h *Handler) list(c *gin.Context, department string) {
	database, err := h.pools.For(department)
	if err != nil {
		httpx.Error(c.Writer, c.Request, http.StatusServiceUnavailable, "AUDIT_UNAVAILABLE", "Audit log is unavailable")
		return
	}
	limit, err := api.PageSize(c)
	if err != nil {
		httpx.Error(c.Writer, c.Request, http.StatusBadRequest, httpx.CodeBadRequest, "Page size is invalid")
		return
	}
	cursor, err := api.Cursor(c)
	if err != nil {
		httpx.Error(c.Writer, c.Request, http.StatusBadRequest, httpx.CodeBadRequest, "Page cursor is invalid")
		return
	}
	from, err := parseDate(c.Query("from"))
	if err != nil {
		httpx.Error(c.Writer, c.Request, http.StatusBadRequest, httpx.CodeBadRequest, "Start date is invalid")
		return
	}
	to, err := parseDate(c.Query("to"))
	if err != nil || (from != nil && to != nil && from.After(*to)) {
		httpx.Error(c.Writer, c.Request, http.StatusBadRequest, httpx.CodeBadRequest, "Date range is invalid")
		return
	}
	query := database.Table("v_"+department+"_audit_log").Where("seq > ?", cursor)
	if from != nil {
		query = query.Where("occurred_at >= ?", *from)
	}
	if to != nil {
		query = query.Where("occurred_at < ?", to.Add(24*time.Hour))
	}
	if rawOfficial := strings.TrimSpace(c.Query("official_id")); rawOfficial != "" {
		officialID, err := strconv.ParseUint(rawOfficial, 10, 64)
		if err != nil || officialID == 0 {
			httpx.Error(c.Writer, c.Request, http.StatusBadRequest, httpx.CodeBadRequest, "Official ID filter is invalid")
			return
		}
		query = query.Where("official_id = ?", officialID)
	}
	if action := strings.TrimSpace(c.Query("action")); action != "" {
		if len(action) > 60 {
			httpx.Error(c.Writer, c.Request, http.StatusBadRequest, httpx.CodeBadRequest, "Action filter is invalid")
			return
		}
		query = query.Where("action = ?", action)
	}
	var rows []row
	if err := query.Order("seq").Limit(limit + 1).Find(&rows).Error; err != nil {
		httpx.Error(c.Writer, c.Request, http.StatusServiceUnavailable, "AUDIT_UNAVAILABLE", "Audit log is unavailable")
		return
	}
	var next any
	if len(rows) > limit {
		next = rows[limit-1].Sequence
		rows = rows[:limit]
	}
	entries := make([]entry, 0, len(rows))
	for _, item := range rows {
		entries = append(entries, entry{
			Sequence: item.Sequence, OccurredAt: item.Occurred.UTC().Format(time.RFC3339Nano),
			OfficialID: item.Official, Action: item.Action, Purpose: item.Purpose, Outcome: item.Outcome,
		})
	}
	httpx.JSON(c.Writer, http.StatusOK, map[string]any{"items": entries, "next_cursor": next})
}

func parseDate(value string) (*time.Time, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	date, err := time.Parse("2006-01-02", value)
	if err != nil {
		return nil, err
	}
	return &date, nil
}
