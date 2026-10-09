// Package middleware provides common HTTP middleware for gofault.
package middleware

import (
	"bytes"
	"compress/gzip"
	"net/http"
	"strings"

	"github.com/gofault/gofault/core"
)

// CompressionConfig holds configuration for the compression middleware.
type CompressionConfig struct {
	// Enabled enables the compression middleware.
	Enabled bool
	// Level is the gzip compression level (1-9, default 6).
	Level int
	// MinSize is the minimum response size to compress (bytes).
	MinSize int
}

// DefaultCompressionConfig returns a default compression configuration.
func DefaultCompressionConfig() CompressionConfig {
	return CompressionConfig{
		Enabled: true,
		Level:   gzip.DefaultCompression,
		MinSize: 1024, // 1KB minimum
	}
}

// compressCapture buffers a handler's output so it can be compressed before
// anything reaches the client.
//
// The body is accumulated in a bytes.Buffer rather than a []byte. Repeatedly
// appending to a slice reallocates and copies everything written so far, which
// made a large response cost O(n^2); bytes.Buffer grows geometrically.
//
// The status code is captured the same way: net/http ignores every WriteHeader
// after the first, so a second call would be silently dropped and the client
// would see a different status than the handler chose.
type compressCapture struct {
	http.ResponseWriter
	statusCode  int
	wroteHeader bool
	body        bytes.Buffer
}

func newCompressCapture(w http.ResponseWriter) *compressCapture {
	return &compressCapture{ResponseWriter: w, statusCode: http.StatusOK}
}

func (r *compressCapture) WriteHeader(statusCode int) {
	if r.wroteHeader {
		return
	}
	r.statusCode = statusCode
	r.wroteHeader = true
}

func (r *compressCapture) Write(data []byte) (int, error) {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}
	// bytes.Buffer.Write never returns an error.
	_, _ = r.body.Write(data)
	return len(data), nil
}

// Flush lets handlers that stream still flush, even though the body is
// buffered. Without it, http.Flusher is unavailable and streaming handlers
// either error or silently buffer everything.
func (r *compressCapture) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// CompressionMiddleware compresses responses using gzip when Accept-Encoding
// contains "gzip".
func CompressionMiddleware(config CompressionConfig) core.MiddlewareFunc {
	if !config.Enabled {
		return nil
	}

	level := config.Level
	if level < gzip.DefaultCompression || level > gzip.BestCompression {
		level = gzip.DefaultCompression
	}

	minSize := config.MinSize
	if minSize < 0 {
		minSize = 0
	}

	return func(ctx *core.Ctx, next core.Handler) error {
		if !acceptsGzip(ctx.Request.Header.Get("Accept-Encoding")) {
			return next(ctx)
		}

		real := ctx.Response
		capture := newCompressCapture(real)
		ctx.Response = capture

		err := next(ctx)

		// When the handler failed, pass its output through untouched. The
		// exception filter still has to write an error response, and having
		// already committed a status line here would make the filter's write
		// a no-op, so the client would receive a 2xx for a failed request.
		if err != nil {
			capture.flushTo(real)
			return err
		}

		// A body is meaningless for these statuses and net/http rejects it, so
		// drop it rather than letting the handler's Write surface as an error.
		if !bodyAllowedForStatus(capture.statusCode) {
			capture.flushTo(real)
			return nil
		}

		if capture.body.Len() >= minSize && capture.body.Len() > 0 {
			gz, gzipErr := gzip.NewWriterLevel(real, level)
			if gzipErr != nil {
				// Fall back to uncompressed rather than dropping the body.
				capture.flushTo(real)
				return err
			}

			// Content-Length no longer applies once the body is compressed, and
			// gzip needs the response to be identified per encoding.
			real.Header().Set("Content-Encoding", "gzip")
			real.Header().Add("Vary", "Accept-Encoding")
			real.Header().Del("Content-Length")

			real.WriteHeader(capture.statusCode)
			if _, writeErr := gz.Write(capture.body.Bytes()); writeErr != nil {
				_ = gz.Close()
				return writeErr
			}
			if closeErr := gz.Close(); closeErr != nil {
				return closeErr
			}
			return nil
		}

		capture.flushTo(real)
		return nil
	}
}

// flushTo writes the captured response to the real writer.
func (r *compressCapture) flushTo(w http.ResponseWriter) {
	// Bodies that must not carry content (204, 304, 1xx) are header-only.
	if bodyAllowedForStatus(r.statusCode) {
		w.WriteHeader(r.statusCode)
		if r.body.Len() > 0 {
			_, _ = w.Write(r.body.Bytes())
		}
		return
	}
	w.WriteHeader(r.statusCode)
}

// bodyAllowedForStatus reports whether a status code permits a body.
// Writing one for 204 or 304 is a protocol violation and some clients abort on it.
func bodyAllowedForStatus(status int) bool {
	switch {
	case status >= 100 && status <= 199:
		return false
	case status == http.StatusNoContent:
		return false
	case status == http.StatusNotModified:
		return false
	}
	return true
}

// acceptsGzip reports whether the client offered gzip. A bare "gzip" substring
// match would also accept encodings such as "x-gzip" or "gzip-extra", so the
// token list is walked instead.
func acceptsGzip(header string) bool {
	for _, part := range strings.Split(header, ",") {
		token := strings.TrimSpace(part)
		if i := strings.IndexByte(token, ';'); i >= 0 {
			// Strip ";q=..." parameters.
			token = strings.TrimSpace(token[:i])
		}
		if strings.EqualFold(token, "gzip") {
			return true
		}
	}
	return false
}
