package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/S-Matsoso/goliath/internal/audit"
	"github.com/S-Matsoso/goliath/internal/db"
	"github.com/S-Matsoso/goliath/internal/platform/httpx"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const maxTokenLifetime = 15 * time.Minute

// Principal is the authenticated, database-backed official attached to a request.
type Principal struct {
	OfficialID uint64
	Subject    string
	Department string
	Role       string
}

// Authenticator verifies OIDC tokens and resolves active officials.
type Authenticator struct {
	verifier *oidc.IDTokenVerifier
	database *gorm.DB
	audit    *audit.Writer
}

// New discovers the issuer's signing keys and prepares the token verifier.
func New(ctx context.Context, database *gorm.DB, writer *audit.Writer, issuer, audience string) (*Authenticator, error) {
	provider, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, fmt.Errorf("discover OIDC issuer: %w", err)
	}
	return &Authenticator{
		verifier: provider.Verifier(&oidc.Config{ClientID: audience}),
		database: database,
		audit:    writer,
	}, nil
}

// Require enforces a department and minimum role for a route.
func (a *Authenticator) Require(department, minimumRole string) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := strings.Fields(c.GetHeader("Authorization"))
		if len(header) != 2 || !strings.EqualFold(header[0], "Bearer") {
			a.reject(c, http.StatusUnauthorized, "UNAUTHORIZED", "Valid bearer token required", nil)
			return
		}

		token, err := a.verifier.Verify(c.Request.Context(), header[1])
		if err != nil {
			a.reject(c, http.StatusUnauthorized, "UNAUTHORIZED", "Invalid bearer token", nil)
			return
		}
		now := time.Now()
		if token.IssuedAt.IsZero() || token.IssuedAt.After(now) ||
			token.Expiry.Before(token.IssuedAt) || token.Expiry.Sub(token.IssuedAt) > maxTokenLifetime {
			a.reject(c, http.StatusUnauthorized, "UNAUTHORIZED", "Token lifetime is invalid", nil)
			return
		}

		var tokenClaims struct {
			Department string `json:"department"`
		}
		if err := token.Claims(&tokenClaims); err != nil || token.Subject == "" || tokenClaims.Department == "" {
			a.reject(c, http.StatusUnauthorized, "UNAUTHORIZED", "Required token claims are missing", nil)
			return
		}

		var official db.Official
		err = a.database.WithContext(c.Request.Context()).
			Where("idp_subject = ? AND active = ?", token.Subject, true).
			First(&official).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			a.reject(c, http.StatusUnauthorized, "UNAUTHORIZED", "Official is not active", nil)
			return
		}
		if err != nil {
			event := audit.Event{
				Department: "unknown",
				Action:     c.FullPath(),
				Outcome:    db.AuditOutcomeError,
				RequestID:  httpx.RequestID(c.Request.Context()),
			}
			if auditErr := a.audit.Write(c.Request.Context(), event); auditErr != nil {
				httpx.Error(c.Writer, c.Request, http.StatusServiceUnavailable, "AUDIT_UNAVAILABLE", "Request could not be audited")
			} else {
				httpx.InternalError(c.Writer, c.Request)
			}
			c.Abort()
			return
		}
		if tokenClaims.Department != official.Department || official.Department != department ||
			roleRank(official.Role) < roleRank(minimumRole) {
			principal := Principal{OfficialID: official.ID, Subject: token.Subject, Department: official.Department, Role: official.Role}
			a.reject(c, http.StatusForbidden, "FORBIDDEN", "Access denied", &principal)
			return
		}

		c.Set("principal", Principal{
			OfficialID: official.ID,
			Subject:    token.Subject,
			Department: official.Department,
			Role:       official.Role,
		})
		c.Next()
	}
}

func (a *Authenticator) reject(c *gin.Context, status int, code, message string, principal *Principal) {
	event := audit.Event{
		Department: "unknown",
		Action:     c.FullPath(),
		Outcome:    db.AuditOutcomeDenied,
		RequestID:  httpx.RequestID(c.Request.Context()),
	}
	if principal != nil {
		id := principal.OfficialID
		event.OfficialID = &id
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

// RequestPrincipal returns the authenticated official attached by Require.
func RequestPrincipal(c *gin.Context) (Principal, bool) {
	principal, ok := c.Get("principal")
	if !ok {
		return Principal{}, false
	}
	value, ok := principal.(Principal)
	return value, ok
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
