package middleware

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/gofault/gofault/core"
	"github.com/gofault/gofault/exception"
)

// JWTConfig holds JWT authentication configuration.
type JWTConfig struct {
	Secret    []byte
	Algorithm string // HS256 only for simplicity
	TokenName string
}

// DefaultJWTConfig returns default JWT configuration.
func DefaultJWTConfig(secret []byte) JWTConfig {
	return JWTConfig{
		Secret:    secret,
		Algorithm: "HS256",
		TokenName: "token",
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

// IsValid checks if claims are valid.
func (c *Claims) IsValid() bool {
	if c.ExpiresAt == 0 {
		return true
	}
	return time.Now().Unix() < c.ExpiresAt
}

// JWTAuth creates JWT authentication middleware.
func JWTAuth(cfg JWTConfig) core.MiddlewareFunc {
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
	return ctx.Request.URL.Query().Get(cfg.TokenName)
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

// GenerateToken creates a new JWT token.
func GenerateToken(claims *Claims, secret []byte, algorithm string) (string, error) {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))

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
