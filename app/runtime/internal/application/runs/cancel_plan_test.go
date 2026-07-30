package runs

import (
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

func TestCancellationPlanPartitionsCanonicalSubtree(t *testing.T) {
	runs := cancellationTree(execution.Running)
	sources := cancellationSources()

	plan, err := newCancellationPlan(
		"run_a",
		runs,
		execution.TurnRef{SessionID: "ses_1", TurnID: "turn_1"},
		sources,
		nil,
	)
	if err != nil {
		t.Fatalf("newCancellationPlan: %v", err)
	}
	if plan.root.run.ID != "run_root" || plan.target.run.ID != "run_a" {
		t.Fatalf("root/target = %q/%q, want run_root/run_a", plan.root.run.ID, plan.target.run.ID)
	}
	if plan.treeState != execution.Running || plan.hasPending {
		t.Fatalf("tree state/pending = %s/%t, want running/false", plan.treeState, plan.hasPending)
	}
	if got, want := cancellationRunIDs(plan.targetSubtree), []string{"run_a0", "run_a1", "run_a"}; !sameStrings(got, want) {
		t.Fatalf("target subtree = %v, want %v", got, want)
	}
	if got, want := cancellationRunIDs(plan.survivingTree), []string{"run_b", "run_root"}; !sameStrings(got, want) {
		t.Fatalf("surviving tree = %v, want %v", got, want)
	}
	if plan.target.source.ProcessID != "process_a" || !plan.target.hasSource {
		t.Fatalf("target source = %+v, bound=%t", plan.target.source, plan.target.hasSource)
	}
}

func TestCancellationPlanRejectsInconsistentTreeFacts(t *testing.T) {
	tests := []struct {
		name   string
		mutate func([]transcript.Run, map[string]ExecutorSource)
		want   string
	}{
		{
			name: "cross session member",
			mutate: func(runs []transcript.Run, _ map[string]ExecutorSource) {
				runs[0].SessionID = "ses_other"
			},
			want: "belongs to session",
		},
		{
			name: "mixed live state",
			mutate: func(runs []transcript.Run, _ map[string]ExecutorSource) {
				runs[0].State = execution.Interrupted
				runs[0].ActiveSegmentID = ""
			},
			want: "while root",
		},
		{
			name: "missing child process",
			mutate: func(_ []transcript.Run, sources map[string]ExecutorSource) {
				delete(sources, "run_a1")
			},
			want: "has no executor binding",
		},
		{
			name: "wrong process parent",
			mutate: func(_ []transcript.Run, sources map[string]ExecutorSource) {
				source := sources["run_a1"]
				source.ParentID = "process_b"
				sources["run_a1"] = source
			},
			want: "differs from parent Run",
		},
		{
			name: "unknown bound Run",
			mutate: func(_ []transcript.Run, sources map[string]ExecutorSource) {
				sources["run_unknown"] = ExecutorSource{ProcessID: "process_unknown"}
			},
			want: "names unknown Run",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runs := cancellationTree(execution.Running)
			sources := cancellationSources()
			test.mutate(runs, sources)
			_, err := newCancellationPlan(
				"run_a",
				runs,
				execution.TurnRef{SessionID: "ses_1", TurnID: "turn_1"},
				sources,
				nil,
			)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("newCancellationPlan error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func cancellationTree(state execution.RunState) []transcript.Run {
	run := func(id, parent string) transcript.Run {
		value := transcript.Run{
			ID: id, SessionID: "ses_1", State: state,
			ActiveSegmentID: "segment_" + id,
		}
		if parent != "" {
			value.SpawnedByItemID = "item_" + id
			value.ParentRunID = parent
			value.RootRunID = "run_root"
		}
		return value
	}
	return []transcript.Run{
		run("run_a0", "run_a"),
		run("run_b", "run_root"),
		run("run_root", ""),
		run("run_a1", "run_a"),
		run("run_a", "run_root"),
	}
}

func cancellationSources() map[string]ExecutorSource {
	return map[string]ExecutorSource{
		"run_root": {ProcessID: "process_root"},
		"run_a": {
			ProcessID: "process_a", ParentID: "process_root", SpawnCallID: "spawn_a",
		},
		"run_a0": {
			ProcessID: "process_a0", ParentID: "process_a", SpawnCallID: "spawn_a0",
		},
		"run_a1": {
			ProcessID: "process_a1", ParentID: "process_a", SpawnCallID: "spawn_a1",
		},
		"run_b": {
			ProcessID: "process_b", ParentID: "process_root", SpawnCallID: "spawn_b",
		},
	}
}

func cancellationRunIDs(runs []cancellationRun) []string {
	ids := make([]string, len(runs))
	for index, run := range runs {
		ids[index] = run.run.ID
	}
	return ids
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
