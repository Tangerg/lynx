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
	"github.com/Tangerg/lynx/app/runtime/internal/domain/todo"
)

// recordedBoundaries answers for the runs it knows and reports "never recorded"
// for the rest, which is what an imported run looks like.
type recordedBoundaries map[string][]todo.Item

func (b recordedBoundaries) Boundary(_ context.Context, runID string) ([]todo.Item, bool, error) {
	items, recorded := b[runID]
	return items, recorded, nil
}

// refusingBoundaries fails any lookup, so a test can assert the coordinator does
// not need one.
type refusingBoundaries struct{}

func (refusingBoundaries) Boundary(context.Context, string) ([]todo.Item, bool, error) {
	return nil, false, errors.New("boundary lookup should not have been needed")
}

func boundaryCoordinator(stores testStores, boundaries TodoBoundaries) *Coordinator {
	deps := testDependencies(stores, Dependencies{Paths: testCwdResolver{}})
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

// TestRollbackPublishesTheBoundaryTodoList: the truncation carries the list the
// kept run recorded, so the session comes back holding the plan it held then
// rather than the plan the discarded work left behind.
func TestRollbackPublishesTheBoundaryTodoList(t *testing.T) {
	var plan RollbackPlan
	c := boundaryCoordinator(idleStores(&plan), recordedBoundaries{
		"run_keep": {{Content: "the plan as of the boundary", Status: todo.StatusPending}},
	})

	if err := c.applyRollback(t.Context(), "ses_A", droppedBoundary("run_keep", 4)); err != nil {
		t.Fatalf("applyRollback: %v", err)
	}
	if !plan.Todos.Recorded {
		t.Fatal("rollback plan reports no recorded boundary, want the kept run's list")
	}
	if len(plan.Todos.Items) != 1 || plan.Todos.Items[0].Content != "the plan as of the boundary" {
		t.Fatalf("rollback plan todos = %+v, want the boundary list", plan.Todos.Items)
	}
}

// TestRollbackLeavesAnUnrecordedBoundaryAlone: a boundary this runtime never
// captured (an imported run) has no value to restore. Clearing the list there
// would be a guess wearing a restore's clothes.
func TestRollbackLeavesAnUnrecordedBoundaryAlone(t *testing.T) {
	var plan RollbackPlan
	c := boundaryCoordinator(idleStores(&plan), recordedBoundaries{})

	if err := c.applyRollback(t.Context(), "ses_A", droppedBoundary("run_imported", 4)); err != nil {
		t.Fatalf("applyRollback: %v", err)
	}
	if plan.Todos.Recorded || plan.Todos.Items != nil {
		t.Fatalf("rollback plan todos = %+v (recorded %v), want nothing to apply", plan.Todos.Items, plan.Todos.Recorded)
	}
}

// TestRollbackToBeforeEveryRunClearsWithoutALookup: dropping the whole timeline
// rewinds past every list the session ever wrote, so the value is the empty list —
// known outright. Asking a store for it would let a "not recorded" answer leave
// work standing that the rollback discarded.
func TestRollbackToBeforeEveryRunClearsWithoutALookup(t *testing.T) {
	var plan RollbackPlan
	c := boundaryCoordinator(idleStores(&plan), refusingBoundaries{})

	boundary := transcript.Boundary{Dropped: []transcript.RunNode{{ID: "run_1"}}}
	if err := c.applyRollback(t.Context(), "ses_A", boundary); err != nil {
		t.Fatalf("applyRollback: %v", err)
	}
	if !plan.Todos.Recorded || len(plan.Todos.Items) != 0 {
		t.Fatalf("rollback plan todos = %+v (recorded %v), want a known empty list", plan.Todos.Items, plan.Todos.Recorded)
	}
}

// TestForkSeedsTheBoundaryTodoList: the branch inherits the list as of the run it
// branches from — NOT the parent's live list, which belongs to work the fork does
// not copy.
func TestForkSeedsTheBoundaryTodoList(t *testing.T) {
	var plan ForkPlan
	stores := coordinatorStores{
		interrupts: &coordinatorInterrupts{pending: map[string]interrupts.Pending{}},
		snapshot: Snapshot{
			Messages: []chat.Message{chat.NewUserMessage(chat.NewTextPart("one"))},
			Runs: []transcript.Run{
				{ID: "run_1", State: execution.Completed, CreatedAt: time.Unix(1, 0), MessageMark: 1},
			},
			Todos: []todo.Item{{Content: "work after the boundary", Status: todo.StatusInProgress}},
		},
		forked: &plan,
	}
	c := boundaryCoordinator(stores, recordedBoundaries{
		"run_1": {{Content: "the plan at the boundary", Status: todo.StatusPending}},
	})

	if _, err := c.Fork(t.Context(), ForkSpec{ParentID: "ses_A"}); err != nil {
		t.Fatalf("Fork: %v", err)
	}
	if len(plan.Todos) != 1 || plan.Todos[0].Content != "the plan at the boundary" {
		t.Fatalf("fork plan todos = %+v, want the boundary list, not the parent's live one", plan.Todos)
	}
}
