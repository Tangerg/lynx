package runtime

import (
	"testing"

	"github.com/Tangerg/lynx/agent/core"
)

type item struct{ Value string }
type mutableItem struct{ Values []string }

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

func TestInMemoryBlackboardSnapshotPreservesVisibility(t *testing.T) {
	blackboard := newInMemoryBlackboard()
	fresh := item{Value: "fresh"}
	stale := item{Value: "stale"}
	if err := blackboard.Bind(fresh); err != nil {
		t.Fatal(err)
	}
	if err := blackboard.Bind(stale); err != nil {
		t.Fatal(err)
	}
	if err := blackboard.Hide(stale); err != nil {
		t.Fatal(err)
	}
	state, err := blackboard.Snapshot()
	if err != nil {
		t.Fatal(err)
	}

	restored := newInMemoryBlackboard()
	if err := restored.Restore(state); err != nil {
		t.Fatal(err)
	}
	value, ok := restored.Lookup(core.LastResultBindingName, "")
	if !ok || value != fresh {
		t.Fatalf("visible value after restore = %v/%v, want %v", value, ok, fresh)
	}
}

// TestInMemoryBlackboardSpawnInheritsHidden guards the un-hide bug: a Clone'd
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
	child, err := parent.Clone()
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := child.Lookup(core.LastResultBindingName, ""); !ok || v.(item).Value != "fresh" {
		t.Fatalf("child visible-latest = %v, want fresh — Clone must propagate the hidden marker (else stale resurfaces)", v)
	}
}

func TestInMemoryBlackboardSpawnInherits(t *testing.T) {
	parent := newInMemoryBlackboard()
	parent.Bind(item{Value: "shared"})

	child, err := parent.Clone()
	if err != nil {
		t.Fatal(err)
	}
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

func TestInMemoryBlackboardOwnsStoredAndReturnedValues(t *testing.T) {
	original := &mutableItem{Values: []string{"original"}}
	blackboard := newInMemoryBlackboard()
	if err := blackboard.Bind(original); err != nil {
		t.Fatal(err)
	}

	original.Values[0] = "producer mutation"
	first, ok := core.Last[*mutableItem](blackboard)
	if !ok || first.Values[0] != "original" {
		t.Fatalf("stored value = %#v, %v; want owned original", first, ok)
	}

	first.Values[0] = "reader mutation"
	second, ok := core.Last[*mutableItem](blackboard)
	if !ok || second.Values[0] != "original" {
		t.Fatalf("second read = %#v, %v; reader mutated blackboard state", second, ok)
	}

	clone, err := blackboard.Clone()
	if err != nil {
		t.Fatal(err)
	}
	cloned, ok := core.Last[*mutableItem](clone)
	if !ok {
		t.Fatal("clone lost mutable item")
	}
	cloned.Values[0] = "clone mutation"
	if got, _ := core.Last[*mutableItem](blackboard); got.Values[0] != "original" {
		t.Fatalf("clone mutated parent state: %#v", got)
	}
}

// TestInMemoryBlackboardCleanChildCannotWriteThroughToParent pins the only
// boundary left between a parent and a clean child: the copied objects
// container. Stored values share their bytes, and ClearWorkingState truncates
// instead of reallocating, so a shared array would send the child's first write
// into the slot still holding the parent's first object — corrupting the parent
// through a child that was supposed to start with nothing.
func TestInMemoryBlackboardCleanChildCannotWriteThroughToParent(t *testing.T) {
	seeded := []string{"first", "second", "third"}
	parent := newInMemoryBlackboard()
	for _, value := range seeded {
		if err := parent.Add(item{Value: value}); err != nil {
			t.Fatal(err)
		}
	}

	child, err := cleanBlackboard(parent)
	if err != nil {
		t.Fatal(err)
	}
	if err := child.Add(item{Value: "child write"}); err != nil {
		t.Fatal(err)
	}

	objects := parent.Objects()
	if len(objects) != len(seeded) {
		t.Fatalf("parent object count = %d, want %d", len(objects), len(seeded))
	}
	for index, want := range seeded {
		got, ok := objects[index].(item)
		if !ok || got.Value != want {
			t.Fatalf("parent objects[%d] = %#v, want %q", index, objects[index], want)
		}
	}
}

func TestInMemoryBlackboardRejectsNonPortableStateAtomically(t *testing.T) {
	blackboard := newInMemoryBlackboard()
	if err := blackboard.Bind(func() {}); err == nil {
		t.Fatal("Bind accepted a function")
	}

	var bindings core.Bindings
	bindings.Set("valid", item{Value: "valid"})
	bindings.Set("invalid", make(chan struct{}))
	if err := blackboard.StoreAll(bindings); err == nil {
		t.Fatal("StoreAll accepted a channel")
	}
	if _, ok := blackboard.Load("valid"); ok {
		t.Fatal("StoreAll partially committed before rejecting invalid state")
	}
}

func TestInMemoryBlackboardConditions(t *testing.T) {
	bb := newInMemoryBlackboard()
	if _, ok := bb.Condition("x"); ok {
		t.Fatal("missing condition should not report ok")
	}
	bb.StoreCondition("x", true)
	v, ok := bb.Condition("x")
	if !ok || !v {
		t.Fatalf("got %v ok=%v", v, ok)
	}
}
