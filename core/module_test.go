package core

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewCtx(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/test", nil)

	ctx := NewCtx(w, r)
	if ctx.Request != r {
		t.Error("Request not set")
	}
	if ctx.Response != w {
		t.Error("Response not set")
	}
	if ctx.StatusCode != http.StatusOK {
		t.Errorf("expected StatusOK, got %d", ctx.StatusCode)
	}
	if ctx.Params == nil {
		t.Error("Params should be initialized")
	}
	if ctx.Locals == nil {
		t.Error("Locals should be initialized")
	}
}

func TestCtx_Locals(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/", nil)
	ctx := NewCtx(w, r)

	// Store and retrieve
	ctx.Locals["user"] = "alice"
	ctx.Locals["score"] = 42

	if v := ctx.Locals["user"]; v != "alice" {
		t.Errorf("expected alice, got %v", v)
	}
	if v := ctx.Locals["score"]; v != 42 {
		t.Errorf("expected 42, got %v", v)
	}
}

func TestNewModule(t *testing.T) {
	m := NewModule("test")
	if m.Name != "test" {
		t.Errorf("expected name 'test', got '%s'", m.Name)
	}
	if m.Depends == nil {
		t.Error("Depends should be initialized")
	}
	if m.Controllers == nil {
		t.Error("Controllers should be initialized")
	}
	if m.Providers == nil {
		t.Error("Providers should be initialized")
	}
	if m.Middleware == nil {
		t.Error("Middleware should be initialized")
	}
}

func TestModule_RegisterControllers(t *testing.T) {
	m := NewModule("test")
	ctrl := &testController{}

	m.RegisterControllers(ctrl)
	if len(m.Controllers) != 1 {
		t.Errorf("expected 1 controller, got %d", len(m.Controllers))
	}
	m.RegisterControllers(ctrl, ctrl)
	if len(m.Controllers) != 3 {
		t.Errorf("expected 3 controllers, got %d", len(m.Controllers))
	}
}

func TestModule_RegisterProviders(t *testing.T) {
	m := NewModule("test")
	prov := &testProvider{}

	m.RegisterProviders(prov)
	if len(m.Providers) != 1 {
		t.Errorf("expected 1 provider, got %d", len(m.Providers))
	}
}

func TestModule_RegisterMiddleware(t *testing.T) {
	m := NewModule("test")
	mw := MiddlewareFunc(func(ctx *Ctx, next Handler) error { return nil })

	m.RegisterMiddleware(mw)
	if len(m.Middleware) != 1 {
		t.Errorf("expected 1 middleware, got %d", len(m.Middleware))
	}
}

func TestModule_RegisterOnInit(t *testing.T) {
	m := NewModule("test")
	hook := &testHook{}

	m.RegisterOnInit(hook)
	if len(m.OnInitHooks) != 1 {
		t.Errorf("expected 1 OnInit hook, got %d", len(m.OnInitHooks))
	}
}

func TestModule_RegisterOnBoot(t *testing.T) {
	m := NewModule("test")
	hook := &testHook{}

	m.RegisterOnBoot(hook)
	if len(m.OnBootHooks) != 1 {
		t.Errorf("expected 1 OnBoot hook, got %d", len(m.OnBootHooks))
	}
}

func TestModule_RegisterOnShutdown(t *testing.T) {
	m := NewModule("test")
	hook := &testHook{}

	m.RegisterOnShutdown(hook)
	if len(m.OnShutdownHooks) != 1 {
		t.Errorf("expected 1 OnShutdown hook, got %d", len(m.OnShutdownHooks))
	}
}

func TestModule_Chaining(t *testing.T) {
	m := NewModule("test")
	ctrl := &testController{}
	mw := MiddlewareFunc(func(ctx *Ctx, next Handler) error { return nil })
	hook := &testHook{}

	m.RegisterControllers(ctrl).RegisterMiddleware(mw).RegisterOnInit(hook)

	if len(m.Controllers) != 1 {
		t.Error("controllers not chained")
	}
	if len(m.Middleware) != 1 {
		t.Error("middleware not chained")
	}
	if len(m.OnInitHooks) != 1 {
		t.Error("onInit not chained")
	}
}

// Mock types for testing

type testController struct{}

func (c *testController) Prefix() string  { return "/test" }
func (c *testController) Routes() []Route { return nil }

type testProvider struct{}

func (p *testProvider) Provide() any { return nil }

type testHook struct{}

func (h *testHook) OnInit() error     { return nil }
func (h *testHook) OnBoot() error     { return nil }
func (h *testHook) OnShutdown() error { return nil }

func TestRoute_Struct(t *testing.T) {
	r := Route{Method: "GET", Path: "/hello/:name", Handler: "Greet"}
	if r.Method != "GET" {
		t.Errorf("expected GET, got %s", r.Method)
	}
	if r.Path != "/hello/:name" {
		t.Errorf("expected /hello/:name, got %s", r.Path)
	}
	if r.Handler != "Greet" {
		t.Errorf("expected Greet, got %s", r.Handler)
	}
}

func TestMiddlewareFunc_Signature(t *testing.T) {
	called := false
	mw := MiddlewareFunc(func(ctx *Ctx, next Handler) error {
		called = true
		return next(ctx)
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	ctx := NewCtx(w, r)

	err := mw(ctx, func(c *Ctx) error { return nil })
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !called {
		t.Error("middleware was not called")
	}
}

func TestHandler_Signature(t *testing.T) {
	h := Handler(func(ctx *Ctx) error {
		ctx.StatusCode = 201
		return nil
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest("PUT", "/item", nil)
	ctx := NewCtx(w, r)

	err := h(ctx)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if ctx.StatusCode != 201 {
		t.Errorf("expected 201, got %d", ctx.StatusCode)
	}
}

func TestCtx_SetLocal(t *testing.T) {
	ctx := NewCtx(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))

	ctx.SetLocal("user", "alice")
	if ctx.Locals["user"] != "alice" {
		t.Errorf("Locals[user] = %v, want alice", ctx.Locals["user"])
	}

	// A Ctx built without NewCtx has a nil Locals map; SetLocal must not panic.
	bare := &Ctx{}
	bare.SetLocal("k", 1)
	if bare.Locals["k"] != 1 {
		t.Errorf("expected SetLocal to create the map, got %v", bare.Locals)
	}
}

func TestCtx_GetVersion(t *testing.T) {
	ctx := NewCtx(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))

	if got := ctx.GetVersion(); got != 0 {
		t.Errorf("GetVersion() with no version = %d, want 0", got)
	}
	ctx.SetLocal("version", 2)
	if got := ctx.GetVersion(); got != 2 {
		t.Errorf("GetVersion() = %d, want 2", got)
	}

	// A value of the wrong type must fall back to the zero value, not panic.
	ctx.SetLocal("version", "two")
	if got := ctx.GetVersion(); got != 0 {
		t.Errorf("GetVersion() with wrong type = %d, want 0", got)
	}
}

func TestCtx_GetVersionStatus(t *testing.T) {
	ctx := NewCtx(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))

	if got := ctx.GetVersionStatus(); got != 0 {
		t.Errorf("GetVersionStatus() with no status = %d, want 0", got)
	}
	ctx.SetLocal("version_status", 1)
	if got := ctx.GetVersionStatus(); got != 1 {
		t.Errorf("GetVersionStatus() = %d, want 1", got)
	}
}

func TestCtx_RespHeader(t *testing.T) {
	w := httptest.NewRecorder()
	ctx := NewCtx(w, httptest.NewRequest("GET", "/", nil))

	ctx.RespHeader().Set("X-Test", "v")
	if got := w.Header().Get("X-Test"); got != "v" {
		t.Errorf("RespHeader did not reach the response writer, got %q", got)
	}
}
