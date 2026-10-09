package gorm

import (
	"fmt"
	"testing"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.MaxOpenConns != 100 {
		t.Errorf("expected MaxOpenConns=100, got %d", cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns != 10 {
		t.Errorf("expected MaxIdleConns=10, got %d", cfg.MaxIdleConns)
	}
	if cfg.ConnMaxLifetime != 3600 {
		t.Errorf("expected ConnMaxLifetime=3600, got %d", cfg.ConnMaxLifetime)
	}
	if cfg.LogLevel != logger.Warn {
		t.Errorf("expected LogLevel=Warn, got %v", cfg.LogLevel)
	}
}

func TestNewDatabase_SQLite(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Dialect = DialectSQLite
	cfg.DSN = ":memory:"
	cfg.Silent = true

	db, err := NewDatabase("test-sqlite", cfg)
	if err != nil {
		t.Fatalf("NewDatabase failed: %v", err)
	}
	if db == nil {
		t.Fatal("db is nil")
	}
	if db.DB == nil {
		t.Fatal("db.DB is nil")
	}
	if db.Module.Name != "test-sqlite" {
		t.Errorf("expected name 'test-sqlite', got '%s'", db.Module.Name)
	}

	// Verify connection works
	sqlDB, err := db.DB.DB()
	if err != nil {
		t.Fatalf("get underlying db: %v", err)
	}
	if err := sqlDB.Ping(); err != nil {
		t.Fatalf("ping failed: %v", err)
	}
}

func TestMustNewDatabase_Panic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for invalid dialect")
		}
	}()

	cfg := DefaultConfig()
	cfg.Dialect = "invalid"
	MustNewDatabase("test", cfg)
}

func TestDatabase_GetDB(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Dialect = DialectSQLite
	cfg.DSN = ":memory:"
	cfg.Silent = true

	db, err := NewDatabase("test", cfg)
	if err != nil {
		t.Fatal(err)
	}

	gormDB := db.GetDB()
	if gormDB != db.DB {
		t.Error("GetDB should return the same DB instance")
	}
}

func TestTransaction_Commit(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Dialect = DialectSQLite
	cfg.DSN = ":memory:"
	cfg.Silent = true

	db, err := NewDatabase("test", cfg)
	if err != nil {
		t.Fatal(err)
	}

	type User struct {
		ID   uint
		Name string
	}

	// Auto-migrate
	if err := db.DB.AutoMigrate(&User{}); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	// Test commit
	err = Transaction(db.DB, func(tx *gorm.DB) error {
		return tx.Create(&User{Name: "alice"}).Error
	})
	if err != nil {
		t.Fatalf("Transaction commit failed: %v", err)
	}

	var count int64
	db.DB.Model(&User{}).Count(&count)
	if count != 1 {
		t.Errorf("expected 1 row, got %d", count)
	}
}

func TestTransaction_Rollback(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Dialect = DialectSQLite
	cfg.DSN = ":memory:"
	cfg.Silent = true

	db, err := NewDatabase("test", cfg)
	if err != nil {
		t.Fatal(err)
	}

	type User struct {
		ID   uint
		Name string
	}

	if err := db.DB.AutoMigrate(&User{}); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	// Create a row outside transaction
	db.DB.Create(&User{Name: "existing"})

	// Force rollback by panicking inside Transaction
	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Error("expected panic")
			}
		}()
		_ = Transaction(db.DB, func(tx *gorm.DB) error {
			tx.Create(&User{Name: "rolled-back-user"})
			panic("force rollback")
		})
	}()

	// Only "existing" row should remain — transaction was rolled back
	var count int64
	db.DB.Model(&User{}).Count(&count)
	if count != 1 {
		t.Errorf("expected 1 row (rolled-back-user should be rolled back), got %d", count)
	}
}

func TestTransaction_RollbackOnError(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Dialect = DialectSQLite
	cfg.DSN = ":memory:"
	cfg.Silent = true

	db, err := NewDatabase("test", cfg)
	if err != nil {
		t.Fatal(err)
	}

	type User struct {
		ID   uint
		Name string
	}

	if err := db.DB.AutoMigrate(&User{}); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	err = Transaction(db.DB, func(tx *gorm.DB) error {
		tx.Create(&User{Name: "alice"})
		return fmt.Errorf("intentional error")
	})
	if err == nil {
		t.Fatal("expected error")
	}

	var count int64
	db.DB.Model(&User{}).Count(&count)
	if count != 0 {
		t.Errorf("expected 0 rows after rollback, got %d", count)
	}
}

func TestOpenDB_InvalidDialect(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Dialect = "oracle"
	cfg.DSN = ""

	_, err := NewDatabase("test", cfg)
	if err == nil {
		t.Fatal("expected error for invalid dialect")
	}
}

func TestDialect_Constants(t *testing.T) {
	if DialectMySQL != "mysql" {
		t.Errorf("expected 'mysql', got '%s'", DialectMySQL)
	}
	if DialectPostgres != "postgres" {
		t.Errorf("expected 'postgres', got '%s'", DialectPostgres)
	}
	if DialectSQLite != "sqlite" {
		t.Errorf("expected 'sqlite', got '%s'", DialectSQLite)
	}
}

// Silent was documented as suppressing all GORM logs but was never read, so
// LogLevel alone decided the verbosity.
func TestOpenDB_SilentSuppressesLogging(t *testing.T) {
	base := DefaultConfig()
	base.Dialect = DialectSQLite
	base.DSN = ":memory:"

	silentCfg := base
	silentCfg.Silent = true
	silent, err := openDB(silentCfg)
	if err != nil {
		t.Fatalf("openDB silent: %v", err)
	}
	defer func() {
		if sqlDB, err := silent.DB(); err == nil {
			sqlDB.Close()
		}
	}()

	if silent.Config.Logger != logger.Discard {
		t.Errorf("with Silent=true the logger is %T, want logger.Discard", silent.Config.Logger)
	}

	// The negative case keeps the test honest: without Silent the logger must
	// still be the real one, otherwise the assertion above proves nothing.
	noisy, err := openDB(base)
	if err != nil {
		t.Fatalf("openDB: %v", err)
	}
	defer func() {
		if sqlDB, err := noisy.DB(); err == nil {
			sqlDB.Close()
		}
	}()

	if noisy.Config.Logger == logger.Discard {
		t.Error("without Silent the logger was discarded too")
	}
	if noisy.Config.Logger == silent.Config.Logger {
		t.Error("Silent had no effect: both loggers are identical")
	}
}

// The shutdown hook closes the pool. It had no coverage, so a broken hook would
// leak the connection pool for the life of the process.
func TestDatabase_ShutdownHookClosesPool(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Dialect = DialectSQLite
	cfg.DSN = ":memory:"
	cfg.Silent = true

	d, err := NewDatabase("shutdown", cfg)
	if err != nil {
		t.Fatalf("NewDatabase: %v", err)
	}

	sqlDB, err := d.DB.DB()
	if err != nil {
		t.Fatalf("db handle: %v", err)
	}

	hook := &gormShutdown{d.DB}
	if err := hook.OnShutdown(); err != nil {
		t.Fatalf("OnShutdown: %v", err)
	}

	if err := sqlDB.Ping(); err == nil {
		t.Error("expected the closed pool to reject a Ping")
	}
}

// NewDatabase registers the shutdown hook; calling it twice must not panic,
// since App.Stop can run more than once.
func TestDatabase_ShutdownHookTwice(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Dialect = DialectSQLite
	cfg.DSN = ":memory:"
	cfg.Silent = true

	d, err := NewDatabase("shutdown-twice", cfg)
	if err != nil {
		t.Fatalf("NewDatabase: %v", err)
	}

	hook := &gormShutdown{d.DB}
	if err := hook.OnShutdown(); err != nil {
		t.Fatalf("first OnShutdown: %v", err)
	}
	// The second call must return rather than panic.
	_ = hook.OnShutdown()
}

func TestMustNewDatabase_PanicsOnBadDSN(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected MustNewDatabase to panic on an unusable DSN")
		}
	}()

	cfg := DefaultConfig()
	cfg.Dialect = DialectMySQL
	// Port 1 is reserved and never listening, so the open must fail.
	cfg.DSN = "user:pass@tcp(127.0.0.1:1)/db?timeout=1s"

	_ = MustNewDatabase("bad", cfg)
}
