package middleware

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gofault/gofault/core"
	"github.com/gofault/gofault/exception"
)

// jwtAlgHS256 is the only signing algorithm this package supports.
const jwtAlgHS256 = "HS256"

// JWTConfig holds JWT authentication configuration.
type JWTConfig struct {
	Secret []byte
	// Algorithm must be HS256. Any other value is rejected at construction.
	Algorithm string
	// TokenName is the query parameter consulted for the token when
	// AllowQueryToken is enabled.
	TokenName string
	// AllowQueryToken permits reading the token from the URL query string.
	// Off by default: query strings land in access logs, Referer headers and
	// browser history, which leaks the credential.
	AllowQueryToken bool
	// RequireExpiry rejects tokens without an exp claim. On by default so a
	// token stays valid only as long as the issuer intended.
	RequireExpiry bool
}

// DefaultJWTConfig returns default JWT configuration.
func DefaultJWTConfig(secret []byte) JWTConfig {
	return JWTConfig{
		Secret:          secret,
		Algorithm:       jwtAlgHS256,
		TokenName:       "token",
		AllowQueryToken: false,
		RequireExpiry:   true,
	}
}

// Claims represents JWT claims.
type Claims struct {
	Subject   string   `json:"sub"`
	UserID    string   `json:"uid"`
	Roles     []string `json:"roles"`
	ExpiresAt int64    `json:"exp"`
	IssuedAt  int64    `json:"iat"`
	Issuer    string   `json:"iss"`
}

// IsValid reports whether the claims are within their validity window.
// A zero ExpiresAt means "no expiry recorded"; callers that require a bounded
// lifetime must check that separately (see JWTConfig.RequireExpiry).
func (c *Claims) IsValid() bool {
	if c.ExpiresAt == 0 {
		return true
	}
	return time.Now().Unix() < c.ExpiresAt
}

// JWTAuth creates JWT authentication middleware.
//
// A misconfigured secret or an unsupported algorithm yields a middleware that
// rejects every request with a diagnostic rather than nil. Returning nil was
// worse than it looks: the documented usage is to register whatever this
// returns, so callers that never checked for nil put a nil in the chain and
// panicked on the first request.
func JWTAuth(cfg JWTConfig) core.MiddlewareFunc {
	switch {
	case len(cfg.Secret) == 0:
		return misconfiguredJWTMiddleware("jwt: secret is empty")
	case cfg.Algorithm == "":
		cfg.Algorithm = jwtAlgHS256
	case cfg.Algorithm != jwtAlgHS256:
		return misconfiguredJWTMiddleware("jwt: unsupported algorithm " + cfg.Algorithm)
	}
	if cfg.TokenName == "" {
		cfg.TokenName = "token"
	}

	return func(ctx *core.Ctx, next core.Handler) error {
		token := extractToken(ctx, cfg)
		if token == "" {
			return exception.Unauthorized("missing authentication token")
		}

		claims, err := parseToken(token, cfg)
		if err != nil {
			return exception.Unauthorized("invalid token: " + err.Error())
		}

		if cfg.RequireExpiry && claims.ExpiresAt == 0 {
			return exception.Unauthorized("token missing expiry")
		}

		if !claims.IsValid() {
			return exception.Unauthorized("token expired")
		}

		// Hand the verified claims to downstream handlers. Without this the
		// middleware authenticates the caller but nothing downstream can read
		// who the caller is.
		ctx.SetLocal("claims", claims)

		return next(ctx)
	}
}

// misconfiguredJWTMiddleware fails closed at request time with a message that
// says what is wrong, instead of at wiring time with a nil the caller may not
// check.
func misconfiguredJWTMiddleware(reason string) core.MiddlewareFunc {
	return func(ctx *core.Ctx, next core.Handler) error {
		return exception.InternalServerError(reason)
	}
}

// MustJWTAuth is JWTAuth for callers that want a wiring-time panic on a
// misconfigured secret rather than a middleware that rejects every request.
// Use it during application setup, where failing fast is the point.
func MustJWTAuth(cfg JWTConfig) core.MiddlewareFunc {
	if len(cfg.Secret) == 0 {
		panic("jwt: secret is empty")
	}
	if cfg.Algorithm != "" && cfg.Algorithm != jwtAlgHS256 {
		panic("jwt: unsupported algorithm " + cfg.Algorithm)
	}
	return JWTAuth(cfg)
}

// LegacyJWTConfig returns a config that reproduces the pre-v3.4.1 behaviour:
// tokens may arrive as a query parameter and one without an expiry is accepted.
//
// It exists so an application can be migrated deliberately rather than by
// accident. Both settings weaken security: a token in a URL leaks into access
// logs, Referer headers and browser history, and a token without an expiry
// never stops working.
func LegacyJWTConfig(secret []byte) JWTConfig {
	cfg := DefaultJWTConfig(secret)
	cfg.AllowQueryToken = true
	cfg.RequireExpiry = false
	return cfg
}

// GetClaims returns the claims verified by JWTAuth, or nil when the route is not
// protected by it.
func GetClaims(ctx *core.Ctx) *Claims {
	if claims, ok := ctx.Locals["claims"].(*Claims); ok {
		return claims
	}
	return nil
}

// extractToken extracts JWT from request.
func extractToken(ctx *core.Ctx, cfg JWTConfig) string {
	auth := ctx.Request.Header.Get("Authorization")
	if auth != "" {
		parts := strings.SplitN(auth, " ", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "bearer") {
			return parts[1]
		}
	}
	if cfg.AllowQueryToken {
		return ctx.Request.URL.Query().Get(cfg.TokenName)
	}
	return ""
}

// jwtHeader is the subset of the JOSE header this package reads.
type jwtHeader struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
}

// parseToken parses and verifies JWT token.
func parseToken(tokenString string, cfg JWTConfig) (*Claims, error) {
	parts := strings.Split(tokenString, ".")
	if len(parts) != 3 {
		return nil, errors.New("invalid token format")
	}

	header, payload, sigEncoded := parts[0], parts[1], parts[2]

	// Verify signature
	expectedSig := sign([]byte(header+"."+payload), cfg.Secret)
	sigBytes, err := base64.RawURLEncoding.DecodeString(sigEncoded)
	if err != nil {
		return nil, errors.New("invalid signature encoding")
	}
	if !hmac.Equal(sigBytes, expectedSig) {
		return nil, errors.New("invalid signature")
	}

	// The signature is checked first so an unauthenticated token can never
	// influence the claims we act on. Once it holds, confirm the header
	// actually declares the algorithm we signed with rather than trusting the
	// token's own claim about itself.
	hdr := &jwtHeader{}
	if err := decodeBase64URL(header, hdr); err != nil {
		return nil, errors.New("invalid header encoding")
	}
	if hdr.Alg != cfg.Algorithm {
		return nil, errors.New("unsupported algorithm")
	}

	// Decode payload
	claims := &Claims{}
	if err := decodeBase64URL(payload, claims); err != nil {
		return nil, err
	}

	return claims, nil
}

// sign creates HMAC SHA256 signature.
func sign(data, secret []byte) []byte {
	h := hmac.New(sha256.New, secret)
	h.Write(data)
	return h.Sum(nil)
}

// decodeBase64URL decodes base64url encoded string into v.
func decodeBase64URL(encoded string, v interface{}) error {
	data, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

// GenerateToken creates a new JWT token signed with algorithm. Only HS256 is
// supported; an empty value defaults to it and anything else returns an error
// rather than silently signing with HS256 under a different label.
func GenerateToken(claims *Claims, secret []byte, algorithm string) (string, error) {
	if len(secret) == 0 {
		return "", errors.New("jwt: secret must not be empty")
	}
	if algorithm == "" {
		algorithm = jwtAlgHS256
	}
	if algorithm != jwtAlgHS256 {
		return "", fmt.Errorf("jwt: unsupported algorithm %q (only %s supported)", algorithm, jwtAlgHS256)
	}
	if claims == nil {
		return "", errors.New("jwt: claims must not be nil")
	}

	headerJSON, err := json.Marshal(&jwtHeader{Alg: algorithm, Typ: "JWT"})
	if err != nil {
		return "", err
	}
	header := base64.RawURLEncoding.EncodeToString(headerJSON)

	if claims.IssuedAt == 0 {
		claims.IssuedAt = time.Now().Unix()
	}
	if claims.ExpiresAt == 0 {
		claims.ExpiresAt = time.Now().Add(24 * time.Hour).Unix()
	}

	payloadBytes, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	payload := base64.RawURLEncoding.EncodeToString(payloadBytes)

	sig := sign([]byte(header+"."+payload), secret)
	sigEncoded := base64.RawURLEncoding.EncodeToString(sig)

	return header + "." + payload + "." + sigEncoded, nil
}
