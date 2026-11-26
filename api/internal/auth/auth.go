package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/S-Matsoso/goliath/internal/audit"
	"github.com/S-Matsoso/goliath/internal/db"
	"github.com/S-Matsoso/goliath/internal/platform/httpx"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const sessionLifetime = 8 * time.Hour

var (
	ErrInvalidCredentials     = errors.New("invalid username or password")
	ErrAuditUnavailable       = errors.New("authentication audit is unavailable")
	ErrBootstrapAdminRequired = errors.New("bootstrap admin credentials are required because no administrator exists")
)

type Principal struct {
	OfficialID         uint64 `json:"official_id"`
	AccountID          uint64 `json:"account_id"`
	Username           string `json:"username"`
	DisplayName        string `json:"display_name"`
	Department         string `json:"department"`
	Role               string `json:"role"`
	MustChangePassword bool   `json:"must_change_password"`
}

type Session struct {
	Token              string    `json:"token"`
	ExpiresAt          time.Time `json:"expires_at"`
	Principal          Principal `json:"principal"`
	MustChangePassword bool      `json:"must_change_password"`
}

type Authenticator struct {
	database *gorm.DB
	audit    *audit.Writer
}

func New(database *gorm.DB, writer *audit.Writer) *Authenticator {
	return &Authenticator{database: database, audit: writer}
}

// BootstrapAdmin creates the first global administrator only when one does not exist.
func BootstrapAdmin(ctx context.Context, database *gorm.DB, username, password string) error {
	var count int64
	err := database.WithContext(ctx).
		Model(&db.Account{}).
		Joins("JOIN officials ON officials.id = accounts.official_id").
		Where("officials.role = ?", "admin").
		Count(&count).Error
	if err != nil {
		return fmt.Errorf("check for an existing administrator: %w", err)
	}
	if count > 0 {
		return nil
	}
	username = NormalizeUsername(username)
	if username == "" || password == "" {
		return ErrBootstrapAdminRequired
	}
	if err := ValidateUsername(username); err != nil {
		return fmt.Errorf("bootstrap admin username: %w", err)
	}
	if err := ValidatePassword(username, password); err != nil {
		return fmt.Errorf("bootstrap admin password: %w", err)
	}
	passwordHash, err := HashPassword(password)
	if err != nil {
		return fmt.Errorf("hash bootstrap admin password: %w", err)
	}

	return database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		department := db.Department{Code: "ADMIN", Name: "Administration", Active: true}
		if err := tx.FirstOrCreate(&department, db.Department{Code: department.Code}).Error; err != nil {
			return fmt.Errorf("create administration department: %w", err)
		}
		official := db.Official{
			IDPSubject: "local:" + username,
			Department: department.Code,
			Role:       "admin",
			Active:     true,
		}
		if err := tx.Create(&official).Error; err != nil {
			return fmt.Errorf("create bootstrap administrator profile: %w", err)
		}
		account := db.Account{
			OfficialID:         official.ID,
			Username:           username,
			DisplayName:        "System Administrator",
			PasswordHash:       passwordHash,
			MustChangePassword: true,
			Active:             true,
		}
		if err := tx.Create(&account).Error; err != nil {
			return fmt.Errorf("create bootstrap administrator account: %w", err)
		}
		return nil
	})
}

func (a *Authenticator) Login(ctx context.Context, username, password, requestID string) (Session, error) {
	username = NormalizeUsername(username)
	var account db.Account
	accountErr := a.database.WithContext(ctx).Where("username = ?", username).First(&account).Error
	if accountErr != nil && !errors.Is(accountErr, gorm.ErrRecordNotFound) {
		return Session{}, fmt.Errorf("load account: %w", accountErr)
	}
	hash := account.PasswordHash
	if errors.Is(accountErr, gorm.ErrRecordNotFound) {
		hash = dummyPasswordHash
	}
	passwordOK := VerifyPassword(hash, password)
	if !passwordOK || accountErr != nil || !account.Active {
		if err := a.audit.Write(ctx, audit.Event{
			Department: "unknown",
			Action:     "LOGIN",
			Outcome:    db.AuditOutcomeDenied,
			RequestID:  requestID,
		}); err != nil {
			return Session{}, fmt.Errorf("%w: %v", ErrAuditUnavailable, err)
		}
		return Session{}, ErrInvalidCredentials
	}

	var official db.Official
	if err := a.database.WithContext(ctx).First(&official, account.OfficialID).Error; err != nil {
		return Session{}, fmt.Errorf("load account profile: %w", err)
	}
	if !official.Active {
		if err := a.audit.Write(ctx, audit.Event{
			OfficialID: &official.ID,
			Department: official.Department,
			Action:     "LOGIN",
			Outcome:    db.AuditOutcomeDenied,
			RequestID:  requestID,
		}); err != nil {
			return Session{}, fmt.Errorf("%w: %v", ErrAuditUnavailable, err)
		}
		return Session{}, ErrInvalidCredentials
	}
	token, tokenHash, err := newToken()
	if err != nil {
		return Session{}, fmt.Errorf("create login session: %w", err)
	}
	now := time.Now().UTC()
	expiresAt := now.Add(sessionLifetime)
	if err := a.audit.InTransaction(ctx, audit.Event{
		OfficialID: &official.ID,
		Department: official.Department,
		Action:     "LOGIN",
		Outcome:    db.AuditOutcomeOK,
		RequestID:  requestID,
	}, func(tx *gorm.DB) error {
		if err := tx.Model(&account).Update("last_login_at", now).Error; err != nil {
			return err
		}
		if err := tx.Where("expires_at < ?", now).Delete(&db.AccountSession{}).Error; err != nil {
			return err
		}
		return tx.Create(&db.AccountSession{
			TokenHash: tokenHash,
			AccountID: account.ID,
			ExpiresAt: expiresAt,
		}).Error
	}); err != nil {
		return Session{}, fmt.Errorf("record login session: %w", err)
	}

	principal := Principal{
		OfficialID:         official.ID,
		AccountID:          account.ID,
		Username:           account.Username,
		DisplayName:        account.DisplayName,
		Department:         official.Department,
		Role:               official.Role,
		MustChangePassword: account.MustChangePassword,
	}
	return Session{
		Token:              token,
		ExpiresAt:          expiresAt,
		Principal:          principal,
		MustChangePassword: account.MustChangePassword,
	}, nil
}

func (a *Authenticator) ChangePassword(ctx context.Context, principal Principal, oldPassword, newPassword, requestID string) (Session, error) {
	var account db.Account
	if err := a.database.WithContext(ctx).First(&account, principal.AccountID).Error; err != nil {
		return Session{}, fmt.Errorf("load account for password change: %w", err)
	}
	if !VerifyPassword(account.PasswordHash, oldPassword) {
		return Session{}, ErrInvalidCredentials
	}
	if err := ValidatePassword(account.Username, newPassword); err != nil {
		return Session{}, err
	}
	if VerifyPassword(account.PasswordHash, newPassword) {
		return Session{}, ErrPasswordReused
	}
	passwordHash, err := HashPassword(newPassword)
	if err != nil {
		return Session{}, fmt.Errorf("hash new password: %w", err)
	}
	token, tokenHash, err := newToken()
	if err != nil {
		return Session{}, fmt.Errorf("create replacement login session: %w", err)
	}
	now := time.Now().UTC()
	expiresAt := now.Add(sessionLifetime)
	if err := a.audit.InTransaction(ctx, audit.Event{
		OfficialID: &principal.OfficialID,
		Department: principal.Department,
		Action:     "CHANGE_PASSWORD",
		Outcome:    db.AuditOutcomeOK,
		RequestID:  requestID,
	}, func(tx *gorm.DB) error {
		if err := tx.Model(&account).Updates(map[string]any{
			"password_hash":        passwordHash,
			"must_change_password": false,
		}).Error; err != nil {
			return err
		}
		if err := tx.Where("account_id = ?", account.ID).Delete(&db.AccountSession{}).Error; err != nil {
			return err
		}
		return tx.Create(&db.AccountSession{
			TokenHash: tokenHash,
			AccountID: account.ID,
			ExpiresAt: expiresAt,
		}).Error
	}); err != nil {
		return Session{}, fmt.Errorf("change account password: %w", err)
	}
	principal.MustChangePassword = false
	return Session{Token: token, ExpiresAt: expiresAt, Principal: principal}, nil
}

func (a *Authenticator) RevokeSession(ctx context.Context, token string) error {
	hash := sha256.Sum256([]byte(token))
	if err := a.database.WithContext(ctx).Where("token_hash = ?", hash[:]).Delete(&db.AccountSession{}).Error; err != nil {
		return fmt.Errorf("revoke login session: %w", err)
	}
	return nil
}

func (a *Authenticator) Require(department, minimumRole string) gin.HandlerFunc {
	return func(c *gin.Context) {
		principal, ok := a.authenticate(c)
		if !ok {
			return
		}
		if principal.MustChangePassword {
			httpx.Error(c.Writer, c.Request, http.StatusForbidden, "PASSWORD_CHANGE_REQUIRED", "Change your temporary password before continuing")
			c.Abort()
			return
		}
		if roleRank(principal.Role) < roleRank(minimumRole) ||
			(!isAdmin(principal.Role) && principal.Department != department) {
			a.reject(c, http.StatusForbidden, "FORBIDDEN", "Access denied", &principal)
			return
		}
		c.Set("principal", principal)
		c.Next()
	}
}

func (a *Authenticator) RequireSelf() gin.HandlerFunc {
	return func(c *gin.Context) {
		principal, ok := a.authenticate(c)
		if !ok {
			return
		}
		c.Set("principal", principal)
		c.Next()
	}
}

func (a *Authenticator) RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		principal, ok := a.authenticate(c)
		if !ok {
			return
		}
		if principal.MustChangePassword {
			httpx.Error(c.Writer, c.Request, http.StatusForbidden, "PASSWORD_CHANGE_REQUIRED", "Change your temporary password before continuing")
			c.Abort()
			return
		}
		if !isAdmin(principal.Role) {
			a.reject(c, http.StatusForbidden, "FORBIDDEN", "Administrator access required", &principal)
			return
		}
		c.Set("principal", principal)
		c.Next()
	}
}

func (a *Authenticator) authenticate(c *gin.Context) (Principal, bool) {
	fields := strings.Fields(c.GetHeader("Authorization"))
	if len(fields) != 2 || !strings.EqualFold(fields[0], "Bearer") || len(fields[1]) < 32 {
		a.reject(c, http.StatusUnauthorized, "UNAUTHORIZED", "Valid bearer token required", nil)
		return Principal{}, false
	}
	tokenHash := sha256.Sum256([]byte(fields[1]))
	var session db.AccountSession
	err := a.database.WithContext(c.Request.Context()).
		Where("token_hash = ? AND expires_at > ?", tokenHash[:], time.Now().UTC()).
		First(&session).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		a.reject(c, http.StatusUnauthorized, "UNAUTHORIZED", "Valid bearer token required", nil)
		return Principal{}, false
	}
	if err != nil {
		a.reject(c, http.StatusInternalServerError, httpx.CodeInternal, httpx.MessageInternal, nil)
		return Principal{}, false
	}
	var account db.Account
	err = a.database.WithContext(c.Request.Context()).First(&account, session.AccountID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		a.reject(c, http.StatusUnauthorized, "UNAUTHORIZED", "Valid bearer token required", nil)
		return Principal{}, false
	}
	if err != nil {
		a.reject(c, http.StatusInternalServerError, httpx.CodeInternal, httpx.MessageInternal, nil)
		return Principal{}, false
	}
	var official db.Official
	err = a.database.WithContext(c.Request.Context()).First(&official, account.OfficialID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		a.reject(c, http.StatusUnauthorized, "UNAUTHORIZED", "Valid bearer token required", nil)
		return Principal{}, false
	}
	if err != nil {
		a.reject(c, http.StatusInternalServerError, httpx.CodeInternal, httpx.MessageInternal, nil)
		return Principal{}, false
	}
	if !account.Active || !official.Active {
		a.reject(c, http.StatusUnauthorized, "UNAUTHORIZED", "Account is not active", nil)
		return Principal{}, false
	}
	if !isAdmin(official.Role) {
		var department db.Department
		err := a.database.WithContext(c.Request.Context()).Where("code = ? AND active = ?", official.Department, true).First(&department).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			a.reject(c, http.StatusForbidden, "FORBIDDEN", "Department is not active", nil)
			return Principal{}, false
		}
		if err != nil {
			a.reject(c, http.StatusInternalServerError, httpx.CodeInternal, httpx.MessageInternal, nil)
			return Principal{}, false
		}
	}
	return Principal{
		OfficialID:         official.ID,
		AccountID:          account.ID,
		Username:           account.Username,
		DisplayName:        account.DisplayName,
		Department:         official.Department,
		Role:               official.Role,
		MustChangePassword: account.MustChangePassword,
	}, true
}

func (a *Authenticator) reject(c *gin.Context, status int, code, message string, principal *Principal) {
	event := audit.Event{
		Department: "unknown",
		Action:     c.FullPath(),
		Outcome:    db.AuditOutcomeDenied,
		RequestID:  httpx.RequestID(c.Request.Context()),
	}
	if principal != nil {
		event.OfficialID = &principal.OfficialID
		event.Department = principal.Department
	}
	if err := a.audit.Write(c.Request.Context(), event); err != nil {
		httpx.Error(c.Writer, c.Request, http.StatusServiceUnavailable, "AUDIT_UNAVAILABLE", "Request could not be audited")
		c.Abort()
		return
	}
	httpx.Error(c.Writer, c.Request, status, code, message)
	c.Abort()
}

func RequestPrincipal(c *gin.Context) (Principal, bool) {
	principal, ok := c.Get("principal")
	if !ok {
		return Principal{}, false
	}
	value, ok := principal.(Principal)
	return value, ok
}

func NormalizeUsername(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
}

func newToken() (string, []byte, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", nil, err
	}
	token := base64.RawURLEncoding.EncodeToString(value)
	hash := sha256.Sum256([]byte(token))
	return token, hash[:], nil
}

func roleRank(role string) int {
	switch strings.ToLower(role) {
	case "clerk":
		return 1
	case "supervisor":
		return 2
	case "auditor":
		return 3
	case "admin":
		return 4
	default:
		return 0
	}
}

func isAdmin(role string) bool {
	return strings.EqualFold(role, "admin")
}

func randomCharacter(characters string) (byte, error) {
	index, err := rand.Int(rand.Reader, big.NewInt(int64(len(characters))))
	if err != nil {
		return 0, err
	}
	return characters[index.Int64()], nil
}

func shuffle(value []byte) error {
	for i := len(value) - 1; i > 0; i-- {
		index, err := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		if err != nil {
			return err
		}
		value[i], value[index.Int64()] = value[index.Int64()], value[i]
	}
	return nil
}
