package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

var (
	ErrPasswordPolicy = errors.New("password does not meet the security requirements")
	ErrPasswordReused = errors.New("new password must be different from the current password")
)

const (
	passwordMinRunes = 15
	passwordMaxBytes = 1024
	argonMemory      = 64 * 1024
	argonIterations  = 3
	argonParallelism = 1
	argonKeyLength   = 32
)

var dummyPasswordHash = encodePasswordHash(
	[]byte("this-is-not-a-user-password"),
	[]byte("0123456789abcdef"),
)

func ValidatePassword(username, password string) error {
	if !utf8.ValidString(password) || utf8.RuneCountInString(password) < passwordMinRunes {
		return fmt.Errorf("%w: use at least %d characters", ErrPasswordPolicy, passwordMinRunes)
	}
	if len(password) > passwordMaxBytes {
		return fmt.Errorf("%w: use no more than %d bytes", ErrPasswordPolicy, passwordMaxBytes)
	}
	if username != "" && strings.Contains(strings.ToLower(password), strings.ToLower(username)) {
		return fmt.Errorf("%w: do not include your username", ErrPasswordPolicy)
	}
	return nil
}

func ValidateUsername(username string) error {
	if len(username) < 3 || len(username) > 64 {
		return fmt.Errorf("username must be between 3 and 64 characters")
	}
	for _, character := range username {
		if !(character >= 'a' && character <= 'z') &&
			!(character >= '0' && character <= '9') &&
			character != '.' && character != '_' && character != '-' {
			return fmt.Errorf("username may contain only lowercase letters, numbers, dots, underscores, and hyphens")
		}
	}
	return nil
}

func HashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}
	return encodePasswordHash([]byte(password), salt), nil
}

func VerifyPassword(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" ||
		parts[3] != "m=65536,t=3,p=1" {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) != 16 {
		return false
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(expected) != argonKeyLength {
		return false
	}
	actual := argon2.IDKey([]byte(password), salt, argonIterations, argonMemory, argonParallelism, argonKeyLength)
	return subtle.ConstantTimeCompare(actual, expected) == 1
}

func GenerateTemporaryPassword() (string, error) {
	const (
		uppercase = "ABCDEFGHJKLMNPQRSTUVWXYZ"
		lowercase = "abcdefghijkmnopqrstuvwxyz"
		digits    = "23456789"
		symbols   = "!@#$%_-+"
		all       = uppercase + lowercase + digits + symbols
	)
	password := make([]byte, 24)
	required := []string{uppercase, lowercase, digits, symbols}
	for i, characters := range required {
		value, err := randomCharacter(characters)
		if err != nil {
			return "", fmt.Errorf("generate temporary password: %w", err)
		}
		password[i] = value
	}
	for i := len(required); i < len(password); i++ {
		value, err := randomCharacter(all)
		if err != nil {
			return "", fmt.Errorf("generate temporary password: %w", err)
		}
		password[i] = value
	}
	if err := shuffle(password); err != nil {
		return "", fmt.Errorf("shuffle temporary password: %w", err)
	}
	return string(password), nil
}

func encodePasswordHash(password, salt []byte) string {
	hash := argon2.IDKey(password, salt, argonIterations, argonMemory, argonParallelism, argonKeyLength)
	return "$argon2id$v=19$m=65536,t=3,p=1$" +
		base64.RawStdEncoding.EncodeToString(salt) + "$" +
		base64.RawStdEncoding.EncodeToString(hash)
}
