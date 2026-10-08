// Package middleware provides common HTTP middleware for gofault.
package middleware

import (
	"bytes"
	"io"
	"log/slog"
	"time"

	"github.com/gofault/gofault/core"
)

// RequestLoggerConfig holds configuration for the request logger middleware.
type RequestLoggerConfig struct {
	// Enabled enables the request logger.
	Enabled bool
	// LogBody enables logging of request and response bodies.
	LogBody bool
	// LogHeaders enables logging of request headers.
	LogHeaders bool
}

// DefaultRequestLoggerConfig returns a default request logger configuration.
func DefaultRequestLoggerConfig() RequestLoggerConfig {
	return RequestLoggerConfig{
		Enabled:    true,
		LogBody:    false,
		LogHeaders: false,
	}
}

// RequestLoggerMiddleware creates a middleware that logs HTTP requests and responses.
func RequestLoggerMiddleware(config RequestLoggerConfig) core.MiddlewareFunc {
	if !config.Enabled {
		return nil
	}

	return func(ctx *core.Ctx, next core.Handler) error {
		start := time.Now()

		// Log request
		logData := map[string]any{
			"method":  ctx.Request.Method,
			"path":    ctx.Request.URL.Path,
			"query":   ctx.Request.URL.RawQuery,
			"ip":      ctx.Request.RemoteAddr,
			"started": start.Format(time.RFC3339),
		}

		if config.LogHeaders {
			logData["headers"] = formatHeaders(ctx.Request.Header)
		}

		if config.LogBody {
			logData["body"] = readRequestBody(ctx)
		}

		// Call next handler
		err := next(ctx)

		// Log response
		duration := time.Since(start)
		logData["duration_ms"] = duration.Milliseconds()
		logData["status"] = ctx.StatusCode

		if config.LogBody {
			logData["response_body"] = "response captured"
		}

		// Emit the log entry. Previously this built logData and discarded it,
		// so the middleware produced no output at all.
		attrs := make([]any, 0, len(logData)*2)
		for k, v := range logData {
			attrs = append(attrs, k, v)
		}
		if err != nil {
			slog.Default().Error("request failed", attrs...)
		} else {
			slog.Default().Info("request", attrs...)
		}

		return err
	}
}

// formatHeaders formats HTTP headers as a map.
func formatHeaders(headers map[string][]string) map[string]string {
	result := make(map[string]string)
	for k, v := range headers {
		if len(v) > 0 {
			result[k] = v[0]
		}
	}
	return result
}

// readRequestBody reads the request body and restores it so downstream
// handlers can read it again. Returns an empty string when LogBody is off or
// the body is empty.
func readRequestBody(ctx *core.Ctx) string {
	if ctx.Request.Body == nil {
		return ""
	}

	body, err := io.ReadAll(ctx.Request.Body)
	if err != nil {
		return ""
	}

	// Restore the body for downstream handlers.
	ctx.Request.Body = io.NopCloser(bytes.NewBuffer(body))

	return string(body)
}
