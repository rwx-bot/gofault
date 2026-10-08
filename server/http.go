// Package server provides the HTTP server implementation.
package server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// DefaultReadHeaderTimeout bounds how long a client may take to send request
// headers. Without it net/http waits forever, so a handful of connections that
// dribble bytes can exhaust the server (Slowloris).
const DefaultReadHeaderTimeout = 10 * time.Second

// HTTP wraps the standard library HTTP server.
type HTTP struct {
	server  *http.Server
	hooks   []func()
	mu      sync.RWMutex
	stopped bool
	// shutdownErr memoises the result so repeated shutdown attempts report the
	// same outcome instead of appearing to succeed.
	shutdownErr error
}

// New creates a new HTTP server that will listen on the given port.
func New(handler http.Handler, port int) *HTTP {
	return NewWithConfig(handler, fmt.Sprintf(":%d", port))
}

// NewWithConfig creates a new HTTP server with custom configuration.
// ReadHeaderTimeout is set to DefaultReadHeaderTimeout; override
// HTTP.Server settings through Server() if you need different values.
func NewWithConfig(handler http.Handler, addr string) *HTTP {
	return &HTTP{
		server: &http.Server{
			Addr:              addr,
			Handler:           handler,
			ReadHeaderTimeout: DefaultReadHeaderTimeout,
		},
	}
}

// Server exposes the underlying http.Server for advanced configuration.
func (s *HTTP) Server() *http.Server {
	return s.server
}

// RegisterShutdownHook registers a function to be called during graceful shutdown.
func (s *HTTP) RegisterShutdownHook(hook func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hooks = append(s.hooks, hook)
}

// Start launches the server and blocks.
// A graceful shutdown surfaces as http.ErrServerClosed, which is a normal
// termination rather than a failure; callers that only care about real errors
// should ignore it.
func (s *HTTP) Start() error {
	err := s.server.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

// listen splits an http.Server Addr into a host and port and binds it.
// Splitting here lets StartWithGracefulShutdown surface a bind failure to the
// caller instead of losing it behind a signal wait.
func listen(addr string) (net.Listener, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("invalid address %q: %w", addr, err)
	}
	return net.Listen("tcp", net.JoinHostPort(host, port))
}

// StartWithGracefulShutdown starts the server and waits for shutdown signals.
func (s *HTTP) StartWithGracefulShutdown(timeout time.Duration) error {
	// Listen before announcing readiness, so a bind failure (port in use,
	// bad address) is reported to the caller instead of being lost while we
	// block waiting for a signal that may never arrive.
	ln, err := listen(s.server.Addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", s.server.Addr, err)
	}

	// Start server in goroutine
	errChan := make(chan error, 1)
	go func() {
		if err := s.server.Serve(ln); err != nil && err != http.ErrServerClosed {
			errChan <- err
		}
		close(errChan)
	}()

	// Wait for interrupt signal, or for the server to fail on its own.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(quit)

	select {
	case err := <-errChan:
		if err != nil {
			return fmt.Errorf("serve: %w", err)
		}
		return nil
	case <-quit:
	}

	return s.GracefulShutdown(timeout)
}

// GracefulShutdown shuts down the server gracefully with a timeout.
//
// It is safe to call more than once: the first call performs the shutdown and
// later calls return the same result. Marking the server stopped only after
// the attempt means a shutdown that times out can be retried instead of being
// silently treated as complete.
func (s *HTTP) GracefulShutdown(timeout time.Duration) error {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return s.shutdownErr
	}
	// Claim the shutdown so concurrent callers do not run the hooks twice.
	s.stopped = true
	hooks := append([]func(){}, s.hooks...)
	s.mu.Unlock()

	// Execute shutdown hooks
	for _, hook := range hooks {
		hook()
	}

	// Create shutdown context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	err := s.server.Shutdown(ctx)

	s.mu.Lock()
	s.shutdownErr = err
	s.mu.Unlock()
	return err
}

// Stop gracefully shuts down the server with a default 30 second timeout.
func (s *HTTP) Stop() error {
	return s.GracefulShutdown(30 * time.Second)
}

// IsStopped returns whether the server has been stopped.
func (s *HTTP) IsStopped() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.stopped
}
