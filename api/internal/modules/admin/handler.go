package admin

import (
	"context"
	"crypto/sha256"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/S-Matsoso/goliath/internal/audit"
	"github.com/S-Matsoso/goliath/internal/auth"
	"github.com/S-Matsoso/goliath/internal/db"
	"github.com/S-Matsoso/goliath/internal/metrics"
	"github.com/S-Matsoso/goliath/internal/platform/httpx"
	"github.com/S-Matsoso/goliath/internal/platform/router"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

var departmentCodePattern = regexp.MustCompile(`^[A-Z][A-Z0-9_-]{1,19}$`)
var errAdministratorAccountProtected = errors.New("administrator accounts cannot be changed here")

type Handler struct {
	database  *gorm.DB
	auth      *auth.Authenticator
	audit     *audit.Writer
	metrics   *metrics.Collector
	serviceID string
}

func NewHandler(database *gorm.DB, authenticator *auth.Authenticator, writer *audit.Writer, collector *metrics.Collector, serviceID string) *Handler {
	return &Handler{
		database:  database,
		auth:      authenticator,
		audit:     writer,
		metrics:   collector,
		serviceID: serviceID,
	}
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

type createUserRequest struct {
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Department  string `json:"department"`
	Role        string `json:"role"`
}

type updateUserRequest struct {
	Active *bool `json:"active"`
}

type createDepartmentRequest struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

type updateDepartmentRequest struct {
	Name   *string `json:"name"`
	Active *bool   `json:"active"`
}

type userResponse struct {
	ID                 uint64     `json:"id"`
	Username           string     `json:"username"`
	DisplayName        string     `json:"display_name"`
	Department         string     `json:"department"`
	DepartmentName     string     `json:"department_name"`
	Role               string     `json:"role"`
	Active             bool       `json:"active"`
	MustChangePassword bool       `json:"must_change_password"`
	CreatedAt          time.Time  `json:"created_at"`
	LastLoginAt        *time.Time `json:"last_login_at,omitempty"`
}

type departmentResponse struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Active      bool   `json:"active"`
	MemberCount int64  `json:"member_count"`
}

type systemMetricsResponse struct {
	Service   string           `json:"service"`
	Status    string           `json:"status"`
	Uptime    metrics.Snapshot `json:"uptime"`
	Database  databaseMetrics  `json:"database"`
	CheckedAt time.Time        `json:"checked_at"`
}

type databaseMetrics struct {
	Status             string `json:"status"`
	OpenConnections    int    `json:"open_connections"`
	InUseConnections   int    `json:"in_use_connections"`
	IdleConnections    int    `json:"idle_connections"`
	MaxConnections     int    `json:"max_connections"`
	WaitCount          int64  `json:"wait_count"`
	WaitDurationMillis int64  `json:"wait_duration_ms"`
}

// Register mounts authentication and global administrator operations.
func (h *Handler) Register(r *router.Router) {
	r.Handle(http.MethodPost, "/auth/login", "Sign in with an administrator-provisioned account", h.login)
	r.Handle(http.MethodPost, "/auth/change-password", "Change the current account password", h.auth.RequireSelf(), h.changePassword)
	r.Handle(http.MethodPost, "/auth/logout", "Revoke the current login session", h.auth.RequireSelf(), h.logout)

	r.Handle(http.MethodGet, "/admin/metrics", "Read system health and runtime metrics", h.auth.RequireAdmin(), h.systemMetrics)
	r.Handle(http.MethodGet, "/admin/users", "List provisioned users", h.auth.RequireAdmin(), h.listUsers)
	r.Handle(http.MethodPost, "/admin/users", "Create a user account and temporary password", h.auth.RequireAdmin(), h.createUser)
	r.Handle(http.MethodPatch, "/admin/users/:id", "Activate or deactivate a user account", h.auth.RequireAdmin(), h.updateUser)
	r.Handle(http.MethodGet, "/admin/departments", "List departments", h.auth.RequireAdmin(), h.listDepartments)
	r.Handle(http.MethodPost, "/admin/departments", "Create a department", h.auth.RequireAdmin(), h.createDepartment)
	r.Handle(http.MethodPatch, "/admin/departments/:code", "Update a department", h.auth.RequireAdmin(), h.updateDepartment)
}

func (h *Handler) login(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16*1024)
	var request loginRequest
	if err := c.ShouldBindJSON(&request); err != nil || request.Username == "" || request.Password == "" {
		httpx.Error(c.Writer, c.Request, http.StatusBadRequest, httpx.CodeBadRequest, "Username and password are required")
		return
	}
	session, err := h.auth.Login(
		c.Request.Context(),
		request.Username,
		request.Password,
		httpx.RequestID(c.Request.Context()),
	)
	if errors.Is(err, auth.ErrInvalidCredentials) {
		httpx.Error(c.Writer, c.Request, http.StatusUnauthorized, "UNAUTHORIZED", "Username or password is incorrect")
		return
	}
	if errors.Is(err, auth.ErrAuditUnavailable) {
		httpx.Error(c.Writer, c.Request, http.StatusServiceUnavailable, "AUDIT_UNAVAILABLE", "Login attempt could not be audited")
		return
	}
	if err != nil {
		h.internalError(c)
		return
	}
	httpx.JSON(c.Writer, http.StatusOK, session)
}

func (h *Handler) changePassword(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16*1024)
	var request changePasswordRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		h.badRequest(c, "CHANGE_PASSWORD", "Current and new passwords are required")
		return
	}
	principal, ok := auth.RequestPrincipal(c)
	if !ok {
		httpx.InternalError(c.Writer, c.Request)
		return
	}
	session, err := h.auth.ChangePassword(
		c.Request.Context(),
		principal,
		request.CurrentPassword,
		request.NewPassword,
		httpx.RequestID(c.Request.Context()),
	)
	if errors.Is(err, auth.ErrInvalidCredentials) {
		if !h.writeFailureAudit(c, "CHANGE_PASSWORD") {
			return
		}
		httpx.Error(c.Writer, c.Request, http.StatusUnauthorized, "UNAUTHORIZED", "Current password is incorrect")
		return
	}
	if err != nil {
		if errors.Is(err, auth.ErrPasswordPolicy) || errors.Is(err, auth.ErrPasswordReused) {
			h.badRequest(c, "CHANGE_PASSWORD", err.Error())
			return
		}
		h.internalError(c)
		return
	}
	httpx.JSON(c.Writer, http.StatusOK, session)
}

func (h *Handler) logout(c *gin.Context) {
	fields := strings.Fields(c.GetHeader("Authorization"))
	if len(fields) != 2 {
		httpx.Error(c.Writer, c.Request, http.StatusUnauthorized, "UNAUTHORIZED", "Valid bearer token required")
		return
	}
	principal, ok := auth.RequestPrincipal(c)
	if !ok {
		httpx.InternalError(c.Writer, c.Request)
		return
	}
	err := h.audit.InTransaction(c.Request.Context(), h.event(c, principal, "LOGOUT", db.AuditOutcomeOK), func(tx *gorm.DB) error {
		hash := tokenHash(fields[1])
		return tx.Where("token_hash = ? AND account_id = ?", hash, principal.AccountID).Delete(&db.AccountSession{}).Error
	})
	if err != nil {
		h.internalError(c)
		return
	}
	httpx.JSON(c.Writer, http.StatusOK, map[string]string{"status": "signed_out"})
}

func (h *Handler) systemMetrics(c *gin.Context) {
	sqlDB, err := h.database.DB()
	if err != nil {
		h.internalError(c)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	pingErr := sqlDB.PingContext(ctx)
	pool := sqlDB.Stats()
	payload := systemMetricsResponse{
		Service: h.serviceID,
		Status:  "healthy",
		Uptime:  h.metrics.Snapshot(),
		Database: databaseMetrics{
			Status:             "healthy",
			OpenConnections:    pool.OpenConnections,
			InUseConnections:   pool.InUse,
			IdleConnections:    pool.Idle,
			MaxConnections:     pool.MaxOpenConnections,
			WaitCount:          pool.WaitCount,
			WaitDurationMillis: pool.WaitDuration.Milliseconds(),
		},
		CheckedAt: time.Now().UTC(),
	}
	if pingErr != nil {
		payload.Status = "degraded"
		payload.Database.Status = "unavailable"
		httpx.JSON(c.Writer, http.StatusOK, payload)
		return
	}
	httpx.JSON(c.Writer, http.StatusOK, payload)
}

func (h *Handler) listUsers(c *gin.Context) {
	principal, ok := auth.RequestPrincipal(c)
	if !ok {
		httpx.InternalError(c.Writer, c.Request)
		return
	}
	users := make([]userResponse, 0)
	err := h.audit.InTransaction(c.Request.Context(), h.event(c, principal, "LIST_USERS", db.AuditOutcomeOK), func(tx *gorm.DB) error {
		return tx.Table("accounts").
			Select("accounts.id, accounts.username, accounts.display_name, officials.department, departments.name AS department_name, officials.role, accounts.active, accounts.must_change_password, accounts.created_at, accounts.last_login_at").
			Joins("JOIN officials ON officials.id = accounts.official_id").
			Joins("JOIN departments ON departments.code = officials.department").
			Order("accounts.username ASC").
			Scan(&users).Error
	})
	if err != nil {
		if !h.writeFailureAudit(c, "LIST_USERS") {
			return
		}
		h.internalError(c)
		return
	}
	httpx.JSON(c.Writer, http.StatusOK, map[string]any{"users": users})
}

func (h *Handler) createUser(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16*1024)
	var request createUserRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		h.badRequest(c, "CREATE_USER", "User details are invalid")
		return
	}
	request.Username = auth.NormalizeUsername(request.Username)
	request.DisplayName = strings.TrimSpace(request.DisplayName)
	request.Department = strings.ToUpper(strings.TrimSpace(request.Department))
	request.Role = strings.ToLower(strings.TrimSpace(request.Role))
	if err := auth.ValidateUsername(request.Username); err != nil ||
		request.DisplayName == "" || len(request.DisplayName) > 120 ||
		!validUserRole(request.Role) ||
		!departmentCodePattern.MatchString(request.Department) || request.Department == "ADMIN" {
		h.badRequest(c, "CREATE_USER", "Username, display name, department, or role is invalid")
		return
	}
	var department db.Department
	err := h.database.WithContext(c.Request.Context()).
		Where("code = ? AND active = ?", request.Department, true).
		First(&department).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		h.badRequest(c, "CREATE_USER", "Select an active department")
		return
	}
	if err != nil {
		h.internalError(c)
		return
	}

	temporaryPassword, err := auth.GenerateTemporaryPassword()
	if err != nil {
		h.internalError(c)
		return
	}
	passwordHash, err := auth.HashPassword(temporaryPassword)
	if err != nil {
		h.internalError(c)
		return
	}
	principal, ok := auth.RequestPrincipal(c)
	if !ok {
		httpx.InternalError(c.Writer, c.Request)
		return
	}
	var account db.Account
	event := h.event(c, principal, "CREATE_USER", db.AuditOutcomeOK)
	err = h.audit.InTransaction(c.Request.Context(), event, func(tx *gorm.DB) error {
		official := db.Official{
			IDPSubject: "local:" + request.Username,
			Department: request.Department,
			Role:       request.Role,
			Active:     true,
		}
		if err := tx.Create(&official).Error; err != nil {
			return err
		}
		account = db.Account{
			OfficialID:         official.ID,
			Username:           request.Username,
			DisplayName:        request.DisplayName,
			PasswordHash:       passwordHash,
			MustChangePassword: true,
			Active:             true,
		}
		return tx.Create(&account).Error
	})
	if err != nil {
		if isUniqueConstraintError(err) {
			h.badRequest(c, "CREATE_USER", "That username is already in use")
			return
		}
		h.internalError(c)
		return
	}
	httpx.JSON(c.Writer, http.StatusCreated, map[string]any{
		"user": userResponse{
			ID:                 account.ID,
			Username:           account.Username,
			DisplayName:        account.DisplayName,
			Department:         department.Code,
			DepartmentName:     department.Name,
			Role:               request.Role,
			Active:             true,
			MustChangePassword: true,
			CreatedAt:          account.CreatedAt,
		},
		"temporary_password": temporaryPassword,
		"message":            "Share this password securely. It will not be shown again.",
	})
}

func (h *Handler) updateUser(c *gin.Context) {
	accountID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || accountID == 0 {
		h.badRequest(c, "UPDATE_USER", "User ID is invalid")
		return
	}
	var request updateUserRequest
	if err := c.ShouldBindJSON(&request); err != nil || request.Active == nil {
		h.badRequest(c, "UPDATE_USER", "Active status is required")
		return
	}
	principal, ok := auth.RequestPrincipal(c)
	if !ok {
		httpx.InternalError(c.Writer, c.Request)
		return
	}
	if accountID == principal.AccountID {
		h.badRequest(c, "UPDATE_USER", "You cannot deactivate your own account")
		return
	}
	err = h.audit.InTransaction(c.Request.Context(), h.event(c, principal, "UPDATE_USER", db.AuditOutcomeOK), func(tx *gorm.DB) error {
		var account db.Account
		if err := tx.First(&account, accountID).Error; err != nil {
			return err
		}
		var official db.Official
		if err := tx.First(&official, account.OfficialID).Error; err != nil {
			return err
		}
		if strings.EqualFold(official.Role, "admin") {
			return errAdministratorAccountProtected
		}
		if err := tx.Model(&account).Update("active", *request.Active).Error; err != nil {
			return err
		}
		if err := tx.Model(&official).Update("active", *request.Active).Error; err != nil {
			return err
		}
		if !*request.Active {
			return tx.Where("account_id = ?", account.ID).Delete(&db.AccountSession{}).Error
		}
		return nil
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		h.notFound(c, "UPDATE_USER")
		return
	}
	if err != nil {
		if errors.Is(err, errAdministratorAccountProtected) {
			h.badRequest(c, "UPDATE_USER", err.Error())
			return
		}
		h.internalError(c)
		return
	}
	httpx.JSON(c.Writer, http.StatusOK, map[string]any{"id": accountID, "active": *request.Active})
}

func (h *Handler) listDepartments(c *gin.Context) {
	principal, ok := auth.RequestPrincipal(c)
	if !ok {
		httpx.InternalError(c.Writer, c.Request)
		return
	}
	departments := make([]departmentResponse, 0)
	err := h.audit.InTransaction(c.Request.Context(), h.event(c, principal, "LIST_DEPARTMENTS", db.AuditOutcomeOK), func(tx *gorm.DB) error {
		return tx.Table("departments").
			Select("departments.code, departments.name, departments.active, COUNT(officials.id) AS member_count").
			Joins("LEFT JOIN officials ON officials.department = departments.code").
			Group("departments.code, departments.name, departments.active").
			Order("departments.code ASC").
			Scan(&departments).Error
	})
	if err != nil {
		if !h.writeFailureAudit(c, "LIST_DEPARTMENTS") {
			return
		}
		h.internalError(c)
		return
	}
	httpx.JSON(c.Writer, http.StatusOK, map[string]any{"departments": departments})
}

func (h *Handler) createDepartment(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16*1024)
	var request createDepartmentRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		h.badRequest(c, "CREATE_DEPARTMENT", "Department details are invalid")
		return
	}
	request.Code = strings.ToUpper(strings.TrimSpace(request.Code))
	request.Name = strings.TrimSpace(request.Name)
	if !departmentCodePattern.MatchString(request.Code) || request.Name == "" || len(request.Name) > 100 {
		h.badRequest(c, "CREATE_DEPARTMENT", "Department code or name is invalid")
		return
	}
	principal, ok := auth.RequestPrincipal(c)
	if !ok {
		httpx.InternalError(c.Writer, c.Request)
		return
	}
	department := db.Department{Code: request.Code, Name: request.Name, Active: true}
	err := h.audit.InTransaction(c.Request.Context(), h.event(c, principal, "CREATE_DEPARTMENT", db.AuditOutcomeOK), func(tx *gorm.DB) error {
		return tx.Create(&department).Error
	})
	if err != nil {
		if isUniqueConstraintError(err) {
			h.badRequest(c, "CREATE_DEPARTMENT", "That department code already exists")
			return
		}
		h.internalError(c)
		return
	}
	httpx.JSON(c.Writer, http.StatusCreated, departmentResponse{
		Code: department.Code, Name: department.Name, Active: department.Active,
	})
}

func (h *Handler) updateDepartment(c *gin.Context) {
	code := strings.ToUpper(strings.TrimSpace(c.Param("code")))
	if !departmentCodePattern.MatchString(code) {
		h.badRequest(c, "UPDATE_DEPARTMENT", "Department code is invalid")
		return
	}
	var request updateDepartmentRequest
	if err := c.ShouldBindJSON(&request); err != nil || (request.Name == nil && request.Active == nil) {
		h.badRequest(c, "UPDATE_DEPARTMENT", "Provide a department name or active status")
		return
	}
	updates := make(map[string]any)
	if request.Name != nil {
		name := strings.TrimSpace(*request.Name)
		if name == "" || len(name) > 100 {
			h.badRequest(c, "UPDATE_DEPARTMENT", "Department name is invalid")
			return
		}
		updates["name"] = name
	}
	if request.Active != nil {
		if code == "ADMIN" && !*request.Active {
			h.badRequest(c, "UPDATE_DEPARTMENT", "The system administration department cannot be deactivated")
			return
		}
		updates["active"] = *request.Active
	}
	principal, ok := auth.RequestPrincipal(c)
	if !ok {
		httpx.InternalError(c.Writer, c.Request)
		return
	}
	result := h.audit.InTransaction(c.Request.Context(), h.event(c, principal, "UPDATE_DEPARTMENT", db.AuditOutcomeOK), func(tx *gorm.DB) error {
		var department db.Department
		if err := tx.Where("code = ?", code).First(&department).Error; err != nil {
			return err
		}
		if err := tx.Model(&department).Updates(updates).Error; err != nil {
			return err
		}
		if request.Active != nil && !*request.Active {
			var accountIDs []uint64
			if err := tx.Model(&db.Account{}).
				Select("accounts.id").
				Joins("JOIN officials ON officials.id = accounts.official_id").
				Where("officials.department = ?", code).
				Scan(&accountIDs).Error; err != nil {
				return err
			}
			return tx.Where("account_id IN ?", accountIDs).Delete(&db.AccountSession{}).Error
		}
		return nil
	})
	if result != nil {
		if errors.Is(result, gorm.ErrRecordNotFound) {
			h.notFound(c, "UPDATE_DEPARTMENT")
			return
		}
		h.internalError(c)
		return
	}
	var department db.Department
	if err := h.database.WithContext(c.Request.Context()).Where("code = ?", code).First(&department).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		h.notFound(c, "UPDATE_DEPARTMENT")
		return
	} else if err != nil {
		h.internalError(c)
		return
	}
	httpx.JSON(c.Writer, http.StatusOK, departmentResponse{
		Code: department.Code, Name: department.Name, Active: department.Active,
	})
}

func (h *Handler) writeFailureAudit(c *gin.Context, action string) bool {
	principal, ok := auth.RequestPrincipal(c)
	if !ok {
		httpx.InternalError(c.Writer, c.Request)
		return false
	}
	if err := h.audit.Write(c.Request.Context(), h.event(c, principal, action, db.AuditOutcomeError)); err != nil {
		httpx.Error(c.Writer, c.Request, http.StatusServiceUnavailable, "AUDIT_UNAVAILABLE", "Request could not be audited")
		return false
	}
	return true
}

func (h *Handler) event(c *gin.Context, principal auth.Principal, action string, outcome db.AuditOutcome) audit.Event {
	return audit.Event{
		OfficialID: &principal.OfficialID,
		Department: principal.Department,
		Action:     action,
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

func (h *Handler) internalError(c *gin.Context) {
	httpx.InternalError(c.Writer, c.Request)
}

func validUserRole(role string) bool {
	return role == "clerk" || role == "supervisor" || role == "auditor"
}

func isUniqueConstraintError(err error) bool {
	return errors.Is(err, gorm.ErrDuplicatedKey)
}

func tokenHash(token string) []byte {
	hash := sha256.Sum256([]byte(token))
	return hash[:]
}
