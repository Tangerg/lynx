package runtime_test

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/runtime"
)

type ssWord struct{ Text string }
type ssWordCount struct{ Count int }

func snapshotRoot(t *testing.T, engine *runtime.Engine, process *runtime.Process) core.ProcessSnapshot {
	t.Helper()
	tree, err := engine.SnapshotTree(t.Context(), process.ID())
	if err != nil {
		t.Fatal(err)
	}
	for _, snapshot := range tree.Snapshots {
		if snapshot.ID == tree.RootID {
			return snapshot
		}
	}
	t.Fatalf("snapshot tree %q has no root", tree.RootID)
	return core.ProcessSnapshot{}
}

func restoreRoot(
	t *testing.T,
	engine *runtime.Engine,
	snapshot core.ProcessSnapshot,
	options core.ProcessOptions,
) *runtime.Process {
	t.Helper()
	process, err := engine.RestoreTree(t.Context(), core.ProcessSnapshotTree{
		RootID:    snapshot.ID,
		Snapshots: []core.ProcessSnapshot{snapshot},
	}, options)
	if err != nil {
		t.Fatal(err)
	}
	return process
}

func buildSnapshotAgent() *core.Agent {
	return agent.New(agent.AgentConfig{
		Name: "snapshot-agent",
		Actions: []agent.Action{agent.NewAction(
			"count",
			func(_ context.Context, _ *core.ProcessContext, in ssWord) (ssWordCount, error) {
				return ssWordCount{Count: len(in.Text)}, nil
			},
			core.ActionConfig{},
		)},
		Goals: []*agent.Goal{
			agent.NewOutputGoal[ssWordCount](core.GoalConfig{Description: "word counted"}),
		},
	})
}

func admissionAgent() *core.Agent {
	return agent.New(agent.AgentConfig{
		Name: "admission-agent",
		Actions: []agent.Action{agent.NewAction(
			"count",
			func(_ context.Context, _ *core.ProcessContext, in word) (wordCount, error) {
				return wordCount{Count: len(in.Text)}, nil
			},
			core.ActionConfig{},
		)},
		Goals: []*agent.Goal{
			agent.NewOutputGoal[wordCount](core.GoalConfig{Description: "counted"}),
		},
	})
}
