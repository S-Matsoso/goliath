package registry

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/S-Matsoso/goliath/internal/audit"
	"github.com/S-Matsoso/goliath/internal/auth"
	"github.com/S-Matsoso/goliath/internal/crypto"
	"github.com/S-Matsoso/goliath/internal/db"
	"github.com/S-Matsoso/goliath/internal/nid"
	"github.com/S-Matsoso/goliath/internal/platform/httpx"
	"github.com/S-Matsoso/goliath/internal/platform/router"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type Handler struct {
	database *gorm.DB
	auth     *auth.Authenticator
	transit  crypto.Transit
	audit    *audit.Writer
	hmacKey  string
}

func NewHandler(database *gorm.DB, authenticator *auth.Authenticator, transit crypto.Transit, writer *audit.Writer, hmacKey string) *Handler {
	return &Handler{database: database, auth: authenticator, transit: transit, audit: writer, hmacKey: hmacKey}
}

type nationalIDRequest struct {
	NationalID string `json:"national_id"`
}

type grantsRequest struct {
	NationalID string `json:"national_id"`
	LastName   string `json:"last_name"`
}

type policeRequest struct {
	NationalID  string `json:"national_id"`
	DateOfBirth string `json:"date_of_birth"`
}

type healthPersonResponse struct {
	NationalID  string  `json:"national_id"`
	FirstNames  string  `json:"first_names"`
	LastName    string  `json:"last_name"`
	DateOfBirth string  `json:"date_of_birth"`
	Sex         db.Sex  `json:"sex"`
	DeceasedAt  *string `json:"deceased_at,omitempty"`
}

type grantsResponse struct {
	Match         bool             `json:"match"`
	Alive         bool             `json:"alive"`
	MaritalStatus db.MaritalStatus `json:"marital_status,omitempty"`
	Province      string           `json:"province,omitempty"`
	City          string           `json:"city,omitempty"`
}

type policeResponse struct {
	Match   bool `json:"match"`
	IsAlive bool `json:"is_alive"`
}

// Register mounts the read and verification endpoints allowed by the design.
func (h *Handler) Register(r *router.Router) {
	r.Handle(http.MethodPost, "/v1/health/persons/lookup", "Read a health person projection", h.auth.Require("health", "clerk"), h.healthPerson)
	r.Handle(http.MethodPost, "/v1/grants/verify", "Verify a grants identity", h.auth.Require("grants", "clerk"), h.verifyGrants)
	r.Handle(http.MethodPost, "/v1/police/verify", "Verify a police identity", h.auth.Require("police", "clerk"), h.verifyPolice)
	r.Handle(http.MethodGet, "/v1/audit/my-department", "Read department audit events", h.auth.Require("audit", "auditor"), h.departmentAudit)
}

func (h *Handler) healthPerson(c *gin.Context) {
	var request nationalIDRequest
	if err := c.ShouldBindJSON(&request); err != nil || nid.ValidateFormat(request.NationalID) != nil {
		h.badRequest(c, "READ_HEALTH_PERSON", "National ID is invalid")
		return
	}
	index, err := h.transit.BlindIndex(c.Request.Context(), h.hmacKey, request.NationalID)
	if err != nil {
		h.internalError(c, "READ_HEALTH_PERSON")
		return
	}
	principal, ok := auth.RequestPrincipal(c)
	if !ok {
		httpx.InternalError(c.Writer, c.Request)
		return
	}
	var response healthPersonResponse
	err = h.audit.InTransaction(c.Request.Context(), h.event(c, principal, "READ_HEALTH_PERSON", index, db.AuditOutcomeOK), func(tx *gorm.DB) error {
		var person db.Person
		if err := tx.Where("nid_idx = ?", index).First(&person).Error; err != nil {
			return err
		}
		if err := nid.Validate(request.NationalID, person.DateOfBirth); err != nil {
			return gorm.ErrRecordNotFound
		}
		response = healthPersonResponse{
			NationalID:  request.NationalID,
			FirstNames:  person.FirstNames,
			LastName:    person.LastName,
			DateOfBirth: person.DateOfBirth.Format("2006-01-02"),
			Sex:         person.Sex,
		}
		if person.DeceasedAt != nil {
			value := person.DeceasedAt.Format("2006-01-02")
			response.DeceasedAt = &value
		}
		return nil
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		h.notFound(c, "READ_HEALTH_PERSON")
		return
	}
	if err != nil {
		h.internalError(c, "READ_HEALTH_PERSON")
		return
	}
	httpx.JSON(c.Writer, http.StatusOK, response)
}

func (h *Handler) verifyGrants(c *gin.Context) {
	var request grantsRequest
	if err := c.ShouldBindJSON(&request); err != nil || nid.ValidateFormat(request.NationalID) != nil || strings.TrimSpace(request.LastName) == "" {
		h.badRequest(c, "VERIFY_GRANTS", "National ID and last name are required and valid")
		return
	}
	index, err := h.transit.BlindIndex(c.Request.Context(), h.hmacKey, request.NationalID)
	if err != nil {
		h.internalError(c, "VERIFY_GRANTS")
		return
	}
	principal, ok := auth.RequestPrincipal(c)
	if !ok {
		httpx.InternalError(c.Writer, c.Request)
		return
	}
	response := grantsResponse{}
	err = h.audit.InTransaction(c.Request.Context(), h.event(c, principal, "VERIFY_GRANTS", index, db.AuditOutcomeOK), func(tx *gorm.DB) error {
		var person db.Person
		if err := tx.Where("nid_idx = ?", index).First(&person).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		if !strings.EqualFold(person.LastName, strings.TrimSpace(request.LastName)) {
			return nil
		}
		response.Match = true
		response.Alive = person.DeceasedAt == nil
		response.MaritalStatus = person.MaritalStatus
		var address db.Address
		err := tx.Where("person_id = ? AND is_current = ?", person.ID, true).First(&address).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err == nil {
			response.Province = address.District
			response.City = address.City
		}
		return nil
	})
	if err != nil {
		h.internalError(c, "VERIFY_GRANTS")
		return
	}
	httpx.JSON(c.Writer, http.StatusOK, response)
}

func (h *Handler) verifyPolice(c *gin.Context) {
	var request policeRequest
	if err := c.ShouldBindJSON(&request); err != nil || request.NationalID == "" {
		h.badRequest(c, "VERIFY_POLICE", "National ID and date_of_birth are required")
		return
	}
	dateOfBirth, err := time.Parse("2006-01-02", request.DateOfBirth)
	if err != nil {
		h.badRequest(c, "VERIFY_POLICE", "date_of_birth must use YYYY-MM-DD format")
		return
	}
	if err := nid.Validate(request.NationalID, dateOfBirth); err != nil {
		h.badRequest(c, "VERIFY_POLICE", "National ID and date_of_birth do not match")
		return
	}
	index, err := h.transit.BlindIndex(c.Request.Context(), h.hmacKey, request.NationalID)
	if err != nil {
		h.internalError(c, "VERIFY_POLICE")
		return
	}
	principal, ok := auth.RequestPrincipal(c)
	if !ok {
		httpx.InternalError(c.Writer, c.Request)
		return
	}
	response := policeResponse{}
	err = h.audit.InTransaction(c.Request.Context(), h.event(c, principal, "VERIFY_POLICE", index, db.AuditOutcomeOK), func(tx *gorm.DB) error {
		var person db.Person
		err := tx.Select("id", "date_of_birth", "deceased_at").
			Where("nid_idx = ?", index).
			First(&person).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		response = policeVerdict(person, dateOfBirth)
		return nil
	})
	if err != nil {
		h.internalError(c, "VERIFY_POLICE")
		return
	}
	httpx.JSON(c.Writer, http.StatusOK, response)
}

func policeVerdict(person db.Person, dateOfBirth time.Time) policeResponse {
	match := person.DateOfBirth.Format("2006-01-02") == dateOfBirth.Format("2006-01-02")
	return policeResponse{Match: match, IsAlive: match && person.DeceasedAt == nil}
}

func (h *Handler) departmentAudit(c *gin.Context) {
	from, err := parseOptionalDate(c.Query("from"))
	if err != nil {
		h.badRequest(c, "READ_DEPARTMENT_AUDIT", "from must use YYYY-MM-DD format")
		return
	}
	to, err := parseOptionalDate(c.Query("to"))
	if err != nil {
		h.badRequest(c, "READ_DEPARTMENT_AUDIT", "to must use YYYY-MM-DD format")
		return
	}
	if from != nil && to != nil && to.Before(*from) {
		h.badRequest(c, "READ_DEPARTMENT_AUDIT", "to must not be before from")
		return
	}
	limit := 100
	if raw := c.Query("limit"); raw != "" {
		limit, err = strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 500 {
			h.badRequest(c, "READ_DEPARTMENT_AUDIT", "limit must be between 1 and 500")
			return
		}
	}
	cursor, err := parseUintQuery(c.Query("cursor"))
	if err != nil {
		h.badRequest(c, "READ_DEPARTMENT_AUDIT", "cursor must be a positive integer")
		return
	}
	principal, ok := auth.RequestPrincipal(c)
	if !ok {
		httpx.InternalError(c.Writer, c.Request)
		return
	}
	events := make([]db.AuditLog, 0, limit)
	err = h.audit.InTransaction(c.Request.Context(), h.event(c, principal, "READ_DEPARTMENT_AUDIT", nil, db.AuditOutcomeOK), func(tx *gorm.DB) error {
		query := tx.Where("department = ?", principal.Department)
		if from != nil {
			query = query.Where("occurred_at >= ?", *from)
		}
		if to != nil {
			query = query.Where("occurred_at < ?", to.AddDate(0, 0, 1))
		}
		if cursor > 0 {
			query = query.Where("id > ?", cursor)
		}
		return query.Order("id ASC").Limit(limit).Find(&events).Error
	})
	if err != nil {
		h.internalError(c, "READ_DEPARTMENT_AUDIT")
		return
	}
	httpx.JSON(c.Writer, http.StatusOK, map[string]any{"events": events})
}

func (h *Handler) event(c *gin.Context, principal auth.Principal, action string, subject []byte, outcome db.AuditOutcome) audit.Event {
	id := principal.OfficialID
	return audit.Event{
		OfficialID: &id,
		Department: principal.Department,
		Action:     action,
		Subject:    subject,
		Outcome:    outcome,
		RequestID:  httpx.RequestID(c.Request.Context()),
	}
}

func (h *Handler) badRequest(c *gin.Context, action, message string) {
	if !h.writeFailureAudit(c, action) {
		return
	}
	httpx.Error(c.Writer, c.Request, http.StatusBadRequest, httpx.CodeBadRequest, message)
}

func (h *Handler) notFound(c *gin.Context, action string) {
	if !h.writeFailureAudit(c, action) {
		return
	}
	httpx.Error(c.Writer, c.Request, http.StatusNotFound, httpx.CodeNotFound, "Resource not found")
}

func (h *Handler) internalError(c *gin.Context, action string) {
	if !h.writeFailureAudit(c, action) {
		return
	}
	httpx.InternalError(c.Writer, c.Request)
}

func (h *Handler) writeFailureAudit(c *gin.Context, action string) bool {
	principal, ok := auth.RequestPrincipal(c)
	if !ok {
		httpx.InternalError(c.Writer, c.Request)
		return false
	}
	if err := h.audit.Write(c.Request.Context(), h.event(c, principal, action, nil, db.AuditOutcomeError)); err != nil {
		httpx.Error(c.Writer, c.Request, http.StatusServiceUnavailable, "AUDIT_UNAVAILABLE", "Request could not be audited")
		return false
	}
	return true
}

func parseOptionalDate(value string) (*time.Time, error) {
	if value == "" {
		return nil, nil
	}
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func parseUintQuery(value string) (uint64, error) {
	if value == "" {
		return 0, nil
	}
	return strconv.ParseUint(value, 10, 64)
}
