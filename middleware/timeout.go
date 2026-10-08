package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/gofault/gofault/core"
)

// TimeoutConfig holds configuration for the timeout middleware.
type TimeoutConfig struct {
	// Enabled enables the timeout middleware.
	Enabled bool
	// Duration is the timeout duration.
	Duration time.Duration
	// ErrorMessage is the message to return on timeout.
	ErrorMessage string
}

// DefaultTimeoutConfig returns a default timeout configuration.
func DefaultTimeoutConfig() TimeoutConfig {
	return TimeoutConfig{
		Enabled:      true,
		Duration:     30 * time.Second,
		ErrorMessage: "Request timeout",
	}
}

// timeoutBody is the payload returned when a handler exceeds its deadline.
type timeoutBody struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

// bufferedResponse is an http.ResponseWriter that accumulates a handler's output
// in memory instead of touching the real ResponseWriter.
//
// TimeoutMiddleware runs the handler on its own goroutine, so a handler that
// outlives the deadline is still running when the timeout response is written.
// If both goroutines wrote to the same ResponseWriter they would race on the
// status line (and net/http would log "superfluous WriteHeader call"). Buffering
// keeps the real writer owned by exactly one goroutine.
type bufferedResponse struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func newBufferedResponse() *bufferedResponse {
	return &bufferedResponse{header: make(http.Header)}
}

func (b *bufferedResponse) Header() http.Header {
	return b.header
}

func (b *bufferedResponse) WriteHeader(status int) {
	if b.status == 0 {
		b.status = status
	}
}

func (b *bufferedResponse) Write(p []byte) (int, error) {
	if b.status == 0 {
		b.status = http.StatusOK
	}
	return b.body.Write(p)
}

// flushTo copies the buffered response onto the real ResponseWriter. It must be
// called from a single goroutine once the handler has returned.
func (b *bufferedResponse) flushTo(w http.ResponseWriter) {
	for name, values := range b.header {
		for _, value := range values {
			w.Header().Add(name, value)
		}
	}
	if b.status != 0 {
		w.WriteHeader(b.status)
	}
	if b.body.Len() > 0 {
		_, _ = w.Write(b.body.Bytes())
	}
}

// TimeoutMiddleware creates a middleware that cancels requests that exceed the timeout.
func TimeoutMiddleware(config TimeoutConfig) core.MiddlewareFunc {
	if !config.Enabled {
		return nil
	}

	if config.Duration <= 0 {
		config.Duration = 30 * time.Second
	}

	return func(ctx *core.Ctx, next core.Handler) error {
		ctxWithTimeout, cancel := context.WithTimeout(ctx.Request.Context(), config.Duration)
		defer cancel()

		ctx.Request = ctx.Request.WithContext(ctxWithTimeout)

		// The handler gets its own Ctx pointing at a private buffer, and shares
		// Params/Locals with the caller so middleware downstream still sees them.
		buffered := newBufferedResponse()
		inner := *ctx
		inner.Response = buffered

		errCh := make(chan error, 1)
		go func() {
			errCh <- next(&inner)
		}()

		select {
		case err := <-errCh:
			buffered.flushTo(ctx.Response)
			return err
		case <-ctxWithTimeout.Done():
			ctx.Response.Header().Set("Content-Type", "application/json")
			ctx.Response.WriteHeader(http.StatusGatewayTimeout)
			payload, _ := json.Marshal(timeoutBody{
				Status:  "error",
				Message: config.ErrorMessage,
			})
			_, _ = ctx.Response.Write(payload)
			return ctxWithTimeout.Err()
		}
	}
}
