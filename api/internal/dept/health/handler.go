package health

import (
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
	mysqlDriver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
)

type Handler struct {
	database      *gorm.DB
	auth          *auth.OIDCAuthenticator
	vault         crypto.Transit
	audit         *audit.Writer
	encryptionKey string
	hmacKey       string
}

type lookupRequest struct {
	NationalID string `json:"national_id"`
	Purpose    string `json:"purpose"`
}

type birthRequest struct {
	FirstNames        string               `json:"first_names"`
	LastName          string               `json:"last_name"`
	DateOfBirth       string               `json:"date_of_birth"`
	Sex               db.Sex               `json:"sex"`
	CitizenshipStatus db.CitizenshipStatus `json:"citizenship_status"`
	PlaceOfBirth      string               `json:"place_of_birth"`
	FacilityCode      *string              `json:"facility_code"`
	MotherPersonID    *uint64              `json:"mother_person_id"`
	FatherPersonID    *uint64              `json:"father_person_id"`
	CertificateNo     string               `json:"certificate_no"`
	Purpose           string               `json:"purpose"`
}

type deathRequest struct {
	NationalID   string  `json:"national_id"`
	DateOfDeath  string  `json:"date_of_death"`
	PlaceOfDeath *string `json:"place_of_death"`
	CauseCode    *string `json:"cause_code"`
	Certificate  string  `json:"certificate_no"`
	Purpose      string  `json:"purpose"`
}

type personView struct {
	NationalIDIndex []byte     `gorm:"column:nid_idx"`
	FirstNames      string     `gorm:"column:first_names"`
	LastName        string     `gorm:"column:last_name"`
	DateOfBirth     time.Time  `gorm:"column:date_of_birth"`
	Sex             db.Sex     `gorm:"column:sex"`
	DeceasedAt      *time.Time `gorm:"column:deceased_at"`
}

type personDTO struct {
	Match       bool   `json:"match"`
	FirstNames  string `json:"first_names,omitempty"`
	LastName    string `json:"last_name,omitempty"`
	DateOfBirth string `json:"date_of_birth,omitempty"`
	Sex         db.Sex `json:"sex,omitempty"`
	IsDeceased  *bool  `json:"is_deceased,omitempty"`
}

type birthSubmissionView struct {
	ID           uint64    `gorm:"column:id"`
	Certificate  string    `gorm:"column:certificate_no"`
	Place        string    `gorm:"column:place_of_birth"`
	Facility     *string   `gorm:"column:facility_code"`
	RegisteredBy uint64    `gorm:"column:registered_by"`
	RegisteredAt time.Time `gorm:"column:registered_at"`
	FirstNames   string    `gorm:"column:first_names"`
	LastName     string    `gorm:"column:last_name"`
	DateOfBirth  time.Time `gorm:"column:date_of_birth"`
}

type deathSubmissionView struct {
	ID           uint64    `gorm:"column:id"`
	Certificate  string    `gorm:"column:certificate_no"`
	DateOfDeath  time.Time `gorm:"column:date_of_death"`
	Place        *string   `gorm:"column:place_of_death"`
	Cause        *string   `gorm:"column:cause_code"`
	RegisteredBy uint64    `gorm:"column:registered_by"`
	RegisteredAt time.Time `gorm:"column:registered_at"`
}

type submissionDTO struct {
	ID           uint64  `json:"id"`
	Certificate  string  `json:"certificate_no"`
	Date         string  `json:"date"`
	FirstNames   string  `json:"first_names,omitempty"`
	LastName     string  `json:"last_name,omitempty"`
	Place        *string `json:"place,omitempty"`
	Facility     *string `json:"facility_code,omitempty"`
	Cause        *string `json:"cause_code,omitempty"`
	RegisteredAt string  `json:"registered_at"`
}

func NewHandler(pools *db.Pools, authenticator *auth.OIDCAuthenticator, vault crypto.Transit, writer *audit.Writer, encryptionKey, hmacKey string) (*Handler, error) {
	database, err := pools.For("health")
	if err != nil {
		return nil, err
	}
	if authenticator == nil || vault == nil || writer == nil {
		return nil, errors.New("health handler requires authentication, transit, and audit services")
	}
	return &Handler{
		database: database, auth: authenticator, vault: vault, audit: writer,
		encryptionKey: encryptionKey, hmacKey: hmacKey,
	}, nil
}

func (h *Handler) Register(r *router.Router) {
	h.route(r, http.MethodPost, "/v1/health/births", "Register a birth", "clerk", h.registerBirth)
	h.route(r, http.MethodPost, "/v1/health/deaths", "Register a death", "clerk", h.registerDeath)
	h.route(r, http.MethodPost, "/v1/health/persons/lookup", "Look up a person", "clerk", h.lookupPerson)
	h.route(r, http.MethodGet, "/v1/health/submissions/births", "List own birth submissions", "clerk", h.birthSubmissions)
	h.route(r, http.MethodGet, "/v1/health/submissions/deaths", "List own death submissions", "clerk", h.deathSubmissions)
}

func (h *Handler) route(r *router.Router, method, path, summary, minimumRole string, handler gin.HandlerFunc) {
	r.HandleProtected(method, path, summary, "health", minimumRole, h.auth.Require("health", minimumRole), handler)
}

func (h *Handler) registerBirth(c *gin.Context) {
	var request birthRequest
	if err := api.DecodeJSON(c, &request, 1<<20); err != nil {
		h.failure(c, "REGISTER_BIRTH", db.AuditOutcomeError, http.StatusBadRequest, httpx.CodeBadRequest, "Birth details are invalid")
		return
	}
	dob, err := time.Parse("2006-01-02", request.DateOfBirth)
	if err != nil || strings.TrimSpace(request.FirstNames) == "" || strings.TrimSpace(request.LastName) == "" ||
		strings.TrimSpace(request.PlaceOfBirth) == "" || strings.TrimSpace(request.CertificateNo) == "" ||
		len(request.FirstNames) > 120 || len(request.LastName) > 80 || len(request.PlaceOfBirth) > 120 ||
		len(request.CertificateNo) > 30 || !validSex(request.Sex) || !validCitizenship(request.CitizenshipStatus) ||
		!api.ValidPurpose(request.Purpose) {
		h.failure(c, "REGISTER_BIRTH", db.AuditOutcomeError, http.StatusBadRequest, httpx.CodeBadRequest, "Birth details are invalid")
		return
	}
	principal, ok := auth.RequestClaims(c)
	if !ok {
		httpx.InternalError(c.Writer, c.Request)
		return
	}
	nationalID, err := nid.Generate(dob, string(request.Sex), string(request.CitizenshipStatus))
	if err != nil {
		h.failure(c, "REGISTER_BIRTH", db.AuditOutcomeError, http.StatusInternalServerError, httpx.CodeInternal, httpx.MessageInternal)
		return
	}
	index, err := h.vault.BlindIndex(c.Request.Context(), h.hmacKey, nationalID)
	if err != nil {
		h.failure(c, "REGISTER_BIRTH", db.AuditOutcomeError, http.StatusInternalServerError, httpx.CodeInternal, httpx.MessageInternal)
		return
	}
	encrypted, err := h.vault.Encrypt(c.Request.Context(), h.encryptionKey, []byte(nationalID))
	if err != nil {
		h.failure(c, "REGISTER_BIRTH", db.AuditOutcomeError, http.StatusInternalServerError, httpx.CodeInternal, httpx.MessageInternal)
		return
	}
	motherID := optionalID(request.MotherPersonID)
	fatherID := optionalID(request.FatherPersonID)
	event := h.event(c, principal, "REGISTER_BIRTH", index, request.Purpose, db.AuditOutcomeOK)
	err = h.audit.WithPreparedEvent(c.Request.Context(), event, func(row db.AuditLog) error {
		return h.database.WithContext(c.Request.Context()).Exec(
			"CALL sp_register_birth(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)",
			index, encrypted, strings.TrimSpace(request.FirstNames), strings.TrimSpace(request.LastName),
			dob, request.Sex, request.CitizenshipStatus, strings.TrimSpace(request.PlaceOfBirth),
			request.FacilityCode, motherID, fatherID, strings.TrimSpace(request.CertificateNo),
			principal.OfficialID, request.Purpose, row.ChainID, row.Seq, row.OccurredAt,
			row.RequestID, row.PreviousHash, row.RowHash,
		).Error
	})
	if err != nil {
		h.procedureFailure(c, err, "REGISTER_BIRTH")
		return
	}
	httpx.JSON(c.Writer, http.StatusCreated, map[string]any{
		"status":             "registered",
		"national_id_masked": "*********" + nationalID[len(nationalID)-4:],
		"certificate_no":     strings.TrimSpace(request.CertificateNo),
	})
}

func (h *Handler) registerDeath(c *gin.Context) {
	var request deathRequest
	if err := api.DecodeJSON(c, &request, 1<<20); err != nil || nid.ValidateFormat(request.NationalID) != nil ||
		!api.ValidPurpose(request.Purpose) || strings.TrimSpace(request.Certificate) == "" {
		h.failure(c, "REGISTER_DEATH", db.AuditOutcomeError, http.StatusBadRequest, httpx.CodeBadRequest, "Death details are invalid")
		return
	}
	dateOfDeath, err := time.Parse("2006-01-02", request.DateOfDeath)
	if err != nil {
		h.failure(c, "REGISTER_DEATH", db.AuditOutcomeError, http.StatusBadRequest, httpx.CodeBadRequest, "Death details are invalid")
		return
	}
	index, err := h.vault.BlindIndex(c.Request.Context(), h.hmacKey, request.NationalID)
	if err != nil {
		h.failure(c, "REGISTER_DEATH", db.AuditOutcomeError, http.StatusInternalServerError, httpx.CodeInternal, httpx.MessageInternal)
		return
	}
	principal, ok := auth.RequestClaims(c)
	if !ok {
		httpx.InternalError(c.Writer, c.Request)
		return
	}
	err = h.audit.WithPreparedEvent(c.Request.Context(), h.event(c, principal, "REGISTER_DEATH", index, request.Purpose, db.AuditOutcomeOK), func(row db.AuditLog) error {
		return h.database.WithContext(c.Request.Context()).Exec(
			"CALL sp_register_death(?,?,?,?,?,?,?,?,?,?,?,?,?)",
			index, dateOfDeath, request.PlaceOfDeath, request.CauseCode, strings.TrimSpace(request.Certificate),
			principal.OfficialID, request.Purpose, row.ChainID, row.Seq, row.OccurredAt, row.RequestID,
			row.PreviousHash, row.RowHash,
		).Error
	})
	if err != nil {
		h.procedureFailure(c, err, "REGISTER_DEATH")
		return
	}
	httpx.JSON(c.Writer, http.StatusCreated, map[string]string{
		"status": "registered", "certificate_no": strings.TrimSpace(request.Certificate),
	})
}

func (h *Handler) lookupPerson(c *gin.Context) {
	var request lookupRequest
	if err := api.DecodeJSON(c, &request, 1<<20); err != nil || nid.ValidateFormat(request.NationalID) != nil ||
		!api.ValidPurpose(request.Purpose) {
		h.failure(c, "LOOKUP_PERSON", db.AuditOutcomeError, http.StatusBadRequest, httpx.CodeBadRequest, "Lookup details are invalid")
		return
	}
	index, err := h.vault.BlindIndex(c.Request.Context(), h.hmacKey, request.NationalID)
	if err != nil {
		h.failure(c, "LOOKUP_PERSON", db.AuditOutcomeError, http.StatusInternalServerError, httpx.CodeInternal, httpx.MessageInternal)
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
		h.event(c, principal, "LOOKUP_PERSON", index, request.Purpose, db.AuditOutcomeOK),
		func(tx *gorm.DB) error {
			if err := tx.Table("v_health_person").Where("nid_idx = ?", index).Take(&person).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return nil
				}
				return err
			}
			found = true
			return nil
		})
	if err != nil {
		h.failure(c, "LOOKUP_PERSON", db.AuditOutcomeError, http.StatusInternalServerError, httpx.CodeInternal, httpx.MessageInternal)
		return
	}
	if !found {
		httpx.JSON(c.Writer, http.StatusOK, map[string]bool{"match": false})
		return
	}
	response := personDTO{
		Match: true, FirstNames: person.FirstNames, LastName: person.LastName,
		DateOfBirth: person.DateOfBirth.Format("2006-01-02"), Sex: person.Sex,
	}
	isDeceased := person.DeceasedAt != nil
	response.IsDeceased = &isDeceased
	httpx.JSON(c.Writer, http.StatusOK, response)
}

func (h *Handler) birthSubmissions(c *gin.Context) {
	h.listSubmissions(c, "v_health_birth_submissions", "READ_BIRTH_SUBMISSIONS", func(tx *gorm.DB, officialID uint64, cursor uint64, limit int) ([]submissionDTO, uint64, bool, error) {
		var rows []birthSubmissionView
		if err := tx.Where("registered_by = ? AND id > ?", officialID, cursor).Order("id").Limit(limit + 1).Find(&rows).Error; err != nil {
			return nil, 0, false, err
		}
		hasMore := len(rows) > limit
		if hasMore {
			rows = rows[:limit]
		}
		result := make([]submissionDTO, 0, len(rows))
		for _, row := range rows {
			result = append(result, submissionDTO{
				ID: row.ID, Certificate: row.Certificate, Date: row.DateOfBirth.Format("2006-01-02"),
				FirstNames: row.FirstNames, LastName: row.LastName,
				Place: &row.Place, Facility: row.Facility,
				RegisteredAt: row.RegisteredAt.UTC().Format(time.RFC3339Nano),
			})
		}
		if hasMore {
			return result, rows[len(rows)-1].ID, true, nil
		}
		return result, 0, false, nil
	})
}

func (h *Handler) deathSubmissions(c *gin.Context) {
	h.listSubmissions(c, "v_health_death_submissions", "READ_DEATH_SUBMISSIONS", func(tx *gorm.DB, officialID uint64, cursor uint64, limit int) ([]submissionDTO, uint64, bool, error) {
		var rows []deathSubmissionView
		if err := tx.Where("registered_by = ? AND id > ?", officialID, cursor).Order("id").Limit(limit + 1).Find(&rows).Error; err != nil {
			return nil, 0, false, err
		}
		hasMore := len(rows) > limit
		if hasMore {
			rows = rows[:limit]
		}
		result := make([]submissionDTO, 0, len(rows))
		for _, row := range rows {
			result = append(result, submissionDTO{
				ID: row.ID, Certificate: row.Certificate, Date: row.DateOfDeath.Format("2006-01-02"),
				Place: row.Place, Cause: row.Cause, RegisteredAt: row.RegisteredAt.UTC().Format(time.RFC3339Nano),
			})
		}
		if hasMore {
			return result, rows[len(rows)-1].ID, true, nil
		}
		return result, 0, false, nil
	})
}

func (h *Handler) listSubmissions(c *gin.Context, view, action string, query func(*gorm.DB, uint64, uint64, int) ([]submissionDTO, uint64, bool, error)) {
	purpose := strings.TrimSpace(c.Query("purpose"))
	if !api.ValidPurpose(purpose) {
		h.failure(c, action, db.AuditOutcomeError, http.StatusBadRequest, httpx.CodeBadRequest, "Purpose is required")
		return
	}
	limit, err := api.PageSize(c)
	if err != nil {
		h.failure(c, action, db.AuditOutcomeError, http.StatusBadRequest, httpx.CodeBadRequest, "Page size is invalid")
		return
	}
	cursor, err := api.Cursor(c)
	if err != nil {
		h.failure(c, action, db.AuditOutcomeError, http.StatusBadRequest, httpx.CodeBadRequest, "Page cursor is invalid")
		return
	}
	principal, ok := auth.RequestClaims(c)
	if !ok {
		httpx.InternalError(c.Writer, c.Request)
		return
	}
	var items []submissionDTO
	var lastID uint64
	var hasMore bool
	err = h.audit.InTransactionOn(c.Request.Context(), h.database,
		h.event(c, principal, action, nil, purpose, db.AuditOutcomeOK),
		func(tx *gorm.DB) error {
			var err error
			items, lastID, hasMore, err = query(tx.Table(view), principal.OfficialID, cursor, limit)
			return err
		})
	if err != nil {
		h.failure(c, action, db.AuditOutcomeError, http.StatusInternalServerError, httpx.CodeInternal, httpx.MessageInternal)
		return
	}
	var next any
	if hasMore {
		next = lastID
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

func (h *Handler) failure(c *gin.Context, action string, outcome db.AuditOutcome, status int, code, message string) {
	principal, ok := auth.RequestClaims(c)
	if !ok {
		httpx.InternalError(c.Writer, c.Request)
		return
	}
	if err := h.audit.Write(c.Request.Context(), h.event(c, principal, action, nil, "", outcome)); err != nil {
		httpx.Error(c.Writer, c.Request, http.StatusServiceUnavailable, "AUDIT_UNAVAILABLE", "Request could not be audited")
		return
	}
	httpx.Error(c.Writer, c.Request, status, code, message)
}

func (h *Handler) procedureFailure(c *gin.Context, err error, action string) {
	var mysqlErr *mysqlDriver.MySQLError
	if errors.As(err, &mysqlErr) && mysqlErr.Number == 1644 {
		h.failure(c, action, db.AuditOutcomeError, http.StatusConflict, "REQUEST_CONFLICT", "The requested health record could not be applied")
		return
	}
	h.failure(c, action, db.AuditOutcomeError, http.StatusServiceUnavailable, "DATABASE_UNAVAILABLE", "The request could not be completed")
}

func optionalID(value *uint64) any {
	if value == nil || *value == 0 {
		return nil
	}
	return *value
}

func validSex(value db.Sex) bool {
	return value == db.SexMale || value == db.SexFemale || value == db.SexOther
}

func validCitizenship(value db.CitizenshipStatus) bool {
	return value == db.CitizenshipCitizen || value == db.CitizenshipPermanentResident ||
		value == db.CitizenshipForeign || value == db.CitizenshipStateless
}
