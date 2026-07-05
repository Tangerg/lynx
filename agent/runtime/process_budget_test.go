package runtime

import (
	"sync"
	"testing"
)

// TestProcessBudget_RemoveChild pins the inverse of addChild used when a child
// fails to fully spawn (e.g. session link fails) and is unregistered: the
// parent's children rollup must drop it, with no stale reference left behind.
// Removing a non-member is a no-op. Child spawn exercises the integration path;
// this pins the primitive directly since the leak is otherwise inert —
// a never-started child contributes 0 to usage.)
func TestProcessBudget_RemoveChild(t *testing.T) {
	var mu sync.RWMutex
	b := &processBudget{lock: &mu}
	a, c := &AgentProcess{}, &AgentProcess{}
	b.addChild(a)
	b.addChild(c)

	b.removeChild(a)
	if len(b.children) != 1 || b.children[0] != c {
		t.Fatalf("after removeChild(a): children = %v, want [c]", b.children)
	}

	// Removing one that isn't tracked leaves the slice untouched.
	b.removeChild(&AgentProcess{})
	if len(b.children) != 1 || b.children[0] != c {
		t.Fatalf("removeChild of a non-member mutated children = %v, want [c]", b.children)
	}

	b.removeChild(c)
	if len(b.children) != 0 {
		t.Fatalf("after removing all: children = %v, want empty", b.children)
	}
}
