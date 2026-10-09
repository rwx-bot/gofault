package middleware

import (
	"compress/gzip"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gofault/gofault/core"
)

// The document used to be built once with an empty paths map, and nothing ever
// called AddPath, so the published spec never described a single route.
func TestOpenAPI_ServesRegisteredPaths(t *testing.T) {
	doc := NewOpenAPIDocument("Demo API", "1.2.3", "demo")
	doc.AddPath("/users", http.MethodGet, map[string]any{
		"summary": "List users",
		"responses": map[string]any{
			"200": map[string]any{"description": "ok"},
		},
	})
	doc.AddPath("/users/:id", http.MethodGet, map[string]any{"summary": "Get user"})

	cfg := DefaultOpenAPIConfig()
	handler := OpenAPIHandler(cfg, doc)

	req := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	w := httptest.NewRecorder()
	ctx := core.NewCtx(w, req)

	if err := handler(ctx); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	var spec struct {
		OpenAPI string                    `json:"openapi"`
		Info    map[string]any            `json:"info"`
		Paths   map[string]map[string]any `json:"paths"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &spec); err != nil {
		t.Fatalf("decode spec: %v", err)
	}

	if len(spec.Paths) != 2 {
		t.Fatalf("paths = %v, want 2 entries", spec.Paths)
	}
	if _, ok := spec.Paths["/users"][http.MethodGet]; !ok {
		t.Errorf("GET /users missing from spec: %v", spec.Paths)
	}
	if _, ok := spec.Paths["/users/:id"][http.MethodGet]; !ok {
		t.Errorf("parameterised path missing from spec: %v", spec.Paths)
	}
	if spec.Info["title"] != "Demo API" {
		t.Errorf("info.title = %v, want Demo API", spec.Info["title"])
	}
}

// AddPath used to type-assert the stored value, panicking if a key had been
// written by anything other than AddPath.
func TestOpenAPI_AddPathToleratesForeignValues(t *testing.T) {
	doc := NewOpenAPIDocument("t", "1", "")

	// Simulate a value written directly, bypassing AddPath.
	doc.Paths["/injected"] = "not a map"

	doc.AddPath("/injected", http.MethodGet, map[string]any{"summary": "ok"})

	methods, ok := doc.Paths["/injected"].(map[string]any)
	if !ok {
		t.Fatalf("AddPath did not replace a non-map value, got %T", doc.Paths["/injected"])
	}
	if _, ok := methods[http.MethodGet]; !ok {
		t.Error("operation was not recorded")
	}
}

func TestOpenAPI_AddPathIgnoresEmptyArgs(t *testing.T) {
	doc := NewOpenAPIDocument("t", "1", "")
	doc.AddPath("", http.MethodGet, nil)
	doc.AddPath("/x", "", nil)

	if len(doc.Paths) != 0 {
		t.Errorf("empty arguments should be ignored, got %v", doc.Paths)
	}
}

// The document is served on every request while routes may still be registered,
// so concurrent AddPath and snapshot must not race or panic.
func TestOpenAPI_ConcurrentAddPathAndServe(t *testing.T) {
	doc := NewOpenAPIDocument("t", "1", "")
	handler := OpenAPIHandler(DefaultOpenAPIConfig(), doc)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			doc.AddPath("/resource/"+string(rune('a'+i%26)), http.MethodGet, map[string]any{"i": i})
		}(i)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
			w := httptest.NewRecorder()
			_ = handler(core.NewCtx(w, req))
		}()
	}
	wg.Wait()
}

func TestOpenAPI_SetInfo(t *testing.T) {
	doc := NewOpenAPIDocument("old", "0.1", "old desc")
	doc.SetInfo("New", "2.0.0", "new desc")

	if doc.Info["title"] != "New" || doc.Info["version"] != "2.0.0" {
		t.Errorf("info not updated: %v", doc.Info)
	}
}

// RegisterOpenAPI should wire the route so callers do not repeat path and method.
func TestRegisterOpenAPI_WiresRoute(t *testing.T) {
	rtr := &fakeRegistrar{}
	doc := NewOpenAPIDocument("t", "1", "")
	doc.AddPath("/ping", http.MethodGet, map[string]any{"summary": "ping"})

	handler, err := RegisterOpenAPI(rtr, DefaultOpenAPIConfig(), doc)
	if err != nil {
		t.Fatalf("RegisterOpenAPI: %v", err)
	}
	if handler == nil {
		t.Fatal("no handler returned")
	}
	if rtr.method != http.MethodGet || rtr.path != "/openapi.json" {
		t.Errorf("registered %s %s, want GET /openapi.json", rtr.method, rtr.path)
	}

	w := httptest.NewRecorder()
	if err := rtr.handler(core.NewCtx(w, httptest.NewRequest(http.MethodGet, "/openapi.json", nil))); err != nil {
		t.Fatalf("serving registered route: %v", err)
	}
	if !strings.Contains(w.Body.String(), "/ping") {
		t.Errorf("served document missing the route: %s", w.Body.String())
	}
}

func TestRegisterOpenAPI_Disabled(t *testing.T) {
	rtr := &fakeRegistrar{}
	cfg := DefaultOpenAPIConfig()
	cfg.Enabled = false

	handler, err := RegisterOpenAPI(rtr, cfg, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if handler != nil {
		t.Error("expected no handler when disabled")
	}
	if rtr.path != "" {
		t.Error("a disabled OpenAPI middleware must not register a route")
	}
}

// Capturing the body in a []byte reallocated and copied everything written so
// far on every Write, making a large response O(n^2).
func TestCompression_LargeBodyIsCompressedCorrectly(t *testing.T) {
	cfg := DefaultCompressionConfig()
	cfg.MinSize = 1024
	mw := CompressionMiddleware(cfg)

	chunk := strings.Repeat("abcdefghij", 100) // 1KB
	next := func(ctx *core.Ctx) error {
		ctx.Response.WriteHeader(http.StatusOK)
		// Many small writes, the pattern that made the old append quadratic.
		for i := 0; i < 200; i++ {
			if _, err := ctx.Response.Write([]byte(chunk)); err != nil {
				return err
			}
		}
		return nil
	}

	req := httptest.NewRequest(http.MethodGet, "/big", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()
	ctx := core.NewCtx(w, req)

	if err := mw(ctx, next); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if w.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", w.Header().Get("Content-Encoding"))
	}

	zr, err := gzip.NewReader(w.Body)
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	defer zr.Close()

	var sb strings.Builder
	buf := make([]byte, 32*1024)
	for {
		n, err := zr.Read(buf)
		sb.Write(buf[:n])
		if err != nil {
			break
		}
	}

	want := strings.Repeat(chunk, 200)
	if sb.Len() != len(want) {
		t.Fatalf("decompressed %d bytes, want %d", sb.Len(), len(want))
	}
	if sb.String() != want {
		t.Error("decompressed content differs from what was written")
	}
}

// Compressing a failed response would commit a 2xx status line, leaving the
// exception filter's error write a no-op.
func TestCompression_HandlerErrorPassesThrough(t *testing.T) {
	cfg := DefaultCompressionConfig()
	cfg.MinSize = 1
	mw := CompressionMiddleware(cfg)

	handlerErr := errors.New("handler failed")
	next := func(ctx *core.Ctx) error {
		ctx.Response.WriteHeader(http.StatusOK)
		ctx.Response.Write([]byte(strings.Repeat("x", 2048)))
		return handlerErr
	}

	req := httptest.NewRequest(http.MethodGet, "/fail", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()
	ctx := core.NewCtx(w, req)

	err := mw(ctx, next)
	if !errors.Is(err, handlerErr) {
		t.Fatalf("error = %v, want the handler error to propagate", err)
	}
	if w.Header().Get("Content-Encoding") == "gzip" {
		t.Error("a failed response must not be compressed, since its status is not final")
	}
}

// A 204 must not carry a body.
func TestCompression_NoBodyForBodylessStatuses(t *testing.T) {
	for _, status := range []int{http.StatusNoContent, http.StatusNotModified} {
		cfg := DefaultCompressionConfig()
		cfg.MinSize = 0
		mw := CompressionMiddleware(cfg)

		next := func(ctx *core.Ctx) error {
			ctx.Response.WriteHeader(status)
			ctx.Response.Write([]byte("should not appear"))
			return nil
		}

		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req.Header.Set("Accept-Encoding", "gzip")
		w := httptest.NewRecorder()
		ctx := core.NewCtx(w, req)

		if err := mw(ctx, next); err != nil {
			t.Fatalf("status %d: %v", status, err)
		}
		// httptest.ResponseRecorder rejects a body for 204 outright, so
		// reaching here at all proves none was written.
		if w.Body.Len() != 0 {
			t.Errorf("status %d: body = %q, want empty", status, w.Body.String())
		}
	}
}

// A bare substring match accepted encodings such as "x-gzip".
func TestAcceptsGzip(t *testing.T) {
	cases := []struct {
		header string
		want   bool
	}{
		{"gzip", true},
		{"gzip, deflate", true},
		{"deflate, gzip;q=1.0", true},
		{"GZIP", true},
		{"deflate", false},
		{"", false},
		{"x-gzip", false},
		{"gzipped", false},
	}
	for _, c := range cases {
		if got := acceptsGzip(c.header); got != c.want {
			t.Errorf("acceptsGzip(%q) = %v, want %v", c.header, got, c.want)
		}
	}
}

// Only the first WriteHeader counts; net/http ignores the rest.
func TestCompressCapture_FirstStatusWins(t *testing.T) {
	w := httptest.NewRecorder()
	capture := newCompressCapture(w)

	capture.WriteHeader(http.StatusCreated)
	capture.WriteHeader(http.StatusTeapot)

	if capture.statusCode != http.StatusCreated {
		t.Errorf("statusCode = %d, want %d", capture.statusCode, http.StatusCreated)
	}
}

func TestCompressCapture_ImplicitStatusOnWrite(t *testing.T) {
	w := httptest.NewRecorder()
	capture := newCompressCapture(w)

	if _, err := capture.Write([]byte("x")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if capture.statusCode != http.StatusOK {
		t.Errorf("statusCode = %d, want 200 when Write precedes WriteHeader", capture.statusCode)
	}
	if !capture.wroteHeader {
		t.Error("Write should mark the header as written")
	}
}

// A panicking checker must not take down the health endpoint.
func TestHealthCheck_PanickingCheckerIsReportedDown(t *testing.T) {
	cfg := DefaultHealthCheckConfig()
	cfg.RegisterHealthCheck("bad", func() HealthCheck {
		panic("checker exploded")
	})
	cfg.RegisterHealthCheck("good", func() HealthCheck {
		return HealthCheck{Status: HealthStatusUp}
	})

	handler := HealthCheckHandler(cfg)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	ctx := core.NewCtx(w, req)

	if err := handler(ctx); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", w.Code)
	}

	var resp HealthCheckResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Checks["good"].Status != HealthStatusUp {
		t.Error("a healthy checker should still report up")
	}
	if resp.Checks["bad"].Status != HealthStatusDown {
		t.Error("a panicking checker should be reported down, not crash the endpoint")
	}
	if !strings.Contains(resp.Checks["bad"].Message, "panicked") {
		t.Errorf("message = %q, want it to mention the panic", resp.Checks["bad"].Message)
	}
}

// Registering checks while the endpoint is serving must not race.
func TestHealthCheck_ConcurrentRegistrationAndScrape(t *testing.T) {
	cfg := DefaultHealthCheckConfig()
	cfg.RegisterHealthCheck("seed", func() HealthCheck { return HealthCheck{Status: HealthStatusUp} })
	handler := HealthCheckHandler(cfg)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			cfg.RegisterHealthCheck("check", func() HealthCheck { return HealthCheck{Status: HealthStatusUp} })
		}(i)
		go func() {
			defer wg.Done()
			w := httptest.NewRecorder()
			_ = handler(core.NewCtx(w, httptest.NewRequest(http.MethodGet, "/health", nil)))
		}()
	}
	wg.Wait()
}

func TestRegisterHealthCheck(t *testing.T) {
	rtr := &fakeRegistrar{}
	cfg := DefaultHealthCheckConfig()
	cfg.RegisterHealthCheck("db", func() HealthCheck { return HealthCheck{Status: HealthStatusUp} })

	handler := RegisterHealthCheckEndpoint(rtr, cfg)
	if handler == nil {
		t.Fatal("no handler returned")
	}
	if rtr.method != http.MethodGet || rtr.path != "/health" {
		t.Errorf("registered %s %s, want GET /health", rtr.method, rtr.path)
	}
}

func TestLatencyHealthCheck_NilFunc(t *testing.T) {
	check := LatencyHealthCheck("db", nil)()
	if check.Status != HealthStatusDown {
		t.Errorf("status = %v, want down for a nil check function", check.Status)
	}
}

// fakeRegistrar records the route a helper registers.
type fakeRegistrar struct {
	method  string
	path    string
	handler core.Handler
}

func (r *fakeRegistrar) Handle(method, path string, handler core.Handler, mw ...core.MiddlewareFunc) {
	r.method = method
	r.path = path
	r.handler = handler
}
