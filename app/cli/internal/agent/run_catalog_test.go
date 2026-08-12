package agent

import (
	"strings"
	"testing"
)

func TestRunQueryRejectsInvalidFilters(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name  string
		query RunQuery
		want  string
	}{
		{name: "negative limit", query: RunQuery{Limit: -1}, want: "limit"},
		{name: "unknown status", query: RunQuery{Statuses: []RunStatus{"paused"}}, want: "paused"},
		{name: "duplicate status", query: RunQuery{Statuses: []RunStatus{RunStatusRunning, RunStatusRunning}}, want: "repeated"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if err := test.query.Validate(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() = %v, want error containing %q", err, test.want)
			}
		})
	}
}

func TestRunPageValidatesItemsIndependentlyOfTreePageBoundaries(t *testing.T) {
	t.Parallel()
	page := RunPage{Items: []Run{{
		ID: "run_child", SessionID: "ses_1",
		Lineage: RunLineage{SpawnedByBlockID: "item_spawn", ParentRunID: "run_parent", RootRunID: "run_root"},
		Status:  RunStatusWaiting,
	}}}
	if err := page.Validate(); err != nil {
		t.Fatalf("Validate() = %v", err)
	}
	page.Items = append(page.Items, page.Items[0])
	if err := page.Validate(); err == nil || !strings.Contains(err.Error(), "repeats id") {
		t.Fatalf("duplicate Validate() = %v", err)
	}
}

func TestRunCancellationClosesRootAndChildResults(t *testing.T) {
	t.Parallel()
	rootCanceled := Run{
		ID: "run_root", SessionID: "ses_1", Status: RunStatusFinished,
		Outcome: Outcome{Status: OutcomeCanceled},
	}
	if err := (RunCancellation{Canceled: rootCanceled, Root: rootCanceled}).Validate(); err != nil {
		t.Fatalf("root cancellation: %v", err)
	}

	childCanceled := Run{
		ID: "run_child", SessionID: "ses_1",
		Lineage: RunLineage{SpawnedByBlockID: "item_spawn", ParentRunID: "run_root", RootRunID: "run_root"},
		Status:  RunStatusFinished, Outcome: Outcome{Status: OutcomeCanceled},
	}
	rootWaiting := Run{ID: "run_root", SessionID: "ses_1", Status: RunStatusWaiting}
	if err := (RunCancellation{Canceled: childCanceled, Root: rootWaiting}).Validate(); err != nil {
		t.Fatalf("child cancellation: %v", err)
	}

	wrongRoot := rootWaiting
	wrongRoot.ID = "run_other"
	if err := (RunCancellation{Canceled: childCanceled, Root: wrongRoot}).Validate(); err == nil || !strings.Contains(err.Error(), "does not belong") {
		t.Fatalf("wrong-root cancellation = %v", err)
	}
	if err := (RunCancellation{Canceled: rootCanceled, Root: rootCanceled}).ValidateTarget("run_other"); err == nil || !strings.Contains(err.Error(), "want") {
		t.Fatalf("wrong-target cancellation = %v", err)
	}
}
