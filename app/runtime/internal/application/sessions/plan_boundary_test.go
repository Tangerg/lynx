package sessions

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/lynx/core/chat"

	planapp "github.com/Tangerg/lynx/app/runtime/internal/application/plans"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/plan"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/testsupport/runfixture"
	"github.com/Tangerg/lynx/app/runtime/internal/testsupport/sessionfixture"
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
	deps := testDependencies(stores, Dependencies{Paths: testWorkspaceResolver{}})
	deps.Plan = &PlanServices{
		Boundaries: boundaries,
		Replacements: planapp.New(planapp.Dependencies{
			Store: boundaryPlanStore{}, Now: func() time.Time { return time.Unix(100, 0) },
		}),
	}
	return mustNewCoordinator(deps)
}

type boundaryPlanStore struct{}

func (boundaryPlanStore) State(context.Context, string) (plan.State, error)      { return plan.State{}, nil }
func (boundaryPlanStore) Save(context.Context, string, uint64, plan.State) error { return nil }

func replacementSteps(replacement *planapp.Replacement) []plan.Step {
	if replacement == nil {
		return nil
	}
	return replacement.State().Steps()
}

func idleStores(rolledBack *RollbackPlan) coordinatorStores {
	return coordinatorStores{
		interrupts: &coordinatorInterrupts{pending: map[string]runs.Pending{}},
		rolledBack: rolledBack,
	}
}

func droppedBoundary(keepRunID string, keepMark int) transcript.Boundary {
	return transcript.Boundary{
		KeepMessageMark: keepMark,
		KeepRunID:       keepRunID,
		Dropped:         []transcript.RunNode{{ID: "run_dropped", MessageMark: keepMark + 2}},
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
	steps := replacementSteps(applied.PlanReplacement)
	if len(steps) != 1 || steps[0].Description != "the plan as of the boundary" {
		t.Fatalf("rollback Plan replacement = %+v, want the boundary list", steps)
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
	if applied.PlanReplacement != nil {
		t.Fatalf("rollback Plan replacement = %+v, want nothing to apply", replacementSteps(applied.PlanReplacement))
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
	if applied.PlanReplacement == nil || len(replacementSteps(applied.PlanReplacement)) != 0 {
		t.Fatalf("rollback Plan replacement = %+v, want a known empty list", replacementSteps(applied.PlanReplacement))
	}
}

// TestForkSeedsTheBoundaryPlanList: the branch inherits the list as of the run it
// branches from — NOT the parent's live list, which belongs to work the fork does
// not copy.
func TestForkSeedsTheBoundaryPlanList(t *testing.T) {
	var applied ForkPlan
	stores := coordinatorStores{
		interrupts: &coordinatorInterrupts{pending: map[string]runs.Pending{}},
		snapshot: Snapshot{
			Session:  sessionfixture.MustRestore(session.Snapshot{ID: "ses_A"}),
			Messages: []chat.Message{chat.NewUserMessage(chat.NewTextPart("one"))},
			Runs: []run.Run{
				runfixture.MustRestore(run.Snapshot{ID: "run_1", SessionID: "ses_A", State: run.Completed, CreatedAt: time.Unix(1, 0), MessageMark: 1}),
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
	steps := replacementSteps(applied.PlanReplacement)
	if len(steps) != 1 || steps[0].Description != "the plan at the boundary" {
		t.Fatalf("fork Plan replacement = %+v, want the boundary list, not the parent's live one", steps)
	}
}
