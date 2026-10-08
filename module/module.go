// Package module provides the application root that bootstraps all registered modules.
package module

import (
	"fmt"
	"sync"

	"github.com/gofault/gofault/controller"
	"github.com/gofault/gofault/core"
	"github.com/gofault/gofault/ioc"
	"github.com/gofault/gofault/provider"
	"github.com/gofault/gofault/router"
	"github.com/gofault/gofault/server"
)

// App is the root application structure that ties everything together.
type App struct {
	container *ioc.Container
	rtr       *router.Router
	modules   []*core.Module // sorted by dependency order
	moduleMap map[string]*core.Module

	// mu guards server, booted and the router wiring below them. Start runs on
	// the caller's goroutine while Stop is conventionally invoked from a
	// signal handler, so those fields are written and read concurrently.
	mu     sync.Mutex
	server *server.HTTP
	booted bool
}

// New creates a new application instance.
func New() *App {
	return &App{
		container: ioc.New(),
		rtr:       router.New(),
		modules:   make([]*core.Module, 0),
		moduleMap: make(map[string]*core.Module),
	}
}

// SetRouter replaces the default router with a custom one (e.g., to add filters).
func (a *App) SetRouter(r *router.Router) {
	a.rtr = r
}

// RegisterModules registers one or more application modules.
func (a *App) RegisterModules(mods ...*core.Module) error {
	for _, m := range mods {
		if m.Name == "" {
			return fmt.Errorf("module name cannot be empty")
		}
		if _, exists := a.moduleMap[m.Name]; exists {
			return fmt.Errorf("duplicate module name: %q", m.Name)
		}
		a.moduleMap[m.Name] = m
		for _, ctrl := range m.Controllers {
			a.container.Register(func() core.Controller { return ctrl })
		}
		for _, prov := range m.Providers {
			a.container.Register(func() provider.Provider { return prov })
		}
		a.modules = append(a.modules, m)
	}
	return nil
}

// makeHandler creates a handler closure that properly captures route values,
// avoiding the classic Go closure capture bug where loop variables are
// captured by reference instead of by value.
func makeHandler(ctrl core.Controller, route core.Route) core.Handler {
	return func(ctx *core.Ctx) error {
		return controller.InvokeHandler(ctrl, route.Method, route.Path, route.Handler, ctx)
	}
}

// sortModules topological sorts modules by their Depends declarations.
// Modules with no dependencies or only resolved dependencies come first.
func (a *App) sortModules() error {
	// Build dependency graph and check for missing dependencies.
	for _, m := range a.modules {
		for _, dep := range m.Depends {
			if _, exists := a.moduleMap[dep]; !exists {
				return fmt.Errorf("module %q depends on unknown module %q", m.Name, dep)
			}
		}
	}

	// Kahn's algorithm for topological sort.
	var sorted []*core.Module
	resolved := make(map[string]bool)
	remaining := make(map[string]*core.Module)
	for _, m := range a.modules {
		remaining[m.Name] = m
	}

	for len(remaining) > 0 {
		progress := false
		for name, m := range remaining {
			allResolved := true
			for _, dep := range m.Depends {
				if !resolved[dep] {
					allResolved = false
					break
				}
			}
			if allResolved {
				sorted = append(sorted, m)
				resolved[name] = true
				delete(remaining, name)
				progress = true
			}
		}
		if !progress && len(remaining) > 0 {
			// Circular dependency detected.
			var cycle []string
			for name := range remaining {
				cycle = append(cycle, name)
			}
			return fmt.Errorf("circular dependency detected among modules: %v", cycle)
		}
	}

	a.modules = sorted
	return nil
}

// Init initializes all modules in dependency order, calling OnInit hooks.
// It is called automatically by Bootstrap, but can be called explicitly for testing.
func (a *App) Init() error {
	if err := a.sortModules(); err != nil {
		return fmt.Errorf("module initialization failed: %w", err)
	}

	for _, mod := range a.modules {
		for _, hook := range mod.OnInitHooks {
			if err := hook.OnInit(); err != nil {
				return fmt.Errorf("module %q OnInit failed: %w", mod.Name, err)
			}
		}
	}
	return nil
}

// Bootstrap finalizes the application setup, registering all routes.
func (a *App) Bootstrap() error {
	if err := a.Init(); err != nil {
		return err
	}
	for _, mod := range a.modules {
		for _, ctrl := range mod.Controllers {
			for _, route := range ctrl.Routes() {
				fullPath := ctrl.Prefix() + route.Path
				// Pass route as parameter to avoid classic Go closure capture bug.
				a.rtr.Handle(route.Method, fullPath, makeHandler(ctrl, route), mod.Middleware...)
			}
		}
	}
	a.mu.Lock()
	a.booted = true
	a.mu.Unlock()
	return nil
}

// Start launches the HTTP server on the given port.
func (a *App) Start(port int) error {
	a.mu.Lock()
	booted := a.booted
	a.mu.Unlock()

	if !booted {
		if err := a.Bootstrap(); err != nil {
			return fmt.Errorf("bootstrap failed: %w", err)
		}
	}

	// Wire container into router for request-scoped dependency injection.
	a.mu.Lock()
	if a.rtr != nil {
		a.rtr.SetContainer(a.container)
	}
	a.mu.Unlock()

	// Run OnBoot hooks.
	for _, mod := range a.modules {
		for _, hook := range mod.OnBootHooks {
			if err := hook.OnBoot(); err != nil {
				return fmt.Errorf("OnBoot hook failed: %w", err)
			}
		}
	}

	srv := server.New(a.rtr, port)
	a.mu.Lock()
	a.server = srv
	a.mu.Unlock()

	return srv.Start()
}

// Stop gracefully shuts down the server and calls OnShutdown hooks.
func (a *App) Stop() error {
	var errs []error

	// Call OnShutdown hooks in reverse order.
	for i := len(a.modules) - 1; i >= 0; i-- {
		mod := a.modules[i]
		for j := len(mod.OnShutdownHooks) - 1; j >= 0; j-- {
			hook := mod.OnShutdownHooks[j]
			if err := hook.OnShutdown(); err != nil {
				errs = append(errs, err)
			}
		}
	}

	a.mu.Lock()
	srv := a.server
	a.mu.Unlock()

	if srv != nil {
		if err := srv.Stop(); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("shutdown errors: %v", errs)
	}
	return nil
}

// Container returns the IoC container (exposed for testing).
func (a *App) Container() *ioc.Container {
	return a.container
}

// Router returns the router the app serves, for tests that need to issue
// requests without going through a listening socket.
func (a *App) Router() *router.Router {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.rtr
}

// serverReady reports whether Start has installed the HTTP server.
func (a *App) serverReady() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.server != nil
}
