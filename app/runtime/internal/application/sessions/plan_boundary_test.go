package sessions

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/lynx/core/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/plan"
)

// recordedBoundaries answers for the runs it knows and reports "never recorded"
// for the rest, which is what an imported run looks like.
type recordedBoundaries map[string][]plan.Step

func (b recordedBoundaries) Boundary(_ context.Context, runID string) ([]plan.Step, bool, error) {
	items, recorded := b[runID]
	return items, recorded, nil
}

// refusingBoundaries fails any lookup, so a test can assert the coordinator does
// not need one.
type refusingBoundaries struct{}

func (refusingBoundaries) Boundary(context.Context, string) ([]plan.Step, bool, error) {
	return nil, false, errors.New("boundary lookup should not have been needed")
}

func boundaryCoordinator(stores testStores, boundaries PlanBoundaries) *Coordinator {
	deps := testDependencies(stores, Dependencies{Paths: testCWDResolver{}})
	deps.Boundaries = boundaries
	return New(deps)
}

func idleStores(rolledBack *RollbackPlan) coordinatorStores {
	return coordinatorStores{
		interrupts: &coordinatorInterrupts{pending: map[string]interrupts.Pending{}},
		rolledBack: rolledBack,
	}
}

func droppedBoundary(keepRunID string, keepMark int) transcript.Boundary {
	return transcript.Boundary{
		KeepMark:  keepMark,
		KeepRunID: keepRunID,
		Dropped:   []transcript.RunNode{{ID: "run_dropped", Mark: keepMark + 2}},
	}
}

// TestRollbackPublishesTheBoundaryPlanList: the truncation carries the list the
// kept run recorded, so the session comes back holding the plan it held then
// rather than the plan the discarded work left behind.
func TestRollbackPublishesTheBoundaryPlanList(t *testing.T) {
	var applied RollbackPlan
	c := boundaryCoordinator(idleStores(&applied), recordedBoundaries{
		"run_keep": {{Description: "the plan as of the boundary", Status: plan.StatusPending}},
	})

	if err := c.applyRollback(t.Context(), "ses_A", droppedBoundary("run_keep", 4)); err != nil {
		t.Fatalf("applyRollback: %v", err)
	}
	if !applied.Plan.Recorded {
		t.Fatal("rollback plan reports no recorded boundary, want the kept run's list")
	}
	if len(applied.Plan.Steps) != 1 || applied.Plan.Steps[0].Description != "the plan as of the boundary" {
		t.Fatalf("rollback plan plan = %+v, want the boundary list", applied.Plan.Steps)
	}
}

// TestRollbackLeavesAnUnrecordedBoundaryAlone: a boundary this runtime never
// captured (an imported run) has no value to restore. Clearing the list there
// would be a guess wearing a restore's clothes.
func TestRollbackLeavesAnUnrecordedBoundaryAlone(t *testing.T) {
	var applied RollbackPlan
	c := boundaryCoordinator(idleStores(&applied), recordedBoundaries{})

	if err := c.applyRollback(t.Context(), "ses_A", droppedBoundary("run_imported", 4)); err != nil {
		t.Fatalf("applyRollback: %v", err)
	}
	if applied.Plan.Recorded || applied.Plan.Steps != nil {
		t.Fatalf("rollback plan plan = %+v (recorded %v), want nothing to apply", applied.Plan.Steps, applied.Plan.Recorded)
	}
}

// TestRollbackToBeforeEveryRunClearsWithoutALookup: dropping the whole timeline
// rewinds past every list the session ever wrote, so the value is the empty list —
// known outright. Asking a store for it would let a "not recorded" answer leave
// work standing that the rollback discarded.
func TestRollbackToBeforeEveryRunClearsWithoutALookup(t *testing.T) {
	var applied RollbackPlan
	c := boundaryCoordinator(idleStores(&applied), refusingBoundaries{})

	boundary := transcript.Boundary{Dropped: []transcript.RunNode{{ID: "run_1"}}}
	if err := c.applyRollback(t.Context(), "ses_A", boundary); err != nil {
		t.Fatalf("applyRollback: %v", err)
	}
	if !applied.Plan.Recorded || len(applied.Plan.Steps) != 0 {
		t.Fatalf("rollback plan plan = %+v (recorded %v), want a known empty list", applied.Plan.Steps, applied.Plan.Recorded)
	}
}

// TestForkSeedsTheBoundaryPlanList: the branch inherits the list as of the run it
// branches from — NOT the parent's live list, which belongs to work the fork does
// not copy.
func TestForkSeedsTheBoundaryPlanList(t *testing.T) {
	var applied ForkPlan
	stores := coordinatorStores{
		interrupts: &coordinatorInterrupts{pending: map[string]interrupts.Pending{}},
		snapshot: Snapshot{
			Messages: []chat.Message{chat.NewUserMessage(chat.NewTextPart("one"))},
			Runs: []transcript.Run{
				{ID: "run_1", State: execution.Completed, CreatedAt: time.Unix(1, 0), MessageMark: 1},
			},
			Plan: []plan.Step{{Description: "work after the boundary", Status: plan.StatusInProgress}},
		},
		forked: &applied,
	}
	c := boundaryCoordinator(stores, recordedBoundaries{
		"run_1": {{Description: "the plan at the boundary", Status: plan.StatusPending}},
	})

	if _, err := c.Fork(t.Context(), ForkSpec{ParentID: "ses_A"}); err != nil {
		t.Fatalf("Fork: %v", err)
	}
	if len(applied.Plan) != 1 || applied.Plan[0].Description != "the plan at the boundary" {
		t.Fatalf("fork plan plan = %+v, want the boundary list, not the parent's live one", applied.Plan)
	}
}
