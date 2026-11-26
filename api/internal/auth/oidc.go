package auth

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/S-Matsoso/goliath/internal/audit"
	"github.com/S-Matsoso/goliath/internal/config"
	"github.com/S-Matsoso/goliath/internal/db"
	"github.com/S-Matsoso/goliath/internal/platform/httpx"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	devOfficialHeader = "X-Dev-Official-ID"
	jwksCacheDuration = 5 * time.Minute
	maxJWTLifetime    = 15 * time.Minute
	jwtClockSkew      = 30 * time.Second
)

var ErrUnauthenticated = errors.New("unauthenticated")

type JWTClaims struct {
	OfficialID uint64
	Subject    string
	Department string
	Role       string
	RequestID  string
}

type OIDCAuthenticator struct {
	config        config.OIDC
	devHeaderAuth bool
	authz         *gorm.DB
	audit         *audit.Writer
	client        *http.Client
	mu            sync.Mutex
	keys          map[string]*rsa.PublicKey
	keysExpiresAt time.Time
	now           func() time.Time
}

type jwtHeader struct {
	Algorithm string `json:"alg"`
	KeyID     string `json:"kid"`
}

type jwtPayload struct {
	Issuer     string          `json:"iss"`
	Audience   json.RawMessage `json:"aud"`
	Subject    string          `json:"sub"`
	Department string          `json:"department"`
	ExpiresAt  int64           `json:"exp"`
	IssuedAt   int64           `json:"iat"`
	NotBefore  int64           `json:"nbf"`
}

type jwksDocument struct {
	Keys []struct {
		KeyID     string `json:"kid"`
		KeyType   string `json:"kty"`
		Use       string `json:"use"`
		Algorithm string `json:"alg"`
		Modulus   string `json:"n"`
		Exponent  string `json:"e"`
	} `json:"keys"`
}

func NewOIDCAuthenticator(cfg config.OIDC, devHeaderAuth bool, authz *gorm.DB, writer *audit.Writer) (*OIDCAuthenticator, error) {
	if authz == nil || writer == nil {
		return nil, errors.New("OIDC authentication requires authorization and audit databases")
	}
	if !devHeaderAuth {
		issuer, err := url.Parse(cfg.Issuer)
		if err != nil || issuer.Host == "" || (issuer.Scheme != "https" && !isLoopbackHost(issuer.Host)) {
			return nil, errors.New("OIDC issuer must be an absolute HTTPS URL")
		}
		if cfg.Audience == "" {
			return nil, errors.New("OIDC audience is required")
		}
		if cfg.JWKSURL != "" {
			jwksURL, err := url.Parse(cfg.JWKSURL)
			if err != nil || jwksURL.Host == "" || (jwksURL.Scheme != "https" && !isLoopbackHost(jwksURL.Host)) {
				return nil, errors.New("OIDC JWKS URL must be an absolute HTTPS URL")
			}
		}
	}
	return &OIDCAuthenticator{
		config:        cfg,
		devHeaderAuth: devHeaderAuth,
		authz:         authz,
		audit:         writer,
		client: &http.Client{
			Timeout: 5 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		keys: make(map[string]*rsa.PublicKey),
		now:  time.Now,
	}, nil
}

func (a *OIDCAuthenticator) Require(department, minimumRole string) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims, err := a.authenticate(c.Request)
		if err != nil {
			if errors.Is(err, ErrUnauthenticated) {
				a.reject(c, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required", nil)
			} else {
				a.reject(c, http.StatusServiceUnavailable, "AUTHORIZATION_UNAVAILABLE", "Authorization service unavailable", nil)
			}
			return
		}
		if c.Request.TLS != nil && len(c.Request.TLS.PeerCertificates) > 0 &&
			!strings.EqualFold(c.Request.TLS.PeerCertificates[0].Subject.CommonName, claims.Department) {
			a.reject(c, http.StatusForbidden, "FORBIDDEN", "Client certificate department mismatch", &claims)
			return
		}
		if department != "" && (claims.Department != department || !roleAllows(claims.Role, minimumRole)) {
			a.reject(c, http.StatusForbidden, "FORBIDDEN", "Access denied", &claims)
			return
		}
		claims.RequestID = httpx.RequestID(c.Request.Context())
		c.Set("oidc_claims", claims)
		c.Next()
	}
}

func (a *OIDCAuthenticator) RequireAny() gin.HandlerFunc {
	return a.Require("", "")
}

func RequestClaims(c *gin.Context) (JWTClaims, bool) {
	value, ok := c.Get("oidc_claims")
	if !ok {
		return JWTClaims{}, false
	}
	claims, ok := value.(JWTClaims)
	return claims, ok
}

func (a *OIDCAuthenticator) authenticate(request *http.Request) (JWTClaims, error) {
	var subject, department string
	if a.devHeaderAuth && request.Header.Get(devOfficialHeader) != "" {
		id, err := strconv.ParseUint(request.Header.Get(devOfficialHeader), 10, 64)
		if err != nil || id == 0 {
			return JWTClaims{}, fmt.Errorf("%w: invalid development official ID", ErrUnauthenticated)
		}
		var official db.Official
		if err := a.authz.WithContext(request.Context()).Where("id = ? AND active = ?", id, true).First(&official).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return JWTClaims{}, fmt.Errorf("%w: development official is unavailable", ErrUnauthenticated)
			}
			return JWTClaims{}, fmt.Errorf("load development official: %w", err)
		}
		var dept db.Department
		if err := a.authz.WithContext(request.Context()).Where("code = ? AND active = ?", official.Department, true).First(&dept).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return JWTClaims{}, fmt.Errorf("%w: development department is unavailable", ErrUnauthenticated)
			}
			return JWTClaims{}, fmt.Errorf("load development department: %w", err)
		}
		return JWTClaims{
			OfficialID: official.ID,
			Subject:    official.IDPSubject,
			Department: official.Department,
			Role:       official.Role,
		}, nil
	}

	fields := strings.Fields(request.Header.Get("Authorization"))
	if len(fields) != 2 || !strings.EqualFold(fields[0], "Bearer") {
		return JWTClaims{}, fmt.Errorf("%w: missing bearer token", ErrUnauthenticated)
	}
	payload, err := a.verifyToken(request.Context(), fields[1])
	if err != nil {
		return JWTClaims{}, err
	}
	subject = payload.Subject
	department = payload.Department

	var official db.Official
	if err := a.authz.WithContext(request.Context()).
		Where("idp_subject = ? AND active = ?", subject, true).
		First(&official).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return JWTClaims{}, fmt.Errorf("%w: official is unavailable", ErrUnauthenticated)
		}
		return JWTClaims{}, fmt.Errorf("load official: %w", err)
	}
	if official.Department != department {
		return JWTClaims{}, fmt.Errorf("%w: token department does not match official", ErrUnauthenticated)
	}
	var dept db.Department
	if err := a.authz.WithContext(request.Context()).
		Where("code = ? AND active = ?", official.Department, true).
		First(&dept).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return JWTClaims{}, fmt.Errorf("%w: department is unavailable", ErrUnauthenticated)
		}
		return JWTClaims{}, fmt.Errorf("load department: %w", err)
	}
	return JWTClaims{
		OfficialID: official.ID,
		Subject:    subject,
		Department: official.Department,
		Role:       official.Role,
	}, nil
}

func (a *OIDCAuthenticator) verifyToken(ctx context.Context, token string) (jwtPayload, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return jwtPayload{}, fmt.Errorf("%w: malformed JWT", ErrUnauthenticated)
	}
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return jwtPayload{}, fmt.Errorf("%w: malformed JWT header", ErrUnauthenticated)
	}
	var header jwtHeader
	if err := json.Unmarshal(headerBytes, &header); err != nil || header.Algorithm != "RS256" || header.KeyID == "" {
		return jwtPayload{}, fmt.Errorf("%w: unsupported JWT header", ErrUnauthenticated)
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return jwtPayload{}, fmt.Errorf("%w: malformed JWT payload", ErrUnauthenticated)
	}
	var payload jwtPayload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return jwtPayload{}, fmt.Errorf("%w: malformed JWT claims", ErrUnauthenticated)
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return jwtPayload{}, fmt.Errorf("%w: malformed JWT signature", ErrUnauthenticated)
	}
	key, err := a.publicKey(ctx, header.KeyID)
	if err != nil {
		return jwtPayload{}, err
	}
	hash := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, hash[:], signature); err != nil {
		return jwtPayload{}, fmt.Errorf("%w: invalid JWT signature", ErrUnauthenticated)
	}
	if err := a.validateClaims(payload); err != nil {
		return jwtPayload{}, fmt.Errorf("%w: %v", ErrUnauthenticated, err)
	}
	return payload, nil
}

func (a *OIDCAuthenticator) validateClaims(claims jwtPayload) error {
	now := a.now()
	audiences, err := parseAudience(claims.Audience)
	if err != nil {
		return errors.New("invalid JWT audience")
	}
	audienceValid := false
	for _, audience := range audiences {
		if audience == a.config.Audience {
			audienceValid = true
			break
		}
	}
	if claims.Issuer != a.config.Issuer || !audienceValid || claims.Subject == "" || claims.Department == "" {
		return errors.New("JWT issuer, audience, or subject is invalid")
	}
	issuedAt := time.Unix(claims.IssuedAt, 0)
	expiresAt := time.Unix(claims.ExpiresAt, 0)
	if claims.IssuedAt == 0 || claims.ExpiresAt == 0 ||
		issuedAt.After(now.Add(jwtClockSkew)) ||
		expiresAt.Before(now.Add(-jwtClockSkew)) ||
		expiresAt.Sub(issuedAt) > maxJWTLifetime {
		return errors.New("JWT lifetime is invalid")
	}
	if claims.NotBefore != 0 && time.Unix(claims.NotBefore, 0).After(now.Add(jwtClockSkew)) {
		return errors.New("JWT is not yet valid")
	}
	return nil
}

func parseAudience(raw json.RawMessage) ([]string, error) {
	var one string
	if err := json.Unmarshal(raw, &one); err == nil {
		return []string{one}, nil
	}
	var many []string
	if err := json.Unmarshal(raw, &many); err != nil {
		return nil, err
	}
	return many, nil
}

func (a *OIDCAuthenticator) publicKey(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if time.Now().Before(a.keysExpiresAt) {
		if key, ok := a.keys[kid]; ok {
			return key, nil
		}
	}
	if err := a.refreshKeys(ctx); err != nil {
		return nil, err
	}
	key, ok := a.keys[kid]
	if !ok {
		return nil, fmt.Errorf("%w: JWT signing key is unknown", ErrUnauthenticated)
	}
	return key, nil
}

func (a *OIDCAuthenticator) refreshKeys(ctx context.Context) error {
	jwksURL := a.config.JWKSURL
	if jwksURL == "" {
		discoveryURL := strings.TrimRight(a.config.Issuer, "/") + "/.well-known/openid-configuration"
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, discoveryURL, nil)
		if err != nil {
			return fmt.Errorf("create OIDC discovery request: %w", err)
		}
		response, err := a.client.Do(request)
		if err != nil {
			return fmt.Errorf("fetch OIDC discovery document: %w", err)
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			return fmt.Errorf("OIDC discovery returned HTTP %d", response.StatusCode)
		}
		var discovery struct {
			JWKSURL string `json:"jwks_uri"`
		}
		if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&discovery); err != nil {
			return fmt.Errorf("decode OIDC discovery document: %w", err)
		}
		jwksURL = discovery.JWKSURL
		if !validHTTPSURL(jwksURL) {
			return errors.New("OIDC discovery returned an invalid JWKS URL")
		}
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, jwksURL, nil)
	if err != nil {
		return fmt.Errorf("create JWKS request: %w", err)
	}
	response, err := a.client.Do(request)
	if err != nil {
		return fmt.Errorf("fetch JWKS: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("JWKS endpoint returned HTTP %d", response.StatusCode)
	}
	var document jwksDocument
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&document); err != nil {
		return fmt.Errorf("decode JWKS: %w", err)
	}
	keys := make(map[string]*rsa.PublicKey, len(document.Keys))
	for _, jwk := range document.Keys {
		if jwk.KeyType != "RSA" || (jwk.Use != "" && jwk.Use != "sig") ||
			(jwk.Algorithm != "" && jwk.Algorithm != "RS256") || jwk.KeyID == "" {
			continue
		}
		modulus, err := base64.RawURLEncoding.DecodeString(jwk.Modulus)
		if err != nil {
			continue
		}
		exponentBytes, err := base64.RawURLEncoding.DecodeString(jwk.Exponent)
		if err != nil || len(exponentBytes) == 0 || len(exponentBytes) > 4 {
			continue
		}
		exponent := 0
		for _, b := range exponentBytes {
			exponent = exponent<<8 | int(b)
		}
		if exponent < 3 || exponent%2 == 0 {
			continue
		}
		key := &rsa.PublicKey{N: new(big.Int).SetBytes(modulus), E: exponent}
		if key.N.BitLen() >= 2048 {
			keys[jwk.KeyID] = key
		}
	}
	if len(keys) == 0 {
		return errors.New("JWKS contains no supported signing keys")
	}
	a.keys = keys
	a.keysExpiresAt = time.Now().Add(jwksCacheDuration)
	return nil
}

func (a *OIDCAuthenticator) reject(c *gin.Context, status int, code, message string, claims *JWTClaims) {
	event := audit.Event{
		Department: "unknown",
		Action:     c.FullPath(),
		Outcome:    db.AuditOutcomeDenied,
		RequestID:  httpx.RequestID(c.Request.Context()),
	}
	if claims != nil {
		event.OfficialID = &claims.OfficialID
		event.Department = claims.Department
	}
	if err := a.audit.Write(c.Request.Context(), event); err != nil {
		httpx.Error(c.Writer, c.Request, http.StatusServiceUnavailable, "AUDIT_UNAVAILABLE", "Request could not be audited")
		c.Abort()
		return
	}
	httpx.Error(c.Writer, c.Request, status, code, message)
	c.Abort()
}

func validHTTPSURL(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && parsed.Host != "" && (parsed.Scheme == "https" || isLoopbackHost(parsed.Host))
}

func isLoopbackHost(hostport string) bool {
	host, _, err := net.SplitHostPort(hostport)
	if err != nil {
		host = hostport
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
