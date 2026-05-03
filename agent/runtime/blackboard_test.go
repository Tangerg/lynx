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
	if _, ok := bb.GetCondition("x"); ok {
		t.Fatal("missing condition should not report ok")
	}
	bb.SetCondition("x", true)
	v, ok := bb.GetCondition("x")
	if !ok || !v {
		t.Fatalf("got %v ok=%v", v, ok)
	}
}
