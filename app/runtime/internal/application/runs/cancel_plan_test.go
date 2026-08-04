package runs

import (
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

func TestCancellationPlanPartitionsCanonicalSubtree(t *testing.T) {
	runs := cancellationTree(execution.Running)
	processes := cancellationProcesses()

	plan, err := newCancellationPlan(
		"run_a",
		runs,
		execution.TurnRef{SessionID: "ses_1", TurnID: "turn_1"},
		processes,
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
	if plan.target.processID != "process_a" || !plan.target.hasProcess {
		t.Fatalf("target process = %q, bound=%t", plan.target.processID, plan.target.hasProcess)
	}
}

func TestCancellationPlanRejectsInconsistentTreeFacts(t *testing.T) {
	tests := []struct {
		name   string
		mutate func([]transcript.Run, map[string]string)
		want   string
	}{
		{
			name: "cross session member",
			mutate: func(runs []transcript.Run, _ map[string]string) {
				runs[0].SessionID = "ses_other"
			},
			want: "belongs to session",
		},
		{
			name: "mixed live state",
			mutate: func(runs []transcript.Run, _ map[string]string) {
				runs[0].State = execution.Interrupted
				runs[0].ActiveSegmentID = ""
			},
			want: "while root",
		},
		{
			name: "missing child process",
			mutate: func(_ []transcript.Run, processes map[string]string) {
				delete(processes, "run_a1")
			},
			want: "has no executor binding",
		},
		{
			name: "duplicate process binding",
			mutate: func(_ []transcript.Run, processes map[string]string) {
				processes["run_a1"] = processes["run_b"]
			},
			want: "is bound to Runs",
		},
		{
			name: "unknown bound Run",
			mutate: func(_ []transcript.Run, processes map[string]string) {
				processes["run_unknown"] = "process_unknown"
			},
			want: "names unknown Run",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runs := cancellationTree(execution.Running)
			processes := cancellationProcesses()
			test.mutate(runs, processes)
			_, err := newCancellationPlan(
				"run_a",
				runs,
				execution.TurnRef{SessionID: "ses_1", TurnID: "turn_1"},
				processes,
				nil,
			)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("newCancellationPlan error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestCancellationRejectsLiveOwnerFactDrift(t *testing.T) {
	spec := testSegment()
	root := runForSegment(spec)
	base := liveSegment{record: Record{
		ID: spec.RunID, SegmentID: spec.SegmentID, SessionID: spec.SessionID,
		CreatedAt: spec.CreatedAt, TurnID: spec.TurnID,
		ModelSelection: spec.ModelSelection, Capabilities: spec.Capabilities,
	}}
	if err := validateCancellationLiveRoot(base, root); err != nil {
		t.Fatalf("coherent live owner: %v", err)
	}
	tests := []struct {
		name   string
		mutate func(*liveSegment)
	}{
		{"segment", func(live *liveSegment) { live.record.SegmentID = "seg_other" }},
		{"creation time", func(live *liveSegment) { live.record.CreatedAt = spec.CreatedAt.Add(time.Second) }},
		{"model", func(live *liveSegment) { live.record.ModelSelection = mustSelection("anthropic", "model") }},
		{"run capabilities", func(live *liveSegment) {
			live.record.Capabilities.ChildRuns = true
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			live := base
			test.mutate(&live)
			if err := validateCancellationLiveRoot(live, root); err == nil {
				t.Fatalf("accepted %s drift between live owner and durable Run", test.name)
			}
		})
	}
}

func cancellationTree(state execution.RunState) []transcript.Run {
	createdAt := time.Date(2026, 7, 30, 1, 2, 3, 0, time.UTC)
	run := func(id, parent string) transcript.Run {
		value := transcript.Run{
			ID: id, SessionID: "ses_1", State: state,
			ActiveSegmentID: "segment_" + id, CreatedAt: createdAt,
			UpdatedAt: createdAt, MessageMark: transcript.UnknownMessageMark,
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

func cancellationProcesses() map[string]string {
	return map[string]string{
		"run_root": "process_root",
		"run_a":    "process_a",
		"run_a0":   "process_a0",
		"run_a1":   "process_a1",
		"run_b":    "process_b",
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
