package runtime_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/runtime"
)

type declaredIn struct{ Text string }

type declaredOut struct{ Text string }

type undeclaredArtifact struct{ Note string }

func declaringAgent() *core.Agent {
	return agent.New(agent.Config{
		Name: "declares-its-state",
		Actions: []agent.Action{agent.NewAction("op", func(context.Context, *core.ProcessContext, declaredIn) (declaredOut, error) {
			return declaredOut{}, nil
		}, core.ActionConfig{})},
		Goals: []*agent.Goal{agent.NewOutputGoal[declaredOut](core.GoalConfig{Description: "done"})},
	})
}

// TestBlackboardWritesAgreeWithTheSnapshotCodec pins the two ends of one
// invariant to each other. The blackboard used to accept any value it could
// serialize while the snapshot codec accepted only a declared type, so an
// action recording an undeclared artifact succeeded and left the process
// unsnapshottable — the write reported success and the failure surfaced at the
// next checkpoint, with nothing connecting it to the write that caused it.
//
// Both directions matter: a gate that rejected declared state too would be
// caught here rather than in the first process that tried to run.
func TestBlackboardWritesAgreeWithTheSnapshotCodec(t *testing.T) {
	definition := declaringAgent()
	engine := agent.MustNewEngine(runtime.Config{})
	blackboard, err := engine.NewBlackboard(definition)
	if err != nil {
		t.Fatalf("NewBlackboard: %v", err)
	}

	for _, declared := range []any{declaredIn{Text: "in"}, declaredOut{Text: "out"}, "builtin", 7, nil} {
		if err := blackboard.Store("value", declared); err != nil {
			t.Fatalf("Store rejected declared state %T: %v", declared, err)
		}
		var bindings core.Bindings
		bindings.Set("value", declared)
		if _, _, err := definition.EncodeBlackboard(bindings, nil); err != nil {
			t.Fatalf("codec rejected declared state %T the blackboard accepted: %v", declared, err)
		}
	}

	writers := map[string]func(any) error{
		"Store": func(value any) error { return blackboard.Store("artifact", value) },
		"Add":   blackboard.Add,
		"Bind":  blackboard.Bind,
		"StoreAll": func(value any) error {
			var bindings core.Bindings
			bindings.Set("artifact", value)
			return blackboard.StoreAll(bindings)
		},
	}
	for name, write := range writers {
		t.Run(name, func(t *testing.T) {
			err := write(undeclaredArtifact{Note: "n"})
			if err == nil {
				t.Fatal("accepted state no snapshot of this process could restore")
			}
			if !errors.Is(err, core.ErrUndeclaredSnapshotType) {
				t.Fatalf("error = %v, want it to wrap core.ErrUndeclaredSnapshotType", err)
			}
			if _, ok := blackboard.Load("artifact"); ok {
				t.Fatal("a rejected write still landed on the blackboard")
			}
		})
	}
}

// TestDeclaredBlackboardKeepsItsGateAcrossClone guards the escape route: a
// branch or child inherits state under a codec, and cloning must not be a way
// to widen what the copy will accept.
func TestDeclaredBlackboardKeepsItsGateAcrossClone(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	blackboard, err := engine.NewBlackboard(declaringAgent())
	if err != nil {
		t.Fatalf("NewBlackboard: %v", err)
	}
	clone, err := blackboard.Clone()
	if err != nil {
		t.Fatalf("Clone: %v", err)
	}
	if err := clone.Store("artifact", undeclaredArtifact{Note: "n"}); !errors.Is(err, core.ErrUndeclaredSnapshotType) {
		t.Fatalf("clone Store error = %v, want core.ErrUndeclaredSnapshotType", err)
	}
	if err := clone.Store("value", declaredIn{Text: "in"}); err != nil {
		t.Fatalf("clone rejected declared state: %v", err)
	}
}

// TestDeclaredBlackboardKeepsTheCaptureSurface pins that the write gate does
// not cost the process its snapshot. A decorator that merely embedded the
// blackboard would hide the optional capture interfaces the engine finds by
// type assertion, turning every gated process into an uncapturable one.
func TestDeclaredBlackboardKeepsTheCaptureSurface(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	blackboard, err := engine.NewBlackboard(declaringAgent())
	if err != nil {
		t.Fatalf("NewBlackboard: %v", err)
	}
	if err := blackboard.Store("value", declaredIn{Text: "in"}); err != nil {
		t.Fatalf("Store: %v", err)
	}

	snapshotter, ok := blackboard.(runtime.BlackboardSnapshotter)
	if !ok {
		t.Fatal("the gated blackboard hides BlackboardSnapshotter")
	}
	state, err := snapshotter.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if _, found := state.Bindings.Get("value"); !found {
		t.Fatal("captured state lost the stored binding")
	}

	restorer, ok := blackboard.(runtime.BlackboardRestorer)
	if !ok {
		t.Fatal("the gated blackboard hides BlackboardRestorer")
	}
	if err := restorer.Restore(state); err != nil {
		t.Fatalf("Restore: %v", err)
	}
}

func TestProcessRejectsExistingBlackboardStateItsAgentCannotRestore(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	blackboard, err := engine.NewBlackboard(declaringAgent())
	if err != nil {
		t.Fatalf("NewBlackboard: %v", err)
	}
	if err := blackboard.Store("input", declaredIn{Text: "in"}); err != nil {
		t.Fatalf("Store: %v", err)
	}
	incompatible := agent.New(agent.Config{
		Name: "different-state-schema",
		Actions: []agent.Action{agent.NewAction("op", func(context.Context, *core.ProcessContext, string) (string, error) {
			return "done", nil
		}, core.ActionConfig{})},
		Goals: []*agent.Goal{agent.NewOutputGoal[string](core.GoalConfig{Description: "done"})},
	})

	process, err := engine.Run(t.Context(), incompatible, core.Bindings{}, core.ProcessOptions{Blackboard: blackboard})
	if process != nil {
		t.Fatalf("Run returned process %q with incompatible existing state", process.ID())
	}
	if !errors.Is(err, core.ErrUndeclaredSnapshotType) {
		t.Fatalf("Run error = %v, want core.ErrUndeclaredSnapshotType", err)
	}
}

func TestDeclaredBlackboardRejectsUndeclaredRestoreState(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	blackboard, err := engine.NewBlackboard(declaringAgent())
	if err != nil {
		t.Fatalf("NewBlackboard: %v", err)
	}
	if err := blackboard.Store("value", declaredIn{Text: "before"}); err != nil {
		t.Fatalf("Store: %v", err)
	}
	restorer := blackboard.(runtime.BlackboardRestorer)

	err = restorer.Restore(runtime.BlackboardState{
		Objects: []any{undeclaredArtifact{Note: "injected"}},
	})
	if !errors.Is(err, core.ErrUndeclaredSnapshotType) {
		t.Fatalf("Restore error = %v, want core.ErrUndeclaredSnapshotType", err)
	}
	value, ok := blackboard.Load("value")
	if !ok || value != (declaredIn{Text: "before"}) {
		t.Fatalf("rejected Restore mutated blackboard: value=%v found=%v", value, ok)
	}
}
