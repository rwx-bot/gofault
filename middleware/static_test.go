package middleware

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gofault/gofault/core"
)

func TestStaticMiddleware_ServeFile(t *testing.T) {
	// Create temp directory and file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	os.WriteFile(testFile, []byte("Hello, World!"), 0644)

	config := StaticConfig{
		Dir:          tmpDir,
		CacheControl: "public, max-age=3600",
	}

	mw := StaticMiddleware(config)

	// Create a request
	req := httptest.NewRequest("GET", "/test.txt", nil)
	w := httptest.NewRecorder()
	ctx := core.NewCtx(w, req)

	err := mw(ctx, func(ctx *core.Ctx) error {
		t.Error("next handler should not be called for existing file")
		return nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	if w.Body.String() != "Hello, World!" {
		t.Errorf("expected body 'Hello, World!', got '%s'", w.Body.String())
	}

	if ct := w.Header().Get("Content-Type"); ct != "text/plain; charset=utf-8" {
		t.Errorf("expected Content-Type 'text/plain; charset=utf-8', got '%s'", ct)
	}

	if cc := w.Header().Get("Cache-Control"); cc != "public, max-age=3600" {
		t.Errorf("expected Cache-Control 'public, max-age=3600', got '%s'", cc)
	}
}

func TestStaticMiddleware_DirectoryIndex(t *testing.T) {
	tmpDir := t.TempDir()
	os.MkdirAll(filepath.Join(tmpDir, "subdir"), 0755)
	os.WriteFile(filepath.Join(tmpDir, "subdir", "index.html"), []byte("<h1>Index</h1>"), 0644)

	config := StaticConfig{
		Dir:   tmpDir,
		Index: "index.html",
	}

	mw := StaticMiddleware(config)

	req := httptest.NewRequest("GET", "/subdir", nil)
	w := httptest.NewRecorder()
	ctx := core.NewCtx(w, req)

	err := mw(ctx, func(ctx *core.Ctx) error {
		t.Error("next should not be called")
		return nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	if !strings.Contains(w.Body.String(), "<h1>Index</h1>") {
		t.Errorf("expected index content, got '%s'", w.Body.String())
	}
}

func TestStaticMiddleware_NotFound_PassesToNext(t *testing.T) {
	tmpDir := t.TempDir()
	config := StaticConfig{Dir: tmpDir}
	mw := StaticMiddleware(config)

	req := httptest.NewRequest("GET", "/nonexistent", nil)
	w := httptest.NewRecorder()
	ctx := core.NewCtx(w, req)

	nextCalled := false
	_ = mw(ctx, func(ctx *core.Ctx) error {
		nextCalled = true
		ctx.Response.WriteHeader(http.StatusNotFound)
		return nil
	})

	if !nextCalled {
		t.Error("next handler should be called for non-existent file")
	}
}

func TestStaticMiddleware_Prefix_Stripped(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "file.txt"), []byte("prefixed"), 0644)

	config := StaticConfig{
		Dir:    tmpDir,
		Prefix: "/static",
	}

	mw := StaticMiddleware(config)

	req := httptest.NewRequest("GET", "/static/file.txt", nil)
	w := httptest.NewRecorder()
	ctx := core.NewCtx(w, req)

	err := mw(ctx, func(ctx *core.Ctx) error {
		t.Error("next should not be called")
		return nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	if w.Body.String() != "prefixed" {
		t.Errorf("expected 'prefixed', got '%s'", w.Body.String())
	}
}

func TestStaticMiddleware_DirectoryListing(t *testing.T) {
	tmpDir := t.TempDir()
	os.MkdirAll(filepath.Join(tmpDir, "mydir"), 0755)
	os.WriteFile(filepath.Join(tmpDir, "mydir", "file.txt"), []byte("content"), 0644)

	config := StaticConfig{
		Dir:    tmpDir,
		Browse: true,
	}

	mw := StaticMiddleware(config)

	req := httptest.NewRequest("GET", "/mydir", nil)
	w := httptest.NewRecorder()
	ctx := core.NewCtx(w, req)

	err := mw(ctx, func(ctx *core.Ctx) error {
		t.Error("next should not be called for directory with Browse enabled")
		return nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "Index of /mydir") {
		t.Errorf("expected directory listing, got '%s'", body)
	}

	if !strings.Contains(body, "file.txt") {
		t.Errorf("expected file.txt in listing, got '%s'", body)
	}

	if ct := w.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Errorf("expected Content-Type 'text/html; charset=utf-8', got '%s'", ct)
	}
}

func TestStaticMiddleware_NoCacheControl(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "file.txt"), []byte("nocache"), 0644)

	config := StaticConfig{
		Dir:          tmpDir,
		CacheControl: "", // disabled
	}

	mw := StaticMiddleware(config)

	req := httptest.NewRequest("GET", "/file.txt", nil)
	w := httptest.NewRecorder()
	ctx := core.NewCtx(w, req)

	err := mw(ctx, func(ctx *core.Ctx) error { return nil })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cc := w.Header().Get("Cache-Control"); cc != "" {
		t.Errorf("expected no Cache-Control, got '%s'", cc)
	}
}

func TestStaticMiddleware_RangeRequest(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "range.txt"), []byte("0123456789"), 0644)

	config := StaticConfig{Dir: tmpDir}
	mw := StaticMiddleware(config)

	req := httptest.NewRequest("GET", "/range.txt", nil)
	req.Header.Set("Range", "bytes=0-4")
	w := httptest.NewRecorder()
	ctx := core.NewCtx(w, req)

	err := mw(ctx, func(ctx *core.Ctx) error { return nil })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if w.Code != http.StatusPartialContent {
		t.Errorf("expected status 206, got %d", w.Code)
	}

	if cr := w.Header().Get("Content-Range"); cr != "0-4/10" {
		t.Errorf("expected Content-Range '0-4/10', got '%s'", cr)
	}

	if w.Body.String() != "01234" {
		t.Errorf("expected '01234', got '%s'", w.Body.String())
	}
}

func TestStaticMiddleware_RangeRequest_Suffix(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "suffix.txt"), []byte("0123456789"), 0644)

	config := StaticConfig{Dir: tmpDir}
	mw := StaticMiddleware(config)

	req := httptest.NewRequest("GET", "/suffix.txt", nil)
	req.Header.Set("Range", "bytes=-5")
	w := httptest.NewRecorder()
	ctx := core.NewCtx(w, req)

	err := mw(ctx, func(ctx *core.Ctx) error { return nil })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if w.Code != http.StatusPartialContent {
		t.Errorf("expected status 206, got %d", w.Code)
	}

	if w.Body.String() != "56789" {
		t.Errorf("expected '56789', got '%s'", w.Body.String())
	}
}

func TestStaticMiddleware_RangeRequest_Unsatisfiable(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "file.txt"), []byte("0123456789"), 0644)

	config := StaticConfig{Dir: tmpDir}
	mw := StaticMiddleware(config)

	req := httptest.NewRequest("GET", "/file.txt", nil)
	req.Header.Set("Range", "bytes=500-600")
	w := httptest.NewRecorder()
	ctx := core.NewCtx(w, req)

	err := mw(ctx, func(ctx *core.Ctx) error { return nil })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if w.Code != http.StatusRequestedRangeNotSatisfiable {
		t.Errorf("expected status 416, got %d", w.Code)
	}
}

func TestStaticMiddleware_ExtraExtensions(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "file.xyz"), []byte("custom"), 0644)

	config := StaticConfig{
		Dir: tmpDir,
		ExtraExtensions: map[string]string{
			".xyz": "application/custom",
		},
	}

	mw := StaticMiddleware(config)

	req := httptest.NewRequest("GET", "/file.xyz", nil)
	w := httptest.NewRecorder()
	ctx := core.NewCtx(w, req)

	err := mw(ctx, func(ctx *core.Ctx) error { return nil })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ct := w.Header().Get("Content-Type"); ct != "application/custom" {
		t.Errorf("expected Content-Type 'application/custom', got '%s'", ct)
	}
}

func TestStaticMiddleware_Security_PathTraversal(t *testing.T) {
	tmpDir := t.TempDir()
	os.MkdirAll(filepath.Join(tmpDir, "allowed"), 0755)
	os.WriteFile(filepath.Join(tmpDir, "allowed", "file.txt"), []byte("allowed"), 0644)

	// Create a "forbidden" directory outside tmpDir
	forbiddenDir := filepath.Join(tmpDir, "..", "forbidden")
	os.MkdirAll(forbiddenDir, 0755)
	forbiddenFile := filepath.Join(forbiddenDir, "secret.txt")
	os.WriteFile(forbiddenFile, []byte("secret"), 0644)
	defer os.RemoveAll(forbiddenDir)

	config := StaticConfig{Dir: tmpDir}
	mw := StaticMiddleware(config)

	// Try to access file outside root via path traversal
	req := httptest.NewRequest("GET", "/../forbidden/secret.txt", nil)
	w := httptest.NewRecorder()
	ctx := core.NewCtx(w, req)

	_ = mw(ctx, func(ctx *core.Ctx) error {
		// Next should be called because path is invalid
		ctx.Response.WriteHeader(http.StatusNotFound)
		return nil
	})

	// Should not have served the forbidden file
	if w.Code == http.StatusOK && w.Body.String() == "secret" {
		t.Error("path traversal should be blocked")
	}
}

func TestStaticMiddleware_HtmlContentType(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "page.html"), []byte("<html></html>"), 0644)

	config := StaticConfig{Dir: tmpDir}
	mw := StaticMiddleware(config)

	req := httptest.NewRequest("GET", "/page.html", nil)
	w := httptest.NewRecorder()
	ctx := core.NewCtx(w, req)

	err := mw(ctx, func(ctx *core.Ctx) error { return nil })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ct := w.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Errorf("expected 'text/html; charset=utf-8', got '%s'", ct)
	}
}

func TestStaticMiddleware_DefaultIndex(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "index.html"), []byte("default index"), 0644)

	config := StaticConfig{
		Dir:   tmpDir,
		Index: "index.html",
	}

	mw := StaticMiddleware(config)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	ctx := core.NewCtx(w, req)

	err := mw(ctx, func(ctx *core.Ctx) error {
		t.Error("next should not be called")
		return nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	if w.Body.String() != "default index" {
		t.Errorf("expected 'default index', got '%s'", w.Body.String())
	}
}

func TestDirectoryListing(t *testing.T) {
	tmpDir := t.TempDir()
	os.MkdirAll(filepath.Join(tmpDir, "dir"), 0755)
	os.WriteFile(filepath.Join(tmpDir, "dir", "file.txt"), []byte("content"), 0644)

	entries, err := os.ReadDir(filepath.Join(tmpDir, "dir"))
	if err != nil {
		t.Fatal(err)
	}

	html := DirectoryListing(filepath.Join(tmpDir, "dir"), entries, "/dir/")

	if !strings.Contains(html, "Index of /dir/") {
		t.Error("expected Index of /dir/")
	}

	if !strings.Contains(html, "file.txt") {
		t.Error("expected file.txt in listing")
	}

	// Should have parent directory link
	if !strings.Contains(html, "..") {
		t.Error("expected parent directory link")
	}
}

func TestEscapeHTML(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"<script>", "&lt;script&gt;"},
		{"a & b", "a &amp; b"},
		{`quote "test"`, "quote &quot;test&quot;"},
		{"normal", "normal"},
	}

	for _, tt := range tests {
		result := escapeHTML(tt.input)
		if result != tt.expected {
			t.Errorf("escapeHTML(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestParseInt64(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
	}{
		{"123", 123},
		{"", 0},
		{"abc", 0},
		{"50", 50},
	}

	for _, tt := range tests {
		result := parseInt64(tt.input)
		if result != tt.expected {
			t.Errorf("parseInt64(%q) = %d, want %d", tt.input, result, tt.expected)
		}
	}
}

func TestDetectContentType(t *testing.T) {
	tests := []struct {
		ext  string
		mime string
	}{
		{".html", "text/html; charset=utf-8"},
		{".css", "text/css; charset=utf-8"},
		{".js", "text/javascript; charset=utf-8"},
		{".json", "application/json"},
		{".png", "image/png"},
		{".jpg", "image/jpeg"},
		{".txt", "text/plain; charset=utf-8"},
		{".xyz", "application/custom"}, // ExtraExtensions takes priority
	}

	for _, tt := range tests {
		config := StaticConfig{
			ExtraExtensions: map[string]string{".xyz": "application/custom"},
		}
		result := detectContentType("/path/file"+tt.ext, config)
		if result != tt.mime {
			t.Errorf("detectContentType(%q) = %q, want %q", tt.ext, result, tt.mime)
		}
	}
}
