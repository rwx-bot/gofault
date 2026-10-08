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

// Store saves a file to the local filesystem.
func (s *LocalStorage) Store(file *multipart.FileHeader, dir string) (string, error) {
	src, err := file.Open()
	if err != nil {
		return "", fmt.Errorf("open uploaded file: %w", err)
	}
	defer src.Close()

	// Ensure parent directory exists
	fullDir := filepath.Join(s.BaseDir, dir)
	if err := os.MkdirAll(fullDir, s.Perm); err != nil {
		return "", fmt.Errorf("create directory: %w", err)
	}

	storedPath := filepath.Join(fullDir, file.Filename)
	dst, err := os.OpenFile(storedPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, s.Perm)
	if err != nil {
		return "", fmt.Errorf("create file: %w", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return "", fmt.Errorf("copy file: %w", err)
	}

	return filepath.Join(dir, file.Filename), nil
}

// Delete removes a file from the local filesystem.
func (s *LocalStorage) Delete(path string) error {
	fullPath := filepath.Join(s.BaseDir, path)
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

			if !allowedFile(f, config.AllowedTypes, config.AllowedExtensions) {
				ctx.Response.WriteHeader(http.StatusUnsupportedMediaType)
				return nil
			}

			storedPath, err := config.Storage.Store(f, "uploads")
			if err != nil {
				return fmt.Errorf("store file: %w", err)
			}

			ext := filepath.Ext(f.Filename)
			uploaded = append(uploaded, FileInfo{
				FieldName:   fieldName,
				FileName:    f.Filename,
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
