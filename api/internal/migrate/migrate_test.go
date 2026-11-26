package migrate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSplitSQL(t *testing.T) {
	script := `CREATE TABLE one (id INT);
DELIMITER $$
CREATE PROCEDURE example()
BEGIN
    SELECT 1;
    SELECT 2;
END$$
DELIMITER ;
CREATE VIEW two AS SELECT 1;`

	got, err := splitSQL(script)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d statements, want 3", len(got))
	}
	if !strings.Contains(got[1], "SELECT 1;\n    SELECT 2;") {
		t.Fatalf("procedure body was split: %q", got[1])
	}
}

func TestSplitSQLRejectsUnterminatedStatement(t *testing.T) {
	if _, err := splitSQL("CREATE TABLE unfinished (id INT)"); err == nil {
		t.Fatal("expected an unterminated statement error")
	}
}

func TestRenderRoleBindingMigration(t *testing.T) {
	rendered, err := renderSQL("002_bind_department_roles.sql.tmpl",
		"GRANT role_identity TO {{.identity_account}};",
		map[string]string{"identity_account": "'svc_identity'@'10.%'"})
	if err != nil {
		t.Fatal(err)
	}
	if rendered != "GRANT role_identity TO 'svc_identity'@'10.%';" {
		t.Fatalf("rendered migration = %q", rendered)
	}
	if _, err := renderSQL("002_bind_department_roles.sql.tmpl", "{{.missing}}", nil); err == nil {
		t.Fatal("missing account binding was accepted")
	}
}

func TestPhaseAMigrationStatements(t *testing.T) {
	path := filepath.Join("..", "..", "..", "migrations", "001_phase_a.sql")
	script, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	statements, err := splitSQL(string(script))
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(statements, "\n")
	for _, required := range []string{
		"CREATE TABLE IF NOT EXISTS persons",
		"ALTER TABLE addresses ADD COLUMN ward_code VARCHAR(16) NULL",
		"CREATE OR REPLACE SQL SECURITY DEFINER VIEW v_electoral_roll",
		"CREATE OR REPLACE PROCEDURE sp_register_birth",
		"CREATE OR REPLACE PROCEDURE sp_register_death",
		"CREATE OR REPLACE PROCEDURE sp_submit_police_change_request",
		"CREATE OR REPLACE PROCEDURE sp_review_change_request",
		"GRANT SELECT ON goliath.v_electoral_roll TO role_electoral",
	} {
		if !strings.Contains(joined, required) {
			t.Errorf("migration is missing %q", required)
		}
	}
}
