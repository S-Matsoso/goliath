package police

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math"
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
	mysqlDriver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
)

type BiometricMatcher interface {
	Match(context.Context, []byte) ([]BiometricCandidate, error)
}

type BiometricCandidate struct {
	NationalIDIndex []byte
	Score           float64
}

type Handler struct {
	database      *gorm.DB
	auth          *auth.OIDCAuthenticator
	vault         crypto.Transit
	audit         *audit.Writer
	encryptionKey string
	hmacKey       string
	matcher       BiometricMatcher
}

type verifyRequest struct {
	NationalID  string `json:"national_id"`
	DateOfBirth string `json:"date_of_birth"`
	Purpose     string `json:"purpose"`
}

type lostStolenRequest struct {
	NationalID    string `json:"national_id"`
	CaseReference string `json:"case_reference"`
	Details       string `json:"details"`
	Purpose       string `json:"purpose"`
}

type unidentifiedDeathRequest struct {
	CaseReference string `json:"case_reference"`
	Description   string `json:"description"`
	FoundAt       string `json:"found_at"`
	DiscoveredAt  string `json:"discovered_at"`
	Purpose       string `json:"purpose"`
}

type biometricRequest struct {
	Template      string `json:"template"`
	CaseReference string `json:"case_reference"`
	Purpose       string `json:"purpose"`
}

type personView struct {
	Index       []byte    `gorm:"column:nid_idx"`
	FirstNames  string    `gorm:"column:first_names"`
	LastName    string    `gorm:"column:last_name"`
	DateOfBirth time.Time `gorm:"column:date_of_birth"`
	Sex         db.Sex    `gorm:"column:sex"`
	Alive       bool      `gorm:"column:is_alive"`
}

type requestView struct {
	ID            uint64     `gorm:"column:id"`
	Type          string     `gorm:"column:request_type"`
	Status        string     `gorm:"column:status"`
	ReviewedAt    *time.Time `gorm:"column:reviewed_at"`
	Reason        *string    `gorm:"column:reason"`
	CreatedAt     time.Time  `gorm:"column:created_at"`
	SubmittedByID uint64     `gorm:"column:submitted_by_official"`
}

type requestDTO struct {
	ID         uint64  `json:"id"`
	Type       string  `json:"type"`
	Status     string  `json:"status"`
	ReviewedAt *string `json:"reviewed_at,omitempty"`
	Reason     *string `json:"reason,omitempty"`
	CreatedAt  string  `json:"created_at"`
}

type biometricResultDTO struct {
	FirstNames  string  `json:"first_names"`
	LastName    string  `json:"last_name"`
	DateOfBirth string  `json:"date_of_birth"`
	Sex         db.Sex  `json:"sex"`
	Alive       bool    `json:"is_alive"`
	Score       float64 `json:"score"`
}

func NewHandler(pools *db.Pools, authenticator *auth.OIDCAuthenticator, vault crypto.Transit, writer *audit.Writer, encryptionKey, hmacKey string, matchers ...BiometricMatcher) (*Handler, error) {
	database, err := pools.For("police")
	if err != nil {
		return nil, err
	}
	if authenticator == nil || vault == nil || writer == nil {
		return nil, errors.New("police handler requires authentication, transit, and audit services")
	}
	handler := &Handler{
		database: database, auth: authenticator, vault: vault, audit: writer,
		encryptionKey: encryptionKey, hmacKey: hmacKey,
	}
	if len(matchers) > 0 {
		handler.matcher = matchers[0]
	}
	return handler, nil
}

func (h *Handler) Register(r *router.Router) {
	h.route(r, http.MethodPost, "/v1/police/verify", "Verify identity", "clerk", h.verify)
	h.route(r, http.MethodPost, "/v1/police/change-requests/lost-stolen", "Submit an ID lost-or-stolen request", "clerk", h.submitLostStolen)
	h.route(r, http.MethodPost, "/v1/police/change-requests/unidentified-death", "Submit an unidentified death notification", "clerk", h.submitUnidentifiedDeath)
	h.route(r, http.MethodGet, "/v1/police/change-requests", "Read own change-request statuses", "clerk", h.ownRequests)
	h.route(r, http.MethodPost, "/v1/police/biometric-match", "Search biometric candidates", "supervisor", h.biometricMatch)
}

func (h *Handler) route(r *router.Router, method, path, summary, minimumRole string, handler gin.HandlerFunc) {
	r.HandleProtected(method, path, summary, "police", minimumRole, h.auth.Require("police", minimumRole), handler)
}

func (h *Handler) verify(c *gin.Context) {
	var request verifyRequest
	if err := api.DecodeJSON(c, &request, 1<<20); err != nil || nid.ValidateFormat(request.NationalID) != nil ||
		!api.ValidPurpose(request.Purpose) {
		h.failure(c, "VERIFY_IDENTITY", db.AuditOutcomeError, http.StatusBadRequest, httpx.CodeBadRequest, "Verification request is invalid")
		return
	}
	dateOfBirth, err := time.Parse("2006-01-02", request.DateOfBirth)
	if err != nil {
		h.failure(c, "VERIFY_IDENTITY", db.AuditOutcomeError, http.StatusBadRequest, httpx.CodeBadRequest, "Verification request is invalid")
		return
	}
	index, err := h.vault.BlindIndex(c.Request.Context(), h.hmacKey, request.NationalID)
	if err != nil {
		h.failure(c, "VERIFY_IDENTITY", db.AuditOutcomeError, http.StatusInternalServerError, httpx.CodeInternal, httpx.MessageInternal)
		return
	}
	principal, ok := auth.RequestClaims(c)
	if !ok {
		httpx.InternalError(c.Writer, c.Request)
		return
	}
	var person personView
	found := false
	err = h.audit.InTransactionOn(c.Request.Context(), h.database,
		h.event(c, principal, "VERIFY_IDENTITY", index, request.Purpose, db.AuditOutcomeOK),
		func(tx *gorm.DB) error {
			if err := tx.Table("v_police_identity").Where("nid_idx = ?", index).Take(&person).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return nil
				}
				return err
			}
			found = true
			return nil
		})
	if err != nil {
		h.failure(c, "VERIFY_IDENTITY", db.AuditOutcomeError, http.StatusInternalServerError, httpx.CodeInternal, httpx.MessageInternal)
		return
	}
	if !found || !person.DateOfBirth.Equal(dateOfBirth) {
		httpx.JSON(c.Writer, http.StatusOK, map[string]bool{"match": false})
		return
	}
	httpx.JSON(c.Writer, http.StatusOK, map[string]any{
		"match": true, "first_names": person.FirstNames, "last_name": person.LastName,
		"date_of_birth": person.DateOfBirth.Format("2006-01-02"), "sex": person.Sex, "is_alive": person.Alive,
	})
}

func (h *Handler) submitLostStolen(c *gin.Context) {
	var request lostStolenRequest
	if err := api.DecodeJSON(c, &request, 1<<20); err != nil || nid.ValidateFormat(request.NationalID) != nil ||
		strings.TrimSpace(request.CaseReference) == "" || strings.TrimSpace(request.Details) == "" ||
		len(request.CaseReference) > 80 || len(request.Details) > 1000 || !api.ValidPurpose(request.Purpose) {
		h.failure(c, "SUBMIT_CHANGE_REQUEST", db.AuditOutcomeError, http.StatusBadRequest, httpx.CodeBadRequest, "Change request is invalid")
		return
	}
	index, err := h.vault.BlindIndex(c.Request.Context(), h.hmacKey, request.NationalID)
	if err != nil {
		h.failure(c, "SUBMIT_CHANGE_REQUEST", db.AuditOutcomeError, http.StatusInternalServerError, httpx.CodeInternal, httpx.MessageInternal)
		return
	}
	payload, err := json.Marshal(map[string]string{
		"case_reference": strings.TrimSpace(request.CaseReference),
		"details":        strings.TrimSpace(request.Details),
	})
	if err != nil {
		h.failure(c, "SUBMIT_CHANGE_REQUEST", db.AuditOutcomeError, http.StatusInternalServerError, httpx.CodeInternal, httpx.MessageInternal)
		return
	}
	h.submitRequest(c, "ID_LOST_STOLEN", index, payload, request.Purpose)
}

func (h *Handler) submitUnidentifiedDeath(c *gin.Context) {
	var request unidentifiedDeathRequest
	if err := api.DecodeJSON(c, &request, 1<<20); err != nil || !api.ValidPurpose(request.Purpose) ||
		strings.TrimSpace(request.CaseReference) == "" || strings.TrimSpace(request.Description) == "" ||
		strings.TrimSpace(request.FoundAt) == "" || len(request.CaseReference) > 80 ||
		len(request.Description) > 1000 || len(request.FoundAt) > 200 {
		h.failure(c, "SUBMIT_CHANGE_REQUEST", db.AuditOutcomeError, http.StatusBadRequest, httpx.CodeBadRequest, "Change request is invalid")
		return
	}
	discoveredAt, err := time.Parse(time.RFC3339, request.DiscoveredAt)
	if err != nil || discoveredAt.After(time.Now().Add(time.Minute)) {
		h.failure(c, "SUBMIT_CHANGE_REQUEST", db.AuditOutcomeError, http.StatusBadRequest, httpx.CodeBadRequest, "Change request is invalid")
		return
	}
	payload, err := json.Marshal(map[string]string{
		"case_reference": strings.TrimSpace(request.CaseReference),
		"description":    strings.TrimSpace(request.Description),
		"found_at":       strings.TrimSpace(request.FoundAt),
		"discovered_at":  discoveredAt.UTC().Format(time.RFC3339),
	})
	if err != nil {
		h.failure(c, "SUBMIT_CHANGE_REQUEST", db.AuditOutcomeError, http.StatusInternalServerError, httpx.CodeInternal, httpx.MessageInternal)
		return
	}
	h.submitRequest(c, "UNIDENTIFIED_DEATH_NOTIFICATION", nil, payload, request.Purpose)
}

func (h *Handler) submitRequest(c *gin.Context, requestType string, targetIndex, payload []byte, purpose string) {
	encrypted, err := h.vault.Encrypt(c.Request.Context(), h.encryptionKey, payload)
	if err != nil {
		h.failure(c, "SUBMIT_CHANGE_REQUEST", db.AuditOutcomeError, http.StatusInternalServerError, httpx.CodeInternal, httpx.MessageInternal)
		return
	}
	principal, ok := auth.RequestClaims(c)
	if !ok {
		httpx.InternalError(c.Writer, c.Request)
		return
	}
	event := h.event(c, principal, "SUBMIT_CHANGE_REQUEST", targetIndex, purpose, db.AuditOutcomeOK)
	var requestID uint64
	err = h.audit.WithPreparedEvent(c.Request.Context(), event, func(row db.AuditLog) error {
		return h.database.WithContext(c.Request.Context()).Connection(func(connection *gorm.DB) error {
			if err := connection.Exec(
				"CALL sp_submit_police_change_request(?,?,?,?,?,?,?,?,?,?,?,@created_id)",
				requestType, targetIndex, encrypted, principal.OfficialID, purpose, row.ChainID, row.Seq,
				row.OccurredAt, row.RequestID, row.PreviousHash, row.RowHash,
			).Error; err != nil {
				return err
			}
			return connection.Raw("SELECT @created_id").Scan(&requestID).Error
		})
	})
	if err != nil {
		h.procedureFailure(c, err, "SUBMIT_CHANGE_REQUEST", purpose)
		return
	}
	httpx.JSON(c.Writer, http.StatusCreated, map[string]any{"id": requestID, "status": "PENDING"})
}

func (h *Handler) ownRequests(c *gin.Context) {
	purpose := strings.TrimSpace(c.Query("purpose"))
	if !api.ValidPurpose(purpose) {
		h.failure(c, "READ_OWN_CHANGE_REQUESTS", db.AuditOutcomeError, http.StatusBadRequest, httpx.CodeBadRequest, "Purpose is required")
		return
	}
	limit, err := api.PageSize(c)
	if err != nil {
		h.failure(c, "READ_OWN_CHANGE_REQUESTS", db.AuditOutcomeError, http.StatusBadRequest, httpx.CodeBadRequest, "Page size is invalid")
		return
	}
	cursor, err := api.Cursor(c)
	if err != nil {
		h.failure(c, "READ_OWN_CHANGE_REQUESTS", db.AuditOutcomeError, http.StatusBadRequest, httpx.CodeBadRequest, "Page cursor is invalid")
		return
	}
	principal, ok := auth.RequestClaims(c)
	if !ok {
		httpx.InternalError(c.Writer, c.Request)
		return
	}
	var rows []requestView
	err = h.audit.InTransactionOn(c.Request.Context(), h.database,
		h.event(c, principal, "READ_OWN_CHANGE_REQUESTS", nil, purpose, db.AuditOutcomeOK),
		func(tx *gorm.DB) error {
			return tx.Table("v_police_change_requests").
				Where("submitted_by_official = ? AND id > ?", principal.OfficialID, cursor).
				Order("id").Limit(limit + 1).Find(&rows).Error
		})
	if err != nil {
		h.failure(c, "READ_OWN_CHANGE_REQUESTS", db.AuditOutcomeError, http.StatusInternalServerError, httpx.CodeInternal, httpx.MessageInternal)
		return
	}
	var next any
	if len(rows) > limit {
		next = rows[limit-1].ID
		rows = rows[:limit]
	}
	items := make([]requestDTO, 0, len(rows))
	for _, row := range rows {
		dto := requestDTO{
			ID: row.ID, Type: row.Type, Status: row.Status,
			CreatedAt: row.CreatedAt.UTC().Format(time.RFC3339Nano), Reason: row.Reason,
		}
		if row.ReviewedAt != nil {
			value := row.ReviewedAt.UTC().Format(time.RFC3339Nano)
			dto.ReviewedAt = &value
		}
		items = append(items, dto)
	}
	httpx.JSON(c.Writer, http.StatusOK, map[string]any{"items": items, "next_cursor": next})
}

func (h *Handler) biometricMatch(c *gin.Context) {
	var request biometricRequest
	if err := api.DecodeJSON(c, &request, 1<<20); err != nil ||
		strings.TrimSpace(request.CaseReference) == "" || len(request.CaseReference) > 80 ||
		!api.ValidPurpose(request.Purpose) {
		h.failure(c, "BIOMETRIC_MATCH", db.AuditOutcomeError, http.StatusBadRequest, httpx.CodeBadRequest, "Biometric query is invalid")
		return
	}
	template, err := base64.StdEncoding.DecodeString(request.Template)
	if err != nil || len(template) == 0 || len(template) > 512<<10 {
		h.failure(c, "BIOMETRIC_MATCH", db.AuditOutcomeError, http.StatusBadRequest, httpx.CodeBadRequest, "Biometric query is invalid")
		return
	}
	principal, ok := auth.RequestClaims(c)
	if !ok {
		httpx.InternalError(c.Writer, c.Request)
		return
	}
	purpose := strings.TrimSpace(request.Purpose) + "; case reference: " + strings.TrimSpace(request.CaseReference)
	if h.matcher == nil {
		h.failureWithPurpose(c, "BIOMETRIC_MATCH", db.AuditOutcomeError, http.StatusServiceUnavailable, "MATCHER_UNAVAILABLE", "Biometric matching is not configured", purpose)
		return
	}
	if err := h.audit.Write(c.Request.Context(), h.event(c, principal, "BIOMETRIC_MATCH", nil, purpose, db.AuditOutcomeOK)); err != nil {
		httpx.Error(c.Writer, c.Request, http.StatusServiceUnavailable, "AUDIT_UNAVAILABLE", "Request could not be audited")
		return
	}
	candidates, err := h.matcher.Match(c.Request.Context(), template)
	if err != nil {
		h.failureWithPurpose(c, "BIOMETRIC_MATCH", db.AuditOutcomeError, http.StatusServiceUnavailable, "MATCHER_UNAVAILABLE", "Biometric matching is unavailable", purpose)
		return
	}
	if len(candidates) > 20 {
		h.failureWithPurpose(c, "BIOMETRIC_MATCH", db.AuditOutcomeError, http.StatusInternalServerError, httpx.CodeInternal, httpx.MessageInternal, purpose)
		return
	}
	if len(candidates) == 0 {
		httpx.JSON(c.Writer, http.StatusOK, map[string]any{"candidates": []biometricResultDTO{}})
		return
	}
	indexes := make([][]byte, 0, len(candidates))
	for _, candidate := range candidates {
		if len(candidate.NationalIDIndex) != 32 || math.IsNaN(candidate.Score) ||
			math.IsInf(candidate.Score, 0) || candidate.Score < 0 || candidate.Score > 1 {
			h.failureWithPurpose(c, "BIOMETRIC_MATCH", db.AuditOutcomeError, http.StatusInternalServerError, httpx.CodeInternal, httpx.MessageInternal, purpose)
			return
		}
		indexes = append(indexes, candidate.NationalIDIndex)
	}
	var people []personView
	err = h.audit.InTransactionOn(c.Request.Context(), h.database,
		h.event(c, principal, "READ_BIOMETRIC_CANDIDATES", nil, purpose, db.AuditOutcomeOK),
		func(tx *gorm.DB) error {
			return tx.Table("v_police_identity").Where("nid_idx IN ?", indexes).Find(&people).Error
		})
	if err != nil {
		h.failureWithPurpose(c, "BIOMETRIC_MATCH", db.AuditOutcomeError, http.StatusInternalServerError, httpx.CodeInternal, httpx.MessageInternal, purpose)
		return
	}
	byIndex := make(map[string]personView, len(people))
	for _, person := range people {
		byIndex[string(person.Index)] = person
	}
	results := make([]biometricResultDTO, 0, len(candidates))
	for _, candidate := range candidates {
		person, ok := byIndex[string(candidate.NationalIDIndex)]
		if !ok {
			continue
		}
		results = append(results, biometricResultDTO{
			FirstNames: person.FirstNames, LastName: person.LastName,
			DateOfBirth: person.DateOfBirth.Format("2006-01-02"), Sex: person.Sex,
			Alive: person.Alive, Score: candidate.Score,
		})
	}
	httpx.JSON(c.Writer, http.StatusOK, map[string]any{"candidates": results})
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

func (h *Handler) failure(c *gin.Context, action string, outcome db.AuditOutcome, status int, code, message string) {
	h.failureWithPurpose(c, action, outcome, status, code, message, "")
}

func (h *Handler) failureWithPurpose(c *gin.Context, action string, outcome db.AuditOutcome, status int, code, message, purpose string) {
	principal, ok := auth.RequestClaims(c)
	if !ok {
		httpx.InternalError(c.Writer, c.Request)
		return
	}
	if err := h.audit.Write(c.Request.Context(), h.event(c, principal, action, nil, purpose, outcome)); err != nil {
		httpx.Error(c.Writer, c.Request, http.StatusServiceUnavailable, "AUDIT_UNAVAILABLE", "Request could not be audited")
		return
	}
	httpx.Error(c.Writer, c.Request, status, code, message)
}

func (h *Handler) procedureFailure(c *gin.Context, err error, action, purpose string) {
	var mysqlErr *mysqlDriver.MySQLError
	if errors.As(err, &mysqlErr) && mysqlErr.Number == 1644 {
		h.failureWithPurpose(c, action, db.AuditOutcomeError, http.StatusConflict, "REQUEST_CONFLICT", "The requested change could not be applied", purpose)
		return
	}
	h.failureWithPurpose(c, action, db.AuditOutcomeError, http.StatusServiceUnavailable, "DATABASE_UNAVAILABLE", "The request could not be completed", purpose)
}
