package middleware

import (
	"encoding/json"
	"net/http"
	"sync"

	"github.com/gofault/gofault/core"
)

// OpenAPIConfig holds configuration for the OpenAPI documentation middleware.
type OpenAPIConfig struct {
	// Enabled enables the OpenAPI middleware.
	Enabled bool
	// Path is the URL path for the OpenAPI endpoint.
	Path string
	// Title is the API title.
	Title string
	// Version is the API version.
	Version string
	// Description is the API description.
	Description string
}

// DefaultOpenAPIConfig returns a default OpenAPI configuration.
func DefaultOpenAPIConfig() OpenAPIConfig {
	return OpenAPIConfig{
		Enabled:     true,
		Path:        "/openapi.json",
		Title:       "API",
		Version:     "1.0.0",
		Description: "API Documentation",
	}
}

// OpenAPIDocument represents an OpenAPI 3.0 document.
//
// Paths are guarded by a mutex because the document is served on every request
// while routes are still being registered, and AddPath is typically called from
// a different goroutine than the one serving.
type OpenAPIDocument struct {
	// OpenAPI is the specification version, default "3.0.0".
	OpenAPI string `json:"openapi"`
	// Info holds title, version and description.
	Info map[string]any `json:"info"`
	// Paths maps a path to its per-method operations.
	Paths map[string]any `json:"paths"`
	// Components holds reusable schemas and security definitions.
	Components map[string]any `json:"components,omitempty"`

	mu sync.RWMutex
}

// NewOpenAPIDocument creates a new OpenAPI document.
func NewOpenAPIDocument(title, version, description string) *OpenAPIDocument {
	return &OpenAPIDocument{
		OpenAPI: "3.0.0",
		Info: map[string]any{
			"title":       title,
			"version":     version,
			"description": description,
		},
		Paths:      make(map[string]any),
		Components: make(map[string]any),
	}
}

// AddPath records an operation under path and method.
//
// The previous implementation type-asserted the existing value to
// map[string]any, which panicked whenever a key had been stored by something
// other than this method.
func (d *OpenAPIDocument) AddPath(path string, method string, operation map[string]any) {
	if path == "" || method == "" {
		return
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if d.Paths == nil {
		d.Paths = make(map[string]any)
	}
	existing, ok := d.Paths[path].(map[string]any)
	if !ok || existing == nil {
		existing = make(map[string]any)
		d.Paths[path] = existing
	}
	existing[method] = operation
}

// SetInfo replaces the info block.
func (d *OpenAPIDocument) SetInfo(title, version, description string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.Info = map[string]any{
		"title":       title,
		"version":     version,
		"description": description,
	}
}

// snapshot returns a deep-enough copy for encoding, so a concurrent AddPath
// cannot mutate the map while json.Marshal walks it.
func (d *OpenAPIDocument) snapshot() map[string]any {
	d.mu.RLock()
	defer d.mu.RUnlock()

	paths := make(map[string]any, len(d.Paths))
	for path, methods := range d.Paths {
		if m, ok := methods.(map[string]any); ok {
			copied := make(map[string]any, len(m))
			for k, v := range m {
				copied[k] = v
			}
			paths[path] = copied
		} else {
			paths[path] = methods
		}
	}

	spec := map[string]any{
		"openapi": d.OpenAPI,
		"info":    d.Info,
		"paths":   paths,
	}
	if len(d.Components) > 0 {
		spec["components"] = d.Components
	}
	return spec
}

// OpenAPIHandler serves an OpenAPIDocument at the configured path.
//
// It returns a handler rather than a middleware because the document is a
// terminal endpoint; RegisterOpenAPI wires the route.
func OpenAPIHandler(config OpenAPIConfig, doc *OpenAPIDocument) core.Handler {
	if !config.Enabled {
		return nil
	}
	if doc == nil {
		doc = NewOpenAPIDocument(config.Title, config.Version, config.Description)
	}

	return func(ctx *core.Ctx) error {
		ctx.Response.Header().Set("Content-Type", "application/json")
		ctx.Response.WriteHeader(http.StatusOK)
		return json.NewEncoder(ctx.Response).Encode(doc.snapshot())
	}
}

// OpenAPIMiddleware returns a handler serving an empty OpenAPI document.
//
// Deprecated: it cannot include routes, because it has no access to the router.
// Use RegisterOpenAPI with a document you add paths to, or OpenAPIHandler.
// It is kept so existing wiring keeps compiling.
func OpenAPIMiddleware(config OpenAPIConfig) core.Handler {
	return OpenAPIHandler(config, nil)
}

// RegisterOpenAPI registers the OpenAPI document on the router and returns the
// handler, so callers do not have to repeat the path and method.
func RegisterOpenAPI(rtr OpenAPIRegistrar, config OpenAPIConfig, doc *OpenAPIDocument) (core.Handler, error) {
	handler := OpenAPIHandler(config, doc)
	if handler == nil {
		return nil, nil
	}
	if rtr == nil {
		return handler, nil
	}
	if config.Path == "" {
		config.Path = "/openapi.json"
	}
	rtr.Handle(http.MethodGet, config.Path, handler)
	return handler, nil
}

// OpenAPIRegistrar is the subset of the router that RegisterOpenAPI needs.
// Both router.Router and any test double satisfy it.
type OpenAPIRegistrar interface {
	Handle(method, path string, handler core.Handler, mw ...core.MiddlewareFunc)
}
