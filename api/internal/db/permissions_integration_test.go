package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"testing"

	mysqlDriver "github.com/go-sql-driver/mysql"
)

func TestPhaseADatabaseAccountBoundaries(t *testing.T) {
	departments := []struct {
		code  string
		view  string
		procs []string
	}{
		{code: "identity"},
		{code: "health", view: "v_health_person", procs: []string{
			"CALL sp_register_birth(NULL,NULL,NULL,NULL,NULL,NULL,NULL,NULL,NULL,NULL,NULL,NULL,0,'test',NULL,0,NULL,NULL,NULL,NULL)",
			"CALL sp_register_death(NULL,NULL,NULL,NULL,NULL,0,'test',NULL,0,NULL,NULL,NULL,NULL)",
		}},
		{code: "electoral", view: "v_electoral_roll"},
		{code: "grants", view: "v_grants_person"},
		{code: "police", view: "v_police_identity", procs: []string{
			"CALL sp_submit_police_change_request('ID_LOST_STOLEN',NULL,NULL,0,'test',NULL,0,NULL,NULL,NULL,NULL,@request_id)",
		}},
	}
	dsns := make(map[string]string, len(departments))
	for _, department := range departments {
		dsns[department.code] = os.Getenv("DB_DSN_" + upper(department.code))
		if dsns[department.code] == "" {
			t.Skip("set all DB_DSN_* department accounts to run MariaDB permission integration tests")
		}
	}

	for _, department := range departments {
		t.Run(department.code, func(t *testing.T) {
			database, err := sql.Open("mysql", dsns[department.code])
			if err != nil {
				t.Fatal(err)
			}
			defer database.Close()
			ctx := context.Background()

			baseTables := []string{"persons", "addresses", "births", "deaths", "marriages", "biometric_templates"}
			for _, table := range baseTables {
				err := queryNoRows(ctx, database, "SELECT * FROM "+table+" LIMIT 0")
				if department.code == "identity" {
					if err != nil {
						t.Errorf("identity cannot read core table %s: %v", table, err)
					}
					continue
				}
				var mysqlErr *mysqlDriver.MySQLError
				if !errors.As(err, &mysqlErr) || mysqlErr.Number != 1142 {
					t.Errorf("SELECT on %s returned %v, want MySQL error 1142", table, err)
				}
			}

			if department.view != "" {
				if err := queryNoRows(ctx, database, "SELECT * FROM "+department.view+" LIMIT 0"); err != nil {
					t.Errorf("department view %s unavailable: %v", department.view, err)
				}
			}
			for _, statement := range department.procs {
				err := database.QueryRowContext(ctx, statement).Err()
				var mysqlErr *mysqlDriver.MySQLError
				if errors.As(err, &mysqlErr) && mysqlErr.Number == 1370 {
					t.Errorf("department procedure denied: %v", err)
				}
			}
		})
	}
}

func queryNoRows(ctx context.Context, database *sql.DB, statement string) error {
	rows, err := database.QueryContext(ctx, statement)
	if err != nil {
		return err
	}
	return rows.Close()
}

func upper(value string) string {
	out := []rune(value)
	for i, r := range out {
		if r >= 'a' && r <= 'z' {
			out[i] = r - ('a' - 'A')
		}
	}
	return fmt.Sprint(string(out))
}
