package runtime

import (
	"strings"
	"testing"
)

func TestChildRunRejectsUnknownBlackboardMode(t *testing.T) {
	const unknownMode childBlackboardMode = 255
	run := childRun{mode: unknownMode}

	_, err := run.processOptions(&Process{}, nil)
	if err == nil {
		t.Fatal("processOptions accepted an unknown child blackboard mode")
	}
	if !strings.Contains(err.Error(), "invalid child blackboard mode 255") {
		t.Fatalf("processOptions error = %q, want invalid mode detail", err)
	}
}

func TestChildRunCopiesParentBlackboardExplicitly(t *testing.T) {
	const stateKey = "working-state"
	parentBlackboard := newInMemoryBlackboard()
	parentBlackboard.Store(stateKey, "parent")
	parent := &Process{blackboard: parentBlackboard}

	options, err := (childRun{mode: childCopiesParentState}).processOptions(parent, nil)
	if err != nil {
		t.Fatalf("processOptions: %v", err)
	}
	if options.Blackboard == parentBlackboard {
		t.Fatal("child blackboard aliases its parent")
	}
	if got, ok := options.Blackboard.Load(stateKey); !ok || got != "parent" {
		t.Fatalf("child state = %v, %v; want parent, true", got, ok)
	}

	options.Blackboard.Store(stateKey, "child")
	if got, _ := parentBlackboard.Load(stateKey); got != "parent" {
		t.Fatalf("parent state changed through child clone: %v", got)
	}
}
