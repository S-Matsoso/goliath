package migrate

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
)

func Run(ctx context.Context, database *sql.DB, directory string, bindings map[string]string) error {
	conn, err := database.Conn(ctx)
	if err != nil {
		return fmt.Errorf("reserve migration connection: %w", err)
	}
	defer conn.Close()

	var acquired int
	if err := conn.QueryRowContext(ctx, "SELECT GET_LOCK('goliath_schema_migrations', 30)").Scan(&acquired); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}
	if acquired != 1 {
		return fmt.Errorf("acquire migration lock: lock unavailable")
	}
	defer conn.ExecContext(context.Background(), "DO RELEASE_LOCK('goliath_schema_migrations')")

	if _, err := conn.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version VARCHAR(255) PRIMARY KEY,
		checksum CHAR(64) NOT NULL,
		applied_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
	) ENGINE=InnoDB`); err != nil {
		return fmt.Errorf("create migration ledger: %w", err)
	}

	entries, err := os.ReadDir(directory)
	if err != nil {
		return fmt.Errorf("read migration directory: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		if entry.IsDir() || !(strings.HasSuffix(entry.Name(), ".sql") || strings.HasSuffix(entry.Name(), ".sql.tmpl")) {
			continue
		}
		if err := applyFile(ctx, conn, directory, entry.Name(), bindings); err != nil {
			return err
		}
	}
	return nil
}

func applyFile(ctx context.Context, conn *sql.Conn, directory, name string, bindings map[string]string) error {
	path := filepath.Join(directory, name)
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read migration %q: %w", name, err)
	}
	sum := sha256.Sum256(content)
	checksum := hex.EncodeToString(sum[:])

	var recorded string
	err = conn.QueryRowContext(ctx, "SELECT checksum FROM schema_migrations WHERE version = ?", name).Scan(&recorded)
	if err == nil {
		if recorded != checksum {
			return fmt.Errorf("migration %q checksum differs from the applied version", name)
		}
		return nil
	}
	if err != sql.ErrNoRows {
		return fmt.Errorf("check migration %q: %w", name, err)
	}

	rendered, err := renderSQL(name, string(content), bindings)
	if err != nil {
		return fmt.Errorf("render migration %q: %w", name, err)
	}
	statements, err := splitSQL(rendered)
	if err != nil {
		return fmt.Errorf("parse migration %q: %w", name, err)
	}
	for i, statement := range statements {
		if _, err := conn.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("apply migration %q statement %d: %w", name, i+1, err)
		}
	}
	if _, err := conn.ExecContext(ctx, "INSERT INTO schema_migrations (version, checksum) VALUES (?, ?)", name, checksum); err != nil {
		return fmt.Errorf("record migration %q: %w", name, err)
	}
	return nil
}

func renderSQL(name, script string, bindings map[string]string) (string, error) {
	if !strings.HasSuffix(name, ".sql.tmpl") {
		return script, nil
	}
	tmpl, err := template.New(name).Option("missingkey=error").Parse(script)
	if err != nil {
		return "", err
	}
	var rendered bytes.Buffer
	if err := tmpl.Execute(&rendered, bindings); err != nil {
		return "", err
	}
	return rendered.String(), nil
}

func splitSQL(script string) ([]string, error) {
	delimiter := ";"
	var statements []string
	var pending strings.Builder

	for _, line := range strings.Split(script, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToUpper(trimmed), "DELIMITER ") {
			if strings.TrimSpace(pending.String()) != "" {
				return nil, fmt.Errorf("delimiter changed before previous statement ended")
			}
			delimiter = strings.TrimSpace(trimmed[len("DELIMITER "):])
			if delimiter == "" {
				return nil, fmt.Errorf("empty statement delimiter")
			}
			continue
		}

		pending.WriteString(line)
		pending.WriteByte('\n')
		current := strings.TrimSpace(pending.String())
		if !strings.HasSuffix(current, delimiter) {
			continue
		}
		statement := strings.TrimSpace(strings.TrimSuffix(current, delimiter))
		if statement != "" {
			statements = append(statements, statement)
		}
		pending.Reset()
	}

	if strings.TrimSpace(pending.String()) != "" {
		return nil, fs.ErrInvalid
	}
	return statements, nil
}
