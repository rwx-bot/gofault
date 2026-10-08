// Package gorm provides GORM database integration for the gofault framework.
package gorm

import (
	"fmt"
	"time"

	"github.com/gofault/gofault/core"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Dialect supported database drivers.
type Dialect string

const (
	DialectMySQL    Dialect = "mysql"
	DialectPostgres Dialect = "postgres"
	DialectSQLite   Dialect = "sqlite"
)

// Config holds database configuration.
type Config struct {
	// Dialect is the database driver (mysql, postgres, sqlite).
	Dialect Dialect
	// DSN is the data source name (connection string).
	DSN string
	// MaxOpenConns sets the maximum number of open connections.
	MaxOpenConns int
	// MaxIdleConns sets the maximum number of idle connections.
	MaxIdleConns int
	// ConnMaxLifetime sets the maximum lifetime of a connection.
	ConnMaxLifetime int // in seconds
	// LogLevel sets GORM log level (default: logger.Warn).
	LogLevel logger.LogLevel
	// Silent suppresses all GORM logs when true.
	Silent bool
}

// DefaultConfig returns a default configuration.
func DefaultConfig() Config {
	return Config{
		MaxOpenConns:    100,
		MaxIdleConns:    10,
		ConnMaxLifetime: 3600,
		LogLevel:        logger.Warn,
		Silent:          false,
	}
}

// Database is the core database module.
type Database struct {
	core.Module
	DB     *gorm.DB
	Config Config
}

// NewDatabase creates a new Database module.
func NewDatabase(name string, config Config) (*Database, error) {
	db, err := openDB(config)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	d := &Database{
		Module: *core.NewModule(name),
		DB:     db,
		Config: config,
	}

	d.RegisterOnShutdown(&gormShutdown{db})

	return d, nil
}

// MustNewDatabase creates a new Database module and panics on error.
func MustNewDatabase(name string, config Config) *Database {
	d, err := NewDatabase(name, config)
	if err != nil {
		panic(err)
	}
	return d
}

// GetDB returns the underlying GORM DB instance.
func (d *Database) GetDB() *gorm.DB {
	return d.DB
}

func openDB(config Config) (*gorm.DB, error) {
	gormConfig := &gorm.Config{
		Logger: logger.Default.LogMode(config.LogLevel),
	}
	if config.Silent {
		// Silent was documented as suppressing all GORM logs but was never
		// consulted, so LogLevel alone decided the verbosity. Discard is used
		// rather than LogMode(Silent) so nothing is emitted at all.
		gormConfig.Logger = logger.Discard
	}

	var dialector gorm.Dialector
	switch config.Dialect {
	case DialectMySQL:
		dialector = mysql.Open(config.DSN)
	case DialectPostgres:
		dialector = postgres.Open(config.DSN)
	case DialectSQLite:
		dialector = sqlite.Open(config.DSN)
	default:
		return nil, fmt.Errorf("unsupported dialect: %s", config.Dialect)
	}

	db, err := gorm.Open(dialector, gormConfig)
	if err != nil {
		return nil, fmt.Errorf("gorm open: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("get underlying db: %w", err)
	}

	if config.MaxOpenConns > 0 {
		sqlDB.SetMaxOpenConns(config.MaxOpenConns)
	}
	if config.MaxIdleConns > 0 {
		sqlDB.SetMaxIdleConns(config.MaxIdleConns)
	}
	if config.ConnMaxLifetime > 0 {
		sqlDB.SetConnMaxLifetime(time.Duration(config.ConnMaxLifetime) * time.Second)
	}

	return db, nil
}

type gormShutdown struct {
	db *gorm.DB
}

func (s *gormShutdown) OnShutdown() error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// Transaction executes fn within a database transaction.
// If fn returns an error, the transaction is rolled back.
// Otherwise, the transaction is committed.
func Transaction(db *gorm.DB, fn func(tx *gorm.DB) error) error {
	tx := db.Begin()
	if tx.Error != nil {
		return fmt.Errorf("begin transaction: %w", tx.Error)
	}

	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		}
	}()

	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback().Error; rbErr != nil {
			return fmt.Errorf("rollback: %s, original error: %w", rbErr, err)
		}
		return err
	}

	if err := tx.Commit().Error; err != nil {
		// Roll back on a failed commit too. database/sql requires it: a
		// transaction whose Commit fails is still open, and the connection
		// stays checked out of the pool until it is rolled back or the pool
		// reclaims it. Note this is not covered by a test -- the sqlite
		// driver used here reclaims the connection on its own, so the two
		// paths cannot be distinguished from the outside.
		if rbErr := tx.Rollback().Error; rbErr != nil {
			return fmt.Errorf("commit: %s, rollback: %w", err, rbErr)
		}
		return fmt.Errorf("commit: %w", err)
	}

	return nil
}
