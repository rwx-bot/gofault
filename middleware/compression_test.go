package middleware

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofault/gofault/core"
)

func TestDefaultCompressionConfig(t *testing.T) {
	config := DefaultCompressionConfig()
	if !config.Enabled {
		t.Error("Expected Enabled to be true")
	}
	if config.Level != gzip.DefaultCompression {
		t.Errorf("Expected Level %d, got %d", gzip.DefaultCompression, config.Level)
	}
	if config.MinSize != 1024 {
		t.Errorf("Expected MinSize 1024, got %d", config.MinSize)
	}
}

func TestCompressionMiddleware_NoAcceptEncoding(t *testing.T) {
	middleware := CompressionMiddleware(DefaultCompressionConfig())

	next := func(ctx *core.Ctx) error {
		ctx.Response.Header().Set("Content-Type", "text/plain")
		ctx.Response.WriteHeader(http.StatusOK)
		ctx.Response.Write([]byte(strings.Repeat("a", 2000)))
		return nil
	}

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	ctx := core.NewCtx(w, req)

	middleware(ctx, next)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	if w.Header().Get("Content-Encoding") == "gzip" {
		t.Error("Expected no Content-Encoding header for missing Accept-Encoding")
	}
}

func TestCompressionMiddleware_Disabled(t *testing.T) {
	config := DefaultCompressionConfig()
	config.Enabled = false

	handler := CompressionMiddleware(config)
	if handler != nil {
		t.Error("Expected nil handler when disabled")
	}
}

func TestCompressCapture_WriteHeader(t *testing.T) {
	w := httptest.NewRecorder()

	capture := &compressCapture{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
	}

	capture.WriteHeader(http.StatusCreated)

	if capture.statusCode != http.StatusCreated {
		t.Errorf("Expected status %d, got %d", http.StatusCreated, capture.statusCode)
	}
}

func TestCompressCapture_Write(t *testing.T) {
	w := httptest.NewRecorder()

	capture := &compressCapture{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
	}

	data := []byte("test data")
	n, err := capture.Write(data)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if n != len(data) {
		t.Errorf("Expected %d bytes, got %d", len(data), n)
	}
	if capture.body.String() != "test data" {
		t.Errorf("Expected body 'test data', got '%s'", capture.body.String())
	}
}

func TestCompressionMiddleware_WithGzip(t *testing.T) {
	middleware := CompressionMiddleware(DefaultCompressionConfig())

	next := func(ctx *core.Ctx) error {
		ctx.Response.Header().Set("Content-Type", "text/plain")
		ctx.Response.WriteHeader(http.StatusOK)
		ctx.Response.Write([]byte(strings.Repeat("x", 2000)))
		return nil
	}

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()
	ctx := core.NewCtx(w, req)

	middleware(ctx, next)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	if w.Header().Get("Content-Encoding") != "gzip" {
		t.Errorf("Expected Content-Encoding 'gzip', got '%s'", w.Header().Get("Content-Encoding"))
	}

	// Verify the body is actually gzip compressed
	reader, err := gzip.NewReader(w.Body)
	if err != nil {
		t.Fatalf("Failed to create gzip reader: %v", err)
	}
	defer reader.Close()

	body, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("Failed to read gzip body: %v", err)
	}

	if len(body) != 2000 {
		t.Errorf("Expected decompressed body length 2000, got %d", len(body))
	}
}

// Flush is provided so streaming handlers keep working through the buffer. It
// had no coverage, so a streaming handler could have silently broken.
func TestCompressCapture_FlushDelegates(t *testing.T) {
	rec := &flushRecorder{header: make(http.Header)}
	capture := newCompressCapture(rec)

	_, _ = capture.Write([]byte("x"))
	capture.Flush()

	if !rec.flushed {
		t.Error("Flush did not reach the underlying writer")
	}
}

// A handler holding the buffered writer as an http.Flusher must be able to
// flush without an error.
func TestCompressCapture_SatisfiesFlusher(t *testing.T) {
	var _ http.Flusher = newCompressCapture(httptest.NewRecorder())
}

type flushRecorder struct {
	header  http.Header
	flushed bool
}

func (f *flushRecorder) Header() http.Header         { return f.header }
func (f *flushRecorder) Write(b []byte) (int, error) { return len(b), nil }
func (f *flushRecorder) WriteHeader(int)             {}
func (f *flushRecorder) Flush()                      { f.flushed = true }
