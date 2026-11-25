package db

import (
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"time"

	"github.com/S-Matsoso/goliath/internal/config"
	mysqldriver "github.com/go-sql-driver/mysql"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// Open establishes a MySQL connection pool from the validated configuration.
func Open(cfg config.Database) (*gorm.DB, error) {
	dsn := (&mysqldriver.Config{
		User:      cfg.User,
		Passwd:    cfg.Password,
		Net:       "tcp",
		Addr:      net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port)),
		DBName:    cfg.Name,
		Params:    map[string]string{"charset": cfg.Charset},
		ParseTime: true,
		Loc:       time.UTC,
	}).FormatDSN()

	database, err := gorm.Open(gormmysql.Open(dsn), &gorm.Config{
		Logger: gormlogger.New(log.New(os.Stderr, "gorm ", log.LstdFlags), gormlogger.Config{
			SlowThreshold:             time.Second,
			LogLevel:                  gormlogger.Error,
			IgnoreRecordNotFoundError: true,
			ParameterizedQueries:      true,
		}),
	})
	if err != nil {
		return nil, fmt.Errorf("open MySQL connection: %w", err)
	}

	sqlDB, err := database.DB()
	if err != nil {
		return nil, fmt.Errorf("get MySQL connection pool: %w", err)
	}
	sqlDB.SetMaxOpenConns(30)
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetConnMaxLifetime(10 * time.Minute)

	return database, nil
}

// AutoMigrate creates or updates the register tables described by the models.
func AutoMigrate(database *gorm.DB) error {
	if err := database.AutoMigrate(migrationModels()...); err != nil {
		return fmt.Errorf("migrate register schema: %w", err)
	}
	return nil
}

func migrationModels() []any {
	return []any{
		&Person{},
		&Department{},
		&Official{},
		&Address{},
		&Birth{},
		&Death{},
		&Marriage{},
		&BiometricTemplate{},
		&AuditLog{},
		&EventOutbox{},
		&EventSubscription{},
		&EventDelivery{},
		&BackupLease{},
		&BackupRun{},
	}
}

// Close releases the underlying SQL connection pool.
func Close(database *gorm.DB) error {
	sqlDB, err := database.DB()
	if err != nil {
		return fmt.Errorf("get MySQL connection pool: %w", err)
	}
	if err := sqlDB.Close(); err != nil {
		return fmt.Errorf("close MySQL connection pool: %w", err)
	}
	return nil
}
