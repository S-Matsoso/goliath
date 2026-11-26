package db

import (
	"sync"
	"testing"

	"gorm.io/gorm/schema"
)

func TestMigrationModelsHaveExpectedTablesAndPrimaryKeys(t *testing.T) {
	wantTables := []string{
		"persons",
		"departments",
		"officials",
		"accounts",
		"account_sessions",
		"addresses",
		"births",
		"deaths",
		"marriages",
		"biometric_templates",
		"audit_log",
		"event_outbox",
		"event_subscriptions",
		"event_deliveries",
		"backup_leases",
		"backup_runs",
	}

	models := migrationModels()
	if len(models) != len(wantTables) {
		t.Fatalf("migration model count = %d, want %d", len(models), len(wantTables))
	}

	seen := make(map[string]bool, len(models))
	for i, model := range models {
		parsed, err := schema.Parse(model, &sync.Map{}, schema.NamingStrategy{})
		if err != nil {
			t.Fatalf("parse migration model %T: %v", model, err)
		}
		if parsed.Table != wantTables[i] {
			t.Errorf("table for %T = %q, want %q", model, parsed.Table, wantTables[i])
		}
		if seen[parsed.Table] {
			t.Errorf("duplicate migration table %q", parsed.Table)
		}
		seen[parsed.Table] = true
		if len(parsed.PrimaryFields) == 0 {
			t.Errorf("migration model %T has no primary key", model)
		}
	}
}
