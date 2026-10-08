package router

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gofault/gofault/core"
	"github.com/gofault/gofault/exception"
)

func dummyHandler(ctx *core.Ctx) error {
	return nil
}

func TestRouter_HandleAndServe(t *testing.T) {
	r := New()
	r.Handle("GET", "/hello/:name", dummyHandler)

	req := httptest.NewRequest("GET", "/hello/world", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestRouter_NotFound(t *testing.T) {
	r := New()
	r.Handle("GET", "/hello/:name", dummyHandler)

	req := httptest.NewRequest("GET", "/nonexistent", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestRouter_MiddlewareChain(t *testing.T) {
	r := New()
	called := false
	r.Middleware(func(ctx *core.Ctx, next core.Handler) error {
		called = true
		return next(ctx)
	})
	r.Handle("GET", "/test", dummyHandler)

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if !called {
		t.Fatal("middleware was not called")
	}
}

func TestRouter_ExceptionFilter(t *testing.T) {
	r := New()

	errHandler := func(ctx *core.Ctx) error {
		return exception.BadRequest("invalid input")
	}

	r.Handle("GET", "/error", errHandler)
	r.ExceptionFilter(exception.NewHTTPExceptionFilter())

	req := httptest.NewRequest("GET", "/error", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// The middleware chain was built with append(r.middleware, ...), which writes
// into the router's backing array whenever it has spare capacity. Concurrent
// requests then clobber each other's per-route middleware and run the wrong
// chain, so the buffer must be allocated per request.
func TestRouter_ConcurrentRequestsDoNotShareChain(t *testing.T) {
	r := New()

	nop := func(ctx *core.Ctx, next core.Handler) error { return next(ctx) }
	r.Middleware(nop)
	r.Middleware(nop)
	r.Middleware(nop)
	if cap(r.middleware) <= len(r.middleware) {
		t.Fatalf("test needs spare capacity to expose aliasing, cap=%d len=%d",
			cap(r.middleware), len(r.middleware))
	}

	r.Handle("GET", "/a", func(ctx *core.Ctx) error {
		_, err := ctx.Response.Write([]byte("one"))
		return err
	}, func(ctx *core.Ctx, next core.Handler) error {
		ctx.Response.Write([]byte("A"))
		return next(ctx)
	})
	r.Handle("GET", "/b", func(ctx *core.Ctx) error {
		_, err := ctx.Response.Write([]byte("two"))
		return err
	}, func(ctx *core.Ctx, next core.Handler) error {
		ctx.Response.Write([]byte("B"))
		return next(ctx)
	})

	var wg sync.WaitGroup
	bad := make(chan string, 256)
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest("GET", "/a", nil))
			if got := w.Body.String(); got != "Aone" {
				bad <- "/a=" + got
			}
		}()
		go func() {
			defer wg.Done()
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest("GET", "/b", nil))
			if got := w.Body.String(); got != "Btwo" {
				bad <- "/b=" + got
			}
		}()
	}
	wg.Wait()
	close(bad)

	for got := range bad {
		t.Errorf("corrupted response: %s", got)
	}
}
