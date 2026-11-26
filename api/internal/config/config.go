// Package config loads and validates application configuration from a .env file
// and process environment variables. It is the only package in the application
// that reads the process environment.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// Environment variable names understood by Load.
const (
	EnvAppName                = "APP_NAME"
	EnvAppEnv                 = "APP_ENV"
	EnvLogLevel               = "LOG_LEVEL"
	EnvServerHost             = "SERVER_HOST"
	EnvServerPort             = "SERVER_PORT"
	EnvServerReadTimeout      = "SERVER_READ_TIMEOUT"
	EnvServerWriteTimeout     = "SERVER_WRITE_TIMEOUT"
	EnvServerIdleTimeout      = "SERVER_IDLE_TIMEOUT"
	EnvServerShutdownTimeout  = "SERVER_SHUTDOWN_TIMEOUT"
	EnvDBHost                 = "DB_HOST"
	EnvDBPort                 = "DB_PORT"
	EnvDBUser                 = "DB_USER"
	EnvDBPassword             = "DB_PASSWORD"
	EnvDBName                 = "DB_NAME"
	EnvDBCharset              = "DB_CHARSET"
	EnvDBDSNIdentity          = "DB_DSN_IDENTITY"
	EnvDBDSNHealth            = "DB_DSN_HEALTH"
	EnvDBDSNElectoral         = "DB_DSN_ELECTORAL"
	EnvDBDSNGrants            = "DB_DSN_GRANTS"
	EnvDBDSNPolice            = "DB_DSN_POLICE"
	EnvDBDSNAudit             = "DB_DSN_AUDIT"
	EnvDBDSNAuthz             = "DB_DSN_AUTHZ"
	EnvDBDSNMigrate           = "DB_DSN_MIGRATE"
	EnvNodeID                 = "NODE_ID"
	EnvBootstrapAdminUsername = "BOOTSTRAP_ADMIN_USERNAME"
	EnvBootstrapAdminPassword = "BOOTSTRAP_ADMIN_PASSWORD"
	EnvVaultAddr              = "VAULT_ADDR"
	EnvVaultToken             = "VAULT_TOKEN"
	EnvVaultTransitMount      = "VAULT_TRANSIT_MOUNT"
	EnvVaultNIDEncryptionKey  = "VAULT_NID_ENCRYPTION_KEY"
	EnvVaultNIDHMACKey        = "VAULT_NID_HMAC_KEY"
	EnvOIDCIssuer             = "OIDC_ISSUER"
	EnvOIDCAudience           = "OIDC_AUDIENCE"
	EnvOIDCJWKSURL            = "OIDC_JWKS_URL"
	EnvTLSCert                = "TLS_CERT"
	EnvTLSKey                 = "TLS_KEY"
	EnvTLSClientCA            = "TLS_CLIENT_CA"
	EnvDevHeaderAuth          = "DEV_HEADER_AUTH"
)

// Development defaults used when an environment variable is unset or empty.
const (
	DefaultAppName            = "register-api"
	DefaultAppEnv             = "development"
	DefaultServerHost         = "127.0.0.1"
	DefaultServerPort         = 8080
	DefaultServerReadTimeout  = 10 * time.Second
	DefaultServerWriteTimeout = 10 * time.Second
	DefaultServerIdleTimeout  = 60 * time.Second
	DefaultShutdownTimeout    = 10 * time.Second
	DefaultDBHost             = "127.0.0.1"
	DefaultDBPort             = 3306
	DefaultDBCharset          = "utf8mb4"
	DefaultVaultTransitMount  = "transit"
)

// Config is the complete, validated application configuration.
type Config struct {
	AppName        string
	AppEnv         string
	LogLevel       slog.Level
	Server         Server
	Database       Database
	DepartmentDSNs map[string]string
	AuditDSN       string
	AuthzDSN       string
	MigrateDSN     string
	NodeID         string
	Vault          Vault
	BootstrapAdmin BootstrapAdmin
	OIDC           OIDC
	DevHeaderAuth  bool
}

// Server groups the HTTP server settings.
type Server struct {
	Host            string
	Port            int
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	ShutdownTimeout time.Duration
	TLSCert         string
	TLSKey          string
	TLSClientCA     string
}

type OIDC struct {
	Issuer   string
	Audience string
	JWKSURL  string
}

// Database groups the MySQL connection settings.
type Database struct {
	Host     string
	Port     int
	User     string
	Password string
	Name     string
	Charset  string
}

// BootstrapAdmin credentials are used only when the database has no admin.
type BootstrapAdmin struct {
	Username string
	Password string
}

// Vault contains the Transit API configuration used for national-ID data.
type Vault struct {
	Address          string
	Token            string
	TransitMount     string
	NIDEncryptionKey string
	NIDHMACKey       string
}

// Addr returns the host:port address the HTTP server listens on.
func (s Server) Addr() string {
	return net.JoinHostPort(s.Host, strconv.Itoa(s.Port))
}

// Load reads .env and process environment variables, applies development
// defaults for unset variables and validates the result. Variables already set
// in the process environment take precedence over values in .env.
func Load() (Config, error) {
	if err := loadEnvFile(".env"); err != nil {
		return Config{}, err
	}

	level, err := envLevel(EnvLogLevel, slog.LevelInfo)
	if err != nil {
		return Config{}, err
	}

	port, err := envInt(EnvServerPort, DefaultServerPort)
	if err != nil {
		return Config{}, err
	}

	dbPort, err := envInt(EnvDBPort, DefaultDBPort)
	if err != nil {
		return Config{}, err
	}

	readTimeout, err := envDuration(EnvServerReadTimeout, DefaultServerReadTimeout)
	if err != nil {
		return Config{}, err
	}

	writeTimeout, err := envDuration(EnvServerWriteTimeout, DefaultServerWriteTimeout)
	if err != nil {
		return Config{}, err
	}

	idleTimeout, err := envDuration(EnvServerIdleTimeout, DefaultServerIdleTimeout)
	if err != nil {
		return Config{}, err
	}

	shutdownTimeout, err := envDuration(EnvServerShutdownTimeout, DefaultShutdownTimeout)
	if err != nil {
		return Config{}, err
	}
	appEnv := envString(EnvAppEnv, DefaultAppEnv)
	devHeaderAuth, err := envBool(EnvDevHeaderAuth, appEnv == "development")
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		AppName:  envString(EnvAppName, DefaultAppName),
		AppEnv:   appEnv,
		LogLevel: level,
		Server: Server{
			Host:            envString(EnvServerHost, DefaultServerHost),
			Port:            port,
			ReadTimeout:     readTimeout,
			WriteTimeout:    writeTimeout,
			IdleTimeout:     idleTimeout,
			ShutdownTimeout: shutdownTimeout,
			TLSCert:         envString(EnvTLSCert, ""),
			TLSKey:          envString(EnvTLSKey, ""),
			TLSClientCA:     envString(EnvTLSClientCA, ""),
		},
		Database: Database{
			Host:     envString(EnvDBHost, DefaultDBHost),
			Port:     dbPort,
			User:     envString(EnvDBUser, ""),
			Password: envString(EnvDBPassword, ""),
			Name:     envString(EnvDBName, ""),
			Charset:  envString(EnvDBCharset, DefaultDBCharset),
		},
		DepartmentDSNs: map[string]string{
			"identity":  envString(EnvDBDSNIdentity, ""),
			"health":    envString(EnvDBDSNHealth, ""),
			"electoral": envString(EnvDBDSNElectoral, ""),
			"grants":    envString(EnvDBDSNGrants, ""),
			"police":    envString(EnvDBDSNPolice, ""),
		},
		AuditDSN:   envString(EnvDBDSNAudit, ""),
		AuthzDSN:   envString(EnvDBDSNAuthz, ""),
		MigrateDSN: envString(EnvDBDSNMigrate, ""),
		NodeID:     envString(EnvNodeID, ""),
		Vault: Vault{
			Address:          envString(EnvVaultAddr, ""),
			Token:            envString(EnvVaultToken, ""),
			TransitMount:     envString(EnvVaultTransitMount, DefaultVaultTransitMount),
			NIDEncryptionKey: envString(EnvVaultNIDEncryptionKey, ""),
			NIDHMACKey:       envString(EnvVaultNIDHMACKey, ""),
		},
		BootstrapAdmin: BootstrapAdmin{
			Username: envString(EnvBootstrapAdminUsername, ""),
			Password: envSecret(EnvBootstrapAdminPassword),
		},
		OIDC: OIDC{
			Issuer:   envString(EnvOIDCIssuer, ""),
			Audience: envString(EnvOIDCAudience, ""),
			JWKSURL:  envString(EnvOIDCJWKSURL, ""),
		},
		DevHeaderAuth: devHeaderAuth,
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func loadEnvFile(path string) error {
	if err := godotenv.Load(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("load environment file %q: %w", path, err)
	}
	return nil
}

// Validate reports configuration values that cannot be used to run a server.
func (c Config) Validate() error {
	if strings.TrimSpace(c.AppName) == "" {
		return fmt.Errorf("%s must not be empty", EnvAppName)
	}
	if strings.TrimSpace(c.AppEnv) == "" {
		return fmt.Errorf("%s must not be empty", EnvAppEnv)
	}
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("%s must be between 1 and 65535, got %d", EnvServerPort, c.Server.Port)
	}
	if c.Database.Port < 1 || c.Database.Port > 65535 {
		return fmt.Errorf("%s must be between 1 and 65535, got %d", EnvDBPort, c.Database.Port)
	}
	for _, department := range []string{"identity", "health", "electoral", "grants", "police"} {
		if strings.TrimSpace(c.DepartmentDSNs[department]) == "" {
			return fmt.Errorf("%s is required", "DB_DSN_"+strings.ToUpper(department))
		}
	}
	if strings.TrimSpace(c.AuditDSN) == "" {
		return fmt.Errorf("%s is required", EnvDBDSNAudit)
	}
	if strings.TrimSpace(c.AuthzDSN) == "" {
		return fmt.Errorf("%s is required", EnvDBDSNAuthz)
	}
	if (c.Server.TLSCert == "") != (c.Server.TLSKey == "") {
		return fmt.Errorf("%s and %s must be configured together", EnvTLSCert, EnvTLSKey)
	}
	host := strings.Trim(c.Server.Host, "[]")
	if ip := net.ParseIP(host); host != "localhost" && (ip == nil || !ip.IsLoopback()) &&
		(c.Server.TLSCert == "" || c.Server.TLSClientCA == "") {
		return fmt.Errorf("non-loopback binds require TLS_CERT, TLS_KEY and TLS_CLIENT_CA")
	}
	if c.AppEnv != "development" {
		if c.OIDC.Issuer == "" || c.OIDC.Audience == "" {
			return fmt.Errorf("%s and %s are required outside development", EnvOIDCIssuer, EnvOIDCAudience)
		}
		if c.Server.TLSCert == "" || c.Server.TLSClientCA == "" {
			return fmt.Errorf("TLS_CERT, TLS_KEY and TLS_CLIENT_CA are required outside development")
		}
	}
	if c.DevHeaderAuth && c.AppEnv != "development" {
		return fmt.Errorf("%s is only allowed in development", EnvDevHeaderAuth)
	}
	if strings.TrimSpace(c.NodeID) == "" {
		return fmt.Errorf("%s must not be empty", EnvNodeID)
	}
	if len(c.NodeID) > 20 {
		return fmt.Errorf("%s must be at most 20 characters", EnvNodeID)
	}
	if len(c.BootstrapAdmin.Username) > 80 {
		return fmt.Errorf("%s must be at most 80 characters", EnvBootstrapAdminUsername)
	}
	if strings.TrimSpace(c.Vault.Address) == "" {
		return fmt.Errorf("%s must not be empty", EnvVaultAddr)
	}
	if strings.TrimSpace(c.Vault.Token) == "" {
		return fmt.Errorf("%s must not be empty", EnvVaultToken)
	}
	if strings.TrimSpace(c.Vault.TransitMount) == "" {
		return fmt.Errorf("%s must not be empty", EnvVaultTransitMount)
	}
	if strings.TrimSpace(c.Vault.NIDEncryptionKey) == "" {
		return fmt.Errorf("%s must not be empty", EnvVaultNIDEncryptionKey)
	}
	if strings.TrimSpace(c.Vault.NIDHMACKey) == "" {
		return fmt.Errorf("%s must not be empty", EnvVaultNIDHMACKey)
	}

	timeouts := []struct {
		name  string
		value time.Duration
	}{
		{EnvServerReadTimeout, c.Server.ReadTimeout},
		{EnvServerWriteTimeout, c.Server.WriteTimeout},
		{EnvServerIdleTimeout, c.Server.IdleTimeout},
		{EnvServerShutdownTimeout, c.Server.ShutdownTimeout},
	}
	for _, t := range timeouts {
		if t.value <= 0 {
			return fmt.Errorf("%s must be a positive duration, got %s", t.name, t.value)
		}
	}
	return nil
}

func envString(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func envSecret(key string) string {
	value, _ := os.LookupEnv(key)
	return value
}

func envInt(key string, fallback int) (int, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer, got %q", key, raw)
	}
	return v, nil
}

func envDuration(key string, fallback time.Duration) (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	v, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be a duration such as \"10s\", got %q", key, raw)
	}
	return v, nil
}

func envBool(key string, fallback bool) (bool, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%s must be true or false", key)
	}
	return value, nil
}

func envLevel(key string, fallback slog.Level) (slog.Level, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	var level slog.Level
	if err := level.UnmarshalText([]byte(raw)); err != nil {
		return 0, fmt.Errorf("%s must be one of debug, info, warn, error, got %q", key, raw)
	}
	return level, nil
}
