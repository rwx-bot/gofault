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
		// net/http requires HEAD to be served wherever GET is: a HEAD is a GET
		// whose body is discarded. This is checked before the method match
		// below, which would otherwise skip the route and report 405.
		headFromGet := req.Method == http.MethodHead && route.method == http.MethodGet

		if route.method != "" && route.method != req.Method && !headFromGet {
			continue
		}
		matches := route.pattern.FindStringSubmatch(req.URL.Path)
		if matches == nil {
			continue
		}

		r.serve(route, w, req, matches, headFromGet)
		return
	}

	// A path that matches but not with this method is a 405, and the response
	// must advertise what is allowed. Reporting 404 hides the route from
	// clients and from monitoring.
	if allowed := r.allowedMethods(req.URL.Path, req.Method); len(allowed) > 0 {
		w.Header().Set("Allow", strings.Join(allowed, ", "))
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	http.NotFound(w, req)
}

// allowedMethods returns the methods registered for a path, excluding the one
// already tried.
func (r *Router) allowedMethods(path, exclude string) []string {
	seen := make(map[string]bool)
	var allowed []string

	for _, route := range r.routes {
		if route.method == "" || route.method == exclude || seen[route.method] {
			continue
		}
		if route.pattern.FindStringSubmatch(path) == nil {
			continue
		}
		seen[route.method] = true
		allowed = append(allowed, route.method)
	}
	return allowed
}

// serve runs the middleware chain for a matched route. head suppresses the
// response body while keeping the status and headers.
func (r *Router) serve(route routeEntry, w http.ResponseWriter, req *http.Request, matches []string, head bool) {
	ctx := core.NewCtx(w, req)
	for i, name := range route.paramNames {
		if i+1 < len(matches) {
			ctx.Params[name] = matches[i+1]
		}
	}

	sink := &discardWriter{real: w}
	if head {
		ctx.Response = sink
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

	if err := runChain(ctx, chain, 0); err != nil {
		// A filter may decline the error, in which case it must fall through to
		// the default handling. Ignoring the return value left the client with
		// a 200 and an empty body.
		if r.exceptionFilter == nil || !r.exceptionFilter.Capture(ctx, err) {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}

// discardWriter accepts a response body and throws it away, used to answer HEAD.
// Headers and the status line still go to the real writer, because a HEAD
// response is defined to carry the same headers as the GET with no body.
type discardWriter struct {
	real   http.ResponseWriter
	status int
}

func (d *discardWriter) Header() http.Header {
	return d.real.Header()
}

func (d *discardWriter) Write(p []byte) (int, error) { return len(p), nil }

func (d *discardWriter) WriteHeader(status int) {
	if d.status == 0 {
		d.status = status
		d.real.WriteHeader(status)
	}
}

func runChain(ctx *core.Ctx, chain []core.MiddlewareFunc, index int) error {
	if index >= len(chain) {
		return nil
	}
	return chain[index](ctx, func(c *core.Ctx) error {
		return runChain(c, chain, index+1)
	})
}
