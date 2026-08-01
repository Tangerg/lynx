package goaltool

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/executionctx"
	"github.com/Tangerg/lynx/app/runtime/internal/application/goals"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/goal"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
)

// in-memory goals.Store for the tool tests.
type memStore struct{ goals map[string]goal.Goal }

func newMemStore() *memStore { return &memStore{goals: map[string]goal.Goal{}} }

func (s *memStore) Get(_ context.Context, id string) (goal.Goal, bool, error) {
	g, ok := s.goals[id]
	return g, ok, nil
}

// put seeds a goal directly (test setup), bypassing the CAS.
func (s *memStore) put(g goal.Goal) { s.goals[g.SessionID] = g }

func (s *memStore) Save(_ context.Context, g goal.Goal, expected goal.Version) (goal.Goal, bool, error) {
	cur, ok := s.goals[g.SessionID]
	if expected == (goal.Version{}) {
		if ok {
			return goal.Goal{}, false, nil
		}
		g.Revision = 1
		s.goals[g.SessionID] = g
		return g, true, nil
	}
	if !ok || cur.Version() != expected {
		return goal.Goal{}, false, nil
	}
	g.Revision = expected.Revision + 1
	s.goals[g.SessionID] = g
	return g, true, nil
}
func (s *memStore) Clear(_ context.Context, id string) error { delete(s.goals, id); return nil }
func (s *memStore) ClearIf(_ context.Context, id string, expected goal.Version) (bool, error) {
	cur, ok := s.goals[id]
	if !ok || cur.Version() != expected {
		return false, nil
	}
	delete(s.goals, id)
	return true, nil
}
func (s *memStore) List(context.Context) ([]goal.Goal, error) { return nil, nil }

// activeGoal builds a stored active goal with an opaque current lease.
func activeGoal(session string) goal.Goal {
	g, _ := goal.New(session, "obj", modelref.Selection{}, goal.Budget{}, "lease-active", time.Unix(0, 0))
	return g
}

func sessionCtx(session string) context.Context {
	return executionctx.WithScope(context.Background(), execution.TurnScope{SessionID: session})
}

func newTool(t *testing.T, store goals.Store) *tool {
	t.Helper()
	return &tool{state: goals.NewState(store)}
}

func TestUpdateGoal_Complete(t *testing.T) {
	store := newMemStore()
	store.put(activeGoal("s1"))

	out, err := newTool(t, store).update(sessionCtx("s1"), updateArgs{Status: "complete"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "complete") {
		t.Fatalf("output = %q", out)
	}
	if got := store.goals["s1"]; got.Status != goal.StatusComplete {
		t.Fatalf("stored status = %q, want complete", got.Status)
	}
}

func TestUpdateGoal_BlockedRequiresReason(t *testing.T) {
	store := newMemStore()
	store.put(activeGoal("s1"))
	tl := newTool(t, store)

	out, _ := tl.update(sessionCtx("s1"), updateArgs{Status: "blocked"})
	if !strings.Contains(out, "reason") {
		t.Fatalf("blocked without reason = %q, want a reason prompt", out)
	}
	if store.goals["s1"].Status != goal.StatusActive {
		t.Fatal("goal should stay active when blocked reason is missing")
	}

	out, _ = tl.update(sessionCtx("s1"), updateArgs{Status: "blocked", Reason: "needs a key"})
	if !strings.Contains(out, "blocked") {
		t.Fatalf("output = %q", out)
	}
	if got := store.goals["s1"]; got.Status != goal.StatusBlocked || got.Reason != (goal.Reason{Cause: goal.ReasonBlockedByModel, Detail: "needs a key"}) {
		t.Fatalf("stored = (%q, %+v)", got.Status, got.Reason)
	}
}

func TestUpdateGoal_NoActiveGoal(t *testing.T) {
	store := newMemStore() // no goal for s1
	out, err := newTool(t, store).update(sessionCtx("s1"), updateArgs{Status: "complete"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "No active goal") {
		t.Fatalf("output = %q, want a no-active-goal message", out)
	}
}

func TestUpdateGoal_NonActiveGoalNotTouched(t *testing.T) {
	store := newMemStore()
	g := activeGoal("s1")
	g.Pause(goal.ReasonStoppedByUser, "", time.Unix(0, 0))
	store.put(g)

	out, _ := newTool(t, store).update(sessionCtx("s1"), updateArgs{Status: "complete"})
	if !strings.Contains(out, "No active goal") {
		t.Fatalf("paused goal should be untouchable via update_goal, got %q", out)
	}
	if store.goals["s1"].Status != goal.StatusPaused {
		t.Fatal("paused goal must not be flipped to complete")
	}
}

// TestUpdateGoal_SupersededStampRefused verifies the race-#4 guard: a run
// stamped with an OLD goal incarnation must not
// signal the current goal, which a fresh Start gave a new lease.
func TestUpdateGoal_SupersededStampRefused(t *testing.T) {
	store := newMemStore()
	current := activeGoal("s1")
	current.LeaseID = "lease-current"
	store.put(current)

	// The run carries the lease it was launched under, since superseded.
	ctx := executionctx.WithScope(context.Background(), execution.TurnScope{
		SessionID:   "s1",
		GoalLeaseID: "lease-stale",
	})

	out, err := newTool(t, store).update(ctx, updateArgs{Status: "complete"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "superseded") {
		t.Fatalf("output = %q, want a superseded-goal refusal", out)
	}
	if store.goals["s1"].Status != goal.StatusActive {
		t.Fatal("a straggler run must not flip the current goal to complete")
	}
}

func TestUpdateGoal_NoSession(t *testing.T) {
	out, err := newTool(t, newMemStore()).update(context.Background(), updateArgs{Status: "complete"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "no active session") {
		t.Fatalf("output = %q, want a no-session message", out)
	}
}
