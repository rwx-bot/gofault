package middleware

import (
	"strings"
	"sync"
	"time"

	"github.com/gofault/gofault/core"
	"github.com/gofault/gofault/exception"
)

// RateLimiterConfig holds rate limiter configuration.
type RateLimiterConfig struct {
	// RequestsPerSecond is the number of requests allowed per second per key.
	RequestsPerSecond float64
	// BurstSize is the maximum burst size (number of requests that can be made at once).
	BurstSize int
	// KeyFunc extracts the rate limit key from the request (e.g., IP address).
	KeyFunc func(*core.Ctx) string
	// MaxKeys caps how many distinct keys are tracked. A client that rotates
	// keys (a spoofed X-Forwarded-For, say) would otherwise grow the map
	// without bound, since timed-out buckets are only reaped on a timer.
	// 0 uses DefaultMaxRateLimitKeys.
	MaxKeys int
}

// DefaultMaxRateLimitKeys bounds per-key tracking so a rotating-key client
// cannot exhaust memory faster than the cleanup ticker can reap it.
const DefaultMaxRateLimitKeys = 10000

// DefaultKeyFunc returns the client IP as the rate limit key.
// X-Forwarded-For may contain a comma-separated proxy chain; the first
// element is the client's real address. Using the whole header would let a
// client rotate the chain to get a fresh bucket per request and bypass the
// limiter.
func DefaultKeyFunc(ctx *core.Ctx) string {
	ip := ctx.Request.Header.Get("X-Forwarded-For")
	if ip != "" {
		if i := strings.IndexByte(ip, ','); i >= 0 {
			ip = strings.TrimSpace(ip[:i])
		}
		return ip
	}
	if ip = ctx.Request.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	return ctx.Request.RemoteAddr
}

// RateLimiter implements a token bucket rate limiter.
type RateLimiter struct {
	config  RateLimiterConfig
	buckets map[string]*tokenBucket
	mu      sync.RWMutex

	// stopOnce guards Close so it is safe to call more than once, which matters
	// because App.Stop may run alongside an explicit shutdown.
	stopOnce sync.Once
	stopped  chan struct{}
}

type tokenBucket struct {
	tokens     float64
	lastUpdate time.Time
}

// NewRateLimiter creates a new rate limiter.
//
// The limiter starts a background goroutine to reap idle buckets. Call Close
// when the limiter is no longer needed; without it the goroutine runs for the
// life of the process, so constructing one per tenant leaks one goroutine each
// time.
func NewRateLimiter(cfg RateLimiterConfig) *RateLimiter {
	if cfg.KeyFunc == nil {
		cfg.KeyFunc = DefaultKeyFunc
	}
	if cfg.BurstSize == 0 {
		cfg.BurstSize = 10
	}
	if cfg.RequestsPerSecond == 0 {
		cfg.RequestsPerSecond = 100
	}
	if cfg.MaxKeys <= 0 {
		cfg.MaxKeys = DefaultMaxRateLimitKeys
	}

	rl := &RateLimiter{
		config:  cfg,
		buckets: make(map[string]*tokenBucket),
		stopped: make(chan struct{}),
	}

	go rl.cleanup()

	return rl
}

// Close stops the background cleanup goroutine. It is safe to call more than
// once.
func (rl *RateLimiter) Close() {
	rl.stopOnce.Do(func() { close(rl.stopped) })
}

// Allow checks if a request should be allowed.
func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	bucket, exists := rl.buckets[key]

	if !exists {
		// Keep the map bounded: a client rotating keys must not be able to grow
		// it faster than the reaper can shrink it.
		if len(rl.buckets) >= rl.config.MaxKeys {
			rl.evictOldestLocked(now)
		}
		bucket = &tokenBucket{
			tokens:     float64(rl.config.BurstSize) - 1,
			lastUpdate: now,
		}
		rl.buckets[key] = bucket
		return true
	}

	// Calculate tokens to add based on elapsed time
	elapsed := now.Sub(bucket.lastUpdate).Seconds()
	bucket.tokens += elapsed * rl.config.RequestsPerSecond
	if bucket.tokens > float64(rl.config.BurstSize) {
		bucket.tokens = float64(rl.config.BurstSize)
	}
	bucket.lastUpdate = now

	if bucket.tokens >= 1 {
		bucket.tokens--
		return true
	}

	return false
}

// evictOldestLocked drops the least recently used bucket. The caller holds
// rl.mu. Evicting one keeps the map at MaxKeys without needing a full sort.
func (rl *RateLimiter) evictOldestLocked(now time.Time) {
	var oldestKey string
	var oldest time.Time

	for k, b := range rl.buckets {
		if oldestKey == "" || b.lastUpdate.Before(oldest) {
			oldestKey, oldest = k, b.lastUpdate
		}
	}
	if oldestKey == "" {
		return
	}

	delete(rl.buckets, oldestKey)
	_ = now
}

// Middleware creates a rate limiting middleware.
func (rl *RateLimiter) Middleware() core.MiddlewareFunc {
	return func(ctx *core.Ctx, next core.Handler) error {
		key := rl.config.KeyFunc(ctx)

		if !rl.Allow(key) {
			return exception.TooManyRequests("rate limit exceeded")
		}

		return next(ctx)
	}
}

// Len returns the number of tracked keys. Exposed for tests and metrics.
func (rl *RateLimiter) Len() int {
	rl.mu.RLock()
	defer rl.mu.RUnlock()
	return len(rl.buckets)
}

// cleanup removes stale buckets periodically until Close is called.
func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-rl.stopped:
			return
		case <-ticker.C:
			rl.mu.Lock()
			now := time.Now()
			for key, bucket := range rl.buckets {
				// Remove buckets that haven't been used for 10 minutes
				if now.Sub(bucket.lastUpdate) > 10*time.Minute {
					delete(rl.buckets, key)
				}
			}
			rl.mu.Unlock()
		}
	}
}
