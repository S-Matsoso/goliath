package identity

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
	database      *gorm.DB
	auth          *auth.Authenticator
	transit       crypto.Transit
	audit         *audit.Writer
	encryptionKey string
	hmacKey       string
}

func NewHandler(database *gorm.DB, authenticator *auth.Authenticator, transit crypto.Transit, writer *audit.Writer, encryptionKey, hmacKey string) *Handler {
	return &Handler{
		database:      database,
		auth:          authenticator,
		transit:       transit,
		audit:         writer,
		encryptionKey: encryptionKey,
		hmacKey:       hmacKey,
	}
}

type createPersonRequest struct {
	NationalID        string               `json:"national_id"`
	FirstNames        string               `json:"first_names"`
	LastName          string               `json:"last_name"`
	DateOfBirth       string               `json:"date_of_birth"`
	Sex               db.Sex               `json:"sex"`
	CitizenshipStatus db.CitizenshipStatus `json:"citizenship_status"`
	MaritalStatus     db.MaritalStatus     `json:"marital_status"`
}

type lookupPersonRequest struct {
	NationalID string `json:"national_id"`
}

type addressRequest struct {
	Line1      string  `json:"line1"`
	City       string  `json:"city"`
	District   string  `json:"district"`
	PostalCode *string `json:"postal_code"`
	ValidFrom  string  `json:"valid_from"`
}

type marriageRequest struct {
	SpouseAID     uint64 `json:"spouse_a_id"`
	SpouseBID     uint64 `json:"spouse_b_id"`
	MarriedOn     string `json:"married_on"`
	CertificateNo string `json:"certificate_no"`
}

type addressResponse struct {
	Line1      string  `json:"line1"`
	City       string  `json:"city"`
	District   string  `json:"district"`
	PostalCode *string `json:"postal_code,omitempty"`
	ValidFrom  string  `json:"valid_from"`
}

type personResponse struct {
	ID                uint64               `json:"id"`
	NationalID        string               `json:"national_id"`
	FirstNames        string               `json:"first_names"`
	LastName          string               `json:"last_name"`
	DateOfBirth       string               `json:"date_of_birth"`
	Sex               db.Sex               `json:"sex"`
	CitizenshipStatus db.CitizenshipStatus `json:"citizenship_status"`
	MaritalStatus     db.MaritalStatus     `json:"marital_status"`
	DeceasedAt        *string              `json:"deceased_at,omitempty"`
	Addresses         []addressResponse    `json:"addresses"`
}

// Register mounts the identity routes allowed by the role-scoped API design.
func (h *Handler) Register(r *router.Router) {
	r.Handle(http.MethodPost, "/v1/identity/persons", "Register a person", h.auth.Require("identity", "supervisor"), h.createPerson)
	r.Handle(http.MethodPost, "/v1/identity/persons/lookup", "Look up a person by national ID", h.auth.Require("identity", "clerk"), h.lookupPerson)
	r.Handle(http.MethodPatch, "/v1/identity/persons/:id/address", "Change a person's address", h.auth.Require("identity", "clerk"), h.changeAddress)
	r.Handle(http.MethodPost, "/v1/identity/marriages", "Record a marriage", h.auth.Require("identity", "supervisor"), h.createMarriage)
}

func (h *Handler) createPerson(c *gin.Context) {
	var request createPersonRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		h.badRequest(c, "REGISTER_PERSON", "Invalid JSON request")
		return
	}
	dateOfBirth, err := time.Parse("2006-01-02", request.DateOfBirth)
	if err != nil || strings.TrimSpace(request.FirstNames) == "" || strings.TrimSpace(request.LastName) == "" ||
		len(request.FirstNames) > 120 || len(request.LastName) > 80 ||
		!validSex(request.Sex) || !validCitizenship(request.CitizenshipStatus) {
		h.badRequest(c, "REGISTER_PERSON", "Person details are invalid")
		return
	}
	if request.MaritalStatus == "" {
		request.MaritalStatus = db.MaritalSingle
	}
	if !validMaritalStatus(request.MaritalStatus) {
		h.badRequest(c, "REGISTER_PERSON", "Marital status is invalid")
		return
	}
	if err := nid.Validate(request.NationalID, dateOfBirth); err != nil {
		h.badRequest(c, "REGISTER_PERSON", "National ID is invalid")
		return
	}

	subjectIndex, err := h.transit.BlindIndex(c.Request.Context(), h.hmacKey, request.NationalID)
	if err != nil {
		h.internalError(c, "REGISTER_PERSON")
		return
	}
	encryptedID, err := h.transit.Encrypt(c.Request.Context(), h.encryptionKey, []byte(request.NationalID))
	if err != nil {
		h.internalError(c, "REGISTER_PERSON")
		return
	}
	person := db.Person{
		NationalIDIndex:     subjectIndex,
		NationalIDEncrypted: encryptedID,
		FirstNames:          request.FirstNames,
		LastName:            request.LastName,
		DateOfBirth:         dateOfBirth,
		Sex:                 request.Sex,
		CitizenshipStatus:   request.CitizenshipStatus,
		MaritalStatus:       request.MaritalStatus,
	}
	principal, ok := auth.RequestPrincipal(c)
	if !ok {
		httpx.InternalError(c.Writer, c.Request)
		return
	}
	err = h.audit.InTransaction(c.Request.Context(), h.event(c, principal, "REGISTER_PERSON", subjectIndex, db.AuditOutcomeOK), func(tx *gorm.DB) error {
		return tx.Create(&person).Error
	})
	if err != nil {
		h.internalError(c, "REGISTER_PERSON")
		return
	}

	httpx.JSON(c.Writer, http.StatusCreated, h.personDTO(person, request.NationalID, []addressResponse{}))
}

func (h *Handler) lookupPerson(c *gin.Context) {
	var request lookupPersonRequest
	if err := c.ShouldBindJSON(&request); err != nil || nid.ValidateFormat(request.NationalID) != nil {
		h.badRequest(c, "READ_PERSON", "National ID is invalid")
		return
	}
	index, err := h.transit.BlindIndex(c.Request.Context(), h.hmacKey, request.NationalID)
	if err != nil {
		h.internalError(c, "READ_PERSON")
		return
	}
	principal, ok := auth.RequestPrincipal(c)
	if !ok {
		httpx.InternalError(c.Writer, c.Request)
		return
	}
	var response personResponse
	err = h.audit.InTransaction(c.Request.Context(), h.event(c, principal, "READ_PERSON", index, db.AuditOutcomeOK), func(tx *gorm.DB) error {
		var person db.Person
		if err := tx.Where("nid_idx = ?", index).First(&person).Error; err != nil {
			return err
		}
		plainID, err := h.transit.Decrypt(c.Request.Context(), h.encryptionKey, person.NationalIDEncrypted)
		if err != nil {
			return err
		}
		response = h.personDTO(person, string(plainID), nil)
		var addresses []db.Address
		if err := tx.Where("person_id = ? AND is_current = ?", person.ID, true).Find(&addresses).Error; err != nil {
			return err
		}
		for _, address := range addresses {
			response.Addresses = append(response.Addresses, addressDTO(address))
		}
		if response.Addresses == nil {
			response.Addresses = []addressResponse{}
		}
		return nil
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		h.notFound(c, "READ_PERSON")
		return
	}
	if err != nil {
		h.internalError(c, "READ_PERSON")
		return
	}
	httpx.JSON(c.Writer, http.StatusOK, response)
}

func (h *Handler) changeAddress(c *gin.Context) {
	personID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || personID == 0 {
		h.badRequest(c, "ADDRESS_CHANGED", "Person ID is invalid")
		return
	}
	var request addressRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		h.badRequest(c, "ADDRESS_CHANGED", "Invalid JSON request")
		return
	}
	if request.ValidFrom == "" {
		request.ValidFrom = time.Now().UTC().Format("2006-01-02")
	}
	validFrom, err := time.Parse("2006-01-02", request.ValidFrom)
	if err != nil || request.Line1 == "" || request.City == "" || request.District == "" {
		h.badRequest(c, "ADDRESS_CHANGED", "Address details are invalid")
		return
	}
	principal, ok := auth.RequestPrincipal(c)
	if !ok {
		httpx.InternalError(c.Writer, c.Request)
		return
	}
	var existing db.Person
	err = h.database.WithContext(c.Request.Context()).Select("id", "nid_idx").First(&existing, personID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		h.notFound(c, "ADDRESS_CHANGED")
		return
	}
	if err != nil {
		h.internalError(c, "ADDRESS_CHANGED")
		return
	}
	err = h.audit.InTransaction(c.Request.Context(), h.event(c, principal, "ADDRESS_CHANGED", existing.NationalIDIndex, db.AuditOutcomeOK), func(tx *gorm.DB) error {
		var person db.Person
		if err := tx.Select("id", "nid_idx").First(&person, personID).Error; err != nil {
			return err
		}
		if err := tx.Model(&db.Address{}).
			Where("person_id = ? AND is_current = ?", personID, true).
			Updates(map[string]any{"is_current": false, "valid_to": validFrom}).Error; err != nil {
			return err
		}
		address := db.Address{
			PersonID:   personID,
			Line1:      request.Line1,
			City:       request.City,
			District:   request.District,
			PostalCode: request.PostalCode,
			IsCurrent:  true,
			ValidFrom:  validFrom,
		}
		return tx.Create(&address).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		h.notFound(c, "ADDRESS_CHANGED")
		return
	}
	if err != nil {
		h.internalError(c, "ADDRESS_CHANGED")
		return
	}
	httpx.JSON(c.Writer, http.StatusOK, map[string]string{"status": "updated"})
}

func (h *Handler) createMarriage(c *gin.Context) {
	var request marriageRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		h.badRequest(c, "REGISTER_MARRIAGE", "Invalid JSON request")
		return
	}
	marriedOn, err := time.Parse("2006-01-02", request.MarriedOn)
	if err != nil || request.SpouseAID == 0 || request.SpouseBID == 0 ||
		request.SpouseAID == request.SpouseBID || request.CertificateNo == "" || marriedOn.After(time.Now()) {
		h.badRequest(c, "REGISTER_MARRIAGE", "Marriage details are invalid")
		return
	}
	principal, ok := auth.RequestPrincipal(c)
	if !ok {
		httpx.InternalError(c.Writer, c.Request)
		return
	}
	marriage := db.Marriage{
		SpouseAID:     request.SpouseAID,
		SpouseBID:     request.SpouseBID,
		MarriedOn:     marriedOn,
		CertificateNo: request.CertificateNo,
	}
	err = h.audit.InTransaction(c.Request.Context(), h.event(c, principal, "REGISTER_MARRIAGE", nil, db.AuditOutcomeOK), func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&db.Person{}).Where("id IN ?", []uint64{request.SpouseAID, request.SpouseBID}).Count(&count).Error; err != nil {
			return err
		}
		if count != 2 {
			return gorm.ErrRecordNotFound
		}
		return tx.Create(&marriage).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		h.badRequest(c, "REGISTER_MARRIAGE", "Both spouses must be registered")
		return
	}
	if err != nil {
		h.internalError(c, "REGISTER_MARRIAGE")
		return
	}
	httpx.JSON(c.Writer, http.StatusCreated, map[string]any{
		"id":             marriage.ID,
		"spouse_a_id":    marriage.SpouseAID,
		"spouse_b_id":    marriage.SpouseBID,
		"married_on":     marriage.MarriedOn.Format("2006-01-02"),
		"certificate_no": marriage.CertificateNo,
	})
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
	event := h.event(c, principal, action, nil, db.AuditOutcomeError)
	if err := h.audit.Write(c.Request.Context(), event); err != nil {
		httpx.Error(c.Writer, c.Request, http.StatusServiceUnavailable, "AUDIT_UNAVAILABLE", "Request could not be audited")
		return false
	}
	return true
}

func (h *Handler) personDTO(person db.Person, nationalID string, addresses []addressResponse) personResponse {
	response := personResponse{
		ID:                person.ID,
		NationalID:        nationalID,
		FirstNames:        person.FirstNames,
		LastName:          person.LastName,
		DateOfBirth:       person.DateOfBirth.Format("2006-01-02"),
		Sex:               person.Sex,
		CitizenshipStatus: person.CitizenshipStatus,
		MaritalStatus:     person.MaritalStatus,
		Addresses:         addresses,
	}
	if person.DeceasedAt != nil {
		value := person.DeceasedAt.Format("2006-01-02")
		response.DeceasedAt = &value
	}
	return response
}

func addressDTO(address db.Address) addressResponse {
	return addressResponse{
		Line1:      address.Line1,
		City:       address.City,
		District:   address.District,
		PostalCode: address.PostalCode,
		ValidFrom:  address.ValidFrom.Format("2006-01-02"),
	}
}

func validSex(value db.Sex) bool {
	return value == db.SexMale || value == db.SexFemale || value == db.SexOther
}

func validCitizenship(value db.CitizenshipStatus) bool {
	return value == db.CitizenshipCitizen || value == db.CitizenshipPermanentResident ||
		value == db.CitizenshipForeign || value == db.CitizenshipStateless
}

func validMaritalStatus(value db.MaritalStatus) bool {
	return value == db.MaritalSingle || value == db.MaritalMarried ||
		value == db.MaritalDivorced || value == db.MaritalWidowed
}
