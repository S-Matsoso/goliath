package auth

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/S-Matsoso/goliath/internal/config"
)

func TestVerifyTokenRefreshesJWKSAndChecksSignature(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	var fetches atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fetches.Add(1)
		exponent := big.NewInt(int64(privateKey.PublicKey.E)).Bytes()
		_ = json.NewEncoder(w).Encode(jwksDocument{
			Keys: []struct {
				KeyID     string `json:"kid"`
				KeyType   string `json:"kty"`
				Use       string `json:"use"`
				Algorithm string `json:"alg"`
				Modulus   string `json:"n"`
				Exponent  string `json:"e"`
			}{{
				KeyID:     "test-key",
				KeyType:   "RSA",
				Use:       "sig",
				Algorithm: "RS256",
				Modulus:   base64.RawURLEncoding.EncodeToString(privateKey.PublicKey.N.Bytes()),
				Exponent:  base64.RawURLEncoding.EncodeToString(exponent),
			}},
		})
	}))
	defer server.Close()

	oidcConfig := configOIDC(server.URL)
	oidcConfig.JWKSURL = server.URL
	authenticator := &OIDCAuthenticator{
		config: oidcConfig,
		client: server.Client(),
		keys:   make(map[string]*rsa.PublicKey),
		now:    time.Now,
	}
	token := signedToken(t, privateKey, server.URL, time.Now())
	for range 2 {
		claims, err := authenticator.verifyToken(t.Context(), token)
		if err != nil {
			t.Fatalf("verifyToken() error = %v", err)
		}
		if claims.Subject != "official-123" || claims.Department != "health" {
			t.Fatalf("claims = %+v", claims)
		}
	}
	if got := fetches.Load(); got != 1 {
		t.Fatalf("JWKS fetched %d times, want one cached fetch", got)
	}
}

func TestValidateClaimsRejectsWrongAudienceAndExcessiveLifetime(t *testing.T) {
	now := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	authenticator := &OIDCAuthenticator{
		config: configOIDC("https://issuer.example"),
		now:    func() time.Time { return now },
	}
	claims := jwtPayload{
		Issuer:     "https://issuer.example",
		Audience:   json.RawMessage(`"register-api"`),
		Subject:    "subject",
		Department: "identity",
		IssuedAt:   now.Unix(),
		ExpiresAt:  now.Add(10 * time.Minute).Unix(),
	}
	if err := authenticator.validateClaims(claims); err != nil {
		t.Fatalf("valid claims rejected: %v", err)
	}
	claims.Audience = json.RawMessage(`"other-api"`)
	if err := authenticator.validateClaims(claims); err == nil {
		t.Fatal("wrong audience accepted")
	}
	claims.Audience = json.RawMessage(`"register-api"`)
	claims.ExpiresAt = now.Add(16 * time.Minute).Unix()
	if err := authenticator.validateClaims(claims); err == nil {
		t.Fatal("JWT lifetime longer than 15 minutes accepted")
	}
}

func TestRoleAuthorizationKeepsAuditorsReadOnly(t *testing.T) {
	tests := []struct {
		role    string
		minimum string
		want    bool
	}{
		{role: "clerk", minimum: "clerk", want: true},
		{role: "clerk", minimum: "supervisor", want: false},
		{role: "supervisor", minimum: "clerk", want: true},
		{role: "supervisor", minimum: "auditor", want: false},
		{role: "auditor", minimum: "auditor", want: true},
		{role: "auditor", minimum: "clerk", want: false},
		{role: "auditor", minimum: "supervisor", want: false},
		{role: "admin", minimum: "auditor", want: true},
		{role: "unknown", minimum: "clerk", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.role+"/"+tt.minimum, func(t *testing.T) {
			if got := roleAllows(tt.role, tt.minimum); got != tt.want {
				t.Fatalf("roleAllows(%q, %q) = %t, want %t", tt.role, tt.minimum, got, tt.want)
			}
		})
	}
}

func configOIDC(issuer string) config.OIDC {
	return config.OIDC{Issuer: issuer, Audience: "register-api"}
}

func signedToken(t *testing.T, key *rsa.PrivateKey, issuer string, now time.Time) string {
	t.Helper()
	header, err := json.Marshal(jwtHeader{Algorithm: "RS256", KeyID: "test-key"})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(jwtPayload{
		Issuer:     issuer,
		Audience:   json.RawMessage(`["register-api"]`),
		Subject:    "official-123",
		Department: "health",
		IssuedAt:   now.Unix(),
		ExpiresAt:  now.Add(10 * time.Minute).Unix(),
	})
	if err != nil {
		t.Fatal(err)
	}
	unsigned := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
	digest := sha256.Sum256([]byte(unsigned))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%s.%s", unsigned, base64.RawURLEncoding.EncodeToString(signature))
}
