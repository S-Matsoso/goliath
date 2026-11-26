package auth

import (
	"strings"
	"testing"
)

func TestPasswordHashVerification(t *testing.T) {
	password := "Correct horse battery staple 2031"
	first, err := HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	second, err := HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("password hashes should use independent salts")
	}
	if !VerifyPassword(first, password) {
		t.Fatal("VerifyPassword rejected the matching password")
	}
	if VerifyPassword(first, "a different password") {
		t.Fatal("VerifyPassword accepted a different password")
	}
}

func TestValidatePasswordPolicy(t *testing.T) {
	tests := []struct {
		name     string
		username string
		password string
		wantErr  bool
	}{
		{name: "long passphrase", username: "staff.user", password: "A unique long passphrase", wantErr: false},
		{name: "too short", username: "staff.user", password: "short phrase", wantErr: true},
		{name: "contains username", username: "staff.user", password: "staff.user has a passphrase", wantErr: true},
		{name: "spaces are allowed in passphrases", username: "staff.user", password: "  another long passphrase  ", wantErr: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidatePassword(test.username, test.password)
			if (err != nil) != test.wantErr {
				t.Fatalf("ValidatePassword() error = %v, wantErr %t", err, test.wantErr)
			}
		})
	}
}

func TestGeneratedTemporaryPasswordMeetsPolicy(t *testing.T) {
	password, err := GenerateTemporaryPassword()
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidatePassword("staff.user", password); err != nil {
		t.Fatalf("generated password does not meet policy: %v", err)
	}
	if len(password) != 24 || !strings.ContainsAny(password, "!@#$%_-+") {
		t.Fatalf("generated password has unexpected shape: %q", password)
	}
}
