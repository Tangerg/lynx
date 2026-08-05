package runtime

import (
	"testing"

	"github.com/Tangerg/lynx/agent/core"
)

type item struct{ Value string }
type mutableItem struct{ Values []string }

func TestInMemoryBlackboardLatestByType(t *testing.T) {
	bb := newInMemoryBlackboard()
	if err := bb.Bind(item{Value: "first"}); err != nil {
		t.Fatal(err)
	}
	if err := bb.Bind(item{Value: "second"}); err != nil {
		t.Fatal(err)
	}

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
	value, ok := restored.Lookup(core.LatestObjectBindingName, "")
	if !ok || value != fresh {
		t.Fatalf("visible value after restore = %v/%v, want %v", value, ok, fresh)
	}
}

// TestInMemoryBlackboardSpawnInheritsHidden verifies that a cloned child owns
// the parent's object visibility as well as its objects. The most-recent object
// is hidden, so both parent and child must resolve the preceding visible value.
func TestInMemoryBlackboardSpawnInheritsHidden(t *testing.T) {
	parent := newInMemoryBlackboard()
	if err := parent.Bind(item{Value: "fresh"}); err != nil {
		t.Fatal(err)
	}
	stale := item{Value: "stale"}
	// Binding stale makes it the latest object; hiding it makes fresh visible.
	if err := parent.Bind(stale); err != nil {
		t.Fatal(err)
	}
	if err := parent.Hide(stale); err != nil {
		t.Fatal(err)
	}

	// Lookup (the hidden-aware path the planner's typed binding + Sequence's
	// latest_object chaining use, unlike core.Last which scans all objects) must
	// skip the hidden latest and return "fresh".
	parentValue, ok := parent.Lookup(core.LatestObjectBindingName, "")
	parentItem, typed := parentValue.(item)
	if !ok || !typed || parentItem.Value != "fresh" {
		t.Fatalf("parent visible-latest = %#v, %v/%v; want fresh", parentValue, ok, typed)
	}
	child, err := parent.Clone()
	if err != nil {
		t.Fatal(err)
	}
	childValue, ok := child.Lookup(core.LatestObjectBindingName, "")
	childItem, typed := childValue.(item)
	if !ok || !typed || childItem.Value != "fresh" {
		t.Fatalf("child visible-latest = %#v, %v/%v; want fresh", childValue, ok, typed)
	}
}

func TestInMemoryBlackboardSpawnInherits(t *testing.T) {
	parent := newInMemoryBlackboard()
	if err := parent.Bind(item{Value: "shared"}); err != nil {
		t.Fatal(err)
	}

	child, err := parent.Clone()
	if err != nil {
		t.Fatal(err)
	}
	got, ok := core.Last[item](child)
	if !ok || got.Value != "shared" {
		t.Fatalf("child should inherit parent item; got %v", got)
	}

	// Mutating the child must not propagate back.
	childBlackboard, ok := child.(*inMemoryBlackboard)
	if !ok {
		t.Fatalf("Clone returned %T, want *inMemoryBlackboard", child)
	}
	if err := childBlackboard.Bind(item{Value: "child-only"}); err != nil {
		t.Fatal(err)
	}
	parentLatest, ok := core.Last[item](parent)
	if !ok || parentLatest.Value != "shared" {
		t.Fatalf("parent value after child mutation = %#v, %v; want shared", parentLatest, ok)
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
	if got, ok := core.Last[*mutableItem](blackboard); !ok || got.Values[0] != "original" {
		t.Fatalf("parent state after clone mutation = %#v, %v; want original", got, ok)
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
	if err := bb.StoreCondition("x", true); err != nil {
		t.Fatal(err)
	}
	v, ok := bb.Condition("x")
	if !ok || !v {
		t.Fatalf("got %v ok=%v", v, ok)
	}
}
