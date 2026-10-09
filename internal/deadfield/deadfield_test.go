package deadfield

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// moduleRoot returns the directory holding the framework's go.mod.
func moduleRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate the test source")
	}
	root := filepath.Dir(filepath.Dir(filepath.Dir(thisFile)))
	if _, err := filepath.Glob(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("go.mod lookup: %v", err)
	}
	return root
}

// NoDeadConfigFields fails when an exported struct field is documented as an
// option but nothing ever reads it.
//
// Such a field is invisible to the compiler and to go vet, so it can ship while
// doing nothing. Six did, two of them with security consequences.
func TestNoDeadConfigFields(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping module scan in short mode")
	}

	rep, err := Scan(moduleRoot(t))
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	if len(rep.Dead) == 0 {
		return
	}

	sort.Slice(rep.Dead, func(i, j int) bool {
		if rep.Dead[i].Struct != rep.Dead[j].Struct {
			return rep.Dead[i].Struct < rep.Dead[j].Struct
		}
		return rep.Dead[i].Name < rep.Dead[j].Name
	})

	var b strings.Builder
	b.WriteString("exported struct fields that nothing reads:\n")
	for _, f := range rep.Dead {
		b.WriteString("  " + f.Struct + "." + f.Name + "  (" + f.DeclaringFile + ")\n")
	}
	b.WriteString("\nEither the field is read somewhere it should not be, or it is dead weight.\n")
	b.WriteString("If it is genuinely part of the public API for callers rather than for\n")
	b.WriteString("the framework itself, add it to the allowlist in deadfield.go with a reason.\n")

	t.Error(b.String())
}

// The allowlist must not rot: an entry that no longer corresponds to a real
// dead field is either a stale exemption or a field that has been wired up.
func TestAllowlistHasNoStaleEntries(t *testing.T) {
	rep, err := Scan(moduleRoot(t))
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	live := map[string]bool{}
	for _, f := range rep.Dead {
		live[f.Struct+"."+f.Name] = true
	}

	for name := range rep.Allowlisted {
		if live[name] {
			t.Errorf("allowlist entry %q is stale: the field is read now, remove it", name)
		}
	}

	// Every allowlist key should look like Struct.Field so typos are obvious.
	for name := range allowlist {
		if strings.Count(name, ".") != 1 {
			t.Errorf("allowlist key %q should be Struct.Field", name)
		}
	}
}

// A parameter that is accepted but never read usually means the signature
// promises behaviour that does not exist. InvokeHandler took an HTTP method and
// path it never consulted, while its name and signature implied it dispatched
// on them.
func TestNoUnusedParameters(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping module scan in short mode")
	}

	rep, err := Scan(moduleRoot(t))
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	if len(rep.UnusedParams) == 0 {
		return
	}

	sort.Slice(rep.UnusedParams, func(i, j int) bool {
		if rep.UnusedParams[i].Func != rep.UnusedParams[j].Func {
			return rep.UnusedParams[i].Func < rep.UnusedParams[j].Func
		}
		return rep.UnusedParams[i].Param < rep.UnusedParams[j].Param
	})

	var b strings.Builder
	b.WriteString("function parameters that are never used:\n")
	for _, p := range rep.UnusedParams {
		b.WriteString("  " + p.Func + "(" + p.Param + ")  (" + p.Position + ")\n")
	}
	b.WriteString("\nEither the parameter should be dropped, or the function should\n")
	b.WriteString("actually use it. If the signature is fixed by an interface, add it\n")
	b.WriteString("to paramAllowlist in deadfield.go with a reason.\n")

	t.Error(b.String())
}

// The allowlist must not rot, same as for fields.
func TestParamAllowlistHasNoStaleEntries(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping module scan in short mode")
	}

	rep, err := Scan(moduleRoot(t))
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	live := map[string]bool{}
	for _, p := range rep.UnusedParams {
		live[p.Func+"."+p.Param] = true
	}

	for name := range rep.ParamAllowlisted {
		if live[name] {
			t.Errorf("param allowlist entry %q is stale: the parameter is used now, remove it", name)
		}
	}

	for name := range paramAllowlist {
		if strings.Count(name, ".") != 1 {
			t.Errorf("param allowlist key %q should be Func.Param", name)
		}
	}
}

// A parameter reused by a := declaration inside the body is a reference, not an
// unused parameter.
func TestScanUnusedParams_IgnoresShadowingAndBlanks(t *testing.T) {
	dir := t.TempDir()
	src := `package sample

func usesShadow(x int) int {
	y := x + 1
	return y
}

func usesBlank(_ int, z int) int {
	return z
}

func ignoresParam(unused int, z int) int {
	return z
}

type impl struct{}

func (i impl) Method(ctx int, ok bool) error { return nil }
`
	if err := os.WriteFile(filepath.Join(dir, "sample.go"), []byte(src), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	rep, err := Scan(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	found := map[string]bool{}
	for _, p := range rep.UnusedParams {
		found[p.Func+"."+p.Param] = true
	}

	if found["usesShadow.x"] {
		t.Error("a parameter referenced by := should not be reported")
	}
	if found["usesBlank._"] {
		t.Error("a blank parameter should never be reported")
	}
	if found["usesBlank.z"] {
		t.Error("a used parameter should not be reported")
	}
	if !found["ignoresParam.unused"] {
		t.Error("a genuinely unused parameter should be reported")
	}

	// Methods are skipped: an interface implementation cannot drop a parameter.
	for _, p := range rep.UnusedParams {
		if p.Func == "Method" {
			t.Error("method parameters should be skipped, they may satisfy an interface")
		}
	}
}

// A parameter used only inside a nested closure still counts as used.
func TestScanUnusedParams_CountsClosureUse(t *testing.T) {
	dir := t.TempDir()
	src := `package sample

func inClosure(n int, other int) int {
	f := func() int { return n }
	return f() + other
}
`
	if err := os.WriteFile(filepath.Join(dir, "sample.go"), []byte(src), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	rep, err := Scan(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	for _, p := range rep.UnusedParams {
		if p.Param == "n" {
			t.Error("a parameter used inside a closure should not be reported")
		}
	}
}
