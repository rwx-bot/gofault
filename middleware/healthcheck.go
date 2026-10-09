package middleware

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gofault/gofault/core"
)

// HealthStatus represents the health status of a component.
type HealthStatus string

const (
	HealthStatusUp   HealthStatus = "up"
	HealthStatusDown HealthStatus = "down"
)

// HealthCheckResponse represents the health check response.
type HealthCheckResponse struct {
	Status    HealthStatus           `json:"status"`
	Timestamp string                 `json:"timestamp"`
	Checks    map[string]HealthCheck `json:"checks,omitempty"`
}

// HealthCheck represents a single health check.
type HealthCheck struct {
	Status  HealthStatus `json:"status"`
	Message string       `json:"message,omitempty"`
	Latency string       `json:"latency,omitempty"`
}

// HealthChecker is a function that performs a health check.
type HealthChecker func() HealthCheck

// HealthCheckConfig holds configuration for the health check middleware.
//
// Checks is guarded by a mutex: the handler reads it on every scrape while
// RegisterHealthCheck may be called at runtime, and a concurrent map read and
// write panics. The mutex lives behind a pointer so copying the config by value
// stays safe, which the existing call sites do.
type HealthCheckConfig struct {
	// Path is the URL path for the health check endpoint.
	Path string
	// Checks is a map of health checkers. Assign to it directly only before
	// the handler starts serving; use RegisterHealthCheck afterwards.
	Checks map[string]HealthChecker
	// IncludeTimestamp controls whether to include timestamp in response.
	IncludeTimestamp bool
	// IncludeChecks controls whether to include individual check details.
	IncludeChecks bool

	mu *sync.RWMutex
}

// lock returns the config mutex, creating it on first use.
func (c *HealthCheckConfig) lock() *sync.RWMutex {
	if c.mu == nil {
		c.mu = &sync.RWMutex{}
	}
	return c.mu
}

// snapshot returns the checkers, read under the lock.
func (c *HealthCheckConfig) snapshot() map[string]HealthChecker {
	mu := c.lock()
	mu.RLock()
	defer mu.RUnlock()
	if len(c.Checks) == 0 {
		return nil
	}
	out := make(map[string]HealthChecker, len(c.Checks))
	for name, check := range c.Checks {
		out[name] = check
	}
	return out
}

// RegisterHealthCheck registers a new health check.
func (c *HealthCheckConfig) RegisterHealthCheck(name string, checker HealthChecker) {
	if checker == nil {
		return
	}

	mu := c.lock()
	mu.Lock()
	defer mu.Unlock()
	if c.Checks == nil {
		c.Checks = make(map[string]HealthChecker)
	}
	c.Checks[name] = checker
}

// DefaultHealthCheckConfig returns a default health check configuration.
func DefaultHealthCheckConfig() HealthCheckConfig {
	return HealthCheckConfig{
		Path:             "/health",
		Checks:           make(map[string]HealthChecker),
		IncludeTimestamp: true,
		IncludeChecks:    true,
	}
}

// HealthCheckHandler creates the health check endpoint handler.
//
// It returns a handler rather than a middleware: the health endpoint is a
// terminal route, so RegisterHealthCheck wires the path for you.
func HealthCheckHandler(config HealthCheckConfig) core.Handler {
	if config.Path == "" {
		config.Path = "/health"
	}

	return func(ctx *core.Ctx) error {
		response := HealthCheckResponse{
			Status: HealthStatusUp,
		}

		if config.IncludeTimestamp {
			response.Timestamp = time.Now().UTC().Format(time.RFC3339)
		}

		checks := config.snapshot()
		if config.IncludeChecks && len(checks) > 0 {
			response.Checks = make(map[string]HealthCheck, len(checks))
			for name, check := range checks {
				// A panicking checker must not take down the health endpoint;
				// report it as down and keep the other results.
				result := runChecker(check)
				response.Checks[name] = result
				if result.Status == HealthStatusDown {
					response.Status = HealthStatusDown
				}
			}
		}

		statusCode := http.StatusOK
		if response.Status == HealthStatusDown {
			statusCode = http.StatusServiceUnavailable
		}

		ctx.Response.Header().Set("Content-Type", "application/json")
		ctx.Response.WriteHeader(statusCode)

		return json.NewEncoder(ctx.Response).Encode(response)
	}
}

// runChecker invokes a checker, converting a panic into a down result.
func runChecker(check HealthChecker) (result HealthCheck) {
	defer func() {
		if r := recover(); r != nil {
			result = HealthCheck{
				Status:  HealthStatusDown,
				Message: fmt.Sprintf("health check panicked: %v", r),
			}
		}
	}()
	if check == nil {
		return HealthCheck{
			Status:  HealthStatusDown,
			Message: "health check is nil",
		}
	}
	return check()
}

// RegisterHealthCheckEndpoint registers the health route on the router and
// returns the handler.
func RegisterHealthCheckEndpoint(rtr HealthCheckRegistrar, config HealthCheckConfig) core.Handler {
	handler := HealthCheckHandler(config)
	if handler == nil || rtr == nil {
		return handler
	}
	if config.Path == "" {
		config.Path = "/health"
	}
	rtr.Handle(http.MethodGet, config.Path, handler)
	return handler
}

// HealthCheckRegistrar is the subset of the router that
// RegisterHealthCheckEndpoint needs.
type HealthCheckRegistrar interface {
	Handle(method, path string, handler core.Handler, mw ...core.MiddlewareFunc)
}

// HealthCheckMiddleware creates a health check endpoint.
//
// Deprecated: it returns a core.Handler, not a middleware. Use
// HealthCheckHandler, or RegisterHealthCheckEndpoint to wire the route.
func HealthCheckMiddleware(config HealthCheckConfig) core.Handler {
	return HealthCheckHandler(config)
}

// NewHealthCheckResponse creates a health check result.
//
// The name says Response but it has always returned a HealthCheck; it is kept
// as-is so existing callers compile. Use HealthCheckResponse for the envelope.
func NewHealthCheckResponse(status HealthStatus, message string) HealthCheck {
	return HealthCheck{
		Status:  status,
		Message: message,
	}
}

// LatencyHealthCheck creates a health check that measures how long fn takes.
// The name is recorded on failure so a multi-check report says which dependency
// is down.
func LatencyHealthCheck(name string, fn func() error) HealthChecker {
	return func() HealthCheck {
		if fn == nil {
			return HealthCheck{
				Status:  HealthStatusDown,
				Message: fmt.Sprintf("%s: no check function provided", name),
			}
		}

		start := time.Now()
		err := fn()
		latency := time.Since(start)

		if err != nil {
			return HealthCheck{
				Status: HealthStatusDown,
				// The error text is kept verbatim; callers that need the check
				// name already have it from the response's Checks map key.
				Message: err.Error(),
				Latency: latency.String(),
			}
		}

		return HealthCheck{
			Status:  HealthStatusUp,
			Latency: latency.String(),
		}
	}
}
