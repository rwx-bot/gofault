// Package router provides HTTP routing with middleware chain support.
package router

import (
	"context"
	"net/http"
	"regexp"
	"strings"
	"sync"

	"github.com/gofault/gofault/core"
	"github.com/gofault/gofault/exception"
	"github.com/gofault/gofault/ioc"
)

// Router matches incoming requests against registered routes and executes the middleware chain.
type Router struct {
	middleware      []core.MiddlewareFunc
	routes          []routeEntry
	exceptionFilter exception.ExceptionFilter

	// container is read on every request but written once by SetContainer,
	// which App.Start calls after routes are wired. Access is guarded so the
	// handoff cannot race with in-flight requests.
	mu        sync.RWMutex
	container *ioc.Container
}

type routeEntry struct {
	method     string
	pattern    *regexp.Regexp
	paramNames []string
	handler    core.Handler
	middleware []core.MiddlewareFunc
}

var _ RouterInterface = (*Router)(nil)

// RouterInterface defines the routing operations exposed to the server.
type RouterInterface interface {
	Handle(method, path string, handler core.Handler, mw ...core.MiddlewareFunc)
	Middleware(mw ...core.MiddlewareFunc)
	ExceptionFilter(f exception.ExceptionFilter)
	ServeHTTP(w http.ResponseWriter, r *http.Request)
}

// New creates a new Router.
func New() *Router {
	return &Router{routes: make([]routeEntry, 0)}
}

// SetContainer binds an IoC container to the router for request scope management.
func (r *Router) SetContainer(c *ioc.Container) {
	r.mu.Lock()
	r.container = c
	r.mu.Unlock()
}

// getContainer returns the bound container, or nil when none is attached.
func (r *Router) getContainer() *ioc.Container {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.container
}

// Middleware appends global middleware to the router.
func (r *Router) Middleware(mw ...core.MiddlewareFunc) {
	r.middleware = append(r.middleware, mw...)
}

// ExceptionFilter sets the exception filter for the router.
func (r *Router) ExceptionFilter(f exception.ExceptionFilter) {
	r.exceptionFilter = f
}

// Handle registers a route with the given method, path pattern, handler, and optional middleware.
func (r *Router) Handle(method, path string, handler core.Handler, mw ...core.MiddlewareFunc) {
	pattern, names := buildPattern(path)
	entry := routeEntry{
		method:     method,
		pattern:    pattern,
		paramNames: names,
		handler:    handler,
		middleware: mw,
	}
	r.routes = append(r.routes, entry)
}

func buildPattern(path string) (*regexp.Regexp, []string) {
	parts := strings.Split(path, "/")
	paramNames := []string{}
	patternParts := []string{""}

	for _, part := range parts[1:] {
		if strings.HasPrefix(part, ":") {
			paramNames = append(paramNames, part[1:])
			patternParts = append(patternParts, "([^/]+)")
		} else {
			patternParts = append(patternParts, regexp.QuoteMeta(part))
		}
	}

	full := strings.Join(patternParts, "/")
	if !strings.HasSuffix(full, "$") {
		full += "$"
	}
	return regexp.MustCompile("^" + full), paramNames
}

// ServeHTTP dispatches to the matching route or returns 404.
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// Begin request scope if a container is attached.
	var reqCtx context.Context
	if c := r.getContainer(); c != nil {
		reqCtx = c.BeginRequest(req.Context())
		req = req.WithContext(reqCtx)
		defer c.EndRequest(reqCtx)
	}

	for _, route := range r.routes {
		if route.method != "" && route.method != req.Method {
			continue
		}
		matches := route.pattern.FindStringSubmatch(req.URL.Path)
		if matches == nil {
			continue
		}

		ctx := core.NewCtx(w, req)
		for i, name := range route.paramNames {
			if i+1 < len(matches) {
				ctx.Params[name] = matches[i+1]
			}
		}

		// Build the chain in a fresh slice. Appending onto r.middleware
		// directly would write into its backing array whenever it has spare
		// capacity, so concurrent requests would overwrite each other's
		// per-route middleware and possibly run the wrong chain.
		chain := make([]core.MiddlewareFunc, 0, len(r.middleware)+len(route.middleware)+1)
		chain = append(chain, r.middleware...)
		chain = append(chain, route.middleware...)
		chain = append(chain, core.MiddlewareFunc(func(ctx *core.Ctx, _ core.Handler) error {
			return route.handler(ctx)
		}))
		err := runChain(ctx, chain, 0)
		if err != nil {
			if r.exceptionFilter != nil {
				r.exceptionFilter.Capture(ctx, err)
			} else {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		}
		return
	}
	http.NotFound(w, req)
}

func runChain(ctx *core.Ctx, chain []core.MiddlewareFunc, index int) error {
	if index >= len(chain) {
		return nil
	}
	return chain[index](ctx, func(c *core.Ctx) error {
		return runChain(c, chain, index+1)
	})
}
