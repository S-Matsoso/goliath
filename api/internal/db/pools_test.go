package db

import (
	"strings"
	"testing"
)

func TestValidatePoolDSNsRequiresDedicatedTLSAccounts(t *testing.T) {
	dsns := map[string]string{
		"identity":  "identity:secret@tcp(127.0.0.1:3306)/register?tls=true&parseTime=true",
		"health":    "health:secret@tcp(127.0.0.1:3306)/register?tls=true&parseTime=true",
		"electoral": "electoral:secret@tcp(127.0.0.1:3306)/register?tls=true&parseTime=true",
		"grants":    "grants:secret@tcp(127.0.0.1:3306)/register?tls=true&parseTime=true",
		"police":    "police:secret@tcp(127.0.0.1:3306)/register?tls=true&parseTime=true",
	}
	if err := validatePoolDSNs(dsns,
		"audit:secret@tcp(127.0.0.1:3306)/register?tls=true",
		"authz:secret@tcp(127.0.0.1:3306)/register?tls=true"); err != nil {
		t.Fatalf("valid dedicated pools rejected: %v", err)
	}
}

func TestValidatePoolDSNsRejectsSharedAccount(t *testing.T) {
	dsns := map[string]string{
		"identity":  "shared:secret@tcp(127.0.0.1:3306)/register?tls=true",
		"health":    "shared:secret@tcp(127.0.0.1:3306)/register?tls=true",
		"electoral": "electoral:secret@tcp(127.0.0.1:3306)/register?tls=true",
		"grants":    "grants:secret@tcp(127.0.0.1:3306)/register?tls=true",
		"police":    "police:secret@tcp(127.0.0.1:3306)/register?tls=true",
	}
	err := validatePoolDSNs(dsns,
		"audit:secret@tcp(127.0.0.1:3306)/register?tls=true",
		"authz:secret@tcp(127.0.0.1:3306)/register?tls=true")
	if err == nil || !strings.Contains(err.Error(), "share an account") {
		t.Fatalf("expected shared-account error, got %v", err)
	}
}

func TestValidatePoolDSNsRejectsMissingDepartment(t *testing.T) {
	err := validatePoolDSNs(map[string]string{}, "audit:secret@tcp(localhost)/register?tls=true", "authz:secret@tcp(localhost)/register?tls=true")
	if err == nil || !strings.Contains(err.Error(), "DB_DSN_IDENTITY") {
		t.Fatalf("expected missing identity DSN error, got %v", err)
	}
}

func TestValidateDSNTransportRequiresTLS(t *testing.T) {
	if err := validateDSNTransport("identity:secret@tcp(localhost:3306)/register"); err == nil {
		t.Fatal("expected clear-text TCP DSN to fail")
	}
	if err := validateDSNTransport("identity:secret@tcp(localhost:3306)/register?tls=true"); err != nil {
		t.Fatalf("TLS DSN rejected: %v", err)
	}
}
