package config

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

var envKeys = []string{
	EnvAppName,
	EnvAppEnv,
	EnvLogLevel,
	EnvServerHost,
	EnvServerPort,
	EnvServerReadTimeout,
	EnvServerWriteTimeout,
	EnvServerIdleTimeout,
	EnvServerShutdownTimeout,
	EnvDBHost,
	EnvDBPort,
	EnvDBUser,
	EnvDBPassword,
	EnvDBName,
	EnvDBCharset,
	EnvNodeID,
	EnvOIDCIssuer,
	EnvOIDCAudience,
	EnvVaultAddr,
	EnvVaultToken,
	EnvVaultTransitMount,
	EnvVaultNIDEncryptionKey,
	EnvVaultNIDHMACKey,
}

func clearEnv(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	for _, key := range envKeys {
		t.Setenv(key, "")
	}
	t.Setenv(EnvDBUser, "test-user")
	t.Setenv(EnvDBName, "test-register")
	t.Setenv(EnvNodeID, "test-node")
	t.Setenv(EnvOIDCIssuer, "https://issuer.example.test")
	t.Setenv(EnvOIDCAudience, "register-api")
	t.Setenv(EnvVaultAddr, "https://vault.example.test")
	t.Setenv(EnvVaultToken, "test-token")
	t.Setenv(EnvVaultNIDEncryptionKey, "nid-encryption")
	t.Setenv(EnvVaultNIDHMACKey, "nid-hmac")
}

func unsetEnvForTest(t *testing.T, key string) {
	t.Helper()
	original, wasSet := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		var err error
		if wasSet {
			err = os.Setenv(key, original)
		} else {
			err = os.Unsetenv(key)
		}
		if err != nil {
			t.Errorf("restore %s: %v", key, err)
		}
	})
}

func TestLoadReadsEnvFileAndPreservesProcessOverrides(t *testing.T) {
	for _, key := range envKeys {
		unsetEnvForTest(t, key)
	}

	dir := t.TempDir()
	envFile := filepath.Join(dir, ".env")
	content := "APP_NAME=file-app\n" +
		"APP_ENV=test\n" +
		"LOG_LEVEL=debug\n" +
		"DB_HOST=db.internal\n" +
		"DB_PORT=3307\n" +
		"DB_USER=file-user\n" +
		"DB_PASSWORD=file-password\n" +
		"DB_NAME=file-register\n" +
		"NODE_ID=test-node\n" +
		"OIDC_ISSUER=https://issuer.example.test\n" +
		"OIDC_AUDIENCE=register-api\n" +
		"VAULT_ADDR=https://vault.example.test\n" +
		"VAULT_TOKEN=test-token\n" +
		"VAULT_NID_ENCRYPTION_KEY=nid-encryption\n" +
		"VAULT_NID_HMAC_KEY=nid-hmac\n"
	if err := os.WriteFile(envFile, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	t.Setenv(EnvAppName, "process-app")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.AppName != "process-app" {
		t.Errorf("AppName = %q, want process-app from process environment", cfg.AppName)
	}
	if cfg.AppEnv != "test" || cfg.LogLevel != slog.LevelDebug {
		t.Errorf("file config = %q/%v, want test/debug", cfg.AppEnv, cfg.LogLevel)
	}
	wantDatabase := Database{
		Host:     "db.internal",
		Port:     3307,
		User:     "file-user",
		Password: "file-password",
		Name:     "file-register",
		Charset:  DefaultDBCharset,
	}
	if cfg.Database != wantDatabase {
		t.Errorf("Database = %+v, want %+v", cfg.Database, wantDatabase)
	}
	if cfg.NodeID != "test-node" || cfg.OIDC.Issuer != "https://issuer.example.test" || cfg.OIDC.Audience != "register-api" {
		t.Errorf("identity config = %q/%+v, want test-node and test issuer/audience", cfg.NodeID, cfg.OIDC)
	}
	if cfg.Vault.Address != "https://vault.example.test" || cfg.Vault.Token != "test-token" ||
		cfg.Vault.TransitMount != DefaultVaultTransitMount || cfg.Vault.NIDEncryptionKey != "nid-encryption" ||
		cfg.Vault.NIDHMACKey != "nid-hmac" {
		t.Errorf("Vault = %+v, want configured test values", cfg.Vault)
	}
}

func validConfig() Config {
	return Config{
		AppName:  DefaultAppName,
		AppEnv:   DefaultAppEnv,
		LogLevel: slog.LevelInfo,
		Server: Server{
			Host:            DefaultServerHost,
			Port:            DefaultServerPort,
			ReadTimeout:     DefaultServerReadTimeout,
			WriteTimeout:    DefaultServerWriteTimeout,
			IdleTimeout:     DefaultServerIdleTimeout,
			ShutdownTimeout: DefaultShutdownTimeout,
		},
		Database: Database{
			Host:    DefaultDBHost,
			Port:    DefaultDBPort,
			User:    "test-user",
			Name:    "test-register",
			Charset: DefaultDBCharset,
		},
		NodeID: "test-node",
		OIDC: OIDC{
			Issuer:   "https://issuer.example.test",
			Audience: "register-api",
		},
		Vault: Vault{
			Address:          "https://vault.example.test",
			Token:            "test-token",
			TransitMount:     DefaultVaultTransitMount,
			NIDEncryptionKey: "nid-encryption",
			NIDHMACKey:       "nid-hmac",
		},
	}
}

func TestLoadDefaults(t *testing.T) {
	clearEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	want := validConfig()
	if cfg.AppName != want.AppName || cfg.AppEnv != want.AppEnv {
		t.Fatalf("app config = %q/%q, want %q/%q", cfg.AppName, cfg.AppEnv, want.AppName, want.AppEnv)
	}
	if cfg.LogLevel != want.LogLevel {
		t.Fatalf("LogLevel = %v, want %v", cfg.LogLevel, want.LogLevel)
	}
	if cfg.Server != want.Server {
		t.Fatalf("Server = %+v, want %+v", cfg.Server, want.Server)
	}
}

func TestLoadOverrides(t *testing.T) {
	clearEnv(t)
	t.Setenv(EnvAppName, "population-api")
	t.Setenv(EnvAppEnv, "test")
	t.Setenv(EnvLogLevel, "debug")
	t.Setenv(EnvServerHost, "127.0.0.2")
	t.Setenv(EnvServerPort, "9090")
	t.Setenv(EnvServerReadTimeout, "3s")
	t.Setenv(EnvServerWriteTimeout, "4s")
	t.Setenv(EnvServerIdleTimeout, "5s")
	t.Setenv(EnvServerShutdownTimeout, "6s")
	t.Setenv(EnvDBHost, "db.internal")
	t.Setenv(EnvDBPort, "3307")
	t.Setenv(EnvDBUser, "register-api")
	t.Setenv(EnvDBPassword, "secret")
	t.Setenv(EnvDBName, "population")
	t.Setenv(EnvDBCharset, "utf8mb4")
	t.Setenv(EnvNodeID, "node-test")
	t.Setenv(EnvOIDCIssuer, "https://issuer.example.test")
	t.Setenv(EnvOIDCAudience, "register-api")
	t.Setenv(EnvVaultAddr, "https://vault.internal")
	t.Setenv(EnvVaultToken, "test-token")
	t.Setenv(EnvVaultTransitMount, "custom-transit")
	t.Setenv(EnvVaultNIDEncryptionKey, "national-id-encryption")
	t.Setenv(EnvVaultNIDHMACKey, "national-id-hmac")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.AppName != "population-api" || cfg.AppEnv != "test" {
		t.Fatalf("app config = %q/%q", cfg.AppName, cfg.AppEnv)
	}
	if cfg.LogLevel != slog.LevelDebug {
		t.Fatalf("LogLevel = %v, want debug", cfg.LogLevel)
	}
	want := Server{
		Host:            "127.0.0.2",
		Port:            9090,
		ReadTimeout:     3 * time.Second,
		WriteTimeout:    4 * time.Second,
		IdleTimeout:     5 * time.Second,
		ShutdownTimeout: 6 * time.Second,
	}
	if cfg.Server != want {
		t.Fatalf("Server = %+v, want %+v", cfg.Server, want)
	}
	wantDatabase := Database{
		Host:     "db.internal",
		Port:     3307,
		User:     "register-api",
		Password: "secret",
		Name:     "population",
		Charset:  "utf8mb4",
	}
	if cfg.Database != wantDatabase {
		t.Fatalf("Database = %+v, want %+v", cfg.Database, wantDatabase)
	}
	if cfg.NodeID != "node-test" || cfg.OIDC != (OIDC{Issuer: "https://issuer.example.test", Audience: "register-api"}) {
		t.Fatalf("identity config = %q/%+v", cfg.NodeID, cfg.OIDC)
	}
	wantVault := Vault{
		Address:          "https://vault.internal",
		Token:            "test-token",
		TransitMount:     "custom-transit",
		NIDEncryptionKey: "national-id-encryption",
		NIDHMACKey:       "national-id-hmac",
	}
	if cfg.Vault != wantVault {
		t.Fatalf("Vault = %+v, want %+v", cfg.Vault, wantVault)
	}
}

func TestLoadRejectsMalformedValues(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
	}{
		{name: "non-numeric port", key: EnvServerPort, value: "http"},
		{name: "malformed duration", key: EnvServerReadTimeout, value: "ten"},
		{name: "unknown log level", key: EnvLogLevel, value: "verbose"},
		{name: "zero port", key: EnvServerPort, value: "0"},
		{name: "malformed database port", key: EnvDBPort, value: "mysql"},
		{name: "zero database port", key: EnvDBPort, value: "0"},
		{name: "missing database user", key: EnvDBUser, value: ""},
		{name: "missing database name", key: EnvDBName, value: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearEnv(t)
			t.Setenv(tt.key, tt.value)

			if _, err := Load(); err == nil {
				t.Fatal("Load() succeeded, want an error")
			}
		})
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr bool
	}{
		{name: "valid"},
		{name: "empty app name", mutate: func(c *Config) { c.AppName = "  " }, wantErr: true},
		{name: "empty environment", mutate: func(c *Config) { c.AppEnv = "" }, wantErr: true},
		{name: "port too low", mutate: func(c *Config) { c.Server.Port = 0 }, wantErr: true},
		{name: "port too high", mutate: func(c *Config) { c.Server.Port = 70000 }, wantErr: true},
		{name: "non-loopback server host", mutate: func(c *Config) { c.Server.Host = "0.0.0.0" }, wantErr: true},
		{name: "zero read timeout", mutate: func(c *Config) { c.Server.ReadTimeout = 0 }, wantErr: true},
		{name: "negative write timeout", mutate: func(c *Config) { c.Server.WriteTimeout = -time.Second }, wantErr: true},
		{name: "zero idle timeout", mutate: func(c *Config) { c.Server.IdleTimeout = 0 }, wantErr: true},
		{name: "zero shutdown timeout", mutate: func(c *Config) { c.Server.ShutdownTimeout = 0 }, wantErr: true},
		{name: "empty database host", mutate: func(c *Config) { c.Database.Host = "" }, wantErr: true},
		{name: "invalid database port", mutate: func(c *Config) { c.Database.Port = 65536 }, wantErr: true},
		{name: "empty database user", mutate: func(c *Config) { c.Database.User = "" }, wantErr: true},
		{name: "empty database name", mutate: func(c *Config) { c.Database.Name = "" }, wantErr: true},
		{name: "empty database charset", mutate: func(c *Config) { c.Database.Charset = "" }, wantErr: true},
		{name: "empty node id", mutate: func(c *Config) { c.NodeID = "" }, wantErr: true},
		{name: "node id too long", mutate: func(c *Config) { c.NodeID = "node-id-that-is-too-long-for-the-column" }, wantErr: true},
		{name: "empty OIDC issuer", mutate: func(c *Config) { c.OIDC.Issuer = "" }, wantErr: true},
		{name: "empty OIDC audience", mutate: func(c *Config) { c.OIDC.Audience = "" }, wantErr: true},
		{name: "empty Vault address", mutate: func(c *Config) { c.Vault.Address = "" }, wantErr: true},
		{name: "empty Vault token", mutate: func(c *Config) { c.Vault.Token = "" }, wantErr: true},
		{name: "empty Vault transit mount", mutate: func(c *Config) { c.Vault.TransitMount = "" }, wantErr: true},
		{name: "empty Vault encryption key", mutate: func(c *Config) { c.Vault.NIDEncryptionKey = "" }, wantErr: true},
		{name: "empty Vault HMAC key", mutate: func(c *Config) { c.Vault.NIDHMACKey = "" }, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			if tt.mutate != nil {
				tt.mutate(&cfg)
			}
			err := cfg.Validate()
			if tt.wantErr && err == nil {
				t.Fatal("Validate() = nil, want an error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("Validate() = %v, want nil", err)
			}
		})
	}
}

func TestServerAddr(t *testing.T) {
	s := Server{Host: "127.0.0.1", Port: 8080}
	if got := s.Addr(); got != "127.0.0.1:8080" {
		t.Fatalf("Addr() = %q, want %q", got, "127.0.0.1:8080")
	}
}
