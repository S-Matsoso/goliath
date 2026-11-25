package config

import (
	"log/slog"
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
}

func clearEnv(t *testing.T) {
	t.Helper()
	for _, key := range envKeys {
		t.Setenv(key, "")
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
	t.Setenv(EnvServerHost, "0.0.0.0")
	t.Setenv(EnvServerPort, "9090")
	t.Setenv(EnvServerReadTimeout, "3s")
	t.Setenv(EnvServerWriteTimeout, "4s")
	t.Setenv(EnvServerIdleTimeout, "5s")
	t.Setenv(EnvServerShutdownTimeout, "6s")

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
		Host:            "0.0.0.0",
		Port:            9090,
		ReadTimeout:     3 * time.Second,
		WriteTimeout:    4 * time.Second,
		IdleTimeout:     5 * time.Second,
		ShutdownTimeout: 6 * time.Second,
	}
	if cfg.Server != want {
		t.Fatalf("Server = %+v, want %+v", cfg.Server, want)
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
		{name: "zero read timeout", mutate: func(c *Config) { c.Server.ReadTimeout = 0 }, wantErr: true},
		{name: "negative write timeout", mutate: func(c *Config) { c.Server.WriteTimeout = -time.Second }, wantErr: true},
		{name: "zero idle timeout", mutate: func(c *Config) { c.Server.IdleTimeout = 0 }, wantErr: true},
		{name: "zero shutdown timeout", mutate: func(c *Config) { c.Server.ShutdownTimeout = 0 }, wantErr: true},
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
