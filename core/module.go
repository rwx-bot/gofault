// Package core defines the fundamental interfaces for the gofault framework.
// All core interfaces are defined here to avoid circular dependencies.
package core

import (
	"context"
	"net/http"
)

// Context wraps the standard HTTP request context with framework-specific data.
type Context = context.Context

// Ctx encapsulates an HTTP request/response pair plus extracted route parameters.
type Ctx struct {
	Request    *http.Request
	Response   http.ResponseWriter
	Params     map[string]string
	StatusCode int
	// Locals holds arbitrary context values, used to pass data between middleware.
	// For example, WebSocket middleware stores the upgraded connection here.
	Locals map[string]any
}

func NewCtx(w http.ResponseWriter, r *http.Request) *Ctx {
	return &Ctx{Request: r, Response: w, Params: make(map[string]string), StatusCode: http.StatusOK, Locals: make(map[string]any)}
}

// SetLocal stores a value in Locals, creating the map when the Ctx was built
// without NewCtx. Middleware should use this instead of assigning into Locals
// directly, so a minimally-constructed Ctx cannot panic on a nil map.
func (c *Ctx) SetLocal(key string, value any) {
	if c.Locals == nil {
		c.Locals = make(map[string]any)
	}
	c.Locals[key] = value
}

// GetVersion returns the API version extracted by versioning middleware, or 0 if not set.
func (c *Ctx) GetVersion() int {
	if v, ok := c.Locals["version"].(int); ok {
		return v
	}
	return 0
}

// GetVersionStatus returns the API version status, or VersionStatusActive if not set.
func (c *Ctx) GetVersionStatus() int {
	if v, ok := c.Locals["version_status"].(int); ok {
		return v
	}
	return 0
}

// RespHeader returns the response headers for middleware convenience.
func (c *Ctx) RespHeader() http.Header {
	return c.Response.Header()
}

// Route describes a single route entry.
type Route struct {
	Method  string
	Path    string
	Handler string // name of the controller method to call (e.g. "Greet", "Index")
}

// Provider is implemented by types that can be registered as injectable dependencies.
type Provider interface {
	Provide() any
}

// Controller is implemented by types that expose routes.
type Controller interface {
	Routes() []Route
	Prefix() string
}

// OnBoot is implemented by types that need to perform setup when the application starts.
// Called after all modules are registered and routes are wired, but before the server starts.
type OnBoot interface {
	OnBoot() error
}

// OnShutdown is implemented by types that need to perform cleanup when the application stops.
// Called during graceful shutdown.
type OnShutdown interface {
	OnShutdown() error
}

// OnInit is implemented by types that need to perform initialization before the module starts.
// Called after all dependencies have been resolved but before the server starts.
// Dependencies are guaranteed to be initialized before the dependent module's OnInit runs.
type OnInit interface {
	OnInit() error
}

// Module is the basic unit of application organization.
type Module struct {
	// Name uniquely identifies the module. Used for dependency resolution.
	Name string
	// Depends declares the names of modules that must be initialized before this one.
	Depends         []string
	Controllers     []Controller
	Providers       []Provider
	Middleware      []MiddlewareFunc
	OnInitHooks     []OnInit
	OnBootHooks     []OnBoot
	OnShutdownHooks []OnShutdown
}

func NewModule(name string) *Module {
	return &Module{
		Name:            name,
		Depends:         []string{},
		Controllers:     []Controller{},
		Providers:       []Provider{},
		Middleware:      []MiddlewareFunc{},
		OnInitHooks:     []OnInit{},
		OnBootHooks:     []OnBoot{},
		OnShutdownHooks: []OnShutdown{},
	}
}

// RegisterControllers appends controllers to the module.
func (m *Module) RegisterControllers(ctrls ...Controller) *Module {
	m.Controllers = append(m.Controllers, ctrls...)
	return m
}

// RegisterProviders appends providers to the module.
func (m *Module) RegisterProviders(providers ...Provider) *Module {
	m.Providers = append(m.Providers, providers...)
	return m
}

// RegisterMiddleware appends middleware to the module.
//
// Nil entries are dropped. Middleware constructors in this framework return nil
// when disabled, and the documented usage is to register whatever they return,
// so appending the nil would put a nil in the chain and panic on every request.
func (m *Module) RegisterMiddleware(mw ...MiddlewareFunc) *Module {
	for _, fn := range mw {
		if fn != nil {
			m.Middleware = append(m.Middleware, fn)
		}
	}
	return m
}

// RegisterOnInit appends OnInit hooks to the module.
func (m *Module) RegisterOnInit(hooks ...OnInit) *Module {
	m.OnInitHooks = append(m.OnInitHooks, hooks...)
	return m
}

// RegisterOnBoot appends OnBoot hooks to the module.
func (m *Module) RegisterOnBoot(hooks ...OnBoot) *Module {
	m.OnBootHooks = append(m.OnBootHooks, hooks...)
	return m
}

// RegisterOnShutdown appends OnShutdown hooks to the module.
func (m *Module) RegisterOnShutdown(hooks ...OnShutdown) *Module {
	m.OnShutdownHooks = append(m.OnShutdownHooks, hooks...)
	return m
}

// MiddlewareFunc is the function signature for HTTP middleware.
type MiddlewareFunc func(ctx *Ctx, next Handler) error

// Handler is the function signature for request handlers.
type Handler func(ctx *Ctx) error
