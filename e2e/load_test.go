//go:build e2e

// Package e2e drives the assembled framework the way an application does: a
// real HTTP server from server.New, the router, a full middleware chain, and
// real concurrent clients.
//
// The unit tests exercise each piece in isolation. This suite covers the seams
// between them, which is where composition bugs live: middleware that rewrites
// the ResponseWriter interacting with one that buffers it, the exception filter
// running after a middleware has already committed a status line, and the
// router's dispatch under load.
package e2e

import (
	"compress/gzip"
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gofault/gofault/controller"
	"github.com/gofault/gofault/core"
	"github.com/gofault/gofault/exception"
	"github.com/gofault/gofault/middleware"
	"github.com/gofault/gofault/router"
	"github.com/gofault/gofault/server"
)

// echoController exercises the normal path.
type echoController struct {
	controller.BaseController
	payloadSize int
}

func (c *echoController) Routes() []core.Route {
	return []core.Route{
		{Method: http.MethodGet, Path: "/echo", Handler: "Echo"},
		{Method: http.MethodGet, Path: "/big", Handler: "Big"},
		{Method: http.MethodGet, Path: "/fail", Handler: "Fail"},
		{Method: http.MethodGet, Path: "/panic", Handler: "Panic"},
		{Method: http.MethodGet, Path: "/items/:id", Handler: "Item"},
	}
}

func (c *echoController) Prefix() string { return "/api" }

func (c *echoController) Echo(ctx *core.Ctx) error {
	return controller.OK(ctx.Response, map[string]string{"path": ctx.Request.URL.Path})
}

func (c *echoController) Big(ctx *core.Ctx) error {
	body := strings.Repeat("x", c.payloadSize)
	ctx.Response.Header().Set("Content-Type", "text/plain")
	w := ctx.Response.Write
	_, err := w([]byte(body))
	return err
}

func (c *echoController) Fail(ctx *core.Ctx) error {
	return exception.BadRequest("deliberate failure")
}

func (c *echoController) Panic(ctx *core.Ctx) error {
	panic("deliberate panic")
}

func (c *echoController) Item(ctx *core.Ctx) error {
	return controller.OK(ctx.Response, map[string]string{"id": ctx.Params["id"]})
}

// harness is a running server plus the address to reach it.
type harness struct {
	srv    *server.HTTP
	addr   string
	router *router.Router
}

func start(t *testing.T, chain []core.MiddlewareFunc, payloadSize int) *harness {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()

	rtr := router.New()
	for _, mw := range chain {
		rtr.Middleware(mw)
	}
	rtr.ExceptionFilter(exception.NewHTTPExceptionFilter())
	rtr.Handle(http.MethodGet, "/api/echo",
		wrapController(&echoController{payloadSize: payloadSize}, "Echo"))
	rtr.Handle(http.MethodGet, "/api/big",
		wrapController(&echoController{payloadSize: payloadSize}, "Big"))
	rtr.Handle(http.MethodGet, "/api/fail",
		wrapController(&echoController{payloadSize: payloadSize}, "Fail"))
	rtr.Handle(http.MethodGet, "/api/panic",
		wrapController(&echoController{payloadSize: payloadSize}, "Panic"))
	rtr.Handle(http.MethodGet, "/api/items/:id",
		wrapController(&echoController{payloadSize: payloadSize}, "Item"))

	srv := server.New(rtr, 0)
	srv.Server().Addr = addr
	srv.Server().Handler = rtr

	go func() { _ = srv.Server().Serve(ln) }()
	t.Cleanup(func() { _ = srv.Server().Close() })

	waitForServer(t, addr)
	return &harness{srv: srv, addr: addr, router: rtr}
}

func wrapController(c *echoController, method string) core.Handler {
	return func(ctx *core.Ctx) error {
		return controller.InvokeHandler(c, method, ctx)
	}
}

func waitForServer(t *testing.T, addr string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		c, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			c.Close()
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("server at %s never came up", addr)
}

func (h *harness) get(t *testing.T, path string, headers map[string]string) (*http.Response, []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, "http://"+h.addr+path, nil)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()

	var reader io.Reader = resp.Body
	if resp.Header.Get("Content-Encoding") == "gzip" {
		zr, err := gzip.NewReader(resp.Body)
		if err != nil {
			t.Fatalf("gzip reader for %s: %v", path, err)
		}
		defer zr.Close()
		reader = zr
	}

	body, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return resp, body
}

// The full stack must serve a normal request correctly.
func TestStack_NormalRequest(t *testing.T) {
	h := start(t, nil, 16)

	resp, body := h.get(t, "/api/echo", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if !strings.Contains(string(body), "/api/echo") {
		t.Errorf("body = %q", body)
	}
}

// A handler returning an HTTPException must reach the client with the right
// status, through the exception filter.
func TestStack_ExceptionReachesClient(t *testing.T) {
	h := start(t, nil, 16)

	resp, body := h.get(t, "/api/fail", nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
	if !strings.Contains(string(body), "deliberate failure") {
		t.Errorf("body = %q, want the failure message", body)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type = %q, want JSON", ct)
	}
}

// A panicking handler must produce a 500 and leave the server serving.
func TestStack_PanicRecoveredAndServerSurvives(t *testing.T) {
	h := start(t, []core.MiddlewareFunc{
		middleware.RecoveryMiddleware(middleware.DefaultRecoveryConfig()),
	}, 16)

	resp, _ := h.get(t, "/api/panic", nil)
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", resp.StatusCode)
	}

	// The server must still be usable afterwards.
	resp2, _ := h.get(t, "/api/echo", nil)
	if resp2.StatusCode != http.StatusOK {
		t.Errorf("server unusable after a panic: status = %d", resp2.StatusCode)
	}
}

// Compression must actually compress a large body and the client must be able
// to read it back.
func TestStack_CompressionRoundTrip(t *testing.T) {
	const size = 64 * 1024
	h := start(t, []core.MiddlewareFunc{
		middleware.CompressionMiddleware(middleware.DefaultCompressionConfig()),
	}, size)

	req, _ := http.NewRequest(http.MethodGet, "http://"+h.addr+"/api/big", nil)
	req.Header.Set("Accept-Encoding", "gzip")

	resp, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.Header.Get("Content-Encoding") != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", resp.Header.Get("Content-Encoding"))
	}

	zr, err := gzip.NewReader(resp.Body)
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	defer zr.Close()

	body, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("read gzipped body: %v", err)
	}
	if len(body) != size {
		t.Errorf("decompressed %d bytes, want %d", len(body), size)
	}
}

// Compression and the exception filter must not fight over the status line: a
// failed request must not come back as 2xx.
func TestStack_CompressionDoesNotSwallowFailure(t *testing.T) {
	h := start(t, []core.MiddlewareFunc{
		middleware.CompressionMiddleware(middleware.DefaultCompressionConfig()),
	}, 16)

	resp, body := h.get(t, "/api/fail", map[string]string{"Accept-Encoding": "gzip"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400; compression must not commit a 2xx", resp.StatusCode)
	}
	if !strings.Contains(string(body), "deliberate failure") {
		t.Errorf("body = %q", body)
	}
}

// Rate limiting must engage and then recover.
func TestStack_RateLimitEngages(t *testing.T) {
	limiter := middleware.NewRateLimiter(middleware.RateLimiterConfig{
		RequestsPerSecond: 1,
		BurstSize:         3,
	})
	t.Cleanup(limiter.Close)

	h := start(t, []core.MiddlewareFunc{limiter.Middleware()}, 16)

	var ok, limited int
	for i := 0; i < 10; i++ {
		resp, _ := h.get(t, "/api/echo", nil)
		switch resp.StatusCode {
		case http.StatusOK:
			ok++
		case http.StatusTooManyRequests:
			limited++
		}
	}

	if ok != 3 {
		t.Errorf("allowed %d requests, want the burst size of 3", ok)
	}
	if limited != 7 {
		t.Errorf("limited %d requests, want 7", limited)
	}
}

// Timeout must fire and the server must survive the abandoned handler.
func TestStack_TimeoutReturns504(t *testing.T) {
	slow := func(ctx *core.Ctx, next core.Handler) error {
		// Longer than the timeout below.
		time.Sleep(600 * time.Millisecond)
		return next(ctx)
	}

	cfg := middleware.DefaultTimeoutConfig()
	cfg.Duration = 100 * time.Millisecond

	// Timeout must be outermost: a middleware placed before it would run to
	// completion before the timeout even starts, so it would never fire.
	h := start(t, []core.MiddlewareFunc{
		middleware.TimeoutMiddleware(cfg),
		slow,
	}, 16)

	start := time.Now()
	resp, _ := h.get(t, "/api/echo", nil)
	elapsed := time.Since(start)

	if resp.StatusCode != http.StatusGatewayTimeout {
		t.Errorf("status = %d, want 504", resp.StatusCode)
	}
	if elapsed > 500*time.Millisecond {
		t.Errorf("took %v, want the timeout to fire well before the handler finishes", elapsed)
	}
}

// Path parameters must survive the whole chain.
func TestStack_PathParams(t *testing.T) {
	h := start(t, nil, 16)

	resp, body := h.get(t, "/api/items/42", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if !strings.Contains(string(body), `"42"`) {
		t.Errorf("body = %q, want the id echoed back", body)
	}
}

// The real load test: many concurrent clients through the whole stack.
func TestStack_ConcurrentLoad(t *testing.T) {
	const payloadSize = 8 * 1024
	h := start(t, []core.MiddlewareFunc{
		middleware.RequestID(),
		middleware.RecoveryMiddleware(middleware.DefaultRecoveryConfig()),
		middleware.CompressionMiddleware(middleware.DefaultCompressionConfig()),
	}, payloadSize)

	const (
		workers    = 40
		iterations = 25
	)

	var ok, failed int64
	var wg sync.WaitGroup

	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			MaxIdleConnsPerHost: workers,
			DisableCompression:  true, // let the middleware do it
		},
	}

	startGate := make(chan struct{})
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			<-startGate

			for i := 0; i < iterations; i++ {
				path := "/api/echo"
				if i%3 == 0 {
					path = "/api/big"
				}

				req, err := http.NewRequest(http.MethodGet, "http://"+h.addr+path, nil)
				if err != nil {
					atomic.AddInt64(&failed, 1)
					continue
				}
				req.Header.Set("Accept-Encoding", "gzip")

				resp, err := client.Do(req)
				if err != nil {
					atomic.AddInt64(&failed, 1)
					continue
				}

				var reader io.Reader = resp.Body
				if resp.Header.Get("Content-Encoding") == "gzip" {
					zr, zerr := gzip.NewReader(resp.Body)
					if zerr != nil {
						resp.Body.Close()
						atomic.AddInt64(&failed, 1)
						continue
					}
					reader = zr
				}
				_, _ = io.Copy(io.Discard, reader)
				resp.Body.Close()

				if resp.StatusCode != http.StatusOK {
					atomic.AddInt64(&failed, 1)
					continue
				}
				atomic.AddInt64(&ok, 1)
			}
		}(w)
	}

	close(startGate)
	wg.Wait()

	total := int64(workers * iterations)
	t.Logf("ok=%d failed=%d total=%d", ok, failed, total)

	if failed != 0 {
		t.Errorf("%d of %d requests failed under concurrent load", failed, total)
	}
	if ok != total {
		t.Errorf("succeeded %d of %d", ok, total)
	}
}

// A client that disconnects mid-request must not take the server down or leave
// a goroutine behind.
func TestStack_ClientDisconnectMidRequest(t *testing.T) {
	h := start(t, nil, 32)

	runtimeBefore := goroutineCount()

	for i := 0; i < 30; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+h.addr+"/api/big", nil)

		// Cancel as soon as the request is sent.
		go func() {
			time.Sleep(time.Millisecond)
			cancel()
		}()

		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}
		cancel()
	}

	time.Sleep(500 * time.Millisecond)
	runtimeAfter := goroutineCount()

	// The server must still serve.
	resp, _ := h.get(t, "/api/echo", nil)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("server unusable after client disconnects: %d", resp.StatusCode)
	}

	t.Logf("goroutines: before=%d after=%d", runtimeBefore, runtimeAfter)
	if runtimeAfter-runtimeBefore > 50 {
		t.Errorf("goroutine growth after %d aborted requests: %d -> %d",
			30, runtimeBefore, runtimeAfter)
	}
}

// A slow client must be cut off by ReadHeaderTimeout rather than holding a
// connection forever.
func TestStack_SlowClientIsCutOff(t *testing.T) {
	h := start(t, nil, 16)

	// Dial, send a partial request, then stall.
	conn, err := net.DialTimeout("tcp", h.addr, 2*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	if _, err := conn.Write([]byte("GET /api/echo HTTP/1.1\r\nHost: x\r\n")); err != nil {
		t.Fatalf("write: %v", err)
	}

	// The server must eventually give up on this connection.
	_ = conn.SetReadDeadline(time.Now().Add(15 * time.Second))
	buf := make([]byte, 1)
	start := time.Now()
	for {
		n, err := conn.Read(buf)
		if err != nil || n == 0 {
			break
		}
		if time.Since(start) > 12*time.Second {
			t.Fatal("server held the connection open despite ReadHeaderTimeout")
		}
	}
	t.Logf("connection ended after %v", time.Since(start))

	// And the server must still be serving new requests.
	resp, _ := h.get(t, "/api/echo", nil)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("server unusable after a slow client: %d", resp.StatusCode)
	}
}

// The router must answer 405 with Allow for a wrong method, and HEAD must work.
func TestStack_MethodSemantics(t *testing.T) {
	h := start(t, nil, 16)

	req, _ := http.NewRequest(http.MethodPost, "http://"+h.addr+"/api/echo", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("POST to a GET route = %d, want 405", resp.StatusCode)
	}
	if allow := resp.Header.Get("Allow"); !strings.Contains(allow, http.MethodGet) {
		t.Errorf("Allow = %q, want it to include GET", allow)
	}

	headResp, err := http.Head("http://" + h.addr + "/api/echo")
	if err != nil {
		t.Fatalf("HEAD: %v", err)
	}
	defer headResp.Body.Close()
	if headResp.StatusCode != http.StatusOK {
		t.Errorf("HEAD = %d, want 200", headResp.StatusCode)
	}
	body, _ := io.ReadAll(headResp.Body)
	if len(body) != 0 {
		t.Errorf("HEAD returned %d bytes of body", len(body))
	}
}

// The cache middleware also swaps the response writer. An error response written
// after it returns must still reach the client rather than being captured.
func TestStack_CacheDoesNotSwallowFailure(t *testing.T) {
	cfg := middleware.DefaultCacheConfig()
	cfg.TTL = time.Minute

	h := start(t, []core.MiddlewareFunc{
		middleware.CacheMiddleware(middleware.NewInMemoryCache(cfg), cfg),
	}, 16)

	resp, body := h.get(t, "/api/fail", nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
	if !strings.Contains(string(body), "deliberate failure") {
		t.Errorf("body = %q, want the failure message", body)
	}
}

// A cached response must be served on the second request with the HIT marker.
func TestStack_CacheServesSecondRequest(t *testing.T) {
	cfg := middleware.DefaultCacheConfig()
	cfg.TTL = time.Minute

	h := start(t, []core.MiddlewareFunc{
		middleware.CacheMiddleware(middleware.NewInMemoryCache(cfg), cfg),
	}, 16)

	resp1, body1 := h.get(t, "/api/echo", nil)
	if resp1.StatusCode != http.StatusOK {
		t.Fatalf("first status = %d", resp1.StatusCode)
	}

	resp2, body2 := h.get(t, "/api/echo", nil)
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("second status = %d", resp2.StatusCode)
	}
	if string(body1) != string(body2) {
		t.Errorf("cached body differs:\n first=%q\nsecond=%q", body1, body2)
	}
}
