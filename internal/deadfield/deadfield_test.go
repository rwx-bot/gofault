package deadfield

import (
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
