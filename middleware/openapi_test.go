package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofault/gofault/core"
)

func TestDefaultOpenAPIConfig(t *testing.T) {
	cfg := DefaultOpenAPIConfig()

	if !cfg.Enabled {
		t.Error("Enabled should be true")
	}
	if cfg.Path != "/openapi.json" {
		t.Errorf("Path = %s, want /openapi.json", cfg.Path)
	}
	if cfg.Title != "API" {
		t.Errorf("Title = %s, want API", cfg.Title)
	}
	if cfg.Version != "1.0.0" {
		t.Errorf("Version = %s, want 1.0.0", cfg.Version)
	}
}

func TestOpenAPIMiddleware_Disabled(t *testing.T) {
	cfg := DefaultOpenAPIConfig()
	cfg.Enabled = false

	if handler := OpenAPIMiddleware(cfg); handler != nil {
		t.Error("disabled middleware should return a nil handler")
	}
}

func TestOpenAPIMiddleware_ServesSpec(t *testing.T) {
	cfg := DefaultOpenAPIConfig()

	handler := OpenAPIMiddleware(cfg)

	req := httptest.NewRequest("GET", "/openapi.json", nil)
	rec := httptest.NewRecorder()
	ctx := core.NewCtx(rec, req)

	if err := handler(ctx); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Errorf("status code = %d, want %d", rec.Code, http.StatusOK)
	}

	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %s, want application/json", ct)
	}

	var spec OpenAPIDocument
	if err := json.Unmarshal(rec.Body.Bytes(), &spec); err != nil {
		t.Fatalf("failed to unmarshal spec: %v", err)
	}

	if spec.OpenAPI != "3.0.0" {
		t.Errorf("openapi = %s, want 3.0.0", spec.OpenAPI)
	}
	if spec.Info["title"] != cfg.Title {
		t.Errorf("info.title = %v, want %s", spec.Info["title"], cfg.Title)
	}
	if spec.Info["version"] != cfg.Version {
		t.Errorf("info.version = %v, want %s", spec.Info["version"], cfg.Version)
	}
}

func TestOpenAPIMiddleware_EmptyPathFallsBack(t *testing.T) {
	cfg := DefaultOpenAPIConfig()
	cfg.Path = ""

	handler := OpenAPIMiddleware(cfg)
	if handler == nil {
		t.Fatal("middleware should not be nil when Enabled is true")
	}

	req := httptest.NewRequest("GET", "/openapi.json", nil)
	rec := httptest.NewRecorder()
	ctx := core.NewCtx(rec, req)

	if err := handler(ctx); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Errorf("status code = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestNewOpenAPIDocument(t *testing.T) {
	doc := NewOpenAPIDocument("Example API", "2.0.0", "An example service")

	if doc.OpenAPI != "3.0.0" {
		t.Errorf("OpenAPI = %s, want 3.0.0", doc.OpenAPI)
	}
	if doc.Info["title"] != "Example API" {
		t.Errorf("info.title = %v, want Example API", doc.Info["title"])
	}
	if doc.Paths == nil {
		t.Error("Paths should be initialised")
	}
	if doc.Components == nil {
		t.Error("Components should be initialised")
	}
}

func TestOpenAPIDocument_AddPath(t *testing.T) {
	doc := NewOpenAPIDocument("Example API", "2.0.0", "An example service")
	op := map[string]any{"summary": "List users"}

	doc.AddPath("/users", "get", op)
	doc.AddPath("/users", "post", map[string]any{"summary": "Create user"})

	paths, ok := doc.Paths["/users"].(map[string]any)
	if !ok {
		t.Fatalf("Paths[/users] has type %T, want map[string]any", doc.Paths["/users"])
	}
	if len(paths) != 2 {
		t.Errorf("methods on /users = %d, want 2", len(paths))
	}
	if paths["get"] == nil {
		t.Error("GET operation was not stored")
	}
}

func TestOpenAPIDocument_AddPath_LazyInit(t *testing.T) {
	doc := &OpenAPIDocument{OpenAPI: "3.0.0"}

	doc.AddPath("/health", "get", map[string]any{"summary": "Health probe"})

	if len(doc.Paths) != 1 {
		t.Errorf("Paths count = %d, want 1", len(doc.Paths))
	}
}
