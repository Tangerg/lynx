package goaltool

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turnctx"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/goal"
)

// in-memory goal.Store for the tool tests.
type memStore struct{ goals map[string]goal.Goal }

func newMemStore() *memStore { return &memStore{goals: map[string]goal.Goal{}} }

func (s *memStore) Get(_ context.Context, id string) (goal.Goal, bool, error) {
	g, ok := s.goals[id]
	return g, ok, nil
}

// put seeds a goal directly (test setup), bypassing the CAS.
func (s *memStore) put(g goal.Goal) { s.goals[g.SessionID] = g }

func (s *memStore) Save(_ context.Context, g goal.Goal, expected goal.Version) (bool, error) {
	cur, ok := s.goals[g.SessionID]
	if expected == (goal.Version{}) {
		if ok {
			return false, nil
		}
		s.goals[g.SessionID] = g
		return true, nil
	}
	if !ok || cur.Version() != expected {
		return false, nil
	}
	s.goals[g.SessionID] = g
	return true, nil
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
	g, _ := goal.New(session, "obj", "", "", goal.Budget{}, time.Unix(0, 0))
	g.RenewLease("lease-active")
	return g
}

// fake ProcessView / blackboard: just enough for turnctx.TurnSession off ctx.
type fakeBlackboard struct {
	core.BlackboardReader
	vals map[string]any
}

func (b fakeBlackboard) Load(key string) (any, bool) { v, ok := b.vals[key]; return v, ok }

type fakeProcessView struct {
	core.ProcessView
	bb core.BlackboardReader
}

func (p fakeProcessView) Blackboard() core.BlackboardReader { return p.bb }

func sessionCtx(session string) context.Context {
	bb := fakeBlackboard{vals: map[string]any{turnctx.SessionBindingKey: session}}
	return core.WithProcessView(context.Background(), fakeProcessView{bb: bb})
}

func newTool(t *testing.T, store goal.Store) *tool {
	t.Helper()
	return &tool{store: store, now: func() time.Time { return time.Unix(0, 0) }}
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
	if got := store.goals["s1"]; got.Status != goal.StatusBlocked || got.Reason != "needs a key" {
		t.Fatalf("stored = (%q, %q)", got.Status, got.Reason)
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
	g.Pause("user stop", time.Unix(0, 0))
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
// stamped with an OLD goal incarnation (turnctx.GoalLeaseBindingKey) must not
// signal the current goal, which a fresh Start gave a new lease.
func TestUpdateGoal_SupersededStampRefused(t *testing.T) {
	store := newMemStore()
	current := activeGoal("s1")
	current.LeaseID = "lease-current"
	store.put(current)

	// The run carries the lease it was launched under, since superseded.
	bb := fakeBlackboard{vals: map[string]any{
		turnctx.SessionBindingKey:   "s1",
		turnctx.GoalLeaseBindingKey: "lease-stale",
	}}
	ctx := core.WithProcessView(context.Background(), fakeProcessView{bb: bb})

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
