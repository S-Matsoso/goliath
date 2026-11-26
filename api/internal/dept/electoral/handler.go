package electoral

import (
	"bytes"
	"encoding/csv"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/S-Matsoso/goliath/internal/audit"
	"github.com/S-Matsoso/goliath/internal/auth"
	"github.com/S-Matsoso/goliath/internal/crypto"
	"github.com/S-Matsoso/goliath/internal/db"
	"github.com/S-Matsoso/goliath/internal/nid"
	"github.com/S-Matsoso/goliath/internal/platform/api"
	"github.com/S-Matsoso/goliath/internal/platform/httpx"
	"github.com/S-Matsoso/goliath/internal/platform/router"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type Handler struct {
	database *gorm.DB
	auth     *auth.OIDCAuthenticator
	vault    crypto.Transit
	audit    *audit.Writer
	hmacKey  string
}

type verifyRequest struct {
	NationalID string `json:"national_id"`
	Purpose    string `json:"purpose"`
}

type voterRow struct {
	Index     []byte  `gorm:"column:nid_idx"`
	FirstName string  `gorm:"column:first_names"`
	LastName  string  `gorm:"column:last_name"`
	Ward      *string `gorm:"column:ward_code"`
}

type voterDTO struct {
	FirstNames string  `json:"first_names"`
	LastName   string  `json:"last_name"`
	Ward       *string `json:"ward"`
}

type eventRow struct {
	ID        uint64    `gorm:"column:id"`
	EventType string    `gorm:"column:event_type"`
	Ward      *string   `gorm:"column:ward_code"`
	Removed   bool      `gorm:"column:removed"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

type eventDTO struct {
	EventType string  `json:"event_type"`
	Ward      *string `json:"ward"`
	Removed   bool    `json:"removed"`
	CreatedAt string  `json:"created_at"`
}

func NewHandler(pools *db.Pools, authenticator *auth.OIDCAuthenticator, vault crypto.Transit, writer *audit.Writer, hmacKey string) (*Handler, error) {
	database, err := pools.For("electoral")
	if err != nil {
		return nil, err
	}
	if authenticator == nil || vault == nil || writer == nil {
		return nil, errors.New("electoral handler requires authentication, transit, and audit services")
	}
	return &Handler{database: database, auth: authenticator, vault: vault, audit: writer, hmacKey: hmacKey}, nil
}

func (h *Handler) Register(r *router.Router) {
	h.route(r, http.MethodPost, "/v1/electoral/verify", "Verify voter eligibility", "clerk", h.verifyVoter)
	h.route(r, http.MethodGet, "/v1/electoral/roll", "Browse the voter roll", "clerk", h.listRoll)
	h.route(r, http.MethodGet, "/v1/electoral/roll/export", "Export a voter roll page", "supervisor", h.exportRoll)
	h.route(r, http.MethodGet, "/v1/electoral/events", "Read voter-roll event feed", "clerk", h.events)
}

func (h *Handler) route(r *router.Router, method, path, summary, minimumRole string, handler gin.HandlerFunc) {
	r.HandleProtected(method, path, summary, "electoral", minimumRole, h.auth.Require("electoral", minimumRole), handler)
}

func (h *Handler) verifyVoter(c *gin.Context) {
	var request verifyRequest
	if err := api.DecodeJSON(c, &request, 1<<20); err != nil || nid.ValidateFormat(request.NationalID) != nil ||
		!api.ValidPurpose(request.Purpose) {
		h.failure(c, "VERIFY_VOTER", http.StatusBadRequest, httpx.CodeBadRequest, "Verification request is invalid")
		return
	}
	index, err := h.vault.BlindIndex(c.Request.Context(), h.hmacKey, request.NationalID)
	if err != nil {
		h.failure(c, "VERIFY_VOTER", http.StatusInternalServerError, httpx.CodeInternal, httpx.MessageInternal)
		return
	}
	principal, ok := auth.RequestClaims(c)
	if !ok {
		httpx.InternalError(c.Writer, c.Request)
		return
	}
	var voter voterRow
	found := false
	err = h.audit.InTransactionOn(c.Request.Context(), h.database,
		h.event(c, principal, "VERIFY_VOTER", index, request.Purpose, db.AuditOutcomeOK),
		func(tx *gorm.DB) error {
			if err := tx.Table("v_electoral_roll").Where("nid_idx = ?", index).Take(&voter).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return nil
				}
				return err
			}
			found = true
			return nil
		})
	if err != nil {
		h.failure(c, "VERIFY_VOTER", http.StatusInternalServerError, httpx.CodeInternal, httpx.MessageInternal)
		return
	}
	if !found {
		httpx.JSON(c.Writer, http.StatusOK, map[string]bool{"match": false})
		return
	}
	httpx.JSON(c.Writer, http.StatusOK, map[string]any{
		"match": true, "first_names": voter.FirstName, "last_name": voter.LastName, "ward": voter.Ward,
	})
}

func (h *Handler) listRoll(c *gin.Context) {
	ward := strings.TrimSpace(c.Query("ward"))
	purpose := strings.TrimSpace(c.Query("purpose"))
	if ward == "" || len(ward) > 16 || !api.ValidPurpose(purpose) {
		h.failure(c, "READ_VOTER_ROLL", http.StatusBadRequest, httpx.CodeBadRequest, "Ward and purpose are required")
		return
	}
	h.readRoll(c, ward, false, purpose)
}

func (h *Handler) exportRoll(c *gin.Context) {
	ward := strings.TrimSpace(c.Query("ward"))
	purpose := strings.TrimSpace(c.Query("purpose"))
	if ward == "" || len(ward) > 16 || !api.ValidPurpose(purpose) {
		h.failure(c, "EXPORT_VOTER_ROLL", http.StatusBadRequest, httpx.CodeBadRequest, "Ward and purpose are required")
		return
	}
	h.readRoll(c, ward, true, purpose)
}

func (h *Handler) readRoll(c *gin.Context, ward string, export bool, purpose string) {
	action := "READ_VOTER_ROLL"
	if export {
		action = "EXPORT_VOTER_ROLL"
	}
	limit, err := api.PageSize(c)
	if err != nil {
		h.failure(c, action, http.StatusBadRequest, httpx.CodeBadRequest, "Page size is invalid")
		return
	}
	if export && limit > 500 {
		h.failure(c, action, http.StatusBadRequest, httpx.CodeBadRequest, "Page size is invalid")
		return
	}
	cursor, err := api.BinaryCursor(c, 32)
	if err != nil {
		h.failure(c, action, http.StatusBadRequest, httpx.CodeBadRequest, "Page cursor is invalid")
		return
	}
	principal, ok := auth.RequestClaims(c)
	if !ok {
		httpx.InternalError(c.Writer, c.Request)
		return
	}
	var rows []voterRow
	var nextCursor []byte
	err = h.audit.InTransactionOn(c.Request.Context(), h.database,
		h.event(c, principal, action, nil, purpose, db.AuditOutcomeOK),
		func(tx *gorm.DB) error {
			query := tx.Table("v_electoral_roll").Where("ward_code = ?", ward)
			if cursor != nil {
				query = query.Where("nid_idx > ?", cursor)
			}
			if err := query.Order("nid_idx").Limit(limit + 1).Find(&rows).Error; err != nil {
				return err
			}
			if len(rows) > limit {
				rows = rows[:limit]
				nextCursor = rows[len(rows)-1].Index
			}
			return nil
		})
	if err != nil {
		h.failure(c, action, http.StatusInternalServerError, httpx.CodeInternal, httpx.MessageInternal)
		return
	}
	if export {
		h.writeCSV(c, rows, nextCursor)
		return
	}
	items := make([]voterDTO, 0, len(rows))
	for _, row := range rows {
		items = append(items, voterDTO{FirstNames: row.FirstName, LastName: row.LastName, Ward: row.Ward})
	}
	var next any
	if len(nextCursor) > 0 {
		next = api.EncodeBinaryCursor(nextCursor)
	}
	httpx.JSON(c.Writer, http.StatusOK, map[string]any{"items": items, "next_cursor": next})
}

func (h *Handler) writeCSV(c *gin.Context, rows []voterRow, nextCursor []byte) {
	var body bytes.Buffer
	writer := csv.NewWriter(&body)
	if err := writer.Write([]string{"first_names", "last_name", "ward"}); err != nil {
		h.failure(c, "EXPORT_VOTER_ROLL", http.StatusInternalServerError, httpx.CodeInternal, httpx.MessageInternal)
		return
	}
	for _, row := range rows {
		if err := writer.Write([]string{row.FirstName, row.LastName, value(row.Ward)}); err != nil {
			h.failure(c, "EXPORT_VOTER_ROLL", http.StatusInternalServerError, httpx.CodeInternal, httpx.MessageInternal)
			return
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		h.failure(c, "EXPORT_VOTER_ROLL", http.StatusInternalServerError, httpx.CodeInternal, httpx.MessageInternal)
		return
	}
	if len(nextCursor) > 0 {
		c.Header("X-Next-Cursor", api.EncodeBinaryCursor(nextCursor))
	}
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", `attachment; filename="voter-roll.csv"`)
	c.Status(http.StatusOK)
	_, _ = c.Writer.Write(body.Bytes())
}

func (h *Handler) events(c *gin.Context) {
	limit, err := api.PageSize(c)
	if err != nil {
		h.failure(c, "READ_VOTER_EVENTS", http.StatusBadRequest, httpx.CodeBadRequest, "Page size is invalid")
		return
	}
	cursor, err := api.Cursor(c)
	if err != nil {
		h.failure(c, "READ_VOTER_EVENTS", http.StatusBadRequest, httpx.CodeBadRequest, "Page cursor is invalid")
		return
	}
	principal, ok := auth.RequestClaims(c)
	if !ok {
		httpx.InternalError(c.Writer, c.Request)
		return
	}
	var rows []eventRow
	err = h.audit.InTransactionOn(c.Request.Context(), h.database,
		h.event(c, principal, "READ_VOTER_EVENTS", nil, "", db.AuditOutcomeOK),
		func(tx *gorm.DB) error {
			return tx.Table("v_electoral_events").Where("id > ?", cursor).Order("id").Limit(limit + 1).Find(&rows).Error
		})
	if err != nil {
		h.failure(c, "READ_VOTER_EVENTS", http.StatusInternalServerError, httpx.CodeInternal, httpx.MessageInternal)
		return
	}
	var next any
	if len(rows) > limit {
		value := rows[limit-1].ID
		next = value
		rows = rows[:limit]
	}
	items := make([]eventDTO, 0, len(rows))
	for _, row := range rows {
		items = append(items, eventDTO{
			EventType: row.EventType, Ward: row.Ward, Removed: row.Removed,
			CreatedAt: row.CreatedAt.UTC().Format(time.RFC3339Nano),
		})
	}
	httpx.JSON(c.Writer, http.StatusOK, map[string]any{"items": items, "next_cursor": next})
}

func (h *Handler) event(c *gin.Context, claims auth.JWTClaims, action string, subject []byte, purpose string, outcome db.AuditOutcome) audit.Event {
	officialID := claims.OfficialID
	var purposeValue *string
	if strings.TrimSpace(purpose) != "" {
		value := strings.TrimSpace(purpose)
		purposeValue = &value
	}
	return audit.Event{
		OfficialID: &officialID, Department: claims.Department, Action: action,
		Subject: subject, Purpose: purposeValue, Outcome: outcome,
		RequestID: httpx.RequestID(c.Request.Context()),
	}
}

func (h *Handler) failure(c *gin.Context, action string, status int, code, message string) {
	principal, ok := auth.RequestClaims(c)
	if !ok {
		httpx.InternalError(c.Writer, c.Request)
		return
	}
	if err := h.audit.Write(c.Request.Context(), h.event(c, principal, action, nil, "", db.AuditOutcomeError)); err != nil {
		httpx.Error(c.Writer, c.Request, http.StatusServiceUnavailable, "AUDIT_UNAVAILABLE", "Request could not be audited")
		return
	}
	httpx.Error(c.Writer, c.Request, status, code, message)
}

func value(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
