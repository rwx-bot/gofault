package middleware

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofault/gofault/core"
)

// CORS wildcard subdomain matching must not accept domains that merely end
// with the target domain. *.example.com must match sub.example.com but reject
// notexample.com and evil-example.com.
func TestCORS_WildcardSubdomainMatching(t *testing.T) {
	cfg := CORSConfig{AllowOrigins: []string{"*.example.com"}}
	mw := CORS(cfg)

	cases := []struct {
		origin string
		want   bool
	}{
		{"sub.example.com", true},
		{"deep.sub.example.com", true},
		{"notexample.com", false},
		{"evil-example.com", false},
		{"example.com", false},
	}

	for _, c := range cases {
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Origin", c.origin)
		w := httptest.NewRecorder()
		ctx := core.NewCtx(w, req)

		if err := mw(ctx, func(ctx *core.Ctx) error { return nil }); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		got := w.Header().Get("Access-Control-Allow-Origin")
		if c.want && got == "" {
			t.Errorf("origin %q: expected allow, got empty", c.origin)
		}
		if !c.want && got != "" {
			t.Errorf("origin %q: expected deny, got %q", c.origin, got)
		}
	}
}

// A preflight request must return 204 No Content, not 200.
func TestCORS_PreflightReturns204(t *testing.T) {
	mw := CORS(DefaultCORSConfig())

	req := httptest.NewRequest("OPTIONS", "/", nil)
	req.Header.Set("Origin", "http://example.com")
	w := httptest.NewRecorder()
	ctx := core.NewCtx(w, req)

	if err := mw(ctx, func(ctx *core.Ctx) error { return nil }); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if w.Code != 204 {
		t.Errorf("preflight status = %d, want 204", w.Code)
	}
}

// X-Forwarded-For may carry a comma-separated proxy chain. The rate limit key
// must be the client IP (first element), not the whole header, or a client can
// rotate the chain to mint a fresh bucket per request.
func TestRateLimit_DefaultKeyFuncUsesClientIP(t *testing.T) {
	rl := NewRateLimiter(RateLimiterConfig{
		RequestsPerSecond: 1,
		BurstSize:         1,
		KeyFunc:           DefaultKeyFunc,
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Forwarded-For", "1.2.3.4, 5.6.7.8")
	if got := rl.config.KeyFunc(&core.Ctx{Request: req}); got != "1.2.3.4" {
		t.Errorf("key = %q, want the first IP 1.2.3.4", got)
	}

	req2 := httptest.NewRequest("GET", "/", nil)
	req2.Header.Set("X-Forwarded-For", "9.9.9.9")
	if got := rl.config.KeyFunc(&core.Ctx{Request: req2}); got != "9.9.9.9" {
		t.Errorf("single IP key = %q, want 9.9.9.9", got)
	}

	req3 := httptest.NewRequest("GET", "/", nil)
	if got := rl.config.KeyFunc(&core.Ctx{Request: req3}); got != req3.RemoteAddr {
		t.Errorf("fallback key = %q, want RemoteAddr %q", got, req3.RemoteAddr)
	}
}

// GetSession must return the session published by SessionMiddleware. It used to
// call a stub that always returned nil.
func TestSession_GetSessionReturnsPublishedSession(t *testing.T) {
	store := NewSessionStore(DefaultSessionConfig())
	mw := SessionMiddleware(store)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	ctx := core.NewCtx(w, req)

	var got *Session
	if err := mw(ctx, func(ctx *core.Ctx) error {
		got = GetSession(ctx)
		return nil
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got == nil {
		t.Fatal("GetSession returned nil inside a SessionMiddleware-protected route")
	}
	if got.ID == "" {
		t.Error("session has an empty ID")
	}
}

// A session cookie must not be issued when the handler fails.
func TestSession_NoCookieOnError(t *testing.T) {
	store := NewSessionStore(DefaultSessionConfig())
	mw := SessionMiddleware(store)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	ctx := core.NewCtx(w, req)

	handlerErr := &sessionTestError{}
	err := mw(ctx, func(ctx *core.Ctx) error { return handlerErr })
	if err != handlerErr {
		t.Fatalf("expected the handler error to propagate, got %v", err)
	}
	if cookies := w.Header().Values("Set-Cookie"); len(cookies) != 0 {
		t.Errorf("Set-Cookie issued on error: %v", cookies)
	}
}

// A handler's own Set-Cookie must survive the session middleware.
func TestSession_PreservesExistingCookies(t *testing.T) {
	store := NewSessionStore(DefaultSessionConfig())
	mw := SessionMiddleware(store)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	ctx := core.NewCtx(w, req)

	if err := mw(ctx, func(ctx *core.Ctx) error {
		ctx.Response.Header().Add("Set-Cookie", "theme=dark")
		return nil
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cookies := w.Header().Values("Set-Cookie")
	if len(cookies) != 2 {
		t.Fatalf("Set-Cookie headers = %v, want 2 (theme + session)", cookies)
	}
}

// A zero PingInterval must disable pinging rather than panic in
// time.NewTicker, which rejects non-positive durations.
func TestWebSocket_ZeroPingIntervalDoesNotPanic(t *testing.T) {
	cfg := DefaultWebSocketConfig()
	cfg.PingInterval = 0

	// The ticker is created inside the middleware closure, so exercise it
	// through a real upgrade against a test server.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mw := WebSocketMiddleware(cfg, WebSocketHandlerFunc{})
		_ = mw(core.NewCtx(w, r), func(ctx *core.Ctx) error { return nil })
	}))
	defer srv.Close()

	// A plain HTTP request fails the upgrade, which is enough to prove the
	// middleware body runs without panicking.
	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp.Body.Close()
}

// normalizePath must collapse dynamic segments. Leaving the raw path in place
// makes every distinct ID a new Prometheus series.
func TestMetrics_NormalizePathCollapsesIDs(t *testing.T) {
	cases := []struct{ in, want string }{
		{"/user/123", "/user/:id"},
		{"/user/123/posts/456", "/user/:id/posts/:id"},
		{"/users", "/users"},
		{"", "/"},
	}

	for _, c := range cases {
		if got := normalizePath(c.in); got != c.want {
			t.Errorf("normalizePath(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// The request logger must actually emit a log entry. It used to build the
// attributes and discard them.
func TestRequestLogger_EmitsLogEntry(t *testing.T) {
	var buf strings.Builder
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})))
	defer slog.SetDefault(prev)

	mw := RequestLoggerMiddleware(DefaultRequestLoggerConfig())

	req := httptest.NewRequest("GET", "/test?foo=bar", nil)
	w := httptest.NewRecorder()
	ctx := core.NewCtx(w, req)

	if err := mw(ctx, func(ctx *core.Ctx) error { return nil }); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if out == "" {
		t.Fatal("RequestLoggerMiddleware produced no log output")
	}
	for _, want := range []string{`path=/test`, `method=GET`} {
		if !strings.Contains(out, want) {
			t.Errorf("log output missing %q: %s", want, out)
		}
	}
}

// readRequestBody must return the body and leave it readable for downstream
// handlers. It used to return an empty string unconditionally.
func TestRequestLogger_ReadsAndRestoresBody(t *testing.T) {
	body := `{"hello":"world"}`
	req := httptest.NewRequest("POST", "/", strings.NewReader(body))
	ctx := core.NewCtx(httptest.NewRecorder(), req)

	if got := readRequestBody(ctx); got != body {
		t.Errorf("readRequestBody = %q, want %q", got, body)
	}

	// The body must still be readable after capture.
	restored, err := io.ReadAll(ctx.Request.Body)
	if err != nil {
		t.Fatalf("read restored body: %v", err)
	}
	if string(restored) != body {
		t.Errorf("restored body = %q, want %q", restored, body)
	}
}

type sessionTestError struct{}

func (e *sessionTestError) Error() string { return "test failure" }
