package builtin

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/executionctx"
	"github.com/Tangerg/lynx/app/runtime/internal/application/goals"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	goalstate "github.com/Tangerg/lynx/app/runtime/internal/domain/goal"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupt"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
)

// in-memory goals.Store for the tool tests.
type memStore struct{ goals map[string]goalstate.Goal }

func newMemStore() *memStore { return &memStore{goals: map[string]goalstate.Goal{}} }

func (m *memStore) Get(_ context.Context, id string) (goalstate.Goal, bool, error) {
	g, ok := m.goals[id]
	return g, ok, nil
}

// put seeds a goal directly (test setup), bypassing the CAS.
func (m *memStore) put(g goalstate.Goal) { m.goals[g.SessionID] = g }

func (m *memStore) Save(_ context.Context, g goalstate.Goal, expected goalstate.Version) (goalstate.Goal, bool, error) {
	cur, ok := m.goals[g.SessionID]
	if expected == (goalstate.Version{}) {
		if ok {
			return goalstate.Goal{}, false, nil
		}
		g.Revision = 1
		m.goals[g.SessionID] = g
		return g, true, nil
	}
	if !ok || cur.Version() != expected {
		return goalstate.Goal{}, false, nil
	}
	g.Revision = expected.Revision + 1
	m.goals[g.SessionID] = g
	return g, true, nil
}
func (m *memStore) Clear(_ context.Context, id string) error { delete(m.goals, id); return nil }
func (m *memStore) ClearIf(_ context.Context, id string, expected goalstate.Version) (bool, error) {
	cur, ok := m.goals[id]
	if !ok || cur.Version() != expected {
		return false, nil
	}
	delete(m.goals, id)
	return true, nil
}
func (m *memStore) List(context.Context) ([]goalstate.Goal, error) { return nil, nil }

// testSessionActiveGoal builds a stored active goal with an opaque current incarnation.
func testSessionActiveGoal() goalstate.Goal {
	g, _ := goalstate.New("s1", "obj", modelref.Selection{}, goalstate.Budget{}, run.Capabilities{}, "lease-active", time.Unix(0, 0))
	return g
}

func testSessionContext() context.Context {
	ctx := executionctx.WithScope(context.Background(), runs.ExecutionScope{SessionID: "s1"})
	return executionctx.WithRunCapabilities(ctx, testGoalRunCapabilities())
}

func testGoalRunCapabilities() run.Capabilities {
	return run.Capabilities{
		ChildRuns:      true,
		InterruptKinds: []interrupt.Kind{interrupt.Approval, interrupt.Question},
	}
}

func newGetter(t *testing.T, store goals.Store) *getter {
	t.Helper()
	return &getter{goals: goals.NewReader(store)}
}

func newReporter(t *testing.T, store goals.Store) *outcomeReporter {
	t.Helper()
	return &outcomeReporter{goals: goals.NewOutcomeReporter(store)}
}

func TestReportGoalOutcomeCompleted(t *testing.T) {
	store := newMemStore()
	store.put(testSessionActiveGoal())

	out, err := newReporter(t, store).report(testSessionContext(), reportArgs{Outcome: "completed"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "completed") {
		t.Fatalf("output = %q", out)
	}
	if got := store.goals["s1"]; got.Status != goalstate.StatusComplete {
		t.Fatalf("stored status = %q, want complete", got.Status)
	}
}

func TestReportGoalOutcomeBlockedRequiresReason(t *testing.T) {
	store := newMemStore()
	store.put(testSessionActiveGoal())
	tl := newReporter(t, store)

	out, _ := tl.report(testSessionContext(), reportArgs{Outcome: "blocked"})
	if !strings.Contains(out, "reason") {
		t.Fatalf("blocked without reason = %q, want a reason prompt", out)
	}
	if store.goals["s1"].Status != goalstate.StatusActive {
		t.Fatal("goal should stay active when blocked reason is missing")
	}

	reason := " needs a key "
	out, _ = tl.report(testSessionContext(), reportArgs{Outcome: "blocked", Reason: &reason})
	if !strings.Contains(out, "blocked") {
		t.Fatalf("output = %q", out)
	}
	if got := store.goals["s1"]; got.Status != goalstate.StatusBlocked || got.Reason != (goalstate.Reason{Code: goalstate.ReasonBlockedByModel, Detail: "needs a key"}) {
		t.Fatalf("stored = (%q, %+v)", got.Status, got.Reason)
	}
}

func TestReportGoalOutcomeCompletedRejectsReason(t *testing.T) {
	store := newMemStore()
	store.put(testSessionActiveGoal())
	reason := "partial caveat"
	out, err := newReporter(t, store).report(testSessionContext(), reportArgs{Outcome: "completed", Reason: &reason})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Omit reason") {
		t.Fatalf("completed with reason = %q, want precise field guidance", out)
	}
	if store.goals["s1"].Status != goalstate.StatusActive {
		t.Fatal("completed reason must be rejected before Goal state changes")
	}
}

func TestReportGoalOutcomeNoActiveGoal(t *testing.T) {
	store := newMemStore() // no goal for s1
	out, err := newReporter(t, store).report(testSessionContext(), reportArgs{Outcome: "completed"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "No active Goal") {
		t.Fatalf("output = %q, want a no-active-goal message", out)
	}
}

func TestReportGoalOutcomeDoesNotTouchPausedGoal(t *testing.T) {
	store := newMemStore()
	g := testSessionActiveGoal()
	g.Pause(goalstate.ReasonStoppedByUser, "", time.Unix(0, 0))
	store.put(g)

	out, _ := newReporter(t, store).report(testSessionContext(), reportArgs{Outcome: "completed"})
	if !strings.Contains(out, "No active Goal") {
		t.Fatalf("paused goal should be untouchable via report_goal_outcome, got %q", out)
	}
	if store.goals["s1"].Status != goalstate.StatusPaused {
		t.Fatal("paused goal must not be flipped to complete")
	}
}

// TestReportGoalOutcomeSupersededStampRefused verifies the race-#4 guard: a run
// stamped with an OLD goal incarnation must not
// signal the current goal, which a fresh Start gave a new incarnation.
func TestReportGoalOutcomeSupersededStampRefused(t *testing.T) {
	store := newMemStore()
	current := testSessionActiveGoal()
	current.IncarnationID = "lease-current"
	store.put(current)

	// The Run carries the incarnation it was launched under, since superseded.
	ctx := executionctx.WithScope(context.Background(), runs.ExecutionScope{
		SessionID:         "s1",
		GoalIncarnationID: "lease-stale",
	})

	out, err := newReporter(t, store).report(ctx, reportArgs{Outcome: "completed"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "superseded") {
		t.Fatalf("output = %q, want a superseded-goal refusal", out)
	}
	if store.goals["s1"].Status != goalstate.StatusActive {
		t.Fatal("a straggler run must not flip the current goal to complete")
	}
}

func TestReportGoalOutcomeNoSession(t *testing.T) {
	out, err := newReporter(t, newMemStore()).report(context.Background(), reportArgs{Outcome: "completed"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "No active session") {
		t.Fatalf("output = %q, want a no-session message", out)
	}
}

func TestGetGoalReturnsActionableViewWithoutOwnershipInternals(t *testing.T) {
	store := newMemStore()
	g := testSessionActiveGoal()
	g.Revision = 42
	g.Budget = goalstate.Budget{MaxRuns: 3}
	g.Used = goalstate.Usage{Runs: 1, Steps: 5}
	store.put(g)

	result, err := newGetter(t, store).get(testSessionContext(), getArgs{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Goal == nil || result.Goal.Objective != "obj" || result.Goal.Budget.MaxRuns != 3 || result.Goal.Usage.Steps != 5 {
		t.Fatalf("goal view = %+v", result.Goal)
	}
	if result.Goal.SessionID != "s1" || result.Goal.Status != "active" {
		t.Fatalf("goal identity/status = %+v", result.Goal)
	}
}

func TestGetGoalReturnsNullWhenAbsent(t *testing.T) {
	result, err := newGetter(t, newMemStore()).get(testSessionContext(), getArgs{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Goal != nil || !strings.Contains(result.Message, "No Goal") {
		t.Fatalf("result = %+v", result)
	}
}

type fakeStarter struct {
	sessionID    string
	objective    string
	selection    modelref.Selection
	budget       goalstate.Budget
	capabilities run.Capabilities
}

func (f *fakeStarter) Start(_ context.Context, sessionID, objective string, selection modelref.Selection, budget goalstate.Budget, capabilities run.Capabilities) (goalstate.Goal, error) {
	f.sessionID = sessionID
	f.objective = objective
	f.selection = selection
	f.budget = budget
	f.capabilities = capabilities.Clone()
	return goalstate.New(sessionID, objective, selection, budget, capabilities, "lease", time.Unix(1, 0))
}

func TestCreateGoalUsesCurrentSessionAndExplicitBudget(t *testing.T) {
	starter := &fakeStarter{}
	result, err := (&creator{goals: starter}).create(testSessionContext(), createArgs{
		Objective: "  finish the migration  ",
		Budget:    &createBudget{MaxRuns: 4, MaxCostUSD: 2.5, MaxSteps: 20},
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
	if starter.budget != (goalstate.Budget{MaxRuns: 4, MaxCostUSD: 2.5, MaxSteps: 20}) {
		t.Fatalf("budget = %+v", starter.budget)
	}
	if !starter.capabilities.Equal(testGoalRunCapabilities()) {
		t.Fatalf("capabilities = %+v", starter.capabilities)
	}
	if result.Goal == nil || !strings.Contains(result.Message, "after the current Run") {
		t.Fatalf("result = %+v", result)
	}
}

func TestGoalToolContractsUseOnePreciseVocabulary(t *testing.T) {
	mem := newMemStore()
	reader := goals.NewReader(mem)
	reporter := goals.NewOutcomeReporter(mem)
	create, err := NewCreate(&fakeStarter{})
	if err != nil {
		t.Fatal(err)
	}
	get, err := NewGet(reader)
	if err != nil {
		t.Fatal(err)
	}
	report, err := NewReport(reporter)
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
	for _, want := range []string{`"objective"`, `"budget"`, `"max_runs"`, `"max_cost_usd"`, `"max_steps"`} {
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
