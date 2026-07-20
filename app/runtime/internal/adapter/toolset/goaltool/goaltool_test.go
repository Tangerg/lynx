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
func (s *memStore) Save(_ context.Context, g goal.Goal) error { s.goals[g.SessionID] = g; return nil }
func (s *memStore) Clear(_ context.Context, id string) error  { delete(s.goals, id); return nil }
func (s *memStore) List(context.Context) ([]goal.Goal, error) { return nil, nil }

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
	g, _ := goal.New("s1", "obj", "", "", goal.Budget{}, time.Unix(0, 0))
	_ = store.Save(context.Background(), g)

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
	g, _ := goal.New("s1", "obj", "", "", goal.Budget{}, time.Unix(0, 0))
	_ = store.Save(context.Background(), g)
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
	g, _ := goal.New("s1", "obj", "", "", goal.Budget{}, time.Unix(0, 0))
	g.Pause("user stop", time.Unix(0, 0))
	_ = store.Save(context.Background(), g)

	out, _ := newTool(t, store).update(sessionCtx("s1"), updateArgs{Status: "complete"})
	if !strings.Contains(out, "No active goal") {
		t.Fatalf("paused goal should be untouchable via update_goal, got %q", out)
	}
	if store.goals["s1"].Status != goal.StatusPaused {
		t.Fatal("paused goal must not be flipped to complete")
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
