package runtime_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/runtime"
)

func TestSnapshotTreeRoundTripKeepsExecutionState(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	definition := buildSnapshotAgent()
	mustDeploy(t, engine, definition)

	process, err := engine.Run(
		t.Context(),
		definition,
		core.Input(ssWord{Text: "lynx"}),
		core.ProcessOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	tree, err := engine.SnapshotTree(t.Context(), process.ID())
	if err != nil {
		t.Fatalf("SnapshotTree: %v", err)
	}
	if len(tree.Snapshots) != 1 || tree.Snapshots[0].ID != process.ID() {
		t.Fatalf("snapshot tree = %#v", tree)
	}
	if err := engine.RemoveTree(t.Context(), process.ID()); err != nil {
		t.Fatalf("RemoveTree: %v", err)
	}

	restored, err := engine.RestoreTree(
		t.Context(),
		tree,
		core.ProcessOptions{},
	)
	if err != nil {
		t.Fatalf("RestoreTree: %v", err)
	}
	result, ok := core.Result[ssWordCount](restored)
	if !ok || result.Count != 4 {
		t.Fatalf("restored result = %#v, %v", result, ok)
	}
	if _, exists := engine.Process(process.ID()); !exists {
		t.Fatal("restored process is not registered")
	}
}

func TestValidateRestoreTreeAcceptsEquivalentDeploymentAcrossEngines(t *testing.T) {
	definition := buildSnapshotAgent()
	source := agent.MustNewEngine(runtime.Config{})
	mustDeploy(t, source, definition)
	process, err := source.Run(
		t.Context(),
		definition,
		core.Input(ssWord{Text: "lynx"}),
		core.ProcessOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	tree, err := source.SnapshotTree(t.Context(), process.ID())
	if err != nil {
		t.Fatal(err)
	}

	target := agent.MustNewEngine(runtime.Config{})
	mustDeploy(t, target, definition)
	if err := target.ValidateRestoreTree(tree); err != nil {
		t.Fatalf("ValidateRestoreTree: %v", err)
	}
}

func TestValidateRestoreTreeRejectsUnknownBlackboardType(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	definition := buildSnapshotAgent()
	mustDeploy(t, engine, definition)
	process, err := engine.Run(
		t.Context(),
		definition,
		core.Input(ssWord{Text: "lynx"}),
		core.ProcessOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	tree, err := engine.SnapshotTree(t.Context(), process.ID())
	if err != nil {
		t.Fatal(err)
	}
	root := &tree.Snapshots[0]
	for key, value := range root.Blackboard {
		value.Type = "test/unknown"
		value.Value = json.RawMessage(`0`)
		root.Blackboard[key] = value
		break
	}
	if err := engine.ValidateRestoreTree(tree); !errors.Is(err, core.ErrInvalidSnapshot) {
		t.Fatalf("ValidateRestoreTree error = %v, want ErrInvalidSnapshot", err)
	}
}

func TestSnapshotTreeRejectsActiveRun(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	entered := make(chan struct{})
	release := make(chan struct{})
	definition := agent.New(agent.AgentConfig{
		Name: "active-snapshot",
		Actions: []agent.Action{agent.NewAction(
			"block",
			func(context.Context, *core.ProcessContext, ssWord) (ssWordCount, error) {
				close(entered)
				<-release
				return ssWordCount{Count: 1}, nil
			},
			core.ActionConfig{},
		)},
		Goals: []*agent.Goal{
			agent.NewOutputGoal[ssWordCount](core.GoalConfig{Description: "done"}),
		},
	})
	segment, err := engine.Start(t.Context(), definition, core.Input(ssWord{Text: "lynx"}), core.ProcessOptions{})
	if err != nil {
		t.Fatal(err)
	}
	<-entered

	_, err = engine.SnapshotTree(t.Context(), segment.Process().ID())
	if !errors.Is(err, runtime.ErrProcessRunning) {
		t.Fatalf("SnapshotTree error = %v, want ErrProcessRunning", err)
	}
	close(release)
	if _, err := segment.Await(t.Context()); err != nil {
		t.Fatal(err)
	}
}
