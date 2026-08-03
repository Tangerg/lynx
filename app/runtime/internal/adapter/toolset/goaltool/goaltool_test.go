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

func newGetTool(t *testing.T, store goals.Store) *getTool {
	t.Helper()
	return &getTool{goals: goals.NewState(store)}
}

func newReportTool(t *testing.T, store goals.Store) *reportTool {
	t.Helper()
	return &reportTool{goals: goals.NewState(store)}
}

func TestReportGoalOutcomeCompleted(t *testing.T) {
	store := newMemStore()
	store.put(activeGoal("s1"))

	out, err := newReportTool(t, store).report(sessionCtx("s1"), reportArgs{Outcome: "completed"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "completed") {
		t.Fatalf("output = %q", out)
	}
	if got := store.goals["s1"]; got.Status != goal.StatusComplete {
		t.Fatalf("stored status = %q, want complete", got.Status)
	}
}

func TestReportGoalOutcomeBlockedRequiresReason(t *testing.T) {
	store := newMemStore()
	store.put(activeGoal("s1"))
	tl := newReportTool(t, store)

	out, _ := tl.report(sessionCtx("s1"), reportArgs{Outcome: "blocked"})
	if !strings.Contains(out, "reason") {
		t.Fatalf("blocked without reason = %q, want a reason prompt", out)
	}
	if store.goals["s1"].Status != goal.StatusActive {
		t.Fatal("goal should stay active when blocked reason is missing")
	}

	out, _ = tl.report(sessionCtx("s1"), reportArgs{Outcome: "blocked", Reason: " needs a key "})
	if !strings.Contains(out, "blocked") {
		t.Fatalf("output = %q", out)
	}
	if got := store.goals["s1"]; got.Status != goal.StatusBlocked || got.Reason != (goal.Reason{Cause: goal.ReasonBlockedByModel, Detail: "needs a key"}) {
		t.Fatalf("stored = (%q, %+v)", got.Status, got.Reason)
	}
}

func TestReportGoalOutcomeNoActiveGoal(t *testing.T) {
	store := newMemStore() // no goal for s1
	out, err := newReportTool(t, store).report(sessionCtx("s1"), reportArgs{Outcome: "completed"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "No active Goal") {
		t.Fatalf("output = %q, want a no-active-goal message", out)
	}
}

func TestReportGoalOutcomeDoesNotTouchPausedGoal(t *testing.T) {
	store := newMemStore()
	g := activeGoal("s1")
	g.Pause(goal.ReasonStoppedByUser, "", time.Unix(0, 0))
	store.put(g)

	out, _ := newReportTool(t, store).report(sessionCtx("s1"), reportArgs{Outcome: "completed"})
	if !strings.Contains(out, "No active Goal") {
		t.Fatalf("paused goal should be untouchable via report_goal_outcome, got %q", out)
	}
	if store.goals["s1"].Status != goal.StatusPaused {
		t.Fatal("paused goal must not be flipped to complete")
	}
}

// TestReportGoalOutcomeSupersededStampRefused verifies the race-#4 guard: a run
// stamped with an OLD goal incarnation must not
// signal the current goal, which a fresh Start gave a new lease.
func TestReportGoalOutcomeSupersededStampRefused(t *testing.T) {
	store := newMemStore()
	current := activeGoal("s1")
	current.LeaseID = "lease-current"
	store.put(current)

	// The run carries the lease it was launched under, since superseded.
	ctx := executionctx.WithScope(context.Background(), execution.TurnScope{
		SessionID:   "s1",
		GoalLeaseID: "lease-stale",
	})

	out, err := newReportTool(t, store).report(ctx, reportArgs{Outcome: "completed"})
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

func TestReportGoalOutcomeNoSession(t *testing.T) {
	out, err := newReportTool(t, newMemStore()).report(context.Background(), reportArgs{Outcome: "completed"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "No active session") {
		t.Fatalf("output = %q, want a no-session message", out)
	}
}

func TestGetGoalReturnsActionableViewWithoutOwnershipInternals(t *testing.T) {
	store := newMemStore()
	g := activeGoal("s1")
	g.Revision = 42
	g.Budget = goal.Budget{MaxTurns: 3}
	g.Used = goal.Usage{Turns: 1, Steps: 5}
	store.put(g)

	result, err := newGetTool(t, store).get(sessionCtx("s1"), getArgs{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Goal == nil || result.Goal.Objective != "obj" || result.Goal.Budget.MaxTurns != 3 || result.Goal.Usage.Steps != 5 {
		t.Fatalf("goal view = %+v", result.Goal)
	}
	if result.Goal.SessionID != "s1" || result.Goal.Status != "active" {
		t.Fatalf("goal identity/status = %+v", result.Goal)
	}
}

func TestGetGoalReturnsNullWhenAbsent(t *testing.T) {
	result, err := newGetTool(t, newMemStore()).get(sessionCtx("s1"), getArgs{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Goal != nil || !strings.Contains(result.Message, "No Goal") {
		t.Fatalf("result = %+v", result)
	}
}

type fakeStarter struct {
	sessionID string
	objective string
	selection modelref.Selection
	budget    goal.Budget
}

func (s *fakeStarter) Start(_ context.Context, sessionID, objective string, selection modelref.Selection, budget goal.Budget) (goal.Goal, error) {
	s.sessionID = sessionID
	s.objective = objective
	s.selection = selection
	s.budget = budget
	return goal.New(sessionID, objective, selection, budget, "lease", time.Unix(1, 0))
}

func TestCreateGoalUsesCurrentSessionAndExplicitBudget(t *testing.T) {
	starter := &fakeStarter{}
	result, err := (&createTool{goals: starter}).create(sessionCtx("s1"), createArgs{
		Objective: "  finish the migration  ",
		Budget:    &createBudget{MaxTurns: 4, MaxCostUSD: 2.5, MaxSteps: 20},
	})
	if err != nil {
		t.Fatal(err)
	}
	if starter.sessionID != "s1" || starter.objective != "finish the migration" {
		t.Fatalf("start identity = (%q, %q)", starter.sessionID, starter.objective)
	}
	if starter.selection.Configured() {
		t.Fatal("create_goal must use the runtime's surrounding model default")
	}
	if starter.budget != (goal.Budget{MaxTurns: 4, MaxCostUSD: 2.5, MaxSteps: 20}) {
		t.Fatalf("budget = %+v", starter.budget)
	}
	if result.Goal == nil || !strings.Contains(result.Message, "after the current Run") {
		t.Fatalf("result = %+v", result)
	}
}

func TestGoalToolContractsUseOnePreciseVocabulary(t *testing.T) {
	store := goals.NewState(newMemStore())
	create, err := NewCreate(&fakeStarter{})
	if err != nil {
		t.Fatal(err)
	}
	get, err := NewGet(store)
	if err != nil {
		t.Fatal(err)
	}
	report, err := NewReport(store)
	if err != nil {
		t.Fatal(err)
	}

	if got := create.Definition().Name; got != "create_goal" {
		t.Fatalf("create name = %q", got)
	}
	if got := get.Definition().Name; got != "get_goal" {
		t.Fatalf("get name = %q", got)
	}
	if got := report.Definition().Name; got != "report_goal_outcome" {
		t.Fatalf("report name = %q", got)
	}
	createSchema := string(create.Definition().InputSchema)
	for _, want := range []string{`"objective"`, `"budget"`, `"max_turns"`, `"max_cost_usd"`, `"max_steps"`} {
		if !strings.Contains(createSchema, want) {
			t.Errorf("create_goal schema %s missing %s", createSchema, want)
		}
	}
	reportSchema := string(report.Definition().InputSchema)
	for _, want := range []string{`"outcome"`, `"completed"`, `"blocked"`, `"reason"`} {
		if !strings.Contains(reportSchema, want) {
			t.Errorf("report_goal_outcome schema %s missing %s", reportSchema, want)
		}
	}
	for _, rejected := range []string{`"status"`, `"complete"`} {
		if strings.Contains(reportSchema, rejected) {
			t.Errorf("report_goal_outcome schema %s contains legacy term %s", reportSchema, rejected)
		}
	}
}
