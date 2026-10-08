package middleware

import (
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gofault/gofault/core"
)

// UploadConfig holds configuration for the upload middleware.
type UploadConfig struct {
	// Enabled enables the upload middleware.
	Enabled bool
	// MaxSize is the maximum file size in bytes (0 = unlimited).
	MaxSize int64
	// AllowedTypes is a list of allowed MIME types. If empty, all types are allowed.
	AllowedTypes []string
	// AllowedExtensions is a list of allowed file extensions (case-insensitive).
	AllowedExtensions []string
	// Storage is the storage backend for uploaded files.
	Storage StorageBackend
	// FieldName is the form field name for file uploads (default: "file").
	FieldName string
	// SkipFunc returns true to skip uploading for a given request.
	SkipFunc func(*core.Ctx) bool
}

// DefaultUploadConfig returns a default upload configuration.
func DefaultUploadConfig() UploadConfig {
	return UploadConfig{
		Enabled:           true,
		MaxSize:           10 * 1024 * 1024, // 10MB
		AllowedTypes:      nil,
		AllowedExtensions: nil,
		Storage:           nil, // must be set by caller
		FieldName:         "file",
		SkipFunc:          nil,
	}
}

// FileInfo holds information about an uploaded file.
type FileInfo struct {
	// FieldName is the form field name.
	FieldName string
	// FileName is the original file name.
	FileName string
	// Size is the file size in bytes.
	Size int64
	// ContentType is the MIME type.
	ContentType string
	// StoredPath is the path where the file was stored.
	StoredPath string
	// Extension is the file extension.
	Extension string
}

// StorageBackend is the interface for file storage backends.
type StorageBackend interface {
	// Store saves a file and returns its stored path.
	Store(file *multipart.FileHeader, dir string) (string, error)
	// Delete removes a stored file.
	Delete(path string) error
}

// LocalStorage implements StorageBackend using the local filesystem.
type LocalStorage struct {
	// BaseDir is the base directory for storing files.
	BaseDir string
	// Permissions for created directories and files.
	Perm os.FileMode
}

// NewLocalStorage creates a new LocalStorage backend.
func NewLocalStorage(baseDir string) *LocalStorage {
	return &LocalStorage{
		BaseDir: baseDir,
		Perm:    0755,
	}
}

// maxFileNameLen bounds the sanitized file name so a crafted upload cannot
// exceed filesystem name limits.
const maxFileNameLen = 255

// sanitizeFileName reduces an attacker-controlled upload name to a single safe
// path element. The client fully controls the multipart filename, so values
// like "../../etc/cron.d/evil" or "..\\..\\windows\\system32\\cfg" would
// otherwise escape the storage root. Only the final element is kept, path
// separators and traversal segments are dropped, and NUL/control characters
// are rejected outright because they truncate paths in syscalls.
func sanitizeFileName(name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("empty file name")
	}
	if strings.ContainsRune(name, 0) {
		return "", fmt.Errorf("file name contains NUL byte")
	}

	// Strip any directory component the client may have supplied. Replacing
	// backslashes first covers Windows-style traversal on every platform.
	cleaned := strings.ReplaceAll(name, "\\", "/")
	cleaned = filepath.Base(cleaned)
	cleaned = strings.TrimSpace(cleaned)

	// filepath.Base already collapses "..", but be explicit: a name that is
	// still a traversal marker must never reach the filesystem.
	if cleaned == "." || cleaned == ".." || strings.Contains(cleaned, "/") {
		return "", fmt.Errorf("invalid file name")
	}

	// Reject remaining control characters.
	for _, r := range cleaned {
		if r < 0x20 || r == 0x7f {
			return "", fmt.Errorf("file name contains control character")
		}
	}

	if len(cleaned) > maxFileNameLen {
		cleaned = cleaned[:maxFileNameLen]
	}
	if cleaned == "" {
		return "", fmt.Errorf("empty file name")
	}
	return cleaned, nil
}

// resolveUnderRoot joins rel onto base and verifies the result stays inside
// base. base must be absolute and already cleaned.
func resolveUnderRoot(base, rel string) (string, error) {
	absBase, err := filepath.Abs(base)
	if err != nil {
		return "", fmt.Errorf("resolve base dir: %w", err)
	}
	absBase = filepath.Clean(absBase)

	target := filepath.Join(absBase, filepath.Clean("/"+rel))
	if target != absBase && !strings.HasPrefix(target, absBase+string(os.PathSeparator)) {
		return "", fmt.Errorf("path escapes storage root")
	}
	return target, nil
}

// Store saves a file to the local filesystem.
func (s *LocalStorage) Store(file *multipart.FileHeader, dir string) (string, error) {
	safeName, err := sanitizeFileName(file.Filename)
	if err != nil {
		return "", fmt.Errorf("sanitize file name: %w", err)
	}

	// Resolve the destination before touching the filesystem so a traversal
	// attempt fails without creating any directory.
	fullDir, err := resolveUnderRoot(s.BaseDir, dir)
	if err != nil {
		return "", fmt.Errorf("resolve storage dir: %w", err)
	}
	if err := os.MkdirAll(fullDir, s.Perm); err != nil {
		return "", fmt.Errorf("create directory: %w", err)
	}

	storedPath := filepath.Join(fullDir, safeName)

	src, err := file.Open()
	if err != nil {
		return "", fmt.Errorf("open uploaded file: %w", err)
	}
	defer src.Close()

	dst, err := os.OpenFile(storedPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, s.Perm)
	if err != nil {
		return "", fmt.Errorf("create file: %w", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return "", fmt.Errorf("copy file: %w", err)
	}

	return filepath.ToSlash(filepath.Join(dir, safeName)), nil
}

// Delete removes a file from the local filesystem.
func (s *LocalStorage) Delete(path string) error {
	fullPath, err := resolveUnderRoot(s.BaseDir, path)
	if err != nil {
		return fmt.Errorf("resolve delete path: %w", err)
	}
	return os.Remove(fullPath)
}

// UploadMiddleware creates a middleware that handles file uploads.
// Uploaded files are stored via the configured Storage backend and
// their info is stored in ctx.Locals["upload_files"] ([]FileInfo).
func UploadMiddleware(config UploadConfig) core.MiddlewareFunc {
	if !config.Enabled {
		return nil
	}

	if config.MaxSize == 0 {
		config.MaxSize = 10 * 1024 * 1024 // default 10MB
	}

	return func(ctx *core.Ctx, next core.Handler) error {
		if config.SkipFunc != nil && config.SkipFunc(ctx) {
			return next(ctx)
		}

		// Only handle multipart form requests
		if !strings.Contains(ctx.Request.Header.Get("Content-Type"), "multipart/form-data") {
			return next(ctx)
		}

		if err := ctx.Request.ParseMultipartForm(config.MaxSize); err != nil {
			ctx.Response.WriteHeader(http.StatusRequestEntityTooLarge)
			return nil
		}

		fieldName := config.FieldName
		if fieldName == "" {
			fieldName = "file"
		}

		files := ctx.Request.MultipartForm.File[fieldName]
		if len(files) == 0 {
			return next(ctx)
		}

		if config.Storage == nil {
			return fmt.Errorf("upload middleware: no storage backend configured")
		}

		var uploaded []FileInfo
		for _, f := range files {
			if config.MaxSize > 0 && f.Size > config.MaxSize {
				ctx.Response.WriteHeader(http.StatusRequestEntityTooLarge)
				return nil
			}

			// Reject traversal names before they reach any storage backend,
			// including third-party ones that may not sanitize themselves.
			safeName, err := sanitizeFileName(f.Filename)
			if err != nil {
				ctx.Response.WriteHeader(http.StatusBadRequest)
				return nil
			}

			if !allowedFile(f, config.AllowedTypes, config.AllowedExtensions) {
				ctx.Response.WriteHeader(http.StatusUnsupportedMediaType)
				return nil
			}

			storedPath, err := config.Storage.Store(f, "uploads")
			if err != nil {
				return fmt.Errorf("store file: %w", err)
			}

			ext := filepath.Ext(safeName)
			uploaded = append(uploaded, FileInfo{
				FieldName:   fieldName,
				FileName:    safeName,
				Size:        f.Size,
				ContentType: f.Header.Get("Content-Type"),
				StoredPath:  storedPath,
				Extension:   ext,
			})
		}

		ctx.SetLocal("upload_files", uploaded)
		return next(ctx)
	}
}

// allowedFile checks if a file is allowed based on type and extension.
func allowedFile(file *multipart.FileHeader, allowedTypes, allowedExts []string) bool {
	if len(allowedExts) > 0 {
		ext := strings.ToLower(filepath.Ext(file.Filename))
		allowed := false
		for _, e := range allowedExts {
			if strings.ToLower(e) == ext {
				allowed = true
				break
			}
		}
		if !allowed {
			return false
		}
	}

	if len(allowedTypes) > 0 {
		contentType := file.Header.Get("Content-Type")
		allowed := false
		for _, t := range allowedTypes {
			if t == contentType {
				allowed = true
				break
			}
		}
		if !allowed {
			return false
		}
	}

	return true
}

// GetUploadFiles retrieves uploaded file info from context.
func GetUploadFiles(ctx *core.Ctx) []FileInfo {
	if files, ok := ctx.Locals["upload_files"].([]FileInfo); ok {
		return files
	}
	return nil
}
