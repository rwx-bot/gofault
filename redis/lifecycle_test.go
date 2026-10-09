package redis

import (
	"context"
	"testing"
	"time"
)

// GetClient exposes the underlying client. It had no coverage, so a nil field
// would have gone unnoticed.
func TestClient_GetClient(t *testing.T) {
	c := &Client{RDB: nil}
	if c.GetClient() != nil {
		t.Error("GetClient on a zero Client should return nil")
	}

	rdb := newTestClient(t)
	c2 := &Client{RDB: rdb}
	if c2.GetClient() != rdb {
		t.Error("GetClient did not return the underlying client")
	}
}

// MustNewClient panics on an unreachable server, and must not panic on a
// reachable one.
func TestMustNewClient_PanicsOnUnreachable(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Addr = "127.0.0.1:1"
	cfg.DialTimeout = 1

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected MustNewClient to panic on an unreachable server")
		}
	}()
	_ = MustNewClient("unreachable", cfg)
}

// The shutdown hook must actually close the pool. It is registered by
// NewClient and had no coverage, so a broken hook would leak connections for
// the life of the process.
func TestClient_ShutdownHookClosesPool(t *testing.T) {
	rdb := newTestClient(t)

	hook := &redisShutdown{rdb}

	if err := hook.OnShutdown(); err != nil {
		t.Fatalf("OnShutdown: %v", err)
	}

	// The pool must refuse further commands once closed.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err == nil {
		t.Error("expected the closed client to reject a Ping")
	}
}

// NewClient registers the shutdown hook; closing twice must be reported rather
// than panicking, since App.Stop may run more than once.
func TestRedisShutdown_Idempotent(t *testing.T) {
	rdb := newTestClient(t)
	hook := &redisShutdown{rdb}

	if err := hook.OnShutdown(); err != nil {
		t.Fatalf("first OnShutdown: %v", err)
	}
	// A second close returns an error from the driver; the important part is
	// that it does not panic.
	_ = hook.OnShutdown()
}

// Cache.Get on a missing key must surface redis.Nil so callers can distinguish
// a miss from a failure.
func TestCache_MissIsRedisNil(t *testing.T) {
	rdb := newTestClient(t)
	cache := NewCache(rdb, "miss-test", time.Minute)

	ctx := context.Background()
	if err := cache.Delete(ctx, "absent"); err != nil {
		t.Fatalf("cleanup: %v", err)
	}

	if _, err := cache.Get(ctx, "absent"); err == nil {
		t.Error("expected an error for a missing key")
	}

	// Exists is the non-error way to ask.
	ok, err := cache.Exists(ctx, "absent")
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if ok {
		t.Error("Exists reported true for a missing key")
	}
}

// prefix must namespace every key so two caches over the same Redis do not
// collide.
func TestCache_PrefixesKeys(t *testing.T) {
	rdb := newTestClient(t)
	cache := NewCache(rdb, "ns", time.Minute)
	ctx := context.Background()

	if err := cache.Set(ctx, "k", "v", 0); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// The unprefixed key must not exist.
	if n, err := rdb.Exists(ctx, "k").Result(); err != nil || n > 0 {
		t.Errorf("unprefixed key %q exists (n=%v, err=%v), keys must be namespaced", "k", n, err)
	}

	if got, err := cache.Get(ctx, "k"); err != nil || got != "v" {
		t.Errorf("Get = %q, %v; want v, nil", got, err)
	}
	if err := cache.Delete(ctx, "k"); err != nil {
		t.Errorf("Delete: %v", err)
	}
}

// A zero ttl argument must fall back to DefaultTTL rather than persisting
// forever. The window is a few seconds because Redis TTL has second resolution,
// so a sub-second default cannot be observed.
func TestCache_DefaultTTLApplied(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping TTL timing check in short mode")
	}

	rdb := newTestClient(t)
	const window = 30 * time.Second
	cache := NewCache(rdb, "ttl-test", window)
	ctx := context.Background()

	if err := cache.Set(ctx, "defaulted", "v", 0); err != nil {
		t.Fatalf("Set: %v", err)
	}
	ttl, err := rdb.TTL(ctx, cache.prefix("defaulted")).Result()
	if err != nil {
		t.Fatalf("TTL: %v", err)
	}
	if ttl <= 0 || ttl > window {
		t.Errorf("TTL = %v, want the %v default to be applied", ttl, window)
	}

	// An explicit ttl must win over the default.
	if err := cache.Set(ctx, "explicit", "v", window*2); err != nil {
		t.Fatalf("Set: %v", err)
	}
	ttl, err = rdb.TTL(ctx, cache.prefix("explicit")).Result()
	if err != nil {
		t.Fatalf("TTL: %v", err)
	}
	if ttl <= window {
		t.Errorf("TTL = %v, want the explicit 2x window to override the default", ttl)
	}
}
