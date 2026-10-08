package middleware

import (
	"encoding/base64"
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gofault/gofault/core"
	"github.com/gofault/gofault/exception"
)

func TestJWTAuth_MissingToken(t *testing.T) {
	cfg := DefaultJWTConfig([]byte("secret"))
	middleware := JWTAuth(cfg)

	req := httptest.NewRequest("GET", "/test", nil)
	ctx := &core.Ctx{Request: req}

	err := middleware(ctx, func(ctx *core.Ctx) error { return nil })
	if err == nil {
		t.Fatal("expected unauthorized error")
	}

	httpErr, ok := exception.HTTPExceptionOf(err)
	if !ok {
		t.Fatalf("expected HTTPException, got %T", err)
	}
	if httpErr.GetStatusCode() != 401 {
		t.Errorf("status = %d, want 401", httpErr.GetStatusCode())
	}
}

func TestJWTAuth_ValidToken(t *testing.T) {
	secret := []byte("test-secret")
	cfg := DefaultJWTConfig(secret)
	middleware := JWTAuth(cfg)

	claims := &Claims{
		Subject:   "user123",
		UserID:    "1",
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
	}
	token, err := GenerateToken(claims, secret, "HS256")
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	ctx := &core.Ctx{Request: req}

	nextCalled := false
	err = middleware(ctx, func(ctx *core.Ctx) error {
		nextCalled = true
		return nil
	})

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !nextCalled {
		t.Error("next handler should be called")
	}
}

func TestJWTAuth_ExpiredToken(t *testing.T) {
	secret := []byte("test-secret")
	cfg := DefaultJWTConfig(secret)
	middleware := JWTAuth(cfg)

	claims := &Claims{
		Subject:   "user123",
		ExpiresAt: time.Now().Add(-time.Hour).Unix(), // Expired
	}
	token, _ := GenerateToken(claims, secret, "HS256")

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	ctx := &core.Ctx{Request: req}

	err := middleware(ctx, func(ctx *core.Ctx) error { return nil })
	if err == nil {
		t.Fatal("expected error for expired token")
	}
}

func TestJWTAuth_InvalidSignature(t *testing.T) {
	secret := []byte("test-secret")
	cfg := DefaultJWTConfig(secret)
	middleware := JWTAuth(cfg)

	claims := &Claims{
		Subject:   "user123",
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
	}
	token, _ := GenerateToken(claims, []byte("wrong-secret"), "HS256")

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	ctx := &core.Ctx{Request: req}

	err := middleware(ctx, func(ctx *core.Ctx) error { return nil })
	if err == nil {
		t.Fatal("expected error for invalid signature")
	}
}

func TestJWTAuth_BearerCaseInsensitive(t *testing.T) {
	secret := []byte("test-secret")
	cfg := DefaultJWTConfig(secret)
	middleware := JWTAuth(cfg)

	claims := &Claims{
		Subject:   "user123",
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
	}
	token, _ := GenerateToken(claims, secret, "HS256")

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "BEARER "+token)
	ctx := &core.Ctx{Request: req}

	err := middleware(ctx, func(ctx *core.Ctx) error { return nil })
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// Query-string tokens are off by default because they leak into access logs,
// Referer headers and browser history.
func TestJWTAuth_TokenFromQueryParamDisabledByDefault(t *testing.T) {
	secret := []byte("test-secret")
	middleware := JWTAuth(DefaultJWTConfig(secret))

	claims := &Claims{
		Subject:   "user123",
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
	}
	token, _ := GenerateToken(claims, secret, "HS256")

	req := httptest.NewRequest("GET", "/test?token="+url.QueryEscape(token), nil)
	ctx := &core.Ctx{Request: req}

	err := middleware(ctx, func(ctx *core.Ctx) error { return nil })
	if err == nil {
		t.Fatal("expected rejection of query token, got nil")
	}
}

func TestJWTAuth_TokenFromQueryParamAllowedWhenOptedIn(t *testing.T) {
	secret := []byte("test-secret")
	cfg := DefaultJWTConfig(secret)
	cfg.AllowQueryToken = true
	middleware := JWTAuth(cfg)

	claims := &Claims{
		Subject:   "user123",
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
	}
	token, _ := GenerateToken(claims, secret, "HS256")

	req := httptest.NewRequest("GET", "/test?token="+url.QueryEscape(token), nil)
	ctx := &core.Ctx{Request: req}

	if err := middleware(ctx, func(ctx *core.Ctx) error { return nil }); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// A token with no exp claim would otherwise stay valid forever.
func TestJWTAuth_RejectsTokenWithoutExpiry(t *testing.T) {
	secret := []byte("test-secret")
	middleware := JWTAuth(DefaultJWTConfig(secret))

	claims := &Claims{Subject: "user123", ExpiresAt: 0}
	token, err := GenerateToken(claims, secret, "HS256")
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}
	// Strip the expiry that GenerateToken fills in, simulating a foreign
	// issuer that never set exp.
	parts := strings.SplitN(token, ".", 3)
	payload, _ := base64.RawURLEncoding.DecodeString(parts[1])
	var m map[string]interface{}
	json.Unmarshal(payload, &m)
	delete(m, "exp")
	newPayload, _ := json.Marshal(m)
	unsigned := parts[0] + "." + base64.RawURLEncoding.EncodeToString(newPayload)
	sig := sign([]byte(unsigned), secret)
	token = unsigned + "." + base64.RawURLEncoding.EncodeToString(sig)

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	ctx := &core.Ctx{Request: req}

	if err := middleware(ctx, func(ctx *core.Ctx) error { return nil }); err == nil {
		t.Fatal("expected rejection of token without exp, got nil")
	}
}

// An unsigned alg=none token must never be accepted.
func TestJWTAuth_RejectsAlgNone(t *testing.T) {
	secret := []byte("test-secret")
	middleware := JWTAuth(DefaultJWTConfig(secret))

	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	claims := &Claims{Subject: "attacker", ExpiresAt: time.Now().Add(time.Hour).Unix()}
	payloadBytes, _ := json.Marshal(claims)
	payload := base64.RawURLEncoding.EncodeToString(payloadBytes)
	token := header + "." + payload + "."

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	ctx := &core.Ctx{Request: req}

	if err := middleware(ctx, func(ctx *core.Ctx) error { return nil }); err == nil {
		t.Fatal("expected rejection of alg=none token, got nil")
	}
}

func TestJWTAuth_NilOnUnusableConfig(t *testing.T) {
	if mw := JWTAuth(JWTConfig{Algorithm: "HS256"}); mw != nil {
		t.Error("expected nil middleware for empty secret")
	}
	if mw := JWTAuth(JWTConfig{Secret: []byte("s"), Algorithm: "RS256"}); mw != nil {
		t.Error("expected nil middleware for unsupported algorithm")
	}
}

func TestGenerateToken_RejectsUnsupportedAlgorithm(t *testing.T) {
	secret := []byte("test-secret")
	claims := &Claims{Subject: "u"}

	if _, err := GenerateToken(claims, secret, "none"); err == nil {
		t.Error("expected error for alg=none")
	}
	if _, err := GenerateToken(claims, secret, "RS256"); err == nil {
		t.Error("expected error for RS256")
	}
	if _, err := GenerateToken(claims, nil, "HS256"); err == nil {
		t.Error("expected error for empty secret")
	}
}

func TestClaims_IsValid(t *testing.T) {
	tests := []struct {
		name      string
		expiresAt int64
		want      bool
	}{
		{"no expiry", 0, true},
		{"future expiry", time.Now().Add(time.Hour).Unix(), true},
		{"past expiry", time.Now().Add(-time.Hour).Unix(), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Claims{ExpiresAt: tt.expiresAt}
			if got := c.IsValid(); got != tt.want {
				t.Errorf("IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGenerateToken(t *testing.T) {
	secret := []byte("test-secret")
	claims := &Claims{
		Subject:   "user123",
		UserID:    "1",
		Roles:     []string{"admin", "user"},
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
		Issuer:    "test",
	}

	token, err := GenerateToken(claims, secret, "HS256")
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}

	if token == "" {
		t.Fatal("token should not be empty")
	}

	parts := 0
	for _, c := range token {
		if c == '.' {
			parts++
		}
	}
	if parts != 2 {
		t.Errorf("token should have 3 parts, got %d parts", parts+1)
	}
}

func TestExtractToken_BearerWithSpaces(t *testing.T) {
	cfg := DefaultJWTConfig([]byte("secret"))

	// Test various Bearer formats
	tests := []struct {
		auth   string
		expect string
	}{
		{"Bearer token123", "token123"},
		{"bearer token456", "token456"},
		{"BEARER token789", "token789"},
		{"Basic token", ""}, // Wrong scheme
		{"token", ""},       // No scheme
		{"", ""},            // Empty
	}

	for _, tt := range tests {
		req := httptest.NewRequest("GET", "/test", nil)
		if tt.auth != "" {
			req.Header.Set("Authorization", tt.auth)
		}
		ctx := &core.Ctx{Request: req}

		got := extractToken(ctx, cfg)
		if got != tt.expect {
			t.Errorf("extractToken(%q) = %q, want %q", tt.auth, got, tt.expect)
		}
	}
}
