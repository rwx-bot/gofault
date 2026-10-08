package server

import (
	"context"
	"io"
	"net"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

// A bind failure must reach the caller. It used to be lost inside a goroutine
// while the main path waited for a signal, so a server that could not start
// hung forever instead of reporting "address already in use".
func TestStartWithGracefulShutdown_BindFailureIsReported(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("occupy port: %v", err)
	}
	defer ln.Close()

	srv := NewWithConfig(nil, ln.Addr().String())

	done := make(chan error, 1)
	go func() { done <- srv.StartWithGracefulShutdown(time.Second) }()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected a bind error, got nil")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("StartWithGracefulShutdown hung on a bind failure")
	}
}

func TestStartWithGracefulShutdown_InvalidAddress(t *testing.T) {
	srv := NewWithConfig(nil, "not-an-address")

	done := make(chan error, 1)
	go func() { done <- srv.StartWithGracefulShutdown(time.Second) }()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected an error for a malformed address")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("StartWithGracefulShutdown hung on a malformed address")
	}
}

// After a graceful shutdown Start must report success, not http.ErrServerClosed,
// which callers would otherwise have to special-case.
func TestStart_ReturnsNilAfterGracefulShutdown(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close()

	srv := NewWithConfig(http.NewServeMux(), addr)

	done := make(chan error, 1)
	go func() { done <- srv.Start() }()

	// Wait for the listener to accept, then shut down.
	deadline := time.Now().Add(3 * time.Second)
	var up bool
	for time.Now().Before(deadline) {
		c, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			c.Close()
			up = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !up {
		t.Fatal("server never came up")
	}

	if err := srv.GracefulShutdown(2 * time.Second); err != nil {
		t.Fatalf("GracefulShutdown: %v", err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Start() = %v, want nil after a graceful shutdown", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return after shutdown")
	}
}

// Repeated shutdowns must report the same outcome rather than the second call
// appearing to succeed when the first actually failed.
func TestGracefulShutdown_MemoisesFailure(t *testing.T) {
	srv := New(nil, 0)
	// Nothing is listening, so Shutdown returns nil; the point here is that
	// the result is stable and the hooks do not run twice.
	var calls int32
	srv.RegisterShutdownHook(func() { atomic.AddInt32(&calls, 1) })

	if err := srv.GracefulShutdown(time.Second); err != nil {
		t.Fatalf("first shutdown: %v", err)
	}
	first := srv.GracefulShutdown(time.Second)

	if n := atomic.LoadInt32(&calls); n != 1 {
		t.Errorf("hooks ran %d times, want 1", n)
	}
	if srv.shutdownErr != first {
		t.Error("shutdownErr was not memoised")
	}
}

// Without ReadHeaderTimeout a client can hold a connection open indefinitely by
// dribbling headers, which is the Slowloris exhaustion pattern.
func TestNew_SetsReadHeaderTimeout(t *testing.T) {
	if got := New(nil, 8080).Server().ReadHeaderTimeout; got != DefaultReadHeaderTimeout {
		t.Errorf("ReadHeaderTimeout = %v, want %v", got, DefaultReadHeaderTimeout)
	}
	if got := NewWithConfig(nil, "127.0.0.1:0").Server().ReadHeaderTimeout; got != DefaultReadHeaderTimeout {
		t.Errorf("ReadHeaderTimeout = %v, want %v", got, DefaultReadHeaderTimeout)
	}
}

// The server must actually answer requests end to end.
func TestStart_ServesRequests(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("pong"))
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close()

	srv := NewWithConfig(mux, addr)
	go srv.Start()
	defer srv.GracefulShutdown(2 * time.Second)

	client := &http.Client{Timeout: 2 * time.Second}
	var body string
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := client.Get("http://" + addr + "/ping")
		if err != nil {
			time.Sleep(20 * time.Millisecond)
			continue
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		body = string(b)
		break
	}

	if body != "pong" {
		t.Errorf("body = %q, want %q", body, "pong")
	}
}

// A request in flight must be allowed to finish during a graceful shutdown.
func TestGracefulShutdown_DrainsInFlightRequest(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{})

	mux := http.NewServeMux()
	mux.HandleFunc("/slow", func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		w.Write([]byte("done"))
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close()

	srv := NewWithConfig(mux, addr)
	go srv.Start()
	defer srv.GracefulShutdown(2 * time.Second)

	type result struct {
		body string
		err  error
	}
	res := make(chan result, 1)
	go func() {
		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Get("http://" + addr + "/slow")
		if err != nil {
			res <- result{err: err}
			return
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		res <- result{body: string(b)}
	}()

	<-started

	shutdownDone := make(chan error, 1)
	go func() { shutdownDone <- srv.GracefulShutdown(5 * time.Second) }()

	// The handler is still running, so the shutdown must still be waiting.
	select {
	case err := <-shutdownDone:
		t.Fatalf("shutdown returned early with %v while a request was in flight", err)
	case <-time.After(300 * time.Millisecond):
	}

	close(release)

	if err := <-shutdownDone; err != nil {
		t.Errorf("GracefulShutdown: %v", err)
	}

	r := <-res
	if r.err != nil {
		t.Fatalf("in-flight request failed during graceful shutdown: %v", r.err)
	}
	if r.body != "done" {
		t.Errorf("body = %q, want %q", r.body, "done")
	}
}

func TestListen(t *testing.T) {
	if _, err := listen("127.0.0.1:0"); err != nil {
		t.Errorf("listen on a valid address: %v", err)
	}
	if _, err := listen("missing-port"); err == nil {
		t.Error("expected an error for an address with no port")
	}
}

var _ = context.Background
