package redis

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Addr != "localhost:6379" {
		t.Errorf("expected Addr=localhost:6379, got %s", cfg.Addr)
	}
	if cfg.PoolSize != 100 {
		t.Errorf("expected PoolSize=100, got %d", cfg.PoolSize)
	}
	if cfg.MinIdleConns != 10 {
		t.Errorf("expected MinIdleConns=10, got %d", cfg.MinIdleConns)
	}
}

func TestNewClient_Invalid(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Addr = "localhost:9999" // no redis running here

	_, err := NewClient("test", cfg)
	if err == nil {
		t.Error("expected error for unreachable Redis")
	}
}

func TestConfig_ZeroConnMaxLifetime(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ConnMaxLifetime = 0 // means no limit
	rdb := redis.NewClient(&redis.Options{
		Addr: cfg.Addr,
		ConnMaxLifetime: func() time.Duration {
			if cfg.ConnMaxLifetime == 0 {
				return 0
			}
			return time.Duration(cfg.ConnMaxLifetime) * time.Second
		}(),
	})
	if rdb == nil {
		t.Fatal("rdb is nil")
	}
	rdb.Close()
}

// redisAddr resolves the address of the Redis instance used by the integration
// tests below. It honours GOFAULT_REDIS_ADDR so CI can point the tests at a
// provisioned service, and otherwise falls back to the conventional local port.
func redisAddr() string {
	if addr := os.Getenv("GOFAULT_REDIS_ADDR"); addr != "" {
		return addr
	}
	return "127.0.0.1:6379"
}

// newTestClient dials Redis and skips the calling test when no server answers.
// The integration tests assert against real server semantics (Lua scripts,
// key expiry), so they need a live instance - but they must not turn a missing
// Redis into a failure on developer machines.
func newTestClient(t *testing.T) *redis.Client {
	t.Helper()

	addr := redisAddr()
	rdb := redis.NewClient(&redis.Options{Addr: addr})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		_ = rdb.Close()
		t.Skipf("redis is not available at %s: %v", addr, err)
	}

	t.Cleanup(func() { _ = rdb.Close() })
	return rdb
}

func TestLock_AcquireRelease(t *testing.T) {
	rdb := newTestClient(t)

	ctx := context.Background()
	lock := NewLock(rdb, "test-lock", "unique-value", 5*time.Second)

	ok, err := lock.Acquire(ctx)
	if err != nil {
		t.Fatalf("Acquire error: %v", err)
	}
	if !ok {
		t.Fatal("expected to acquire lock")
	}

	// Release
	err = lock.Release(ctx)
	if err != nil {
		t.Errorf("Release error: %v", err)
	}

	// Should be able to re-acquire after release
	ok, err = lock.Acquire(ctx)
	if err != nil {
		t.Fatalf("Acquire after release error: %v", err)
	}
	if !ok {
		t.Fatal("expected to re-acquire lock after release")
	}
	lock.Release(ctx)
}

func TestLock_ReleaseWrongOwner(t *testing.T) {
	rdb := newTestClient(t)

	ctx := context.Background()
	lock1 := NewLock(rdb, "test-lock2", "owner1", 5*time.Second)
	lock2 := NewLock(rdb, "test-lock2", "owner2", 5*time.Second)

	ok, err := lock1.Acquire(ctx)
	if err != nil {
		t.Fatalf("Acquire error: %v", err)
	}
	if !ok {
		t.Fatal("expected to acquire lock")
	}

	// lock2 should not be able to release lock1's lock
	err = lock2.Release(ctx)
	if err != nil {
		t.Errorf("Release error (expected nil for Lua script result): %v", err)
	}

	// lock1 should still be able to release
	err = lock1.Release(ctx)
	if err != nil {
		t.Errorf("Release error: %v", err)
	}
}

func TestCache_Operations(t *testing.T) {
	rdb := newTestClient(t)

	cache := NewCache(rdb, "testns", 10*time.Second)
	ctx := context.Background()

	// Set and Get
	err := cache.Set(ctx, "key1", "value1", 0)
	if err != nil {
		t.Fatalf("Set error: %v", err)
	}

	val, err := cache.Get(ctx, "key1")
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if val != "value1" {
		t.Errorf("expected 'value1', got '%s'", val)
	}

	// Exists
	exists, err := cache.Exists(ctx, "key1")
	if err != nil {
		t.Fatalf("Exists error: %v", err)
	}
	if !exists {
		t.Error("expected key1 to exist")
	}

	// Delete
	err = cache.Delete(ctx, "key1")
	if err != nil {
		t.Fatalf("Delete error: %v", err)
	}

	exists, err = cache.Exists(ctx, "key1")
	if err != nil {
		t.Fatalf("Exists after delete error: %v", err)
	}
	if exists {
		t.Error("expected key1 to not exist after delete")
	}
}

func TestCache_KeyPrefix(t *testing.T) {
	rdb := newTestClient(t)

	cache := NewCache(rdb, "myapp", 10*time.Second)
	ctx := context.Background()

	cache.Set(ctx, "user:1", "alice", 0)

	// Verify prefix is used
	val, err := cache.Get(ctx, "user:1")
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if val != "alice" {
		t.Errorf("expected 'alice', got '%s'", val)
	}
}

func TestCache_DefaultTTL(t *testing.T) {
	rdb := newTestClient(t)

	// 100ms TTL
	cache := NewCache(rdb, "ttltest", 100*time.Millisecond)
	ctx := context.Background()

	cache.Set(ctx, "short", "val", 0)
	cache.Set(ctx, "long", "val", 5*time.Second)

	// Wait 150ms
	time.Sleep(150 * time.Millisecond)

	// short should be expired
	_, err := cache.Get(ctx, "short")
	if err != redis.Nil {
		t.Error("expected short key to be expired")
	}

	// long should still exist
	_, err = cache.Get(ctx, "long")
	if err != nil {
		t.Error("expected long key to still exist")
	}
}

func TestSetGetDelete(t *testing.T) {
	rdb := newTestClient(t)

	ctx := context.Background()

	err := Set(ctx, rdb, "setkey", "setval", time.Second)
	if err != nil {
		t.Fatalf("Set error: %v", err)
	}

	val, err := Get(ctx, rdb, "setkey")
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if val != "setval" {
		t.Errorf("expected 'setval', got '%s'", val)
	}

	err = Delete(ctx, rdb, "setkey")
	if err != nil {
		t.Fatalf("Delete error: %v", err)
	}

	_, err = Get(ctx, rdb, "setkey")
	if err != redis.Nil {
		t.Error("expected redis.Nil after delete")
	}
}

func TestExists(t *testing.T) {
	rdb := newTestClient(t)

	ctx := context.Background()

	ok, err := Exists(ctx, rdb, "nonexistent-key")
	if err != nil {
		t.Fatalf("Exists error: %v", err)
	}
	if ok {
		t.Error("expected false for nonexistent key")
	}

	Set(ctx, rdb, "exists-key", "val", time.Second)
	ok, err = Exists(ctx, rdb, "exists-key")
	if err != nil {
		t.Fatalf("Exists error: %v", err)
	}
	if !ok {
		t.Error("expected true for existing key")
	}
}
