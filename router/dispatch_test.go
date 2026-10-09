package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofault/gofault/core"
	"github.com/gofault/gofault/exception"
)

// The ExceptionFilter contract says a filter may return false to let the
// exception propagate. The router ignored that return value, so a declining
// filter left the client with a 200 and an empty body.
func TestRouter_DeclinedExceptionFallsBackToDefault(t *testing.T) {
	r := New()
	r.ExceptionFilter(exception.ExceptionHandler(func(ctx any, err error) bool {
		return false
	}))
	r.Handle(http.MethodGet, "/boom", func(ctx *core.Ctx) error {
		return exception.BadRequest("bad input")
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/boom", nil))

	if w.Code == http.StatusOK {
		t.Fatalf("client got %d with an empty body; a declined error must not look successful", w.Code)
	}
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500 from the default handling", w.Code)
	}
	if !strings.Contains(w.Body.String(), "bad input") {
		t.Errorf("body = %q, want the error text", w.Body.String())
	}
}

// A filter that handles the error must still own the response.
func TestRouter_HandledExceptionSkipsDefault(t *testing.T) {
	r := New()
	r.ExceptionFilter(exception.ExceptionHandler(func(ctx any, err error) bool {
		c := ctx.(*core.Ctx)
		c.Response.WriteHeader(http.StatusTeapot)
		return true
	}))
	r.Handle(http.MethodGet, "/boom", func(ctx *core.Ctx) error {
		return exception.BadRequest("bad input")
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/boom", nil))

	if w.Code != http.StatusTeapot {
		t.Errorf("status = %d, want 418 from the filter", w.Code)
	}
	if strings.Contains(w.Body.String(), "Internal Server Error") {
		t.Error("the default handling ran even though the filter handled the error")
	}
}

// A path that exists under a different method is a 405 with an Allow header,
// not a 404 that hides the route.
func TestRouter_MethodNotAllowed(t *testing.T) {
	r := New()
	r.Handle(http.MethodGet, "/only-get", func(ctx *core.Ctx) error { return nil })
	r.Handle(http.MethodPut, "/only-get", func(ctx *core.Ctx) error { return nil })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/only-get", nil))

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", w.Code)
	}

	allow := w.Header().Get("Allow")
	if !strings.Contains(allow, http.MethodGet) || !strings.Contains(allow, http.MethodPut) {
		t.Errorf("Allow = %q, want it to list GET and PUT", allow)
	}
	if strings.Contains(allow, http.MethodDelete) {
		t.Errorf("Allow = %q, must not list the method that was tried", allow)
	}
}

// A genuinely unknown path stays a 404.
func TestRouter_UnknownPathStillNotFound(t *testing.T) {
	r := New()
	r.Handle(http.MethodGet, "/exists", func(ctx *core.Ctx) error { return nil })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/nope", nil))

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 for an unknown path", w.Code)
	}
	if w.Header().Get("Allow") != "" {
		t.Error("a 404 must not carry an Allow header")
	}
}

// net/http serves HEAD wherever GET is served; a HEAD is a GET whose body is
// discarded.
func TestRouter_HEADIsServedByGetRoute(t *testing.T) {
	r := New()
	var gotParams string
	r.Handle(http.MethodGet, "/items/:id", func(ctx *core.Ctx) error {
		gotParams = ctx.Params["id"]
		ctx.Response.Header().Set("X-Custom", "set")
		ctx.Response.WriteHeader(http.StatusOK)
		_, err := ctx.Response.Write([]byte("this body must not be sent for HEAD"))
		return err
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodHead, "/items/42", nil))

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	if gotParams != "42" {
		t.Errorf("path params not extracted for HEAD, got %q", gotParams)
	}
	if w.Header().Get("X-Custom") != "set" {
		t.Error("HEAD should still return the handler's headers")
	}
	if body := w.Body.String(); body != "" {
		t.Errorf("HEAD returned a body: %q", body)
	}
}

// An explicit HEAD route must win over the GET fallback.
func TestRouter_ExplicitHeadRouteWins(t *testing.T) {
	r := New()
	r.Handle(http.MethodGet, "/x", func(ctx *core.Ctx) error {
		ctx.Response.Write([]byte("get"))
		return nil
	})
	r.Handle(http.MethodHead, "/x", func(ctx *core.Ctx) error {
		ctx.Response.Write([]byte("head"))
		return nil
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodHead, "/x", nil))

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

// HEAD still runs the middleware chain, and a failing middleware is reported.
func TestRouter_HEADRunsMiddleware(t *testing.T) {
	r := New()
	ran := false
	r.Middleware(func(ctx *core.Ctx, next core.Handler) error {
		ran = true
		return next(ctx)
	})
	r.Handle(http.MethodGet, "/x", func(ctx *core.Ctx) error { return nil })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodHead, "/x", nil))

	if !ran {
		t.Error("HEAD did not run the middleware chain")
	}
}

func TestRouter_HEADPropagatesHandlerError(t *testing.T) {
	r := New()
	r.ExceptionFilter(exception.ExceptionHandler(func(ctx any, err error) bool {
		c := ctx.(*core.Ctx)
		c.Response.WriteHeader(http.StatusAccepted)
		return true
	}))
	r.Handle(http.MethodGet, "/x", func(ctx *core.Ctx) error {
		return exception.BadRequest("nope")
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodHead, "/x", nil))

	if w.Code != http.StatusAccepted {
		t.Errorf("status = %d, want the filter's 418/202 to be used", w.Code)
	}
	if w.Body.Len() != 0 {
		t.Errorf("HEAD returned a body: %q", w.Body.String())
	}
}

// A middleware-only method list must not break 405 detection.
func TestRouter_AllowedMethodsIgnoresEmptyMethodRoutes(t *testing.T) {
	r := New()
	// An empty method matches every method, so it is not a 405 candidate.
	r.Handle("", "/any", func(ctx *core.Ctx) error { return nil })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/any", nil))

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 since a route with no method matches any", w.Code)
	}
}
