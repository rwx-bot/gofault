package module

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofault/gofault/controller"
	"github.com/gofault/gofault/core"
	"github.com/gofault/gofault/router"
)

// dummyController is a test controller.
type dummyController struct {
	controller.BaseController
	calledMethod string
}

func (c *dummyController) Routes() []core.Route {
	return []core.Route{
		{Method: "GET", Path: "/", Handler: "Index"},
		{Method: "GET", Path: "/greet/:name", Handler: "Greet"},
	}
}

func (c *dummyController) Prefix() string { return "/test" }

func (c *dummyController) Index(ctx *core.Ctx) error {
	return controller.OK(ctx.Response, map[string]string{"message": "index"})
}

func (c *dummyController) Greet(ctx *core.Ctx) error {
	name := controller.Param(ctx, "name")
	return controller.OK(ctx.Response, map[string]string{"message": "Hello, " + name})
}

// dummyHook tracks boot/shutdown calls.
type dummyHook struct {
	bootCalled     bool
	shutdownCalled bool
	bootErr        error
	shutdownErr    error
}

func (h *dummyHook) OnBoot() error {
	h.bootCalled = true
	return h.bootErr
}

func (h *dummyHook) OnShutdown() error {
	h.shutdownCalled = true
	return h.shutdownErr
}

func TestApp_New(t *testing.T) {
	app := New()
	if app == nil {
		t.Fatal("New() returned nil")
	}
	if app.Container() == nil {
		t.Fatal("Container() returned nil")
	}
}

func TestApp_RegisterModules(t *testing.T) {
	app := New()
	mod := core.NewModule("test")
	mod.RegisterControllers(&dummyController{})

	app.RegisterModules(mod)

	if len(app.modules) != 1 {
		t.Fatalf("expected 1 module, got %d", len(app.modules))
	}
}

func TestApp_Bootstrap_DoesNotPanic(t *testing.T) {
	app := New()
	mod := core.NewModule("test")
	mod.RegisterControllers(&dummyController{})
	app.RegisterModules(mod)

	err := app.Bootstrap()
	if err != nil {
		t.Fatalf("Bootstrap failed: %v", err)
	}
}

func TestApp_SetRouter(t *testing.T) {
	app := New()
	app.SetRouter(nil) // nil router is allowed, just not used
}

func TestApp_LifecycleHooks(t *testing.T) {
	app := New()
	mod := core.NewModule("test")

	hook1 := &dummyHook{}
	hook2 := &dummyHook{}

	mod.RegisterOnBoot(hook1)
	mod.RegisterOnShutdown(hook2)

	app.RegisterModules(mod)
	app.Bootstrap()

	// Run boot hooks manually (normally called by Start).
	for _, m := range app.modules {
		for _, h := range m.OnBootHooks {
			if err := h.OnBoot(); err != nil {
				t.Fatalf("OnBoot failed: %v", err)
			}
		}
	}

	if !hook1.bootCalled {
		t.Fatal("boot hook was not called")
	}

	// Run shutdown hooks in reverse.
	for i := len(app.modules) - 1; i >= 0; i-- {
		m := app.modules[i]
		for j := len(m.OnShutdownHooks) - 1; j >= 0; j-- {
			h := m.OnShutdownHooks[j]
			if err := h.OnShutdown(); err != nil {
				t.Fatalf("OnShutdown failed: %v", err)
			}
		}
	}

	if !hook2.shutdownCalled {
		t.Fatal("shutdown hook was not called")
	}
}

func TestApp_Bootstrap_MultipleControllers(t *testing.T) {
	app := New()
	mod := core.NewModule("test")
	mod.RegisterControllers(&dummyController{}, &dummyController{})
	app.RegisterModules(mod)

	err := app.Bootstrap()
	if err != nil {
		t.Fatalf("Bootstrap failed: %v", err)
	}
}

func TestApp_MultipleModules(t *testing.T) {
	app := New()

	mod1 := core.NewModule("mod1")
	mod1.RegisterControllers(&dummyController{})

	mod2 := core.NewModule("mod2")
	mod2.RegisterControllers(&dummyController{})

	app.RegisterModules(mod1, mod2)

	if len(app.modules) != 2 {
		t.Fatalf("expected 2 modules, got %d", len(app.modules))
	}
}

func TestApp_Start_WithoutBootstrap_DoesBootstrap(t *testing.T) {
	app := New()
	mod := core.NewModule("test")
	mod.RegisterControllers(&dummyController{})
	app.RegisterModules(mod)

	// Verify not booted yet.
	if app.booted {
		t.Fatal("app should not be booted before Bootstrap")
	}

	// Calling Bootstrap directly.
	err := app.Bootstrap()
	if err != nil {
		t.Fatalf("Bootstrap failed: %v", err)
	}
	if !app.booted {
		t.Fatal("app should be booted after Bootstrap")
	}
}

func TestApp_RouteHandlerDispatch(t *testing.T) {
	rtr := router.New()
	app := New()
	app.SetRouter(rtr)

	mod := core.NewModule("test")
	mod.RegisterControllers(&dummyController{})
	app.RegisterModules(mod)
	app.Bootstrap()

	// Test index route.
	req := httptest.NewRequest("GET", "/test/", nil)
	w := httptest.NewRecorder()
	rtr.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	// Test greet route.
	req = httptest.NewRequest("GET", "/test/greet/alice", nil)
	w = httptest.NewRecorder()
	rtr.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", w.Code, w.Body.String())
	}
}

func TestApp_ProvidersRegistered(t *testing.T) {
	app := New()
	mod := core.NewModule("test")
	app.RegisterModules(mod)

	// Just verify that registration doesn't panic.
}

// errInitHook fails OnInit so Init error handling can be exercised.
type errInitHook struct{ dummyHook }

func (h *errInitHook) OnInit() error { return errors.New("init boom") }

func TestApp_Init_RunsHooksInDependencyOrder(t *testing.T) {
	app := New()

	base := core.NewModule("base")
	dependent := core.NewModule("dependent")
	dependent.Depends = []string{"base"}

	order := []string{}
	base.RegisterOnInit(&recordingHook{name: "base", order: &order})
	dependent.RegisterOnInit(&recordingHook{name: "dependent", order: &order})

	app.RegisterModules(dependent, base)

	if err := app.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if len(order) != 2 || order[0] != "base" || order[1] != "dependent" {
		t.Errorf("init order = %v, want [base dependent]", order)
	}
}

func TestApp_Init_PropagatesHookError(t *testing.T) {
	app := New()
	mod := core.NewModule("boom")
	mod.RegisterOnInit(&errInitHook{})
	app.RegisterModules(mod)

	if err := app.Init(); err == nil {
		t.Error("expected Init to surface the OnInit failure")
	}
}

func TestApp_Init_RejectsUnknownDependency(t *testing.T) {
	app := New()
	mod := core.NewModule("a")
	mod.Depends = []string{"nope"}
	app.RegisterModules(mod)

	if err := app.Init(); err == nil {
		t.Error("expected Init to reject a dependency on an unregistered module")
	}
}

func TestApp_Init_RejectsDependencyCycle(t *testing.T) {
	app := New()
	a := core.NewModule("a")
	b := core.NewModule("b")
	a.Depends = []string{"b"}
	b.Depends = []string{"a"}
	app.RegisterModules(a, b)

	if err := app.Init(); err == nil {
		t.Error("expected Init to reject a dependency cycle")
	}
}

// Start blocks, so run it on a goroutine and shut it down via Stop.
func TestApp_StartAndStop(t *testing.T) {
	app := New()
	mod := core.NewModule("test")
	mod.RegisterControllers(&dummyController{})
	hook := &dummyHook{}
	mod.RegisterOnBoot(hook)
	mod.RegisterOnShutdown(hook)
	app.RegisterModules(mod)

	errCh := make(chan error, 1)
	go func() { errCh <- app.Start(0) }()

	// Wait for the server to be up, then verify the route serves.
	var lastCode int
	for i := 0; i < 100; i++ {
		time.Sleep(10 * time.Millisecond)
		if !app.serverReady() {
			continue
		}
		req := httptest.NewRequest("GET", "/test/", nil)
		w := httptest.NewRecorder()
		app.rtr.ServeHTTP(w, req)
		lastCode = w.Code
		break
	}
	if lastCode != http.StatusOK {
		t.Fatalf("expected the running server to answer 200, got %d", lastCode)
	}
	if !hook.bootCalled {
		t.Error("OnBoot hook was not called by Start")
	}

	if err := app.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if !hook.shutdownCalled {
		t.Error("OnShutdown hook was not called by Stop")
	}

	// Start returns once the listener is closed.
	select {
	case <-errCh:
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return after Stop")
	}
}

func TestApp_Stop_WithoutStartIsSafe(t *testing.T) {
	app := New()
	app.RegisterModules(core.NewModule("test"))

	if err := app.Stop(); err != nil {
		t.Errorf("Stop without a running server should be a no-op, got %v", err)
	}
}

func TestApp_Stop_CollectsHookErrors(t *testing.T) {
	app := New()
	mod := core.NewModule("test")
	mod.RegisterOnShutdown(&dummyHook{shutdownErr: errors.New("shutdown boom")})
	app.RegisterModules(mod)

	if err := app.Stop(); err == nil {
		t.Error("expected Stop to report the OnShutdown failure")
	}
}

func TestApp_Start_PropagatesBootHookError(t *testing.T) {
	app := New()
	mod := core.NewModule("test")
	mod.RegisterOnBoot(&dummyHook{bootErr: errors.New("boot boom")})
	app.RegisterModules(mod)

	if err := app.Start(0); err == nil {
		t.Error("expected Start to surface the OnBoot failure")
	}
}

func TestApp_Start_WiresContainerIntoRouter(t *testing.T) {
	app := New()
	mod := core.NewModule("test")
	mod.RegisterControllers(&dummyController{})
	app.RegisterModules(mod)
	if err := app.Bootstrap(); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}

	go func() { _ = app.Start(0) }()
	for i := 0; i < 100 && !app.serverReady(); i++ {
		time.Sleep(10 * time.Millisecond)
	}
	defer app.Stop()

	// The container must be reachable from a request-scoped provider.
	if app.Container() == nil {
		t.Fatal("expected a container on the app")
	}
	rtr := app.Router()
	req := httptest.NewRequest("GET", "/test/", nil)
	w := httptest.NewRecorder()
	rtr.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 after Start wired the container, got %d", w.Code)
	}
}

// recordingHook appends its name to order when OnInit runs.
type recordingHook struct {
	dummyHook
	name  string
	order *[]string
}

func (h *recordingHook) OnInit() error {
	*h.order = append(*h.order, h.name)
	return nil
}
