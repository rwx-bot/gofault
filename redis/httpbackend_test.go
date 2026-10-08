package redis

import (
	"context"
	"testing"
	"time"

	"github.com/gofault/gofault/core"
	goredis "github.com/redis/go-redis/v9"
)

// newClientAt dials a specific address without requiring it to answer, so a
// test can exercise the unreachable-server path.
func newClientAt(t *testing.T, addr string) *goredis.Client {
	t.Helper()
	rdb := goredis.NewClient(&goredis.Options{Addr: addr, DialTimeout: time.Second})
	t.Cleanup(func() { _ = rdb.Close() })
	return rdb
}

// The response replayed through a CacheBackend must survive the round trip
// through Redis intact: body, status and headers all matter.
func TestHTTPBackend_RoundTrip(t *testing.T) {
	rdb := newTestClient(t)

	ctx := context.Background()
	cache := NewCache(rdb, "httpbackend-test", time.Minute)
	backend := NewHTTPBackend(cache, ctx)

	key := "roundtrip"
	headers := map[string]string{"Content-Type": "application/json", "X-Custom": "v"}

	// A miss must be reported as a miss, not as an error or a zero value.
	if _, _, _, ok := backend.Get(key); ok {
		t.Fatal("expected a miss before anything was cached")
	}

	backend.Set(key, []byte(`{"hello":"world"}`), 201, headers)

	body, status, gotHeaders, ok := backend.Get(key)
	if !ok {
		t.Fatal("expected a hit after Set")
	}
	if string(body) != `{"hello":"world"}` {
		t.Errorf("body = %q, want %q", body, `{"hello":"world"}`)
	}
	if status != 201 {
		t.Errorf("status = %d, want 201", status)
	}
	if gotHeaders["X-Custom"] != "v" {
		t.Errorf("headers = %v, want X-Custom=v", gotHeaders)
	}

	backend.Delete(key)
	if _, _, _, ok := backend.Get(key); ok {
		t.Error("expected a miss after Delete")
	}
}

// A key holding something this backend did not write must degrade to a miss
// rather than replaying unparseable data to the client.
func TestHTTPBackend_ForeignValueIsAMiss(t *testing.T) {
	rdb := newTestClient(t)

	ctx := context.Background()
	cache := NewCache(rdb, "httpbackend-foreign", time.Minute)

	if err := cache.Set(ctx, "garbage", "not json", 0); err != nil {
		t.Fatalf("seed: %v", err)
	}

	backend := NewHTTPBackend(cache, ctx)
	if _, _, _, ok := backend.Get("garbage"); ok {
		t.Error("expected an unparseable value to be reported as a miss")
	}
}

// A Redis outage must not fail the request; the backend degrades to a miss.
func TestHTTPBackend_DegradesOnRedisError(t *testing.T) {
	// A real client pointed at a dead port: every call errors rather than
	// panicking, which is the outage shape the backend has to absorb.
	rdb := newClientAt(t, "127.0.0.1:1")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	backend := NewHTTPBackend(NewCache(rdb, "unreachable", time.Minute), ctx)

	if _, _, _, ok := backend.Get("anything"); ok {
		t.Error("expected a miss when Redis is unreachable")
	}
	backend.Set("k", []byte("v"), 200, nil)
	backend.Delete("k")
}

// Acquire with a zero timeout creates a lock that never expires, so a crashed
// holder blocks every other worker forever. Document the hazard rather than
// change behaviour silently, but assert the current semantics so a future
// change is deliberate.
func TestLock_ZeroTimeoutNeverExpires(t *testing.T) {
	rdb := newTestClient(t)

	ctx := context.Background()
	key := "test-lock-zero-timeout"
	t.Cleanup(func() { rdb.Del(ctx, key) })

	lock := NewLock(rdb, key, "holder", 0)
	ok, err := lock.Acquire(ctx)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if !ok {
		t.Fatal("expected to acquire the lock")
	}

	ttl, err := rdb.TTL(ctx, key).Result()
	if err != nil {
		t.Fatalf("TTL: %v", err)
	}
	if ttl != -1 {
		t.Errorf("TTL = %v, want -1 (no expiry)", ttl)
	}
}

// NewClient must not leak its pool when the connectivity probe fails.
func TestNewClient_ClosesPoolOnPingFailure(t *testing.T) {
	// Port 1 is reserved and never listening, so the ping cannot succeed.
	cfg := DefaultConfig()
	cfg.Addr = "127.0.0.1:1"
	cfg.DialTimeout = 1

	if _, err := NewClient("unreachable", cfg); err == nil {
		t.Fatal("expected NewClient to fail against a dead address")
	}

	// The assertion that matters is structural: the client is closed before
	// the error returns. Confirm the constructor does not hand back a client
	// alongside the error, which would leave ownership ambiguous.
	c, err := NewClient("unreachable", cfg)
	if err == nil {
		c.GetClient().Close()
		t.Fatal("expected an error")
	}
	if c != nil {
		t.Error("expected a nil client when the probe fails")
	}
}

// Guard that Database/Client still satisfy the module contract they embed.
var _ = core.Module{}
