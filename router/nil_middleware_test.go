package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofault/gofault/core"
	"github.com/gofault/gofault/middleware"
)

// Every middleware constructor in this framework returns nil when disabled, and
// the documented usage is to register whatever the constructor returned. Before
// nil entries were filtered, that put a nil in the chain and panicked on the
// first request.
func TestRouter_DisabledMiddlewareDoesNotPanic(t *testing.T) {
	constructors := map[string]func() core.MiddlewareFunc{
		"compression": func() core.MiddlewareFunc {
			cfg := middleware.DefaultCompressionConfig()
			cfg.Enabled = false
			return middleware.CompressionMiddleware(cfg)
		},
		"recovery": func() core.MiddlewareFunc {
			cfg := middleware.DefaultRecoveryConfig()
			cfg.Enabled = false
			return middleware.RecoveryMiddleware(cfg)
		},
		"upload": func() core.MiddlewareFunc {
			cfg := middleware.DefaultUploadConfig()
			cfg.Enabled = false
			return middleware.UploadMiddleware(cfg)
		},
		"timeout": func() core.MiddlewareFunc {
			cfg := middleware.DefaultTimeoutConfig()
			cfg.Enabled = false
			return middleware.TimeoutMiddleware(cfg)
		},
		"requestLogger": func() core.MiddlewareFunc {
			cfg := middleware.DefaultRequestLoggerConfig()
			cfg.Enabled = false
			return middleware.RequestLoggerMiddleware(cfg)
		},
	}

	for name, build := range constructors {
		t.Run(name, func(t *testing.T) {
			mw := build()
			if mw != nil {
				t.Skipf("%s is enabled or does not return nil when disabled", name)
			}

			r := New()
			r.Handle(http.MethodGet, "/x", func(ctx *core.Ctx) error {
				ctx.Response.WriteHeader(http.StatusOK)
				return nil
			}, mw)

			w := httptest.NewRecorder()
			// The point of the test: no panic.
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))

			if w.Code != http.StatusOK {
				t.Errorf("status = %d, want 200", w.Code)
			}
		})
	}
}

// A nil mixed in with real middleware must be dropped without removing the
// others or changing their order.
func TestRouter_NilMiddlewareIsDroppedNotReordered(t *testing.T) {
	r := New()

	var order []string
	first := func(ctx *core.Ctx, next core.Handler) error {
		order = append(order, "first")
		return next(ctx)
	}
	second := func(ctx *core.Ctx, next core.Handler) error {
		order = append(order, "second")
		return next(ctx)
	}

	r.Handle(http.MethodGet, "/x", func(ctx *core.Ctx) error { return nil },
		nil, first, nil, second, nil)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))

	if len(order) != 2 || order[0] != "first" || order[1] != "second" {
		t.Errorf("middleware ran as %v, want [first second]", order)
	}
}

// Global middleware goes through the same filter.
func TestRouter_GlobalNilMiddlewareIsDropped(t *testing.T) {
	r := New()

	ran := false
	r.Middleware(nil, func(ctx *core.Ctx, next core.Handler) error {
		ran = true
		return next(ctx)
	})
	r.Handle(http.MethodGet, "/x", func(ctx *core.Ctx) error { return nil }, nil)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))

	if !ran {
		t.Error("the non-nil global middleware did not run")
	}
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

// Module.RegisterMiddleware is the other entry point and must filter too.
func TestModule_RegisterMiddlewareDropsNil(t *testing.T) {
	m := core.NewModule("test")

	ran := false
	m.RegisterMiddleware(nil, func(ctx *core.Ctx, next core.Handler) error {
		ran = true
		return next(ctx)
	}, nil)

	if len(m.Middleware) != 1 {
		t.Fatalf("module has %d middleware entries, want 1", len(m.Middleware))
	}

	if err := m.Middleware[0](nil, func(ctx *core.Ctx) error { return nil }); err != nil {
		t.Fatalf("middleware: %v", err)
	}
	if !ran {
		t.Error("the registered middleware did not run")
	}
}

// A misconfigured JWTAuth returns a failing middleware, not nil, so registering
// it the documented way cannot panic.
func TestRouter_MisconfiguredJWTAuthDoesNotPanic(t *testing.T) {
	r := New()
	r.Handle(http.MethodGet, "/secure", func(ctx *core.Ctx) error {
		ctx.Response.WriteHeader(http.StatusOK)
		return nil
	}, middleware.JWTAuth(middleware.JWTConfig{Algorithm: "HS256"})) // no secret

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/secure", nil))

	if w.Code == http.StatusOK {
		t.Error("a misconfigured auth middleware should not allow the request through")
	}
}
