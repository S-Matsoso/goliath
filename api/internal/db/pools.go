package db

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	mysqlDriver "github.com/go-sql-driver/mysql"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

var requiredDepartments = []string{"identity", "health", "electoral", "grants", "police"}

type Pools struct {
	byDepartment map[string]*gorm.DB
	Audit        *gorm.DB
	Authz        *gorm.DB
	all          []*gorm.DB
}

func OpenPools(departmentDSNs map[string]string, auditDSN, authzDSN string) (*Pools, error) {
	if err := validatePoolDSNs(departmentDSNs, auditDSN, authzDSN); err != nil {
		return nil, err
	}

	pools := &Pools{byDepartment: make(map[string]*gorm.DB, len(requiredDepartments))}
	open := func(name, dsn string) (*gorm.DB, error) {
		database, err := openDSN(dsn)
		if err != nil {
			return nil, fmt.Errorf("open %s database pool: %w", name, err)
		}
		pools.all = append(pools.all, database)
		return database, nil
	}
	closeOnError := func(err error) (*Pools, error) {
		return nil, errors.Join(err, pools.Close())
	}

	for _, department := range requiredDepartments {
		database, err := open(department, departmentDSNs[department])
		if err != nil {
			return closeOnError(err)
		}
		pools.byDepartment[department] = database
	}
	var err error
	if pools.Audit, err = open("audit", auditDSN); err != nil {
		return closeOnError(err)
	}
	if pools.Authz, err = open("authorization", authzDSN); err != nil {
		return closeOnError(err)
	}
	return pools, nil
}

func (p *Pools) For(department string) (*gorm.DB, error) {
	database, ok := p.byDepartment[department]
	if !ok {
		return nil, fmt.Errorf("unknown department database %q", department)
	}
	return database, nil
}

func (p *Pools) Close() error {
	var closeErrors []error
	for _, database := range p.all {
		sqlDB, err := database.DB()
		if err == nil {
			err = sqlDB.Close()
		}
		if err != nil {
			closeErrors = append(closeErrors, err)
		}
	}
	return errors.Join(closeErrors...)
}

func validatePoolDSNs(departmentDSNs map[string]string, auditDSN, authzDSN string) error {
	users := make(map[string]string, len(requiredDepartments)+2)
	accounts := make([]struct{ name, dsn string }, 0, len(requiredDepartments)+2)

	for _, department := range requiredDepartments {
		if strings.TrimSpace(departmentDSNs[department]) == "" {
			return fmt.Errorf("DB_DSN_%s is required", strings.ToUpper(department))
		}
		accounts = append(accounts, struct{ name, dsn string }{"department " + department, departmentDSNs[department]})
	}
	if strings.TrimSpace(auditDSN) == "" {
		return errors.New("DB_DSN_AUDIT is required")
	}
	accounts = append(accounts, struct{ name, dsn string }{"audit", auditDSN})
	if strings.TrimSpace(authzDSN) == "" {
		return errors.New("DB_DSN_AUTHZ is required")
	}
	accounts = append(accounts, struct{ name, dsn string }{"authorization", authzDSN})

	var schema string
	for _, account := range accounts {
		name, dsn := account.name, account.dsn
		parsed, err := mysqlDriver.ParseDSN(dsn)
		if err != nil {
			return fmt.Errorf("invalid %s database DSN", name)
		}
		if parsed.User == "" {
			return fmt.Errorf("%s database DSN must name a dedicated account", name)
		}
		if prior, exists := users[parsed.User]; exists {
			return fmt.Errorf("%s and %s database pools share an account", prior, name)
		}
		users[parsed.User] = name
		if schema == "" {
			schema = parsed.DBName
		} else if schema != parsed.DBName {
			return fmt.Errorf("%s database pool targets a different schema", name)
		}
	}
	return nil
}

func openDSN(dsn string) (*gorm.DB, error) {
	if err := validateDSNTransport(dsn); err != nil {
		return nil, err
	}
	database, err := gorm.Open(gormmysql.Open(dsn), &gorm.Config{
		TranslateError: true,
		Logger: gormlogger.New(log.New(os.Stderr, "gorm ", log.LstdFlags), gormlogger.Config{
			SlowThreshold:             time.Second,
			LogLevel:                  gormlogger.Error,
			IgnoreRecordNotFoundError: true,
			ParameterizedQueries:      true,
		}),
	})
	if err != nil {
		return nil, fmt.Errorf("connect database: %w", err)
	}
	sqlDB, err := database.DB()
	if err != nil {
		return nil, fmt.Errorf("get SQL pool: %w", err)
	}
	sqlDB.SetMaxOpenConns(30)
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetConnMaxLifetime(10 * time.Minute)
	return database, nil
}

func validateDSNTransport(dsn string) error {
	parsed, err := mysqlDriver.ParseDSN(dsn)
	if err != nil {
		return errors.New("invalid database DSN")
	}
	if parsed.Net == "tcp" && parsed.TLSConfig == "" {
		return errors.New("department database DSNs must enable TLS")
	}
	return nil
}

func DSNUser(dsn string) (string, error) {
	parsed, err := mysqlDriver.ParseDSN(dsn)
	if err != nil {
		return "", errors.New("invalid database DSN")
	}
	return parsed.User, nil
}

