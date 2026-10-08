// Package middleware provides common HTTP middleware for gofault.
package middleware

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gofault/gofault/core"
)

// CORSConfig holds CORS configuration options.
type CORSConfig struct {
	AllowOrigins     []string
	AllowMethods     []string
	AllowHeaders     []string
	ExposeHeaders    []string
	AllowCredentials bool
	MaxAge           int
}

// DefaultCORSConfig returns the default CORS configuration.
func DefaultCORSConfig() CORSConfig {
	return CORSConfig{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization"},
		ExposeHeaders:    []string{},
		AllowCredentials: false,
		MaxAge:           86400,
	}
}

// CORS creates a CORS middleware with the given configuration.
func CORS(cfg CORSConfig) core.MiddlewareFunc {
	return func(ctx *core.Ctx, next core.Handler) error {
		origin := ctx.Request.Header.Get("Origin")

		// Check if origin is allowed
		allowedOrigin := ""
		for _, o := range cfg.AllowOrigins {
			if o == "*" || o == origin {
				allowedOrigin = o
				break
			}
			// Support wildcard subdomain matching: *.example.com matches
			// sub.example.com but NOT notexample.com or evil-example.com.
			// The suffix must be preceded by a dot.
			if strings.HasPrefix(o, "*.") {
				domain := strings.TrimPrefix(o, "*.")
				if strings.HasSuffix(origin, "."+domain) {
					allowedOrigin = origin
					break
				}
			}
		}

		if allowedOrigin != "" {
			ctx.Response.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
			if cfg.AllowCredentials {
				ctx.Response.Header().Set("Access-Control-Allow-Credentials", "true")
			}
			if len(cfg.ExposeHeaders) > 0 {
				ctx.Response.Header().Set("Access-Control-Expose-Headers", strings.Join(cfg.ExposeHeaders, ", "))
			}
			if cfg.MaxAge > 0 {
				ctx.Response.Header().Set("Access-Control-Max-Age", strconv.Itoa(cfg.MaxAge))
			}
		}

		// Handle preflight request
		if ctx.Request.Method == "OPTIONS" {
			if len(cfg.AllowMethods) > 0 {
				ctx.Response.Header().Set("Access-Control-Allow-Methods", strings.Join(cfg.AllowMethods, ", "))
			}
			if len(cfg.AllowHeaders) > 0 {
				ctx.Response.Header().Set("Access-Control-Allow-Headers", strings.Join(cfg.AllowHeaders, ", "))
			}
			ctx.StatusCode = http.StatusNoContent
			ctx.Response.WriteHeader(http.StatusNoContent)
			return nil
		}

		return next(ctx)
	}
}
