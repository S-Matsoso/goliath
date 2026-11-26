package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	mysqlDriver "github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"

	"github.com/S-Matsoso/goliath/internal/migrate"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "migration failed:", err)
		os.Exit(1)
	}
}

func run() error {
	if err := godotenv.Load(".env"); err != nil {
		return fmt.Errorf("load environment file: %w", err)
	}
	dsn := os.Getenv("DB_DSN_MIGRATE")
	if dsn == "" {
		return fmt.Errorf("DB_DSN_MIGRATE is required")
	}
	directory := os.Getenv("MIGRATIONS_DIR")
	if directory == "" {
		directory = filepath.Join("..", "migrations")
	}
	accountHost := os.Getenv("DB_ACCOUNT_HOST")
	if accountHost == "" {
		accountHost = "10.%"
	}
	bindings, err := roleBindings(accountHost)
	if err != nil {
		return err
	}

	database, err := sql.Open("mysql", dsn)
	if err != nil {
		return fmt.Errorf("open migration database: %w", err)
	}
	defer database.Close()
	if err := migrate.Run(context.Background(), database, directory, bindings); err != nil {
		return err
	}
	fmt.Println("database migrations are current")
	return nil
}

func roleBindings(host string) (map[string]string, error) {
	environments := map[string]string{
		"identity_account":  "DB_DSN_IDENTITY",
		"health_account":    "DB_DSN_HEALTH",
		"electoral_account": "DB_DSN_ELECTORAL",
		"grants_account":    "DB_DSN_GRANTS",
		"police_account":    "DB_DSN_POLICE",
		"audit_account":     "DB_DSN_AUDIT",
		"authz_account":     "DB_DSN_AUTHZ",
	}
	bindings := make(map[string]string, len(environments))
	seen := make(map[string]string, len(environments))
	for key, envName := range environments {
		dsn := os.Getenv(envName)
		if dsn == "" {
			return nil, fmt.Errorf("%s is required to bind database roles", envName)
		}
		parsed, err := mysqlDriver.ParseDSN(dsn)
		if err != nil || parsed.User == "" {
			return nil, fmt.Errorf("%s must contain a valid database account", envName)
		}
		if prior, exists := seen[parsed.User]; exists {
			return nil, fmt.Errorf("%s and %s share a database account", prior, envName)
		}
		seen[parsed.User] = envName
		bindings[key] = sqlAccount(parsed.User, host)
	}
	return bindings, nil
}

func sqlAccount(user, host string) string {
	quote := func(value string) string {
		return "'" + strings.ReplaceAll(value, "'", "''") + "'"
	}
	return quote(user) + "@" + quote(host)
}
