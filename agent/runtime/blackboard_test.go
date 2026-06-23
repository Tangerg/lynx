package runtime

import (
	"testing"

	"github.com/Tangerg/lynx/agent/core"
)

type item struct{ Value string }

func TestInMemoryBlackboardLatestByType(t *testing.T) {
	bb := newInMemoryBlackboard()
	bb.Bind(item{Value: "first"})
	bb.Bind(item{Value: "second"})

	got, ok := core.Last[item](bb)
	if !ok {
		t.Fatal("expected item on blackboard")
	}
	if got.Value != "second" {
		t.Fatalf("got %q want %q", got.Value, "second")
	}
}

// TestInMemoryBlackboardSpawnInheritsHidden guards the un-hide bug: a Spawn'd
// child inherits the parent's objects, so it must inherit the parent's hidden
// markers too — else an object the parent deliberately hid (to stop actions
// re-binding it) resurfaces via the child's type lookup. Here the HIDDEN object
// is the most-recent one, so without the marker the child would return it.
func TestInMemoryBlackboardSpawnInheritsHidden(t *testing.T) {
	parent := newInMemoryBlackboard()
	parent.Bind(item{Value: "fresh"})
	stale := item{Value: "stale"}
	parent.Bind(stale) // stale is now the latest object…
	parent.Hide(stale) // …but hidden, so the hidden-aware lookups skip it.

	// Lookup (the hidden-aware path the planner's typed binding + Sequence's
	// last_result chaining use, unlike core.Last which scans all objects) must
	// skip the hidden latest and return "fresh".
	if v, ok := parent.Lookup(core.LastResultBindingName, ""); !ok || v.(item).Value != "fresh" {
		t.Fatalf("parent visible-latest = %v, want fresh (stale is hidden)", v)
	}
	child := parent.Spawn()
	if v, ok := child.Lookup(core.LastResultBindingName, ""); !ok || v.(item).Value != "fresh" {
		t.Fatalf("child visible-latest = %v, want fresh — Spawn must propagate the hidden marker (else stale resurfaces)", v)
	}
}

func TestInMemoryBlackboardSpawnInherits(t *testing.T) {
	parent := newInMemoryBlackboard()
	parent.Bind(item{Value: "shared"})

	child := parent.Spawn()
	got, ok := core.Last[item](child)
	if !ok || got.Value != "shared" {
		t.Fatalf("child should inherit parent item; got %v", got)
	}

	// Mutating the child must not propagate back.
	if cm, ok := child.(*inMemoryBlackboard); ok {
		cm.Bind(item{Value: "child-only"})
	}
	parentLatest, _ := core.Last[item](parent)
	if parentLatest.Value != "shared" {
		t.Fatalf("parent leaked from child mutation: %q", parentLatest.Value)
	}
}

func TestInMemoryBlackboardConditions(t *testing.T) {
	bb := newInMemoryBlackboard()
	if _, ok := bb.Condition("x"); ok {
		t.Fatal("missing condition should not report ok")
	}
	bb.SetCondition("x", true)
	v, ok := bb.Condition("x")
	if !ok || !v {
		t.Fatalf("got %v ok=%v", v, ok)
	}
}
