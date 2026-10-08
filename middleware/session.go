package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gofault/gofault/core"
)

// SessionConfig holds session configuration.
type SessionConfig struct {
	Name     string
	MaxAge   int    // Session max age in seconds (0 = until browser closes)
	Path     string // Cookie path
	Domain   string // Cookie domain
	Secure   bool   // Secure cookie flag
	HTTPOnly bool   // HTTPOnly flag
}

// DefaultSessionConfig returns default session configuration.
func DefaultSessionConfig() SessionConfig {
	return SessionConfig{
		Name:     "gofault_session",
		MaxAge:   86400, // 24 hours
		Path:     "/",
		HTTPOnly: true,
	}
}

// Session represents a user session.
type Session struct {
	ID      string
	Values  map[string]any
	Created time.Time
	Expires time.Time
}

// SessionStore manages sessions in memory.
type SessionStore struct {
	sessions map[string]*Session
	mu       sync.RWMutex
	config   SessionConfig
}

// NewSessionStore creates a new session store.
func NewSessionStore(cfg SessionConfig) *SessionStore {
	if cfg.Name == "" {
		cfg = DefaultSessionConfig()
	}
	return &SessionStore{
		sessions: make(map[string]*Session),
		config:   cfg,
	}
}

// GetSession retrieves a session by ID.
func (s *SessionStore) GetSession(id string) (*Session, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	session, exists := s.sessions[id]
	if !exists {
		return nil, false
	}

	if !session.Expires.IsZero() && time.Now().After(session.Expires) {
		return nil, false
	}

	return session, true
}

// CreateSession creates a new session.
func (s *SessionStore) CreateSession() (*Session, error) {
	id, err := generateSessionID()
	if err != nil {
		return nil, err
	}

	session := &Session{
		ID:      id,
		Values:  make(map[string]any),
		Created: time.Now(),
		Expires: time.Now().Add(time.Duration(s.config.MaxAge) * time.Second),
	}

	s.mu.Lock()
	s.sessions[id] = session
	s.mu.Unlock()

	return session, nil
}

// DeleteSession removes a session.
func (s *SessionStore) DeleteSession(id string) {
	s.mu.Lock()
	delete(s.sessions, id)
	s.mu.Unlock()
}

// SessionMiddleware creates session handling middleware.
func SessionMiddleware(store *SessionStore) core.MiddlewareFunc {
	return func(ctx *core.Ctx, next core.Handler) error {
		cookie, err := ctx.Request.Cookie(store.config.Name)
		var session *Session

		if err == nil && cookie != nil && cookie.Value != "" {
			if s, exists := store.GetSession(cookie.Value); exists {
				session = s
			}
		}

		if session == nil {
			session, err = store.CreateSession()
			if err != nil {
				return err
			}
		}

		// Publish the session so downstream handlers can read it via GetSession.
		ctx.SetLocal("session", session)

		err = next(ctx)

		// Only set the cookie when the request succeeded. On error the response
		// may already carry an error body, and issuing a fresh session would
		// suggest the request was accepted.
		if err != nil {
			return err
		}

		httpCookie := &http.Cookie{
			Name:     store.config.Name,
			Value:    session.ID,
			Path:     store.config.Path,
			Domain:   store.config.Domain,
			MaxAge:   store.config.MaxAge,
			Secure:   store.config.Secure,
			HttpOnly: store.config.HTTPOnly,
		}

		// Add rather than Set: a handler may already have set its own cookies,
		// and Set would replace them.
		ctx.Response.Header().Add("Set-Cookie", formatCookie(httpCookie))

		return nil
	}
}

// formatCookie formats a cookie as a Set-Cookie header value.
func formatCookie(c *http.Cookie) string {
	s := c.Name + "=" + c.Value
	if c.Path != "" {
		s += "; Path=" + c.Path
	}
	if c.Domain != "" {
		s += "; Domain=" + c.Domain
	}
	if c.MaxAge > 0 {
		s += "; Max-Age=" + strconv.Itoa(c.MaxAge)
	}
	if c.Secure {
		s += "; Secure"
	}
	if c.HttpOnly {
		s += "; HttpOnly"
	}
	return s
}

// generateSessionID generates a random session ID.
func generateSessionID() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// GetSession retrieves the session attached to the request by
// SessionMiddleware. It returns nil when the route is not protected by it.
func GetSession(ctx *core.Ctx) *Session {
	if ctx == nil || ctx.Locals == nil {
		return nil
	}
	if s, ok := ctx.Locals["session"].(*Session); ok {
		return s
	}
	return nil
}

// SetValue sets a value in the session.
func (s *Session) SetValue(key string, value any) {
	s.Values[key] = value
}

// GetValue retrieves a value from the session.
func (s *Session) GetValue(key string) (any, bool) {
	v, ok := s.Values[key]
	return v, ok
}

// DeleteValue removes a value from the session.
func (s *Session) DeleteValue(key string) {
	delete(s.Values, key)
}
