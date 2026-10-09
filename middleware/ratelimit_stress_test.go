package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gofault/gofault/core"
)

// Serve many concurrent requests through a middleware stack and watch for
// goroutine growth.
func TestStack_NoGoroutineLeak(t *testing.T) {
	stack := []core.MiddlewareFunc{
		RequestID(),
		RequestLoggerMiddleware(RequestLoggerConfig{Enabled: true}),
		func(ctx *core.Ctx, next core.Handler) error { return next(ctx) },
	}

	run := func(n int) {
		var wg sync.WaitGroup
		for i := 0; i < n; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				h := core.Handler(func(ctx *core.Ctx) error {
					ctx.Response.WriteHeader(http.StatusOK)
					_, err := ctx.Response.Write([]byte(strings.Repeat("x", 512)))
					return err
				})
				for i := len(stack) - 1; i >= 0; i-- {
					fn := stack[i]
					inner := h
					h = func(ctx *core.Ctx) error { return fn(ctx, inner) }
				}
				req := httptest.NewRequest(http.MethodGet, "/x", nil)
				_ = h(core.NewCtx(httptest.NewRecorder(), req))
			}()
		}
		wg.Wait()
	}

	run(200)
	time.Sleep(200 * time.Millisecond)
	runtime.GC()
	before := runtime.NumGoroutine()

	for round := 0; round < 5; round++ {
		run(2000)
	}

	time.Sleep(500 * time.Millisecond)
	runtime.GC()
	after := runtime.NumGoroutine()

	t.Logf("goroutines: before=%d after=%d delta=%d", before, after, after-before)
	if after-before > 50 {
		t.Errorf("goroutine leak suspected: %d -> %d", before, after)
	}
}

// Close must actually stop the cleanup goroutine. Constructing one limiter per
// tenant without closing them leaked one goroutine each, for the life of the
// process.
func TestRateLimiter_CloseStopsCleanupGoroutine(t *testing.T) {
	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	runtime.GC()
	before := runtime.NumGoroutine()

	const n = 100
	limiters := make([]*RateLimiter, 0, n)
	for i := 0; i < n; i++ {
		limiters = append(limiters, NewRateLimiter(RateLimiterConfig{
			RequestsPerSecond: 1,
			BurstSize:         1,
		}))
	}

	// Without Close this is where the leak shows.
	for _, rl := range limiters {
		rl.Close()
	}

	time.Sleep(300 * time.Millisecond)
	runtime.GC()
	after := runtime.NumGoroutine()

	t.Logf("goroutines: before=%d after=%d delta=%d (constructed and closed %d limiters)",
		before, after, after-before, n)
	if after-before > 10 {
		t.Errorf("Close did not stop the cleanup goroutines: %d -> %d", before, after)
	}
}

// Close must be safe to call twice, since shutdown paths can overlap.
func TestRateLimiter_CloseIsIdempotent(t *testing.T) {
	rl := NewRateLimiter(RateLimiterConfig{RequestsPerSecond: 1, BurstSize: 1})
	rl.Close()
	rl.Close() // must not panic on a double close of the channel
}

// A client rotating keys must not be able to grow the map faster than the
// reaper can shrink it.
func TestRateLimiter_KeyMapIsBounded(t *testing.T) {
	const maxKeys = 100
	rl := NewRateLimiter(RateLimiterConfig{
		RequestsPerSecond: 1,
		BurstSize:         1,
		MaxKeys:           maxKeys,
		KeyFunc:           func(ctx *core.Ctx) string { return ctx.Request.URL.Query().Get("k") },
	})
	t.Cleanup(rl.Close)

	for i := 0; i < 5000; i++ {
		rl.Allow(fmt.Sprintf("client-%d", i))
	}

	got := rl.Len()
	t.Logf("buckets after 5000 distinct clients: %d (max %d)", got, maxKeys)
	if got > maxKeys {
		t.Errorf("key map holds %d entries, want at most %d", got, maxKeys)
	}
}

// The default cap must apply when MaxKeys is left at zero.
func TestRateLimiter_DefaultMaxKeys(t *testing.T) {
	rl := NewRateLimiter(RateLimiterConfig{RequestsPerSecond: 1, BurstSize: 1})
	t.Cleanup(rl.Close)

	if rl.config.MaxKeys != DefaultMaxRateLimitKeys {
		t.Errorf("MaxKeys = %d, want the default %d", rl.config.MaxKeys, DefaultMaxRateLimitKeys)
	}
}
