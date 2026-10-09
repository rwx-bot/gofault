package ioc

import (
	"errors"
	"testing"
)

type svcA struct{ n int }
type svcB struct{ n int }

// Registering two constructors for one type used to silently replace the first,
// so a later typo looked like it had taken effect.
func TestRegister_RejectsDuplicate(t *testing.T) {
	c := New()

	if err := c.Register(func() *svcA { return &svcA{n: 1} }); err != nil {
		t.Fatalf("first Register: %v", err)
	}

	err := c.Register(func() *svcA { return &svcA{n: 2} })
	if err == nil {
		t.Fatal("expected a duplicate registration error")
	}
	if !errors.Is(err, ErrDuplicateRegistration) {
		t.Errorf("error = %v, want it to match ErrDuplicateRegistration", err)
	}

	// The original binding must still be the one in effect.
	inst, resolveErr := c.Resolve((*svcA)(nil))
	if resolveErr != nil {
		t.Fatalf("Resolve: %v", resolveErr)
	}
	if got := inst.(*svcA).n; got != 1 {
		t.Errorf("resolved n = %d, want the first registration's 1", got)
	}
}

// The same type in different scopes is not a duplicate.
func TestRegister_SameTypeAcrossScopesAllowed(t *testing.T) {
	c := New()

	if err := c.Register(func() *svcA { return &svcA{n: 1} }); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := c.RegisterScoped(func() *svcA { return &svcA{n: 2} }); err != nil {
		t.Errorf("request-scoped registration should not clash with the singleton: %v", err)
	}
	if err := c.RegisterTransient(func() *svcA { return &svcA{n: 3} }); err != nil {
		t.Errorf("transient registration should not clash with the singleton: %v", err)
	}
}

// A constructor that wants to report a failure is rejected, so callCtor's
// error branch could never run.
func TestRegister_RejectsMultiReturnConstructor(t *testing.T) {
	c := New()

	err := c.Register(func() (*svcA, error) { return nil, errors.New("boom") })
	if err == nil {
		t.Fatal("expected an error for a constructor returning two values")
	}
}

// A constructor whose single return value is an error is still fine; the value
// is stored, not inspected.
func TestRegister_AcceptsErrorReturningService(t *testing.T) {
	c := New()
	if err := c.Register(func() *error { return nil }); err != nil {
		t.Errorf("registering a service whose type is error should be allowed: %v", err)
	}
}

func TestRegister_RejectsNonFunc(t *testing.T) {
	c := New()
	if err := c.Register("not a function"); err == nil {
		t.Error("expected an error for a non-function constructor")
	}
}

// Every scope must reject a duplicate independently.
func TestRegister_DuplicatePerScope(t *testing.T) {
	scopes := map[string]func(*container, any) error{
		"singleton": func(c *container, ctor any) error { return c.Register(ctor) },
		"request":   func(c *container, ctor any) error { return c.RegisterScoped(ctor) },
		"transient": func(c *container, ctor any) error { return c.RegisterTransient(ctor) },
	}

	for name, register := range scopes {
		t.Run(name, func(t *testing.T) {
			c := New()
			ctor := func() *svcB { return &svcB{} }

			if err := register(c, ctor); err != nil {
				t.Fatalf("first: %v", err)
			}
			if err := register(c, ctor); err == nil {
				t.Error("expected a duplicate error on the second registration")
			}
		})
	}
}
