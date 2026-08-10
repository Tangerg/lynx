package runs

import (
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	runfixture "github.com/Tangerg/lynx/app/runtime/internal/testsupport/runfixture"
)

func TestCancellationPlanPartitionsCanonicalSubtree(t *testing.T) {
	runs := cancellationTree(run.Running)
	members := cancellationMembers()

	plan, err := newCancellationPlan(
		"run_a",
		runs,
		ExecutorRef{SessionID: "ses_1", ExecutorID: "turn_1"},
		members,
		nil,
	)
	if err != nil {
		t.Fatalf("newCancellationPlan: %v", err)
	}
	if plan.root.run.ID() != "run_root" || plan.target.run.ID() != "run_a" {
		t.Fatalf("root/target = %q/%q, want run_root/run_a", plan.root.run.ID(), plan.target.run.ID())
	}
	if plan.treeState != run.Running || plan.hasPending {
		t.Fatalf("tree state/pending = %s/%t, want running/false", plan.treeState, plan.hasPending)
	}
	if got, want := cancellationRunIDs(plan.targetSubtree), []string{"run_a0", "run_a1", "run_a"}; !sameStrings(got, want) {
		t.Fatalf("target subtree = %v, want %v", got, want)
	}
	if got, want := cancellationRunIDs(plan.survivingTree), []string{"run_b", "run_root"}; !sameStrings(got, want) {
		t.Fatalf("surviving tree = %v, want %v", got, want)
	}
	if plan.target.memberID != "member_a" || !plan.target.hasMember {
		t.Fatalf("target member = %q, bound=%t", plan.target.memberID, plan.target.hasMember)
	}
}

func TestCancellationPlanRejectsInconsistentTreeFacts(t *testing.T) {
	tests := []struct {
		name   string
		mutate func([]run.Run, map[string]string)
		want   string
	}{
		{
			name: "cross session member",
			mutate: func(runs []run.Run, _ map[string]string) {
				snapshot := runs[0].Snapshot()
				snapshot.SessionID = "ses_other"
				runs[0] = runfixture.MustRestore(snapshot)
			},
			want: "belongs to session",
		},
		{
			name: "mixed live state",
			mutate: func(runs []run.Run, _ map[string]string) {
				waiting, err := runs[0].Suspend(runs[0].UpdatedAt())
				if err != nil {
					panic(err)
				}
				runs[0] = waiting
			},
			want: "while root",
		},
		{
			name: "missing child executor member",
			mutate: func(_ []run.Run, members map[string]string) {
				delete(members, "run_a1")
			},
			want: "has no executor binding",
		},
		{
			name: "duplicate executor member binding",
			mutate: func(_ []run.Run, members map[string]string) {
				members["run_a1"] = members["run_b"]
			},
			want: "is bound to Runs",
		},
		{
			name: "unknown bound Run",
			mutate: func(_ []run.Run, members map[string]string) {
				members["run_unknown"] = "member_unknown"
			},
			want: "names unknown Run",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runs := cancellationTree(run.Running)
			members := cancellationMembers()
			test.mutate(runs, members)
			_, err := newCancellationPlan(
				"run_a",
				runs,
				ExecutorRef{SessionID: "ses_1", ExecutorID: "turn_1"},
				members,
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
		CreatedAt: spec.CreatedAt, ExecutorID: spec.ExecutorID,
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

func cancellationTree(state run.State) []run.Run {
	createdAt := time.Date(2026, 7, 30, 1, 2, 3, 0, time.UTC)
	makeRun := func(id, parent string) run.Run {
		lineage := run.Lineage{}
		if parent != "" {
			lineage = run.Lineage{SpawnedByItemID: "item_" + id, ParentRunID: parent, RootRunID: "run_root"}
		}
		return runfixture.MustRestore(run.Snapshot{ID: id, SessionID: "ses_1", State: state,
			ActiveSegmentID: "segment_" + id, CreatedAt: createdAt,
			UpdatedAt: createdAt, MessageMark: run.UnknownMessageMark, Lineage: lineage})
	}
	return []run.Run{
		makeRun("run_a0", "run_a"),
		makeRun("run_b", "run_root"),
		makeRun("run_root", ""),
		makeRun("run_a1", "run_a"),
		makeRun("run_a", "run_root"),
	}
}

func cancellationMembers() map[string]string {
	return map[string]string{
		"run_root": "member_root",
		"run_a":    "member_a",
		"run_a0":   "member_a0",
		"run_a1":   "member_a1",
		"run_b":    "member_b",
	}
}

func cancellationRunIDs(runs []cancellationRun) []string {
	ids := make([]string, len(runs))
	for index, run := range runs {
		ids[index] = run.run.ID()
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
