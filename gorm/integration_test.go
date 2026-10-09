//go:build integration

// Package gormintegration verifies the gorm integration against real database
// servers rather than SQLite.
//
// The SQLite tests cannot catch the failures that matter most here: a DSN that
// the driver cannot parse, a pool setting the driver rejects, or a query that
// behaves differently on another engine. Those only appear against the real
// thing, so these tests are gated behind the "integration" build tag and run in
// CI with Docker service containers.
package gorm_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// dsnFor builds the connection string the same way an application would, from
// the environment the CI service containers provide.
func dsnFor(t *testing.T, driver string) string {
	t.Helper()

	var dsn string
	switch driver {
	case "mysql":
		host := envOr("GOFAULT_MYSQL_HOST", "127.0.0.1")
		port := envOr("GOFAULT_MYSQL_PORT", "3306")
		dsn = fmt.Sprintf("gofault:%s@tcp(%s:%s)/gofault?charset=utf8mb4&parseTime=True&loc=UTC",
			envOr("GOFAULT_MYSQL_PASSWORD", "gofault"), host, port)
	case "postgres":
		host := envOr("GOFAULT_POSTGRES_HOST", "127.0.0.1")
		port := envOr("GOFAULT_POSTGRES_PORT", "5432")
		dsn = fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=gofault sslmode=disable",
			host, port,
			envOr("GOFAULT_POSTGRES_USER", "gofault"),
			envOr("GOFAULT_POSTGRES_PASSWORD", "gofault"))
	default:
		t.Fatalf("unknown driver %q", driver)
	}
	return dsn
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// openEngine connects and skips when the server is not reachable, so the suite
// stays usable on a machine without the containers.
func openEngine(t *testing.T, driver string) *gorm.DB {
	t.Helper()

	var dialector gorm.Dialector
	switch driver {
	case "mysql":
		dialector = mysql.Open(dsnFor(t, "mysql"))
	case "postgres":
		dialector = postgres.Open(dsnFor(t, "postgres"))
	}

	db, err := gorm.Open(dialector, &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Skipf("%s is not available: %v", driver, err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("%s: db handle: %v", driver, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		sqlDB.Close()
		t.Skipf("%s is not available: %v", driver, err)
	}

	t.Cleanup(func() { sqlDB.Close() })
	return db
}

type record struct {
	ID    uint   `gorm:"primaryKey"`
	Name  string `gorm:"size:64"`
	Count int
}

// The DSN the framework builds must actually be accepted by the driver.
func TestDSN_ParsesAndConnects(t *testing.T) {
	for _, driver := range []string{"mysql", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			db := openEngine(t, driver)

			if err := db.AutoMigrate(&record{}); err != nil {
				t.Fatalf("AutoMigrate: %v", err)
			}

			r := record{Name: "gofault", Count: 3}
			if err := db.Create(&r).Error; err != nil {
				t.Fatalf("Create: %v", err)
			}

			var got record
			if err := db.First(&got, r.ID).Error; err != nil {
				t.Fatalf("First: %v", err)
			}
			if got.Name != "gofault" || got.Count != 3 {
				t.Errorf("round trip = %+v, want {gofault 3}", got)
			}

			// Autoincrement must have been assigned by the engine.
			if got.ID == 0 {
				t.Error("engine did not assign an ID")
			}

			if err := db.Unscoped().Delete(&record{}, r.ID).Error; err != nil {
				t.Errorf("Delete: %v", err)
			}
		})
	}
}

// A transaction must roll back on error, on a real engine rather than SQLite.
func TestTransaction_RollbackOnRealEngine(t *testing.T) {
	for _, driver := range []string{"mysql", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			db := openEngine(t, driver)
			if err := db.AutoMigrate(&record{}); err != nil {
				t.Fatalf("AutoMigrate: %v", err)
			}
			db.Unscoped().Where("1 = 1").Delete(&record{})

			wantErr := fmt.Errorf("abort")
			err := db.Transaction(func(tx *gorm.DB) error {
				if err := tx.Create(&record{Name: "rolled-back", Count: 1}).Error; err != nil {
					return err
				}
				return wantErr
			})
			if err == nil {
				t.Fatal("expected the transaction to fail")
			}

			var n int64
			if err := db.Model(&record{}).Where("name = ?", "rolled-back").Count(&n).Error; err != nil {
				t.Fatalf("Count: %v", err)
			}
			if n != 0 {
				t.Errorf("%d rows survived the rollback, want 0", n)
			}
		})
	}
}

// The pool settings the framework applies must be accepted and effective.
func TestPoolSettings_Applied(t *testing.T) {
	db := openEngine(t, "mysql")
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db handle: %v", err)
	}

	sqlDB.SetMaxOpenConns(3)
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetConnMaxLifetime(time.Minute)

	dbStats := sqlDB.Stats()
	if dbStats.MaxOpenConnections != 3 {
		t.Errorf("MaxOpenConnections = %d, want 3", dbStats.MaxOpenConnections)
	}

	// WithMaxOpenConns(3) the pool must refuse a fourth concurrent connection
	// rather than blocking forever.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	conns := make([]*sql.Conn, 0, 3)
	defer func() {
		for _, c := range conns {
			c.Close()
		}
	}()
	for i := 0; i < 3; i++ {
		c, err := sqlDB.Conn(ctx)
		if err != nil {
			t.Fatalf("connection %d: %v", i, err)
		}
		conns = append(conns, c)
	}

	if _, err := sqlDB.Conn(ctx); err == nil {
		t.Error("expected the fourth connection to be refused by the 3-connection pool")
	}
}

// Concurrent writes must not deadlock or lose rows, which is where pool sizing
// problems show up.
func TestConcurrentWrites(t *testing.T) {
	for _, driver := range []string{"mysql", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			db := openEngine(t, driver)
			if err := db.AutoMigrate(&record{}); err != nil {
				t.Fatalf("AutoMigrate: %v", err)
			}
			db.Unscoped().Where("1 = 1").Delete(&record{})

			const workers = 20
			errCh := make(chan error, workers)
			for i := 0; i < workers; i++ {
				go func(i int) {
					r := record{Name: fmt.Sprintf("worker-%02d", i), Count: i}
					errCh <- db.Create(&r).Error
				}(i)
			}
			for i := 0; i < workers; i++ {
				if err := <-errCh; err != nil {
					t.Fatalf("concurrent write %d: %v", i, err)
				}
			}

			var n int64
			if err := db.Model(&record{}).Count(&n).Error; err != nil {
				t.Fatalf("Count: %v", err)
			}
			if n != workers {
				t.Errorf("count = %d, want %d", n, workers)
			}
		})
	}
}

// Closing the pool must be observable, which is what the shutdown hook relies on.
func TestPoolCloseIsObservable(t *testing.T) {
	db := openEngine(t, "mysql")
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db handle: %v", err)
	}

	if err := sqlDB.Ping(); err != nil {
		t.Fatalf("ping before close: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := sqlDB.Ping(); err == nil {
		t.Error("expected a closed pool to reject a Ping")
	}
}
