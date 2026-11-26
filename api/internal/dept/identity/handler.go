package identity

import (
	"encoding/json"
	"errors"
	"fmt"
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

type registerPersonRequest struct {
	FirstNames        string               `json:"first_names"`
	LastName          string               `json:"last_name"`
	DateOfBirth       string               `json:"date_of_birth"`
	Sex               db.Sex               `json:"sex"`
	CitizenshipStatus db.CitizenshipStatus `json:"citizenship_status"`
	Purpose           string               `json:"purpose"`
}

type identityLookupRequest struct {
	NationalID string `json:"national_id"`
	Purpose    string `json:"purpose"`
}

type identitySearchRequest struct {
	FirstNames  string `json:"first_names"`
	LastName    string `json:"last_name"`
	DateOfBirth string `json:"date_of_birth"`
	Purpose     string `json:"purpose"`
}

type personIDRequest struct {
	PersonID uint64 `json:"person_id"`
	Purpose  string `json:"purpose"`
}

type addressRequest struct {
	Line1      string  `json:"line1"`
	Suburb     *string `json:"suburb"`
	City       string  `json:"city"`
	Province   string  `json:"province"`
	District   string  `json:"district"`
	WardCode   *string `json:"ward_code"`
	PostalCode *string `json:"postal_code"`
	ValidFrom  string  `json:"valid_from"`
	Purpose    string  `json:"purpose"`
}

type marriageRequest struct {
	SpouseAID     uint64 `json:"spouse_a_id"`
	SpouseBID     uint64 `json:"spouse_b_id"`
	MarriedOn     string `json:"married_on"`
	CertificateNo string `json:"certificate_no"`
	Purpose       string `json:"purpose"`
}

type changeDecisionRequest struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
	Purpose  string `json:"purpose"`
}

type idStatusRequest struct {
	Blocked bool   `json:"blocked"`
	Reason  string `json:"reason"`
	Purpose string `json:"purpose"`
}

type personDTO struct {
	ID                uint64               `json:"id"`
	NationalIDMasked  string               `json:"national_id_masked"`
	FirstNames        string               `json:"first_names"`
	LastName          string               `json:"last_name"`
	DateOfBirth       string               `json:"date_of_birth"`
	Sex               db.Sex               `json:"sex"`
	CitizenshipStatus db.CitizenshipStatus `json:"citizenship_status"`
	MaritalStatus     db.MaritalStatus     `json:"marital_status"`
	DeceasedAt        *string              `json:"deceased_at,omitempty"`
	IDBlocked         bool                 `json:"id_blocked"`
	IDBlockReason     *string              `json:"id_block_reason,omitempty"`
	Addresses         []addressDTO         `json:"addresses"`
	Marriages         []marriageDTO        `json:"marriages"`
	Biometrics        []biometricDTO       `json:"biometrics"`
}

type addressDTO struct {
	Line1      string  `json:"line1"`
	Suburb     *string `json:"suburb,omitempty"`
	City       string  `json:"city"`
	Province   string  `json:"province"`
	District   string  `json:"district"`
	WardCode   *string `json:"ward_code,omitempty"`
	PostalCode *string `json:"postal_code,omitempty"`
	ValidFrom  string  `json:"valid_from"`
	ValidTo    *string `json:"valid_to,omitempty"`
	IsCurrent  bool    `json:"is_current"`
}

type marriageDTO struct {
	ID            uint64  `json:"id"`
	SpouseAID     uint64  `json:"spouse_a_id"`
	SpouseBID     uint64  `json:"spouse_b_id"`
	MarriedOn     string  `json:"married_on"`
	DissolvedOn   *string `json:"dissolved_on,omitempty"`
	CertificateNo string  `json:"certificate_no"`
}

type biometricDTO struct {
	Kind       string `json:"kind"`
	CapturedAt string `json:"captured_at"`
}

type revealDTO struct {
	NationalID string `json:"national_id"`
}

type searchResultDTO struct {
	ID               uint64 `json:"id"`
	FirstNames       string `json:"first_names"`
	LastName         string `json:"last_name"`
	DateOfBirth      string `json:"date_of_birth"`
	NationalIDMasked string `json:"national_id_masked"`
}

type requestDTO struct {
	ID         uint64  `json:"id"`
	Type       string  `json:"type"`
	Payload    any     `json:"payload"`
	Department string  `json:"submitted_by_department"`
	OfficialID uint64  `json:"submitted_by_official"`
	Status     string  `json:"status"`
	ReviewedBy *uint64 `json:"reviewed_by,omitempty"`
	ReviewedAt *string `json:"reviewed_at,omitempty"`
	Reason     *string `json:"reason,omitempty"`
	CreatedAt  string  `json:"created_at"`
}

type addressModel struct {
	ID         uint64     `gorm:"column:id;primaryKey"`
	PersonID   uint64     `gorm:"column:person_id"`
	Line1      string     `gorm:"column:line1"`
	Suburb     *string    `gorm:"column:suburb"`
	City       string     `gorm:"column:city"`
	Province   string     `gorm:"column:province"`
	District   string     `gorm:"column:district"`
	WardCode   *string    `gorm:"column:ward_code"`
	PostalCode *string    `gorm:"column:postal_code"`
	IsCurrent  bool       `gorm:"column:is_current"`
	ValidFrom  time.Time  `gorm:"column:valid_from"`
	ValidTo    *time.Time `gorm:"column:valid_to"`
}

func (addressModel) TableName() string { return "addresses" }

type changeRequestModel struct {
	ID                    uint64     `gorm:"column:id"`
	RequestType           string     `gorm:"column:request_type"`
	TargetNIDIndex        []byte     `gorm:"column:target_nid_idx"`
	PayloadEncrypted      []byte     `gorm:"column:payload_enc"`
	SubmittedByDepartment string     `gorm:"column:submitted_by_department"`
	SubmittedByOfficial   uint64     `gorm:"column:submitted_by_official"`
	Status                string     `gorm:"column:status"`
	ReviewedBy            *uint64    `gorm:"column:reviewed_by"`
	ReviewedAt            *time.Time `gorm:"column:reviewed_at"`
	Reason                *string    `gorm:"column:reason"`
	CreatedAt             time.Time  `gorm:"column:created_at"`
}

func (changeRequestModel) TableName() string { return "change_requests" }

func NewHandler(pools *db.Pools, authenticator *auth.OIDCAuthenticator, vault crypto.Transit, writer *audit.Writer, encryptionKey, hmacKey string) (*Handler, error) {
	database, err := pools.For("identity")
	if err != nil {
		return nil, err
	}
	if authenticator == nil || vault == nil || writer == nil {
		return nil, errors.New("identity handler requires authentication, transit, and audit services")
	}
	return &Handler{
		database:      database,
		auth:          authenticator,
		vault:         vault,
		audit:         writer,
		encryptionKey: encryptionKey,
		hmacKey:       hmacKey,
	}, nil
}

func (h *Handler) Register(r *router.Router) {
	h.route(r, http.MethodPost, "/v1/identity/persons", "Register a person", "supervisor", h.registerPerson)
	h.route(r, http.MethodPost, "/v1/identity/persons/search", "Search by name and date of birth", "supervisor", h.searchPersons)
	h.route(r, http.MethodPost, "/v1/identity/persons/lookup", "Look up a person by national ID", "clerk", h.lookupPerson)
	h.route(r, http.MethodPost, "/v1/identity/persons/reveal", "Reveal a national ID", "supervisor", h.revealNationalID)
	h.route(r, http.MethodGet, "/v1/identity/persons/:id", "Read a full identity record", "clerk", h.readPerson)
	h.route(r, http.MethodGet, "/v1/identity/persons/:id/addresses", "Read address history", "clerk", h.addressHistory)
	h.route(r, http.MethodPost, "/v1/identity/persons/:id/addresses", "Change a person's address", "clerk", h.changeAddress)
	h.route(r, http.MethodPost, "/v1/identity/marriages", "Record a marriage", "supervisor", h.recordMarriage)
	h.route(r, http.MethodGet, "/v1/identity/change-requests", "Read the change-request inbox", "supervisor", h.listChangeRequests)
	h.route(r, http.MethodPost, "/v1/identity/change-requests/:id/review", "Approve or reject a change request", "supervisor", h.reviewChangeRequest)
	h.route(r, http.MethodPatch, "/v1/identity/persons/:id/id-status", "Block or unblock an identity document", "supervisor", h.setIDStatus)
	h.route(r, http.MethodGet, "/v1/identity/persons/:id/biometrics", "Read biometric enrollment status", "clerk", h.biometricStatus)
}

func (h *Handler) route(r *router.Router, method, path, summary, minimumRole string, handler gin.HandlerFunc) {
	r.HandleProtected(method, path, summary, "identity", minimumRole, h.auth.Require("identity", minimumRole), handler)
}

func (h *Handler) registerPerson(c *gin.Context) {
	var request registerPersonRequest
	if err := api.DecodeJSON(c, &request, 1<<20); err != nil {
		h.badRequest(c, "REGISTER_PERSON", "Invalid request")
		return
	}
	dateOfBirth, err := time.Parse("2006-01-02", request.DateOfBirth)
	if err != nil || strings.TrimSpace(request.FirstNames) == "" || strings.TrimSpace(request.LastName) == "" ||
		len(request.FirstNames) > 120 || len(request.LastName) > 80 ||
		!validSex(request.Sex) || !validCitizenship(request.CitizenshipStatus) || !validPurpose(request.Purpose) {
		h.badRequest(c, "REGISTER_PERSON", "Person details are invalid")
		return
	}
	principal, ok := auth.RequestClaims(c)
	if !ok {
		httpx.InternalError(c.Writer, c.Request)
		return
	}

	var person db.Person
	var masked string
	for attempt := 0; attempt < 5; attempt++ {
		nationalID, err := nid.Generate(dateOfBirth, string(request.Sex), string(request.CitizenshipStatus))
		if err != nil {
			h.internalError(c, "REGISTER_PERSON")
			return
		}
		index, err := h.vault.BlindIndex(c.Request.Context(), h.hmacKey, nationalID)
		if err != nil {
			h.internalError(c, "REGISTER_PERSON")
			return
		}
		encrypted, err := h.vault.Encrypt(c.Request.Context(), h.encryptionKey, []byte(nationalID))
		if err != nil {
			h.internalError(c, "REGISTER_PERSON")
			return
		}
		person = db.Person{
			NationalIDIndex:     index,
			NationalIDEncrypted: encrypted,
			FirstNames:          strings.TrimSpace(request.FirstNames),
			LastName:            strings.TrimSpace(request.LastName),
			DateOfBirth:         dateOfBirth,
			Sex:                 request.Sex,
			CitizenshipStatus:   request.CitizenshipStatus,
			MaritalStatus:       db.MaritalSingle,
		}
		masked = "*********" + nationalID[len(nationalID)-4:]
		err = h.audit.InTransactionOn(c.Request.Context(), h.database, eventWithPurpose(c, principal, "REGISTER_PERSON", index, request.Purpose, db.AuditOutcomeOK), func(tx *gorm.DB) error {
			if err := tx.Create(&person).Error; err != nil {
				return err
			}
			return appendOutbox(tx, "PERSON_REGISTERED", person.ID)
		})
		if err == nil {
			break
		}
		if !errors.Is(err, gorm.ErrDuplicatedKey) || attempt == 4 {
			h.internalError(c, "REGISTER_PERSON")
			return
		}
	}
	httpx.JSON(c.Writer, http.StatusCreated, map[string]any{
		"id": person.ID, "national_id_masked": masked, "first_names": person.FirstNames,
		"last_name": person.LastName, "date_of_birth": dateOfBirth.Format("2006-01-02"),
		"sex": person.Sex, "citizenship_status": person.CitizenshipStatus,
	})
}

func (h *Handler) lookupPerson(c *gin.Context) {
	var request identityLookupRequest
	if err := api.DecodeJSON(c, &request, 1<<20); err != nil || nid.ValidateFormat(request.NationalID) != nil || !validPurpose(request.Purpose) {
		h.badRequest(c, "READ_PERSON", "National ID is invalid")
		return
	}
	index, err := h.vault.BlindIndex(c.Request.Context(), h.hmacKey, request.NationalID)
	if err != nil {
		h.internalError(c, "READ_PERSON")
		return
	}
	principal, ok := auth.RequestClaims(c)
	if !ok {
		httpx.InternalError(c.Writer, c.Request)
		return
	}
	var response personDTO
	found := false
	err = h.audit.InTransactionOn(c.Request.Context(), h.database, eventWithPurpose(c, principal, "READ_PERSON", index, request.Purpose, db.AuditOutcomeOK), func(tx *gorm.DB) error {
		var person db.Person
		if err := tx.Where("nid_idx = ?", index).First(&person).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		found = true
		var err error
		response, err = h.personResponse(tx, c, person)
		return err
	})
	if err != nil {
		h.internalError(c, "READ_PERSON")
		return
	}
	if !found {
		httpx.Error(c.Writer, c.Request, http.StatusNotFound, httpx.CodeNotFound, "Person not found")
		return
	}
	httpx.JSON(c.Writer, http.StatusOK, response)
}

func (h *Handler) searchPersons(c *gin.Context) {
	var request identitySearchRequest
	if err := api.DecodeJSON(c, &request, 1<<20); err != nil {
		h.badRequest(c, "SEARCH_PERSONS", "Search details are invalid")
		return
	}
	request.FirstNames = strings.TrimSpace(request.FirstNames)
	request.LastName = strings.TrimSpace(request.LastName)
	dateOfBirth, err := time.Parse("2006-01-02", request.DateOfBirth)
	if err != nil || request.FirstNames == "" || request.LastName == "" ||
		len(request.FirstNames) > 120 || len(request.LastName) > 80 {
		h.badRequest(c, "SEARCH_PERSONS", "Search details are invalid")
		return
	}
	request.Purpose = strings.TrimSpace(request.Purpose)
	if !validPurpose(request.Purpose) {
		h.badRequest(c, "SEARCH_PERSONS", "Purpose is required")
		return
	}
	limit, err := api.PageSize(c)
	if err != nil {
		h.badRequest(c, "SEARCH_PERSONS", "Page size is invalid")
		return
	}
	cursor, err := api.Cursor(c)
	if err != nil {
		h.badRequest(c, "SEARCH_PERSONS", "Page cursor is invalid")
		return
	}
	principal, ok := auth.RequestClaims(c)
	if !ok {
		httpx.InternalError(c.Writer, c.Request)
		return
	}
	var response []searchResultDTO
	var nextCursor *uint64
	err = h.audit.InTransactionOn(c.Request.Context(), h.database, eventWithPurpose(c, principal, "SEARCH_PERSONS", nil, request.Purpose, db.AuditOutcomeOK), func(tx *gorm.DB) error {
		var people []db.Person
		if err := tx.Where("first_names = ? AND last_name = ? AND date_of_birth = ? AND id > ?",
			request.FirstNames, request.LastName, dateOfBirth, cursor).
			Order("id").Limit(limit + 1).Find(&people).Error; err != nil {
			return err
		}
		if len(people) > limit {
			value := people[limit-1].ID
			nextCursor = &value
			people = people[:limit]
		}
		response = make([]searchResultDTO, 0, len(people))
		for _, person := range people {
			masked, err := h.maskedNationalID(c, person.NationalIDEncrypted)
			if err != nil {
				return err
			}
			response = append(response, searchResultDTO{
				ID: person.ID, FirstNames: person.FirstNames, LastName: person.LastName,
				DateOfBirth: person.DateOfBirth.Format("2006-01-02"), NationalIDMasked: masked,
			})
		}
		return nil
	})
	if err != nil {
		h.internalError(c, "SEARCH_PERSONS")
		return
	}
	httpx.JSON(c.Writer, http.StatusOK, map[string]any{"items": response, "next_cursor": nextCursor})
}

func (h *Handler) readPerson(c *gin.Context) {
	personID, ok := pathID(c, "id")
	purpose := strings.TrimSpace(c.Query("purpose"))
	if !ok || !validPurpose(purpose) {
		h.badRequest(c, "READ_PERSON", "Person ID is invalid")
		return
	}
	principal, ok := auth.RequestClaims(c)
	if !ok {
		httpx.InternalError(c.Writer, c.Request)
		return
	}
	var response personDTO
	found := false
	err := h.audit.InTransactionOn(c.Request.Context(), h.database, eventWithPurpose(c, principal, "READ_PERSON", nil, purpose, db.AuditOutcomeOK), func(tx *gorm.DB) error {
		var person db.Person
		if err := tx.First(&person, personID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		found = true
		var err error
		response, err = h.personResponse(tx, c, person)
		return err
	})
	if err != nil {
		h.internalError(c, "READ_PERSON")
		return
	}
	if !found {
		httpx.Error(c.Writer, c.Request, http.StatusNotFound, httpx.CodeNotFound, "Person not found")
		return
	}
	httpx.JSON(c.Writer, http.StatusOK, response)
}

func (h *Handler) revealNationalID(c *gin.Context) {
	var request personIDRequest
	if err := api.DecodeJSON(c, &request, 1<<20); err != nil || request.PersonID == 0 || !validPurpose(request.Purpose) {
		h.badRequest(c, "REVEAL_NATIONAL_ID", "Person ID is invalid")
		return
	}
	principal, ok := auth.RequestClaims(c)
	if !ok {
		httpx.InternalError(c.Writer, c.Request)
		return
	}
	var value revealDTO
	found := false
	err := h.audit.InTransactionOn(c.Request.Context(), h.database, eventWithPurpose(c, principal, "REVEAL_NATIONAL_ID", nil, request.Purpose, db.AuditOutcomeOK), func(tx *gorm.DB) error {
		var person db.Person
		if err := tx.Select("id", "nid_enc").First(&person, request.PersonID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		found = true
		nationalID, err := h.vault.Decrypt(c.Request.Context(), h.encryptionKey, person.NationalIDEncrypted)
		if err != nil {
			return err
		}
		value.NationalID = string(nationalID)
		return nil
	})
	if err != nil {
		h.internalError(c, "REVEAL_NATIONAL_ID")
		return
	}
	if !found {
		httpx.Error(c.Writer, c.Request, http.StatusNotFound, httpx.CodeNotFound, "Person not found")
		return
	}
	httpx.JSON(c.Writer, http.StatusOK, value)
}

func (h *Handler) addressHistory(c *gin.Context) {
	personID, ok := pathID(c, "id")
	purpose := strings.TrimSpace(c.Query("purpose"))
	if !ok || !validPurpose(purpose) {
		h.badRequest(c, "READ_ADDRESS_HISTORY", "Person ID is invalid")
		return
	}
	limit, err := api.PageSize(c)
	if err != nil {
		h.badRequest(c, "READ_ADDRESS_HISTORY", "Page size is invalid")
		return
	}
	cursor, err := api.Cursor(c)
	if err != nil {
		h.badRequest(c, "READ_ADDRESS_HISTORY", "Page cursor is invalid")
		return
	}
	principal, ok := auth.RequestClaims(c)
	if !ok {
		httpx.InternalError(c.Writer, c.Request)
		return
	}
	var response []addressDTO
	var nextCursor *uint64
	err = h.audit.InTransactionOn(c.Request.Context(), h.database, eventWithPurpose(c, principal, "READ_ADDRESS_HISTORY", nil, purpose, db.AuditOutcomeOK), func(tx *gorm.DB) error {
		var addresses []addressModel
		if err := tx.Where("person_id = ? AND id > ?", personID, cursor).Order("id").Limit(limit + 1).Find(&addresses).Error; err != nil {
			return err
		}
		if len(addresses) > limit {
			value := addresses[limit-1].ID
			nextCursor = &value
			addresses = addresses[:limit]
		}
		response = make([]addressDTO, 0, len(addresses))
		for _, address := range addresses {
			response = append(response, toAddressDTO(address))
		}
		return nil
	})
	if err != nil {
		h.internalError(c, "READ_ADDRESS_HISTORY")
		return
	}
	httpx.JSON(c.Writer, http.StatusOK, map[string]any{"items": response, "next_cursor": nextCursor})
}

func (h *Handler) changeAddress(c *gin.Context) {
	personID, ok := pathID(c, "id")
	var request addressRequest
	if !ok || api.DecodeJSON(c, &request, 1<<20) != nil {
		h.badRequest(c, "ADDRESS_CHANGED", "Address request is invalid")
		return
	}
	if !validPurpose(request.Purpose) {
		h.badRequest(c, "ADDRESS_CHANGED", "Purpose is required")
		return
	}
	if request.ValidFrom == "" {
		request.ValidFrom = time.Now().UTC().Format("2006-01-02")
	}
	validFrom, err := time.Parse("2006-01-02", request.ValidFrom)
	if err != nil || strings.TrimSpace(request.Line1) == "" || strings.TrimSpace(request.City) == "" ||
		strings.TrimSpace(request.Province) == "" || strings.TrimSpace(request.District) == "" {
		h.badRequest(c, "ADDRESS_CHANGED", "Address details are invalid")
		return
	}
	principal, ok := auth.RequestClaims(c)
	if !ok {
		httpx.InternalError(c.Writer, c.Request)
		return
	}
	found := false
	err = h.audit.InTransactionOn(c.Request.Context(), h.database, eventWithPurpose(c, principal, "ADDRESS_CHANGED", nil, request.Purpose, db.AuditOutcomeOK), func(tx *gorm.DB) error {
		var person db.Person
		if err := tx.Select("id").First(&person, personID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		found = true
		if err := tx.Model(&addressModel{}).Where("person_id = ? AND is_current = ?", personID, true).
			Updates(map[string]any{"is_current": false, "valid_to": validFrom}).Error; err != nil {
			return err
		}
		address := addressModel{
			PersonID: personID, Line1: strings.TrimSpace(request.Line1), Suburb: request.Suburb,
			City: strings.TrimSpace(request.City), Province: strings.TrimSpace(request.Province),
			District: strings.TrimSpace(request.District), WardCode: request.WardCode,
			PostalCode: request.PostalCode, IsCurrent: true, ValidFrom: validFrom,
		}
		if err := tx.Create(&address).Error; err != nil {
			return err
		}
		return appendOutbox(tx, "ADDRESS_CHANGED", personID)
	})
	if err != nil {
		h.internalError(c, "ADDRESS_CHANGED")
		return
	}
	if !found {
		httpx.Error(c.Writer, c.Request, http.StatusNotFound, httpx.CodeNotFound, "Person not found")
		return
	}
	httpx.JSON(c.Writer, http.StatusOK, map[string]string{"status": "updated"})
}

func (h *Handler) recordMarriage(c *gin.Context) {
	var request marriageRequest
	if err := api.DecodeJSON(c, &request, 1<<20); err != nil {
		h.badRequest(c, "REGISTER_MARRIAGE", "Marriage request is invalid")
		return
	}
	marriedOn, err := time.Parse("2006-01-02", request.MarriedOn)
	if err != nil || request.SpouseAID == 0 || request.SpouseBID == 0 ||
		request.SpouseAID == request.SpouseBID || strings.TrimSpace(request.CertificateNo) == "" || !validPurpose(request.Purpose) {
		h.badRequest(c, "REGISTER_MARRIAGE", "Marriage details are invalid")
		return
	}
	principal, ok := auth.RequestClaims(c)
	if !ok {
		httpx.InternalError(c.Writer, c.Request)
		return
	}
	marriage := db.Marriage{
		SpouseAID: request.SpouseAID, SpouseBID: request.SpouseBID,
		MarriedOn: marriedOn, CertificateNo: strings.TrimSpace(request.CertificateNo),
	}
	err = h.audit.InTransactionOn(c.Request.Context(), h.database, eventWithPurpose(c, principal, "REGISTER_MARRIAGE", nil, request.Purpose, db.AuditOutcomeOK), func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&db.Person{}).Where("id IN ?", []uint64{request.SpouseAID, request.SpouseBID}).Count(&count).Error; err != nil {
			return err
		}
		if count != 2 {
			return gorm.ErrRecordNotFound
		}
		if err := tx.Create(&marriage).Error; err != nil {
			return err
		}
		if err := tx.Model(&db.Person{}).Where("id IN ?", []uint64{request.SpouseAID, request.SpouseBID}).
			Update("marital_status", db.MaritalMarried).Error; err != nil {
			return err
		}
		if err := appendOutbox(tx, "MARITAL_STATUS_CHANGED", request.SpouseAID); err != nil {
			return err
		}
		return appendOutbox(tx, "MARITAL_STATUS_CHANGED", request.SpouseBID)
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			h.badRequest(c, "REGISTER_MARRIAGE", "Both spouses must be registered")
			return
		}
		h.internalError(c, "REGISTER_MARRIAGE")
		return
	}
	httpx.JSON(c.Writer, http.StatusCreated, map[string]any{
		"id": marriage.ID, "spouse_a_id": marriage.SpouseAID, "spouse_b_id": marriage.SpouseBID,
		"married_on": marriage.MarriedOn.Format("2006-01-02"), "certificate_no": marriage.CertificateNo,
	})
}

func (h *Handler) listChangeRequests(c *gin.Context) {
	purpose := strings.TrimSpace(c.Query("purpose"))
	if !validPurpose(purpose) {
		h.badRequest(c, "READ_CHANGE_REQUESTS", "Purpose is required")
		return
	}
	limit, err := api.PageSize(c)
	if err != nil {
		h.badRequest(c, "READ_CHANGE_REQUESTS", "Page size is invalid")
		return
	}
	cursor, err := api.Cursor(c)
	if err != nil {
		h.badRequest(c, "READ_CHANGE_REQUESTS", "Page cursor is invalid")
		return
	}
	principal, ok := auth.RequestClaims(c)
	if !ok {
		httpx.InternalError(c.Writer, c.Request)
		return
	}
	var response []requestDTO
	var nextCursor *uint64
	err = h.audit.InTransactionOn(c.Request.Context(), h.database, eventWithPurpose(c, principal, "READ_CHANGE_REQUESTS", nil, purpose, db.AuditOutcomeOK), func(tx *gorm.DB) error {
		var requests []changeRequestModel
		if err := tx.Where("status = ? AND id > ?", "PENDING", cursor).Order("id").Limit(limit + 1).Find(&requests).Error; err != nil {
			return err
		}
		if len(requests) > limit {
			value := requests[limit-1].ID
			nextCursor = &value
			requests = requests[:limit]
		}
		response = make([]requestDTO, 0, len(requests))
		for _, request := range requests {
			payload, err := h.vault.Decrypt(c.Request.Context(), h.encryptionKey, request.PayloadEncrypted)
			if err != nil {
				return err
			}
			var decoded any
			if err := json.Unmarshal(payload, &decoded); err != nil {
				return fmt.Errorf("decode encrypted request payload: %w", err)
			}
			dto := requestDTO{
				ID: request.ID, Type: request.RequestType, Payload: decoded,
				Department: request.SubmittedByDepartment, OfficialID: request.SubmittedByOfficial,
				Status: request.Status, CreatedAt: request.CreatedAt.UTC().Format(time.RFC3339Nano),
			}
			if request.ReviewedBy != nil {
				dto.ReviewedBy = request.ReviewedBy
			}
			if request.ReviewedAt != nil {
				value := request.ReviewedAt.UTC().Format(time.RFC3339Nano)
				dto.ReviewedAt = &value
			}
			dto.Reason = request.Reason
			response = append(response, dto)
		}
		return nil
	})
	if err != nil {
		h.internalError(c, "READ_CHANGE_REQUESTS")
		return
	}
	httpx.JSON(c.Writer, http.StatusOK, map[string]any{"items": response, "next_cursor": nextCursor})
}

func (h *Handler) reviewChangeRequest(c *gin.Context) {
	requestID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	var request changeDecisionRequest
	if err != nil || requestID == 0 || api.DecodeJSON(c, &request, 1<<20) != nil {
		h.badRequest(c, "REVIEW_CHANGE_REQUEST", "Review request is invalid")
		return
	}
	request.Decision = strings.ToUpper(strings.TrimSpace(request.Decision))
	request.Reason = strings.TrimSpace(request.Reason)
	if (request.Decision != "APPROVED" && request.Decision != "REJECTED") || request.Reason == "" || len(request.Reason) > 500 || !validPurpose(request.Purpose) {
		h.badRequest(c, "REVIEW_CHANGE_REQUEST", "Decision and reason are required")
		return
	}
	principal, ok := auth.RequestClaims(c)
	if !ok {
		httpx.InternalError(c.Writer, c.Request)
		return
	}
	err = h.audit.WithPreparedEvent(c.Request.Context(), eventWithPurpose(c, principal, "REVIEW_CHANGE_REQUEST", nil, request.Purpose, db.AuditOutcomeOK), func(row db.AuditLog) error {
		return h.database.WithContext(c.Request.Context()).Exec(
			"CALL sp_review_change_request(?,?,?,?,?,?,?,?,?,?,?)",
			requestID, request.Decision, request.Reason, principal.OfficialID, request.Purpose, row.ChainID, row.Seq,
			row.OccurredAt, row.RequestID, row.PreviousHash, row.RowHash,
		).Error
	})
	if err != nil {
		h.procedureError(c, err, "REVIEW_CHANGE_REQUEST")
		return
	}
	httpx.JSON(c.Writer, http.StatusOK, map[string]string{"status": strings.ToLower(request.Decision)})
}

func (h *Handler) setIDStatus(c *gin.Context) {
	personID, ok := pathID(c, "id")
	var request idStatusRequest
	if !ok || api.DecodeJSON(c, &request, 1<<20) != nil {
		h.badRequest(c, "SET_ID_STATUS", "Identity document status is invalid")
		return
	}
	request.Reason = strings.TrimSpace(request.Reason)
	if request.Blocked && (request.Reason == "" || len(request.Reason) > 255) {
		h.badRequest(c, "SET_ID_STATUS", "A reason is required to block an identity document")
		return
	}
	if !validPurpose(request.Purpose) {
		h.badRequest(c, "SET_ID_STATUS", "Purpose is required")
		return
	}
	principal, ok := auth.RequestClaims(c)
	if !ok {
		httpx.InternalError(c.Writer, c.Request)
		return
	}
	blocked := 0
	if request.Blocked {
		blocked = 1
	}
	err := h.audit.WithPreparedEvent(c.Request.Context(), eventWithPurpose(c, principal, "SET_ID_STATUS", nil, request.Purpose, db.AuditOutcomeOK), func(row db.AuditLog) error {
		return h.database.WithContext(c.Request.Context()).Exec(
			"CALL sp_set_id_block_status(?,?,?,?,?,?,?,?,?,?,?)",
			personID, blocked, request.Reason, principal.OfficialID, request.Purpose, row.ChainID, row.Seq,
			row.OccurredAt, row.RequestID, row.PreviousHash, row.RowHash,
		).Error
	})
	if err != nil {
		h.procedureError(c, err, "SET_ID_STATUS")
		return
	}
	httpx.JSON(c.Writer, http.StatusOK, map[string]any{"blocked": request.Blocked})
}

func (h *Handler) biometricStatus(c *gin.Context) {
	personID, ok := pathID(c, "id")
	purpose := strings.TrimSpace(c.Query("purpose"))
	if !ok || !validPurpose(purpose) {
		h.badRequest(c, "READ_BIOMETRIC_STATUS", "Person ID is invalid")
		return
	}
	principal, ok := auth.RequestClaims(c)
	if !ok {
		httpx.InternalError(c.Writer, c.Request)
		return
	}
	var response []biometricDTO
	found := false
	err := h.audit.InTransactionOn(c.Request.Context(), h.database, eventWithPurpose(c, principal, "READ_BIOMETRIC_STATUS", nil, purpose, db.AuditOutcomeOK), func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&db.Person{}).Where("id = ?", personID).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			return nil
		}
		found = true
		return tx.Model(&db.BiometricTemplate{}).
			Select("kind", "captured_at").
			Where("person_id = ?", personID).
			Order("kind").
			Find(&response).Error
	})
	if err != nil {
		h.internalError(c, "READ_BIOMETRIC_STATUS")
		return
	}
	if !found {
		httpx.Error(c.Writer, c.Request, http.StatusNotFound, httpx.CodeNotFound, "Person not found")
		return
	}
	httpx.JSON(c.Writer, http.StatusOK, response)
}

func (h *Handler) personResponse(tx *gorm.DB, c *gin.Context, person db.Person) (personDTO, error) {
	masked, err := h.maskedNationalID(c, person.NationalIDEncrypted)
	if err != nil {
		return personDTO{}, err
	}
	response := personDTO{
		ID: person.ID, NationalIDMasked: masked, FirstNames: person.FirstNames,
		LastName: person.LastName, DateOfBirth: person.DateOfBirth.Format("2006-01-02"),
		Sex: person.Sex, CitizenshipStatus: person.CitizenshipStatus,
		MaritalStatus: person.MaritalStatus, IDBlocked: person.IDBlockedAt != nil,
		IDBlockReason: person.IDBlockReason,
		Addresses:     []addressDTO{}, Marriages: []marriageDTO{}, Biometrics: []biometricDTO{},
	}
	if person.DeceasedAt != nil {
		value := person.DeceasedAt.Format("2006-01-02")
		response.DeceasedAt = &value
	}

	var addresses []addressModel
	if err := tx.Where("person_id = ?", person.ID).Order("valid_from DESC, id DESC").Find(&addresses).Error; err != nil {
		return personDTO{}, err
	}
	for _, address := range addresses {
		response.Addresses = append(response.Addresses, toAddressDTO(address))
	}

	var marriages []db.Marriage
	if err := tx.Where("spouse_a_id = ? OR spouse_b_id = ?", person.ID, person.ID).Order("married_on DESC").Find(&marriages).Error; err != nil {
		return personDTO{}, err
	}
	for _, marriage := range marriages {
		dto := marriageDTO{
			ID: marriage.ID, SpouseAID: marriage.SpouseAID, SpouseBID: marriage.SpouseBID,
			MarriedOn: marriage.MarriedOn.Format("2006-01-02"), CertificateNo: marriage.CertificateNo,
		}
		if marriage.DissolvedOn != nil {
			value := marriage.DissolvedOn.Format("2006-01-02")
			dto.DissolvedOn = &value
		}
		response.Marriages = append(response.Marriages, dto)
	}
	var biometrics []biometricDTO
	if err := tx.Model(&db.BiometricTemplate{}).
		Select("kind", "captured_at").Where("person_id = ?", person.ID).Order("kind").Find(&biometrics).Error; err != nil {
		return personDTO{}, err
	}
	response.Biometrics = biometrics
	return response, nil
}

func (h *Handler) maskedNationalID(c *gin.Context, encrypted []byte) (string, error) {
	value, err := h.vault.Decrypt(c.Request.Context(), h.encryptionKey, encrypted)
	if err != nil {
		return "", err
	}
	if len(value) < 4 {
		return "", errors.New("decrypted national ID is malformed")
	}
	return "*********" + string(value[len(value)-4:]), nil
}

func (h *Handler) badRequest(c *gin.Context, action, message string) {
	h.writeFailure(c, action, db.AuditOutcomeError, http.StatusBadRequest, httpx.CodeBadRequest, message)
}

func (h *Handler) internalError(c *gin.Context, action string) {
	h.writeFailure(c, action, db.AuditOutcomeError, http.StatusInternalServerError, httpx.CodeInternal, httpx.MessageInternal)
}

func (h *Handler) procedureError(c *gin.Context, err error, action string) {
	var mysqlErr *mysqlDriver.MySQLError
	if errors.As(err, &mysqlErr) && mysqlErr.Number == 1644 {
		h.writeFailure(c, action, db.AuditOutcomeError, http.StatusConflict, "REQUEST_CONFLICT", "The requested change could not be applied")
		return
	}
	h.writeFailure(c, action, db.AuditOutcomeError, http.StatusServiceUnavailable, "DATABASE_UNAVAILABLE", "The request could not be completed")
}

func (h *Handler) writeFailure(c *gin.Context, action string, outcome db.AuditOutcome, status int, code, message string) {
	principal, ok := auth.RequestClaims(c)
	if !ok {
		httpx.InternalError(c.Writer, c.Request)
		return
	}
	if err := h.audit.Write(c.Request.Context(), event(c, principal, action, nil, outcome)); err != nil {
		httpx.Error(c.Writer, c.Request, http.StatusServiceUnavailable, "AUDIT_UNAVAILABLE", "Request could not be audited")
		return
	}
	httpx.Error(c.Writer, c.Request, status, code, message)
}

func event(c *gin.Context, claims auth.JWTClaims, action string, subject []byte, outcome db.AuditOutcome) audit.Event {
	officialID := claims.OfficialID
	return audit.Event{
		OfficialID: &officialID, Department: claims.Department, Action: action,
		Subject: subject, Outcome: outcome, RequestID: httpx.RequestID(c.Request.Context()),
	}
}

func eventWithPurpose(c *gin.Context, claims auth.JWTClaims, action string, subject []byte, purpose string, outcome db.AuditOutcome) audit.Event {
	value := strings.TrimSpace(purpose)
	result := event(c, claims, action, subject, outcome)
	result.Purpose = &value
	return result
}

func validPurpose(value string) bool {
	return api.ValidPurpose(value)
}

func pathID(c *gin.Context, name string) (uint64, bool) {
	value, err := strconv.ParseUint(c.Param(name), 10, 64)
	return value, err == nil && value != 0
}

func toAddressDTO(address addressModel) addressDTO {
	dto := addressDTO{
		Line1: address.Line1, Suburb: address.Suburb, City: address.City,
		Province: address.Province, District: address.District, WardCode: address.WardCode,
		PostalCode: address.PostalCode, ValidFrom: address.ValidFrom.Format("2006-01-02"),
		IsCurrent: address.IsCurrent,
	}
	if address.ValidTo != nil {
		value := address.ValidTo.Format("2006-01-02")
		dto.ValidTo = &value
	}
	return dto
}

func appendOutbox(tx *gorm.DB, eventType string, personID uint64) error {
	outbox := db.EventOutbox{EventType: eventType, PersonID: personID}
	if err := tx.Create(&outbox).Error; err != nil {
		return err
	}
	return tx.Exec(`INSERT INTO event_deliveries (outbox_id, department)
		SELECT ?, department FROM event_subscriptions WHERE event_type = ? AND active = 1`,
		outbox.ID, eventType).Error
}

func validSex(value db.Sex) bool {
	return value == db.SexMale || value == db.SexFemale || value == db.SexOther
}

func validCitizenship(value db.CitizenshipStatus) bool {
	return value == db.CitizenshipCitizen || value == db.CitizenshipPermanentResident ||
		value == db.CitizenshipForeign || value == db.CitizenshipStateless
}
