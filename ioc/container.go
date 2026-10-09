// Package ioc provides the inversion-of-control container for dependency injection.
package ioc

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
)

// ErrDuplicateRegistration is returned when two constructors are registered for
// the same type in the same scope. Silently replacing the first binding made a
// later typo look like it had taken effect.
var ErrDuplicateRegistration = errors.New("duplicate registration")

// contextKey is a custom type to avoid collisions in context.WithValue.
type contextKey string

const requestCtxKey contextKey = "gofault-request-scope"

// Scope defines the lifecycle scope of a registered service.
type Scope int

const (
	ScopeSingleton Scope = iota
	ScopeRequest
	ScopeTransient
)

// Container is the publicly exposed IoC container type (alias of the internal container).
type Container = container

// container is the inversion-of-control container.
type container struct {
	mu         sync.RWMutex
	singletons map[reflect.Type]*entry
	transients map[reflect.Type]*entry
	request    map[reflect.Type]*entry
	// requestScope stores per-context instances for request-scoped resolution.
	// The key is the context value returned by context.WithValue.
	requestScope map[contextKey]map[reflect.Type]any
	// counter for generating unique request scope keys.
	scopeCounter uint64
}

type entry struct {
	ctor any
	inst any // nil means not-yet-instantiated (for singletons)
}

// New creates a new container.
func New() *container {
	return &container{
		singletons:   make(map[reflect.Type]*entry),
		transients:   make(map[reflect.Type]*entry),
		request:      make(map[reflect.Type]*entry),
		requestScope: make(map[contextKey]map[reflect.Type]any),
	}
}

// BeginRequest creates a request scope bound to the returned context.
// All request-scoped resolutions within this context will share the same instances.
// Call EndRequest when the request completes to clean up.
func (c *container) BeginRequest(ctx context.Context) context.Context {
	c.mu.Lock()
	c.scopeCounter++
	key := contextKey(fmt.Sprintf("%d", c.scopeCounter))
	if c.requestScope[key] == nil {
		c.requestScope[key] = make(map[reflect.Type]any)
	}
	c.mu.Unlock()
	return context.WithValue(ctx, requestCtxKey, key)
}

// EndRequest releases all request-scoped instances associated with the context.
// Call this when the request handling is complete.
func (c *container) EndRequest(ctx context.Context) {
	key, ok := ctx.Value(requestCtxKey).(contextKey)
	if !ok {
		return
	}
	c.mu.Lock()
	delete(c.requestScope, key)
	c.mu.Unlock()
}

// Register registers a singleton constructor. The constructor must be a function
// that returns a single value.
func (c *container) Register(ctor any) error {
	return c.register(ctor, ScopeSingleton)
}

// RegisterScoped registers a request-scoped constructor.
func (c *container) RegisterScoped(ctor any) error {
	return c.register(ctor, ScopeRequest)
}

// RegisterTransient registers a transient (every-resolve) constructor.
func (c *container) RegisterTransient(ctor any) error {
	return c.register(ctor, ScopeTransient)
}

func (c *container) register(ctor any, scope Scope) error {
	fn := reflect.ValueOf(ctor)
	if fn.Kind() != reflect.Func {
		return fmt.Errorf("ctor must be a function, got %T", ctor)
	}
	t := fn.Type()
	if t.NumOut() != 1 {
		return fmt.Errorf("ctor must return exactly one value, got %d", t.NumOut())
	}
	key := t.Out(0)

	c.mu.Lock()
	defer c.mu.Unlock()

	var target map[reflect.Type]*entry
	switch scope {
	case ScopeSingleton:
		target = c.singletons
	case ScopeTransient:
		target = c.transients
	case ScopeRequest:
		target = c.request
	default:
		return fmt.Errorf("unknown scope %d", scope)
	}

	// Registering two constructors for the same type used to silently replace
	// the first, so a typo in a later Register left the earlier binding in
	// place with no indication that anything was wrong.
	if _, dup := target[key]; dup {
		return fmt.Errorf("%w: %s is already registered", ErrDuplicateRegistration, key)
	}

	target[key] = &entry{ctor: ctor}
	return nil
}

// Resolve instantiates or returns the cached instance for the given type.
// target must be a pointer to the desired type (e.g., &MyService{}).
// This method uses a background context and is suitable for singleton/transient resolution.
func (c *container) Resolve(target any) (any, error) {
	return c.ResolveFromCtx(context.Background(), target)
}

// ResolveFromCtx is like Resolve but resolves request-scoped instances
// within the given context. Request-scoped instances are shared across
// all ResolveFromCtx calls within the same request context.
func (c *container) ResolveFromCtx(ctx context.Context, target any) (any, error) {
	t := reflect.TypeOf(target)
	if t == nil {
		return nil, fmt.Errorf("nil type")
	}
	if t.Kind() != reflect.Ptr {
		return nil, fmt.Errorf("target must be a pointer, got %T", target)
	}

	// Check singleton.
	c.mu.RLock()
	e, isSingleton := c.singletons[t]
	c.mu.RUnlock()
	if isSingleton {
		return c.resolveSingleton(e, t)
	}

	// Check transient.
	c.mu.RLock()
	e, ok := c.transients[t]
	c.mu.RUnlock()
	if ok {
		return c.callCtor(e.ctor)
	}

	// Check request.
	c.mu.RLock()
	e, ok = c.request[t]
	c.mu.RUnlock()
	if ok {
		return c.resolveRequestScoped(ctx, e, t)
	}

	return nil, fmt.Errorf("no registered service for type %v", t)
}

// resolveRequestScoped resolves or creates a request-scoped instance.
func (c *container) resolveRequestScoped(ctx context.Context, e *entry, t reflect.Type) (any, error) {
	key, ok := ctx.Value(requestCtxKey).(contextKey)
	if !ok {
		// No request context: fall back to creating a new instance each time.
		return c.callCtor(e.ctor)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	scope := c.requestScope[key]
	if scope == nil {
		scope = make(map[reflect.Type]any)
		c.requestScope[key] = scope
	}

	if inst, exists := scope[t]; exists {
		return inst, nil
	}

	inst, err := c.callCtor(e.ctor)
	if err != nil {
		return nil, err
	}
	scope[t] = inst
	return inst, nil
}

// resolveSingleton returns the singleton instance, creating it lazily if needed.
//
// e.inst is written under c.mu, so it must also be read under c.mu. Reading it
// lock-free was a data race: concurrent resolves of the same not-yet-created
// singleton raced on the field, and the fast path could observe a partially
// written interface value.
func (c *container) resolveSingleton(e *entry, t reflect.Type) (any, error) {
	// Fast path: already instantiated.
	c.mu.RLock()
	inst := e.inst
	c.mu.RUnlock()
	if inst != nil {
		return inst, nil
	}

	// Slow path. Build the instance outside the lock so nested callCtor calls
	// can take the lock themselves without deadlocking, then publish it under
	// the lock with a double-check.
	created, err := c.callCtor(e.ctor)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if e.inst != nil {
		// Another goroutine created it first; discard ours.
		return e.inst, nil
	}
	e.inst = created
	return created, nil
}

// callCtor invokes a constructor, resolving each argument through the
// container.
func (c *container) callCtor(ctor any) (any, error) {
	fn := reflect.ValueOf(ctor)
	in := make([]reflect.Value, fn.Type().NumIn())
	for i := range in {
		argType := fn.Type().In(i)
		arg, err := c.resolveType(argType)
		if err != nil {
			return nil, fmt.Errorf("cannot resolve arg %d (%v): %w", i, argType, err)
		}
		in[i] = reflect.ValueOf(arg)
	}
	out := fn.Call(in)
	// register rejects any constructor that does not return exactly one value,
	// so there is no error return to inspect here.
	return out[0].Interface(), nil
}

func (c *container) resolveType(t reflect.Type) (any, error) {
	// Check if it's a registered singleton.
	c.mu.RLock()
	e, isSingleton := c.singletons[t]
	c.mu.RUnlock()
	if isSingleton {
		// Same path as ResolveFromCtx, so both entry points share one
		// implementation and one locking discipline.
		return c.resolveSingleton(e, t)
	}

	// Check transients.
	c.mu.RLock()
	e, ok := c.transients[t]
	c.mu.RUnlock()
	if ok {
		return c.callCtor(e.ctor)
	}

	// Check request-scoped.
	c.mu.RLock()
	e, ok = c.request[t]
	c.mu.RUnlock()
	if ok {
		return c.callCtor(e.ctor)
	}

	return nil, fmt.Errorf("cannot resolve %v", t)
}
