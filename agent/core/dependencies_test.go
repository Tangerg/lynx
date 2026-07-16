package core_test

import (
	"errors"
	"sync"
	"testing"

	"github.com/Tangerg/lynx/agent/core"
)

type dependencyFixture struct{ Value string }

func TestDependenciesTypedHierarchy(t *testing.T) {
	key := core.MustDependencyKey[*dependencyFixture]("fixture")
	root := core.NewDependencies()
	if err := core.RegisterDependency(root, key, &dependencyFixture{Value: "engine"}); err != nil {
		t.Fatalf("RegisterDependency root: %v", err)
	}

	process := root.Child()
	got, err := core.LookupDependency(process, key)
	if err != nil || got.Value != "engine" {
		t.Fatalf("Resolve inherited = %#v, %v; want engine", got, err)
	}
	if err := core.RegisterDependency(process, key, &dependencyFixture{Value: "process"}); err != nil {
		t.Fatalf("RegisterDependency process: %v", err)
	}
	action := process.Child()
	got, err = core.LookupDependency(action, key)
	if err != nil || got.Value != "process" {
		t.Fatalf("Resolve shadow = %#v, %v; want process", got, err)
	}

	rootGot, err := core.LookupDependency(root, key)
	if err != nil || rootGot.Value != "engine" {
		t.Fatalf("root mutated = %#v, %v", rootGot, err)
	}
}

func TestDependenciesErrors(t *testing.T) {
	stringKey := core.MustDependencyKey[string]("shared")
	intKey := core.MustDependencyKey[int]("shared")
	dependencies := core.NewDependencies()

	if _, err := core.LookupDependency(dependencies, stringKey); !errors.Is(err, core.ErrDependencyNotFound) {
		t.Fatalf("missing error = %v, want ErrDependencyNotFound", err)
	}
	if err := core.RegisterDependency(dependencies, stringKey, "first"); err != nil {
		t.Fatalf("RegisterDependency: %v", err)
	}
	if err := core.RegisterDependency(dependencies, stringKey, "second"); !errors.Is(err, core.ErrDependencyExists) {
		t.Fatalf("duplicate error = %v, want ErrDependencyExists", err)
	}
	if _, err := core.LookupDependency(dependencies.Child(), intKey); !errors.Is(err, core.ErrDependencyTypeMismatch) {
		t.Fatalf("type error = %v, want ErrDependencyTypeMismatch", err)
	}

	dependencies.Freeze()
	other := core.MustDependencyKey[int]("other")
	if err := core.RegisterDependency(dependencies, other, 1); !errors.Is(err, core.ErrDependenciesFrozen) {
		t.Fatalf("frozen error = %v, want ErrDependenciesFrozen", err)
	}
	if !dependencies.Frozen() {
		t.Fatal("dependencies should report frozen")
	}
	child := dependencies.Child()
	if child.Parent() != dependencies {
		t.Fatal("child parent mismatch")
	}
	if err := core.RegisterDependency(child, other, 2); err != nil {
		t.Fatalf("frozen parent should not freeze child: %v", err)
	}
}

func TestDependenciesRejectInvalidKeyAndNil(t *testing.T) {
	if _, err := core.NewDependencyKey[string](""); !errors.Is(err, core.ErrInvalidDependencyKey) {
		t.Fatalf("empty key error = %v", err)
	}
	if _, err := core.NewDependencyKey[string](" padded "); !errors.Is(err, core.ErrInvalidDependencyKey) {
		t.Fatalf("padded key error = %v", err)
	}

	dependencies := core.NewDependencies()
	key := core.MustDependencyKey[*dependencyFixture]("nil")
	if err := core.RegisterDependency(dependencies, key, (*dependencyFixture)(nil)); !errors.Is(err, core.ErrNilDependency) {
		t.Fatalf("nil error = %v, want ErrNilDependency", err)
	}
	var zero core.DependencyKey[string]
	if _, err := core.LookupDependency(dependencies, zero); !errors.Is(err, core.ErrInvalidDependencyKey) {
		t.Fatalf("zero key error = %v, want ErrInvalidDependencyKey", err)
	}
}

func TestDependenciesConcurrentLookup(t *testing.T) {
	key := core.MustDependencyKey[*dependencyFixture]("fixture")
	dependencies := core.NewDependencies()
	want := &dependencyFixture{Value: "stable"}
	if err := core.RegisterDependency(dependencies, key, want); err != nil {
		t.Fatalf("RegisterDependency: %v", err)
	}
	dependencies.Freeze()

	var wg sync.WaitGroup
	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				got, err := core.LookupDependency(dependencies.Child(), key)
				if err != nil || got != want {
					t.Errorf("Resolve = %#v, %v", got, err)
					return
				}
			}
		}()
	}
	wg.Wait()
}
