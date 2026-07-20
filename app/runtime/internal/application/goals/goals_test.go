package goals_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/Tangerg/lynx/app/runtime/internal/application/goals"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/goal"
)

// memStore is an in-memory goal.Store.
type memStore struct {
	mu    sync.Mutex
	goals map[string]goal.Goal
}

func newMemStore() *memStore { return &memStore{goals: map[string]goal.Goal{}} }

func (s *memStore) Get(_ context.Context, id string) (goal.Goal, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	g, ok := s.goals[id]
	return g, ok, nil
}
func (s *memStore) Save(_ context.Context, g goal.Goal) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.goals[g.SessionID] = g
	return nil
}
func (s *memStore) Clear(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.goals, id)
	return nil
}
func (s *memStore) List(context.Context) ([]goal.Goal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]goal.Goal, 0, len(s.goals))
	for _, g := range s.goals {
		out = append(out, g)
	}
	return out, nil
}

// turn scripts one autonomous turn's outcome. setStatus simulates the model
// calling update_goal mid-turn (the driver re-reads the store after the run).
type turn struct {
	setStatus goal.Status
	reason    string
	outcome   execution.Outcome
	cost      float64
	steps     int
	park      bool // close the stream without a terminal
}

type fakeRuns struct {
	t      *testing.T
	store  *memStore
	script []turn
	hold   chan struct{} // when non-nil, a run holds its terminal until this closes
	mu     sync.Mutex
	calls  int
}

func (f *fakeRuns) Start(ctx context.Context, cmd runs.StartCommand) (runs.StartResult, error) {
	f.mu.Lock()
	i := f.calls
	f.calls++
	f.mu.Unlock()

	events := make(chan runs.Event, 2)
	if i >= len(f.script) {
		f.t.Errorf("unexpected extra run (call %d, script has %d)", i, len(f.script))
		close(events)
		return runs.StartResult{SessionID: cmd.SessionID, Events: events}, nil
	}
	tn := f.script[i]
	if tn.setStatus != "" {
		g, _, _ := f.store.Get(ctx, cmd.SessionID)
		g.Status = tn.setStatus
		g.Reason = tn.reason
		_ = f.store.Save(ctx, g)
	}
	go func() {
		if f.hold != nil {
			<-f.hold
		}
		if !tn.park {
			cost := tn.cost
			run := transcript.Run{
				SessionID: cmd.SessionID,
				ID:        "run",
				Outcome:   &tn.outcome,
				Result:    &transcript.RunResult{Steps: tn.steps, Usage: &transcript.Usage{ModelUsage: transcript.ModelUsage{CostUSD: &cost}}},
			}
			events <- runs.Event{Payload: runs.SegmentFinished{Run: run}}
		}
		close(events)
	}()
	return runs.StartResult{RunID: "run", SessionID: cmd.SessionID, Events: events}, nil
}

func newDriver(t *testing.T, store *memStore, script ...turn) *goals.Driver {
	t.Helper()
	d := goals.NewDriver(store, &fakeRuns{t: t, store: store, script: script})
	t.Cleanup(func() { _ = d.Close() })
	return d
}

// waitGoal polls until the session's goal satisfies cond, or fails on timeout.
func waitGoal(t *testing.T, store *memStore, sessionID string, cond func(goal.Goal, bool) bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		g, ok, _ := store.Get(context.Background(), sessionID)
		if cond(g, ok) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	g, ok, _ := store.Get(context.Background(), sessionID)
	t.Fatalf("goal never reached the expected state: %+v (present=%v)", g, ok)
}

func TestDriverCompletesAndClears(t *testing.T) {
	store := newMemStore()
	d := newDriver(t, store, turn{setStatus: goal.StatusComplete})
	if _, err := d.Start(context.Background(), "s1", "do it", "p", "m", goal.Budget{}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitGoal(t, store, "s1", func(_ goal.Goal, ok bool) bool { return !ok }) // completed → cleared
}

func TestDriverBlocksOnTurnBudget(t *testing.T) {
	store := newMemStore()
	// Two completed turns; MaxTurns=2 blocks after the second.
	d := newDriver(t, store, turn{outcome: execution.OutcomeCompleted}, turn{outcome: execution.OutcomeCompleted})
	if _, err := d.Start(context.Background(), "s1", "do it", "p", "m", goal.Budget{MaxTurns: 2}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitGoal(t, store, "s1", func(g goal.Goal, ok bool) bool { return ok && g.Status == goal.StatusBlocked })
	g, _, _ := store.Get(context.Background(), "s1")
	if g.Used.Turns != 2 {
		t.Fatalf("used turns = %d, want 2", g.Used.Turns)
	}
}

func TestDriverPausesOnRunError(t *testing.T) {
	store := newMemStore()
	d := newDriver(t, store, turn{outcome: execution.OutcomeError})
	if _, err := d.Start(context.Background(), "s1", "do it", "p", "m", goal.Budget{}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitGoal(t, store, "s1", func(g goal.Goal, ok bool) bool { return ok && g.Status == goal.StatusPaused })
}

func TestDriverAccumulatesCostBudget(t *testing.T) {
	store := newMemStore()
	// Each turn costs 0.5; MaxCostUSD 1.0 blocks after the second (used 1.0).
	d := newDriver(t, store,
		turn{outcome: execution.OutcomeCompleted, cost: 0.5},
		turn{outcome: execution.OutcomeCompleted, cost: 0.5})
	if _, err := d.Start(context.Background(), "s1", "do it", "p", "m", goal.Budget{MaxCostUSD: 1.0}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitGoal(t, store, "s1", func(g goal.Goal, ok bool) bool { return ok && g.Status == goal.StatusBlocked })
	g, _, _ := store.Get(context.Background(), "s1")
	if g.Used.CostUSD != 1.0 {
		t.Fatalf("used cost = %v, want 1.0", g.Used.CostUSD)
	}
}

func TestDriverRefusesConcurrentStart(t *testing.T) {
	store := newMemStore()
	g, _ := goal.New("s1", "obj", "", "", goal.Budget{}, time.Unix(0, 0))
	_ = store.Save(context.Background(), g)
	d := newDriver(t, store) // no script needed; Start is rejected before any run
	if _, err := d.Start(context.Background(), "s1", "obj2", "", "", goal.Budget{}); err != goals.ErrGoalActive {
		t.Fatalf("Start on active goal = %v, want ErrGoalActive", err)
	}
}

// TestDriverStopPausesRunningGoal stops a goal while a run is in flight and
// asserts it settles on paused without launching another run — the loop must
// honor the pause, never re-affirm active over it (the checkpoint-vs-Stop race).
func TestDriverStopPausesRunningGoal(t *testing.T) {
	store := newMemStore()
	hold := make(chan struct{})
	fake := &fakeRuns{t: t, store: store, script: []turn{{outcome: execution.OutcomeCompleted}}, hold: hold}
	d := goals.NewDriver(store, fake)
	t.Cleanup(func() { _ = d.Close() })

	if _, err := d.Start(context.Background(), "s1", "do it", "p", "m", goal.Budget{}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitCalls(t, fake, 1) // the loop launched the run and is draining its terminal

	if _, err := d.Stop(context.Background(), "s1"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	close(hold) // let the in-flight run finish

	waitGoal(t, store, "s1", func(g goal.Goal, ok bool) bool { return ok && g.Status == goal.StatusPaused })
	time.Sleep(50 * time.Millisecond) // give any stray checkpoint a chance to clobber
	if g, _, _ := store.Get(context.Background(), "s1"); g.Status != goal.StatusPaused {
		t.Fatalf("goal not stably paused after stop: %q", g.Status)
	}
	fake.mu.Lock()
	calls := fake.calls
	fake.mu.Unlock()
	if calls != 1 {
		t.Fatalf("stopped goal launched %d runs, want 1", calls)
	}
}

// TestDriverEmitsTurnSpan proves the observability is real (not just no-op): a
// goal.turn span carries the session, turn ordinal, and the run's outcome/usage.
func TestDriverEmitsTurnSpan(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() {
		otel.SetTracerProvider(prev)
		_ = tp.Shutdown(context.Background())
	})

	store := newMemStore()
	// One completed turn; MaxTurns=1 blocks after it, so the span has run.outcome.
	d := newDriver(t, store, turn{outcome: execution.OutcomeCompleted, cost: 0.3, steps: 2})
	if _, err := d.Start(context.Background(), "s1", "do it", "p", "m", goal.Budget{MaxTurns: 1}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitGoal(t, store, "s1", func(g goal.Goal, ok bool) bool { return ok && g.Status == goal.StatusBlocked })

	var span *tracetest.SpanStub
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && span == nil {
		for _, s := range exporter.GetSpans() {
			if s.Name == "goal.turn" {
				stub := s
				span = &stub
				break
			}
		}
		if span == nil {
			time.Sleep(5 * time.Millisecond)
		}
	}
	if span == nil {
		t.Fatal("no goal.turn span was emitted")
	}
	attrs := map[string]string{}
	for _, a := range span.Attributes {
		attrs[string(a.Key)] = a.Value.String()
	}
	if attrs["goal.session"] != "s1" || attrs["goal.turn"] != "1" || attrs["run.outcome"] != "completed" {
		t.Fatalf("goal.turn span attributes = %v, want session s1 / turn 1 / outcome completed", attrs)
	}
}

func waitCalls(t *testing.T, f *fakeRuns, n int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		f.mu.Lock()
		c := f.calls
		f.mu.Unlock()
		if c >= n {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("run count never reached %d", n)
}

func TestReconcileDegradesActiveAndClearsComplete(t *testing.T) {
	store := newMemStore()
	now := time.Unix(0, 0)
	active, _ := goal.New("live", "obj", "", "", goal.Budget{}, now)
	done, _ := goal.New("done", "obj", "", "", goal.Budget{}, now)
	done.Complete(now)
	paused, _ := goal.New("held", "obj", "", "", goal.Budget{}, now)
	paused.Pause("earlier", now)
	for _, g := range []goal.Goal{active, done, paused} {
		_ = store.Save(context.Background(), g)
	}

	d := newDriver(t, store)
	if err := d.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if g, _, _ := store.Get(context.Background(), "live"); g.Status != goal.StatusPaused {
		t.Fatalf("active goal not degraded: %q", g.Status)
	}
	if _, ok, _ := store.Get(context.Background(), "done"); ok {
		t.Fatal("complete goal not cleared")
	}
	if g, _, _ := store.Get(context.Background(), "held"); g.Status != goal.StatusPaused || g.Reason != "earlier" {
		t.Fatalf("paused goal was disturbed: %+v", g)
	}
}
