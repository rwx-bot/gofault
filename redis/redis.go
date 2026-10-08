// Package redis provides Redis integration for the gofault framework.
package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gofault/gofault/core"
	"github.com/gofault/gofault/middleware"
	"github.com/redis/go-redis/v9"
)

// Config holds Redis configuration.
type Config struct {
	// Addr is the Redis address (host:port).
	Addr string
	// Password for Redis AUTH (empty = no auth).
	Password string
	// DB is the database number (0-15).
	DB int
	// PoolSize sets the maximum number of open connections.
	PoolSize int
	// MinIdleConns sets the minimum idle connections.
	MinIdleConns int
	// DialTimeout sets the connection timeout.
	DialTimeout int // in seconds
	// ReadTimeout sets the read timeout.
	ReadTimeout int // in seconds
	// WriteTimeout sets the write timeout.
	WriteTimeout int // in seconds
	// ConnMaxLifetime sets the maximum lifetime of a connection.
	ConnMaxLifetime int // in seconds, 0 = no limit
}

// DefaultConfig returns a default Redis configuration.
func DefaultConfig() Config {
	return Config{
		Addr:         "localhost:6379",
		Password:     "",
		DB:           0,
		PoolSize:     100,
		MinIdleConns: 10,
		DialTimeout:  5,
		ReadTimeout:  3,
		WriteTimeout: 3,
	}
}

// Client is the Redis module.
type Client struct {
	core.Module
	RDB    *redis.Client
	Config Config
}

// NewClient creates a new Redis client module.
func NewClient(name string, config Config) (*Client, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:         config.Addr,
		Password:     config.Password,
		DB:           config.DB,
		PoolSize:     config.PoolSize,
		MinIdleConns: config.MinIdleConns,
		DialTimeout:  time.Duration(config.DialTimeout) * time.Second,
		ReadTimeout:  time.Duration(config.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(config.WriteTimeout) * time.Second,
		ConnMaxLifetime: func() time.Duration {
			if config.ConnMaxLifetime == 0 {
				return 0
			}
			return time.Duration(config.ConnMaxLifetime) * time.Second
		}(),
	})

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		// The client opened a pool before the probe; without closing it a
		// failed NewClient leaks the connection and its goroutines.
		rdb.Close()
		return nil, fmt.Errorf("redis ping: %w", err)
	}

	c := &Client{
		Module: *core.NewModule(name),
		RDB:    rdb,
		Config: config,
	}

	c.RegisterOnShutdown(&redisShutdown{rdb})

	return c, nil
}

// MustNewClient creates a new Redis client and panics on error.
func MustNewClient(name string, config Config) *Client {
	c, err := NewClient(name, config)
	if err != nil {
		panic(err)
	}
	return c
}

// GetClient returns the underlying redis.Client.
func (c *Client) GetClient() *redis.Client {
	return c.RDB
}

// Set stores a key-value pair with an optional expiration.
func Set(ctx context.Context, rdb *redis.Client, key string, value interface{}, expiration time.Duration) error {
	return rdb.Set(ctx, key, value, expiration).Err()
}

// Get retrieves a value by key.
func Get(ctx context.Context, rdb *redis.Client, key string) (string, error) {
	return rdb.Get(ctx, key).Result()
}

// Delete removes a key.
func Delete(ctx context.Context, rdb *redis.Client, keys ...string) error {
	return rdb.Del(ctx, keys...).Err()
}

// Exists checks if a key exists.
func Exists(ctx context.Context, rdb *redis.Client, key string) (bool, error) {
	n, err := rdb.Exists(ctx, key).Result()
	return n > 0, err
}

// Lock represents a distributed lock.
type Lock struct {
	rdb     *redis.Client
	key     string
	value   string
	timeout time.Duration
}

// NewLock creates a new distributed lock.
func NewLock(rdb *redis.Client, key, value string, timeout time.Duration) *Lock {
	return &Lock{rdb: rdb, key: key, value: value, timeout: timeout}
}

// Acquire attempts to acquire the lock.
func (l *Lock) Acquire(ctx context.Context) (bool, error) {
	ok, err := l.rdb.SetNX(ctx, l.key, l.value, l.timeout).Result()
	return ok, err
}

// Release releases the lock if the value matches.
func (l *Lock) Release(ctx context.Context) error {
	// Use Lua script to ensure atomic check-and-delete
	script := redis.NewScript(`
		if redis.call("GET", KEYS[1]) == ARGV[1] then
			return redis.call("DEL", KEYS[1])
		else
			return 0
		end
	`)
	_, err := script.Run(ctx, l.rdb, []string{l.key}, l.value).Result()
	return err
}

type redisShutdown struct {
	rdb *redis.Client
}

func (s *redisShutdown) OnShutdown() error {
	return s.rdb.Close()
}

// Cache is a namespaced Redis key/value store with a default TTL. To serve as
// the backend for middleware.CacheMiddleware, wrap it with NewHTTPBackend.
type Cache struct {
	RDB        *redis.Client
	KeyPrefix  string
	DefaultTTL time.Duration
}

// NewCache creates a new Redis cache backend.
func NewCache(rdb *redis.Client, keyPrefix string, defaultTTL time.Duration) *Cache {
	return &Cache{
		RDB:        rdb,
		KeyPrefix:  keyPrefix,
		DefaultTTL: defaultTTL,
	}
}

// Get retrieves a cached value.
func (c *Cache) Get(ctx context.Context, key string) (string, error) {
	return c.RDB.Get(ctx, c.prefix(key)).Result()
}

// Set stores a value with optional TTL (uses default if 0).
func (c *Cache) Set(ctx context.Context, key string, value string, ttl time.Duration) error {
	if ttl == 0 {
		ttl = c.DefaultTTL
	}
	return c.RDB.Set(ctx, c.prefix(key), value, ttl).Err()
}

// Delete removes a cached value.
func (c *Cache) Delete(ctx context.Context, key string) error {
	return c.RDB.Del(ctx, c.prefix(key)).Err()
}

// Exists checks if a key exists.
func (c *Cache) Exists(ctx context.Context, key string) (bool, error) {
	n, err := c.RDB.Exists(ctx, c.prefix(key)).Result()
	return n > 0, err
}

func (c *Cache) prefix(key string) string {
	return c.KeyPrefix + ":" + key
}

// httpCacheEntry is the payload an HTTPBackend stores: a cached response is
// the body plus the status and headers needed to replay it.
type httpCacheEntry struct {
	Value      []byte            `json:"value"`
	StatusCode int               `json:"status_code"`
	Headers    map[string]string `json:"headers"`
}

// HTTPBackend adapts Cache to middleware.CacheBackend so several application
// instances can share one cache. It satisfies the interface structurally; the
// assertion below is a compile-time guard against signature drift.
type HTTPBackend struct {
	cache  *Cache
	ctx    context.Context
	prefix string
}

// NewHTTPBackend wraps a Cache so it can be passed to middleware.CacheMiddleware.
// ctx bounds every operation, since the CacheBackend interface takes no context.
func NewHTTPBackend(c *Cache, ctx context.Context) *HTTPBackend {
	return &HTTPBackend{cache: c, ctx: ctx}
}

var _ middleware.CacheBackend = (*HTTPBackend)(nil)

// Get implements middleware.CacheBackend. A missing key is a miss, not an error;
// any other Redis failure is also reported as a miss so a cache outage degrades
// to serving the request rather than failing it.
func (h *HTTPBackend) Get(key string) ([]byte, int, map[string]string, bool) {
	raw, err := h.cache.Get(h.ctx, h.prefix+key)
	if err != nil {
		return nil, 0, nil, false
	}

	var entry httpCacheEntry
	if err := json.Unmarshal([]byte(raw), &entry); err != nil {
		// A value written by an older or unrelated client: treat it as a miss
		// instead of replaying garbage.
		return nil, 0, nil, false
	}
	return entry.Value, entry.StatusCode, entry.Headers, true
}

// Set implements middleware.CacheBackend.
func (h *HTTPBackend) Set(key string, value []byte, statusCode int, headers map[string]string) {
	payload, err := json.Marshal(httpCacheEntry{
		Value:      value,
		StatusCode: statusCode,
		Headers:    headers,
	})
	if err != nil {
		return
	}
	// Ignore the error: failing to populate a cache must not fail the request
	// that was already served correctly.
	_ = h.cache.Set(h.ctx, h.prefix+key, string(payload), 0)
}

// Delete implements middleware.CacheBackend.
func (h *HTTPBackend) Delete(key string) {
	_ = h.cache.Delete(h.ctx, h.prefix+key)
}
