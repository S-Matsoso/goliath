// Package config loads and validates application configuration from environment
// variables. It is the only package in the application that reads the process
// environment.
package config

import (
	"fmt"
	"log/slog"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

// Environment variable names understood by Load.
const (
	EnvAppName               = "APP_NAME"
	EnvAppEnv                = "APP_ENV"
	EnvLogLevel              = "LOG_LEVEL"
	EnvServerHost            = "SERVER_HOST"
	EnvServerPort            = "SERVER_PORT"
	EnvServerReadTimeout     = "SERVER_READ_TIMEOUT"
	EnvServerWriteTimeout    = "SERVER_WRITE_TIMEOUT"
	EnvServerIdleTimeout     = "SERVER_IDLE_TIMEOUT"
	EnvServerShutdownTimeout = "SERVER_SHUTDOWN_TIMEOUT"
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
)

// Config is the complete, validated application configuration.
type Config struct {
	AppName  string
	AppEnv   string
	LogLevel slog.Level
	Server   Server
}

// Server groups the HTTP server settings.
type Server struct {
	Host            string
	Port            int
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	ShutdownTimeout time.Duration
}

// Addr returns the host:port address the HTTP server listens on.
func (s Server) Addr() string {
	return net.JoinHostPort(s.Host, strconv.Itoa(s.Port))
}

// Load reads configuration from the environment, applies development defaults
// for unset variables and validates the result.
func Load() (Config, error) {
	level, err := envLevel(EnvLogLevel, slog.LevelInfo)
	if err != nil {
		return Config{}, err
	}

	port, err := envInt(EnvServerPort, DefaultServerPort)
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

	cfg := Config{
		AppName:  envString(EnvAppName, DefaultAppName),
		AppEnv:   envString(EnvAppEnv, DefaultAppEnv),
		LogLevel: level,
		Server: Server{
			Host:            envString(EnvServerHost, DefaultServerHost),
			Port:            port,
			ReadTimeout:     readTimeout,
			WriteTimeout:    writeTimeout,
			IdleTimeout:     idleTimeout,
			ShutdownTimeout: shutdownTimeout,
		},
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
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
