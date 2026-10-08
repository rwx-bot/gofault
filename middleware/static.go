package middleware

import (
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gofault/gofault/core"
)

// StaticConfig holds configuration for the static file middleware.
type StaticConfig struct {
	// Dir is the root directory to serve files from.
	Dir string
	// Prefix is the URL path prefix to strip before looking up files.
	Prefix string
	// Index is the default index file name.
	Index string
	// CacheControl sets the Cache-Control header value. Empty string disables.
	CacheControl string
	// Browse enables directory listing.
	Browse bool
	// FollowSymLinks follows symbolic links.
	FollowSymLinks bool
	// ExtraExtensions maps file extensions to custom MIME types.
	ExtraExtensions map[string]string
}

// DefaultStaticConfig returns a default static file configuration.
func DefaultStaticConfig() StaticConfig {
	return StaticConfig{
		Index:          "index.html",
		CacheControl:   "public, max-age=3600",
		Browse:         false,
		FollowSymLinks: false,
	}
}

// StaticMiddleware creates a middleware that serves static files.
func StaticMiddleware(config StaticConfig) core.MiddlewareFunc {
	if config.Index == "" {
		config.Index = "index.html"
	}

	return func(ctx *core.Ctx, next core.Handler) error {
		reqPath := ctx.Request.URL.Path

		// Strip prefix if configured
		if config.Prefix != "" {
			if !strings.HasPrefix(reqPath, config.Prefix) {
				return next(ctx)
			}
			reqPath = strings.TrimPrefix(reqPath, config.Prefix)
			// Ensure we have leading slash
			if !strings.HasPrefix(reqPath, "/") {
				reqPath = "/" + reqPath
			}
		}

		// Resolve file path
		filePath := filepath.Join(config.Dir, filepath.Clean(reqPath))

		// Security: ensure file path is within root directory. A plain
		// HasPrefix check would also accept sibling directories that merely
		// share the root's name (root /var/www would match /var/www-secret),
		// so the root is compared with a trailing separator.
		absDir, err := filepath.Abs(config.Dir)
		if err != nil {
			return next(ctx)
		}
		absPath, err := filepath.Abs(filePath)
		if err != nil {
			return next(ctx)
		}
		if !isWithinRoot(absDir, absPath) {
			return next(ctx)
		}

		// A lexical check cannot see through a symlink: <root>/link -> /etc
		// passes isWithinRoot yet resolves outside the root. os.Stat below
		// follows links, so the real target has to be validated too unless
		// following links was explicitly requested.
		if !config.FollowSymLinks {
			resolved, err := filepath.EvalSymlinks(absPath)
			if err != nil {
				// Broken or unresolvable link: nothing to serve.
				return next(ctx)
			}
			resolvedDir, err := filepath.EvalSymlinks(absDir)
			if err != nil {
				resolvedDir = absDir
			}
			if !isWithinRoot(resolvedDir, resolved) {
				return next(ctx)
			}
		}

		// Check if path is a directory
		info, err := os.Stat(absPath)
		if err != nil {
			if os.IsNotExist(err) {
				return next(ctx)
			}
			return err
		}

		// Handle directory
		if info.IsDir() {
			// Look for index file
			indexPath := filepath.Join(absPath, config.Index)
			if stat, err := os.Stat(indexPath); err == nil && !stat.IsDir() {
				absPath = indexPath
				filePath = filepath.Join(filePath, config.Index)
				info = stat
			} else if config.Browse {
				// Serve directory listing
				entries, err := os.ReadDir(absPath)
				if err != nil {
					return err
				}
				html := DirectoryListing(absPath, entries, reqPath)
				ctx.Response.Header().Set("Content-Type", "text/html; charset=utf-8")
				ctx.Response.WriteHeader(http.StatusOK)
				ctx.Response.Write([]byte(html))
				return nil
			} else {
				// No index and browsing disabled
				return next(ctx)
			}
		}

		// Serve the file
		return serveFile(ctx, absPath, info, config)
	}
}

// isWithinRoot reports whether target is root itself or lives underneath it.
// Both paths must be absolute and already cleaned. The separator is appended
// to root so that a sibling directory sharing the root's name prefix is
// rejected rather than served.
func isWithinRoot(root, target string) bool {
	if root == target {
		return true
	}
	withSep := root
	if !strings.HasSuffix(withSep, string(os.PathSeparator)) {
		withSep += string(os.PathSeparator)
	}
	return strings.HasPrefix(target, withSep)
}

// serveFile writes the file to the response.
func serveFile(ctx *core.Ctx, absPath string, info os.FileInfo, config StaticConfig) error {
	// Set cache control if configured
	if config.CacheControl != "" {
		ctx.Response.Header().Set("Cache-Control", config.CacheControl)
	}

	// Set content type
	contentType := detectContentType(absPath, config)
	if contentType != "" {
		ctx.Response.Header().Set("Content-Type", contentType)
	}

	// Set content length
	ctx.Response.Header().Set("Content-Length", strconv.FormatInt(info.Size(), 10))

	// Handle range requests
	if rangeHeader := ctx.Request.Header.Get("Range"); rangeHeader != "" {
		return serveRangeRequest(ctx, absPath, info, rangeHeader)
	}

	// Open the file
	file, err := os.Open(absPath)
	if err != nil {
		return err
	}
	defer file.Close()

	// Set last modified
	ctx.Response.Header().Set("Last-Modified", info.ModTime().UTC().Format(http.TimeFormat))

	// ServeContent needs the request, name (file name), modtime, and a ReadSeeker
	http.ServeContent(ctx.Response, ctx.Request, filepath.Base(absPath), info.ModTime(), file)
	return nil
}

// serveRangeRequest handles HTTP range requests.
func serveRangeRequest(ctx *core.Ctx, absPath string, info os.FileInfo, rangeHeader string) error {
	// Parse range header (simple parsing, handles "bytes=start-end" format)
	rangePart := strings.TrimPrefix(rangeHeader, "bytes=")
	parts := strings.Split(rangePart, "-")
	if len(parts) != 2 {
		ctx.Response.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
		return nil
	}

	var start, end int64
	if parts[0] == "" {
		// suffix range, e.g., "-500"
		start = info.Size() - parseInt64(parts[1])
		if start < 0 {
			start = 0
		}
		end = info.Size() - 1
	} else {
		start = parseInt64(parts[0])
		if parts[1] == "" {
			end = info.Size() - 1
		} else {
			end = parseInt64(parts[1])
		}
	}

	if start >= info.Size() || end >= info.Size() || start > end {
		ctx.Response.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
		return nil
	}

	file, err := os.Open(absPath)
	if err != nil {
		return err
	}
	defer file.Close()

	// Seek to start
	if _, err := file.Seek(start, os.SEEK_SET); err != nil {
		return err
	}

	// Set headers
	contentLength := end - start + 1
	ctx.Response.Header().Set("Content-Range", formatContentRange(start, end, info.Size()))
	ctx.Response.Header().Set("Content-Length", strconv.FormatInt(contentLength, 10))
	ctx.Response.WriteHeader(http.StatusPartialContent)

	// Copy range
	io.CopyN(ctx.Response, file, contentLength)
	return nil
}

// DirectoryListing generates an HTML directory listing.
func DirectoryListing(dir string, entries []os.DirEntry, reqPath string) string {
	var buf strings.Builder
	buf.WriteString("<!DOCTYPE html><html><head><meta charset=\"utf-8\"><title>Index of ")
	buf.WriteString(escapeHTML(reqPath))
	buf.WriteString("</title><style>")
	buf.WriteString("body{font-family:system-ui;max-width:800px;margin:0 auto;padding:20px}")
	buf.WriteString("h1{border-bottom:1px solid #ccc;padding-bottom:10px}")
	buf.WriteString("a{display:block;padding:5px 0}")
	buf.WriteString("a:hover{background:#f5f5f5}")
	buf.WriteString(".size{color:#666;margin-left:10px}")
	buf.WriteString("</style></head><body>")
	buf.WriteString("<h1>Index of ")
	buf.WriteString(escapeHTML(reqPath))
	buf.WriteString("</h1><hr><pre>")

	// Parent directory link
	if reqPath != "/" {
		buf.WriteString("<a href=\"")
		buf.WriteString(escapeHTML(filepath.Dir(reqPath)))
		buf.WriteString("/\">..</a>\n")
	}

	for _, entry := range entries {
		name := entry.Name()
		href := filepath.Join(reqPath, name)
		if entry.IsDir() {
			href += "/"
		}

		buf.WriteString("<a href=\"")
		buf.WriteString(escapeHTML(href))
		buf.WriteString("\">")
		buf.WriteString(escapeHTML(name))
		if entry.IsDir() {
			buf.WriteString("/")
		}
		buf.WriteString("</a>")
		buf.WriteString("\n")
	}

	buf.WriteString("</pre><hr></body></html>")
	return buf.String()
}

// detectContentType determines the MIME type for a file.
func detectContentType(filePath string, config StaticConfig) string {
	ext := filepath.Ext(filePath)

	// Check extra extensions first
	if config.ExtraExtensions != nil {
		if mime, ok := config.ExtraExtensions[ext]; ok {
			return mime
		}
	}

	// Use standard library
	if mime := mime.TypeByExtension(ext); mime != "" {
		return mime
	}

	return "application/octet-stream"
}

// formatContentRange formats a Content-Range header value.
func formatContentRange(start, end, total int64) string {
	return strconv.FormatInt(start, 10) + "-" + strconv.FormatInt(end, 10) + "/" + strconv.FormatInt(total, 10)
}

// parseInt64 parses a string to int64, returns 0 on error.
func parseInt64(s string) int64 {
	var n int64
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int64(c-'0')
	}
	return n
}

// escapeHTML escapes HTML special characters.
func escapeHTML(s string) string {
	var buf strings.Builder
	for _, c := range s {
		switch c {
		case '<':
			buf.WriteString("&lt;")
		case '>':
			buf.WriteString("&gt;")
		case '&':
			buf.WriteString("&amp;")
		case '"':
			buf.WriteString("&quot;")
		default:
			buf.WriteRune(c)
		}
	}
	return buf.String()
}
