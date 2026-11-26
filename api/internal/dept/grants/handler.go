package grants

import (
	"bytes"
	"encoding/csv"
	"errors"
	"io"
	"net/http"
	"strconv"
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
	LastName   string `json:"last_name"`
	Purpose    string `json:"purpose"`
}

type personView struct {
	LastName    string    `gorm:"column:last_name"`
	DateOfBirth time.Time `gorm:"column:date_of_birth"`
	Alive       bool      `gorm:"column:is_alive"`
	District    *string   `gorm:"column:district"`
}

type deathAlertRow struct {
	ID       uint64    `gorm:"column:id"`
	Deceased time.Time `gorm:"column:deceased_at"`
	District *string   `gorm:"column:district"`
}

type deathAlertDTO struct {
	DateOfDeath string  `json:"date_of_death"`
	District    *string `json:"district,omitempty"`
}

func NewHandler(pools *db.Pools, authenticator *auth.OIDCAuthenticator, vault crypto.Transit, writer *audit.Writer, hmacKey string) (*Handler, error) {
	database, err := pools.For("grants")
	if err != nil {
		return nil, err
	}
	if authenticator == nil || vault == nil || writer == nil {
		return nil, errors.New("grants handler requires authentication, transit, and audit services")
	}
	return &Handler{database: database, auth: authenticator, vault: vault, audit: writer, hmacKey: hmacKey}, nil
}

func (h *Handler) Register(r *router.Router) {
	h.route(r, http.MethodPost, "/v1/grants/verify", "Verify a beneficiary", "clerk", h.verify)
	h.route(r, http.MethodPost, "/v1/grants/verify/batch", "Batch verify beneficiaries", "supervisor", h.verifyBatch)
	h.route(r, http.MethodGet, "/v1/grants/death-alerts", "Read death alerts", "supervisor", h.deathAlerts)
}

func (h *Handler) route(r *router.Router, method, path, summary, minimumRole string, handler gin.HandlerFunc) {
	r.HandleProtected(method, path, summary, "grants", minimumRole, h.auth.Require("grants", minimumRole), handler)
}

func (h *Handler) verify(c *gin.Context) {
	var request verifyRequest
	if err := api.DecodeJSON(c, &request, 1<<20); err != nil ||
		nid.ValidateFormat(request.NationalID) != nil || strings.TrimSpace(request.LastName) == "" ||
		len(request.LastName) > 80 || !api.ValidPurpose(request.Purpose) {
		h.failure(c, "VERIFY_BENEFICIARY", http.StatusBadRequest, httpx.CodeBadRequest, "Verification request is invalid")
		return
	}
	index, err := h.vault.BlindIndex(c.Request.Context(), h.hmacKey, request.NationalID)
	if err != nil {
		h.failure(c, "VERIFY_BENEFICIARY", http.StatusInternalServerError, httpx.CodeInternal, httpx.MessageInternal)
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
		h.event(c, principal, "VERIFY_BENEFICIARY", index, request.Purpose, db.AuditOutcomeOK),
		func(tx *gorm.DB) error {
			if err := tx.Table("v_grants_person").Where("nid_idx = ?", index).Take(&person).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return nil
				}
				return err
			}
			found = true
			return nil
		})
	if err != nil {
		h.failure(c, "VERIFY_BENEFICIARY", http.StatusInternalServerError, httpx.CodeInternal, httpx.MessageInternal)
		return
	}
	if !found || !strings.EqualFold(strings.TrimSpace(request.LastName), person.LastName) {
		httpx.JSON(c.Writer, http.StatusOK, map[string]bool{"match": false})
		return
	}
	httpx.JSON(c.Writer, http.StatusOK, map[string]any{
		"match":         true,
		"is_alive":      person.Alive,
		"date_of_birth": person.DateOfBirth.Format("2006-01-02"),
		"district":      person.District,
	})
}

type batchInput struct {
	NationalID string
	LastName   string
}

type batchPersonView struct {
	Index       []byte    `gorm:"column:nid_idx"`
	LastName    string    `gorm:"column:last_name"`
	DateOfBirth time.Time `gorm:"column:date_of_birth"`
	Alive       bool      `gorm:"column:is_alive"`
	District    *string   `gorm:"column:district"`
}

type batchResult struct {
	Match       bool
	Alive       bool
	DateOfBirth string
	District    string
}

func (h *Handler) verifyBatch(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8<<20)
	file, _, err := c.Request.FormFile("file")
	if err != nil {
		h.failure(c, "VERIFY_BENEFICIARIES_BATCH", http.StatusBadRequest, httpx.CodeBadRequest, "A valid CSV file is required")
		return
	}
	defer file.Close()

	purpose := strings.TrimSpace(c.Request.FormValue("purpose"))
	if !api.ValidPurpose(purpose) {
		h.failure(c, "VERIFY_BENEFICIARIES_BATCH", http.StatusBadRequest, httpx.CodeBadRequest, "Purpose is required")
		return
	}
	rows, err := parseBatchCSV(file)
	if err != nil {
		h.failureWithPurpose(c, "VERIFY_BENEFICIARIES_BATCH", http.StatusBadRequest, httpx.CodeBadRequest, "CSV must contain 1 to 5,000 valid rows with national_id and last_name columns", purpose)
		return
	}

	principal, ok := auth.RequestClaims(c)
	if !ok {
		httpx.InternalError(c.Writer, c.Request)
		return
	}
	indexByRow := make(map[int][]byte, len(rows))
	indexes := make([][]byte, 0, len(rows))
	for rowNumber, row := range rows {
		if nid.ValidateFormat(row.NationalID) != nil || strings.TrimSpace(row.LastName) == "" || len(row.LastName) > 80 {
			continue
		}
		index, err := h.vault.BlindIndex(c.Request.Context(), h.hmacKey, row.NationalID)
		if err != nil {
			h.failureWithPurpose(c, "VERIFY_BENEFICIARIES_BATCH", http.StatusInternalServerError, httpx.CodeInternal, httpx.MessageInternal, purpose)
			return
		}
		indexByRow[rowNumber] = index
		indexes = append(indexes, index)
	}

	peopleByIndex := make(map[string]batchPersonView, len(indexes))
	err = h.audit.InTransactionOn(c.Request.Context(), h.database,
		h.event(c, principal, "VERIFY_BENEFICIARIES_BATCH", nil, purpose, db.AuditOutcomeOK),
		func(tx *gorm.DB) error {
			if len(indexes) == 0 {
				return nil
			}
			var people []batchPersonView
			if err := tx.Table("v_grants_person").Where("nid_idx IN ?", indexes).Find(&people).Error; err != nil {
				return err
			}
			for _, person := range people {
				peopleByIndex[string(person.Index)] = person
			}
			return nil
		})
	if err != nil {
		h.failureWithPurpose(c, "VERIFY_BENEFICIARIES_BATCH", http.StatusInternalServerError, httpx.CodeInternal, httpx.MessageInternal, purpose)
		return
	}

	results := make([]batchResult, len(rows))
	for rowNumber, row := range rows {
		index, valid := indexByRow[rowNumber]
		if !valid {
			continue
		}
		person, found := peopleByIndex[string(index)]
		if !found || !strings.EqualFold(strings.TrimSpace(row.LastName), person.LastName) {
			continue
		}
		results[rowNumber] = batchResult{
			Match: true, Alive: person.Alive,
			DateOfBirth: person.DateOfBirth.Format("2006-01-02"),
		}
		if person.District != nil {
			results[rowNumber].District = *person.District
		}
	}

	var output bytes.Buffer
	if err := writeBatchCSV(&output, results); err != nil {
		h.failureWithPurpose(c, "VERIFY_BENEFICIARIES_BATCH", http.StatusInternalServerError, httpx.CodeInternal, httpx.MessageInternal, purpose)
		return
	}
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", `attachment; filename="beneficiary-verification.csv"`)
	c.Status(http.StatusOK)
	_, _ = c.Writer.Write(output.Bytes())
}

func parseBatchCSV(input io.Reader) ([]batchInput, error) {
	reader := csv.NewReader(input)
	reader.FieldsPerRecord = -1
	header, err := reader.Read()
	if err != nil || len(header) != 2 || strings.TrimSpace(header[0]) != "national_id" || strings.TrimSpace(header[1]) != "last_name" {
		return nil, errors.New("invalid CSV header")
	}
	rows := make([]batchInput, 0, 100)
	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil || len(record) != 2 {
			return nil, errors.New("invalid CSV row")
		}
		if len(rows) == 5000 {
			return nil, errors.New("too many CSV rows")
		}
		rows = append(rows, batchInput{NationalID: strings.TrimSpace(record[0]), LastName: strings.TrimSpace(record[1])})
	}
	if len(rows) == 0 {
		return nil, errors.New("CSV has no data rows")
	}
	return rows, nil
}

func boolString(match, alive bool) string {
	if !match {
		return ""
	}
	return strconv.FormatBool(alive)
}

func writeBatchCSV(output io.Writer, results []batchResult) error {
	writer := csv.NewWriter(output)
	if err := writer.Write([]string{"row_number", "match", "is_alive", "date_of_birth", "district"}); err != nil {
		return err
	}
	for rowNumber, result := range results {
		if err := writer.Write([]string{
			strconv.Itoa(rowNumber + 1), strconv.FormatBool(result.Match),
			boolString(result.Match, result.Alive), result.DateOfBirth, result.District,
		}); err != nil {
			return err
		}
	}
	writer.Flush()
	return writer.Error()
}

func (h *Handler) deathAlerts(c *gin.Context) {
	purpose := strings.TrimSpace(c.Query("purpose"))
	if !api.ValidPurpose(purpose) {
		h.failure(c, "READ_DEATH_ALERTS", http.StatusBadRequest, httpx.CodeBadRequest, "Purpose is required")
		return
	}
	limit, err := api.PageSize(c)
	if err != nil {
		h.failure(c, "READ_DEATH_ALERTS", http.StatusBadRequest, httpx.CodeBadRequest, "Page size is invalid")
		return
	}
	cursor, err := api.Cursor(c)
	if err != nil {
		h.failure(c, "READ_DEATH_ALERTS", http.StatusBadRequest, httpx.CodeBadRequest, "Page cursor is invalid")
		return
	}
	principal, ok := auth.RequestClaims(c)
	if !ok {
		httpx.InternalError(c.Writer, c.Request)
		return
	}
	var rows []deathAlertRow
	err = h.audit.InTransactionOn(c.Request.Context(), h.database,
		h.event(c, principal, "READ_DEATH_ALERTS", nil, purpose, db.AuditOutcomeOK),
		func(tx *gorm.DB) error {
			return tx.Table("v_grants_death_alerts").Where("id > ?", cursor).Order("id").Limit(limit + 1).Find(&rows).Error
		})
	if err != nil {
		h.failure(c, "READ_DEATH_ALERTS", http.StatusInternalServerError, httpx.CodeInternal, httpx.MessageInternal)
		return
	}
	var next any
	if len(rows) > limit {
		next = rows[limit-1].ID
		rows = rows[:limit]
	}
	items := make([]deathAlertDTO, 0, len(rows))
	for _, row := range rows {
		items = append(items, deathAlertDTO{
			DateOfDeath: row.Deceased.Format("2006-01-02"), District: row.District,
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
	h.failureWithPurpose(c, action, status, code, message, "")
}

func (h *Handler) failureWithPurpose(c *gin.Context, action string, status int, code, message, purpose string) {
	principal, ok := auth.RequestClaims(c)
	if !ok {
		httpx.InternalError(c.Writer, c.Request)
		return
	}
	if err := h.audit.Write(c.Request.Context(), h.event(c, principal, action, nil, purpose, db.AuditOutcomeError)); err != nil {
		httpx.Error(c.Writer, c.Request, http.StatusServiceUnavailable, "AUDIT_UNAVAILABLE", "Request could not be audited")
		return
	}
	httpx.Error(c.Writer, c.Request, status, code, message)
}
