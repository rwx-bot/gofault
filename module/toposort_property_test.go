package module

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/gofault/gofault/core"
)

// buildRandomDAG produces a random acyclic dependency graph over n modules.
// Edges only point from a higher index to a lower one, so a cycle is impossible
// by construction and any ordering the sort produces is valid.
type randomApp struct {
	app     *App
	names   []string
	depends map[string][]string
}

func buildRandomDAG(t *testing.T, n int, seed int64) *randomApp {
	t.Helper()
	rng := rand.New(rand.NewSource(seed))

	ra := &randomApp{
		app:     New(),
		depends: map[string][]string{},
	}

	for i := 0; i < n; i++ {
		ra.names = append(ra.names, fmt.Sprintf("m%02d", i))
	}

	// Register in a shuffled order so registration order does not already
	// match the dependency order.
	order := rng.Perm(n)
	for _, idx := range order {
		mod := core.NewModule(ra.names[idx])
		ra.app.RegisterModules(mod)
	}

	// Each module may depend on any subset of the lower-indexed modules.
	for i := range ra.names {
		var deps []string
		for j := 0; j < i; j++ {
			if rng.Intn(3) == 0 {
				deps = append(deps, ra.names[j])
			}
		}
		ra.depends[ra.names[i]] = deps
		ra.app.moduleMap[ra.names[i]].Depends = deps
	}

	return ra
}

// A topological sort of a DAG must satisfy every declared dependency, and must
// be reproducible for a given registration order.
func TestSortModules_RandomGraphsAreOrderedAndStable(t *testing.T) {
	for seed := int64(1); seed <= 200; seed++ {
		ra := buildRandomDAG(t, 12, seed)

		if err := ra.app.Init(); err != nil {
			t.Fatalf("seed %d: Init: %v", seed, err)
		}

		// Every module present exactly once.
		if len(ra.app.modules) != len(ra.names) {
			t.Fatalf("seed %d: got %d modules, want %d", seed, len(ra.app.modules), len(ra.names))
		}
		seen := map[string]int{}
		for i, m := range ra.app.modules {
			seen[m.Name] = i
		}
		for _, n := range ra.names {
			if _, ok := seen[n]; !ok {
				t.Fatalf("seed %d: module %s missing from the sorted list", seed, n)
			}
		}

		// Every dependency precedes its dependent.
		for _, n := range ra.names {
			for _, dep := range ra.depends[n] {
				if seen[dep] > seen[n] {
					t.Fatalf("seed %d: %q sorted at %d but depends on %q at %d",
						seed, n, seen[n], dep, seen[dep])
				}
			}
		}
	}
}

// The same registration and dependency set must always sort identically.
func TestSortModules_RandomGraphsAreReproducible(t *testing.T) {
	for seed := int64(1); seed <= 100; seed++ {
		ra := buildRandomDAG(t, 10, seed)

		var reference []string
		for run := 0; run < 3; run++ {
			app := New()
			// Rebuild the identical app each time.
			for _, n := range ra.names {
				app.RegisterModules(core.NewModule(n))
			}
			for _, n := range ra.names {
				app.moduleMap[n].Depends = ra.depends[n]
			}
			if err := app.Init(); err != nil {
				t.Fatalf("seed %d run %d: %v", seed, run, err)
			}

			var order []string
			for _, m := range app.modules {
				order = append(order, m.Name)
			}

			if run == 0 {
				reference = order
				continue
			}
			if len(order) != len(reference) {
				t.Fatalf("seed %d run %d: length %d, want %d", seed, run, len(order), len(reference))
			}
			for i := range order {
				if order[i] != reference[i] {
					t.Fatalf("seed %d run %d: order %v differs from %v", seed, run, order, reference)
				}
			}
		}
	}
}

// Registration order must be preserved among modules that have no ordering
// constraint between them, which is what makes the sort reproducible.
func TestSortModules_PreservesRegistrationOrderAmongIndependentModules(t *testing.T) {
	// A chain of dependent modules plus four that relate to nothing. The four
	// must come out in exactly the order they went in.
	app := New()

	var registered []string
	for _, n := range []string{"freeA", "chainA", "freeB", "chainB", "freeC", "chainC", "freeD"} {
		app.RegisterModules(core.NewModule(n))
		registered = append(registered, n)
	}
	app.moduleMap["chainB"].Depends = []string{"chainA"}
	app.moduleMap["chainC"].Depends = []string{"chainB"}

	if err := app.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	constrained := map[string]bool{"chainA": true, "chainB": true, "chainC": true}

	var got []string
	for _, m := range app.modules {
		if !constrained[m.Name] {
			got = append(got, m.Name)
		}
	}

	want := []string{"freeA", "freeB", "freeC", "freeD"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unconstrained modules reordered:\n got %v\nwant %v", got, want)
		}
	}

	// The chain must still be ordered.
	pos := map[string]int{}
	for i, m := range app.modules {
		pos[m.Name] = i
	}
	if pos["chainA"] > pos["chainB"] || pos["chainB"] > pos["chainC"] {
		t.Errorf("chain not ordered: %v", app.modules)
	}
}
