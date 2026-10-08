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
}

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
}

type tokenBucket struct {
	tokens     float64
	lastUpdate time.Time
}

// NewRateLimiter creates a new rate limiter.
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

	rl := &RateLimiter{
		config:  cfg,
		buckets: make(map[string]*tokenBucket),
	}

	// Start cleanup goroutine
	go rl.cleanup()

	return rl
}

// Allow checks if a request should be allowed.
func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	bucket, exists := rl.buckets[key]

	if !exists {
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

// cleanup removes stale buckets periodically.
func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
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
