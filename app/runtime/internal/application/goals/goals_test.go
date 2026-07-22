package goals_test

import (
	"context"
	"errors"
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
	mu       sync.Mutex
	goals    map[string]goal.Goal
	changed  chan struct{}
	failSave error
}

func newMemStore() *memStore {
	return &memStore{
		goals:   map[string]goal.Goal{},
		changed: make(chan struct{}),
	}
}

func (s *memStore) Get(_ context.Context, id string) (goal.Goal, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	g, ok := s.goals[id]
	return g, ok, nil
}
func (s *memStore) Save(_ context.Context, g goal.Goal, expected goal.Version) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failSave != nil {
		err := s.failSave
		s.failSave = nil
		return false, err
	}
	cur, ok := s.goals[g.SessionID]
	if expected == (goal.Version{}) {
		if ok {
			return false, nil
		}
	} else if !ok || cur.Version() != expected {
		return false, nil
	}
	s.goals[g.SessionID] = g
	s.notifyLocked()
	return true, nil
}

func (s *memStore) failNextSave(err error) {
	s.mu.Lock()
	s.failSave = err
	s.mu.Unlock()
}
func (s *memStore) Clear(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.goals, id)
	s.notifyLocked()
	return nil
}
func (s *memStore) ClearIf(_ context.Context, id string, expected goal.Version) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, ok := s.goals[id]
	if !ok || cur.Version() != expected {
		return false, nil
	}
	delete(s.goals, id)
	s.notifyLocked()
	return true, nil
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

func (s *memStore) observe(id string) (goal.Goal, bool, <-chan struct{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	g, ok := s.goals[id]
	return g, ok, s.changed
}

func (s *memStore) notifyLocked() {
	close(s.changed)
	s.changed = make(chan struct{})
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
	t       *testing.T
	store   *memStore
	script  []turn
	hold    chan struct{} // when non-nil, a run holds its terminal until this closes
	started chan struct{}
	mu      sync.Mutex
	calls   int
}

func (f *fakeRuns) Start(ctx context.Context, cmd runs.StartCommand) (runs.StartResult, error) {
	f.mu.Lock()
	i := f.calls
	f.calls++
	f.mu.Unlock()
	if f.started != nil {
		select {
		case f.started <- struct{}{}:
		default:
		}
	}

	events := make(chan runs.Event, 2)
	if i >= len(f.script) {
		f.t.Errorf("unexpected extra run (call %d, script has %d)", i, len(f.script))
		close(events)
		return runs.StartResult{SessionID: cmd.SessionID, Events: events}, nil
	}
	tn := f.script[i]
	if tn.setStatus != "" {
		// Simulate the model calling update_goal mid-turn: a CAS on the current
		// version while retaining the loop's lease.
		g, _, _ := f.store.Get(ctx, cmd.SessionID)
		g.Status = tn.setStatus
		g.Reason = tn.reason
		expected := g.Version()
		g.AdvanceRevision()
		_, _ = f.store.Save(ctx, g, expected)
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

// fakeSessions is the driver's session-existence check; sessions exist unless
// listed in deleted (nil map = all exist).
type fakeSessions struct{ deleted map[string]bool }

func (f *fakeSessions) Exists(_ context.Context, id string) (bool, error) {
	return !f.deleted[id], nil
}

func newDriver(t *testing.T, store *memStore, script ...turn) *goals.Driver {
	t.Helper()
	d := goals.NewDriver(store, &fakeRuns{t: t, store: store, script: script}, &fakeSessions{})
	t.Cleanup(func() { _ = d.Close() })
	return d
}

// waitGoal blocks on store changes until the session's goal satisfies cond.
func waitGoal(t *testing.T, store *memStore, sessionID string, cond func(goal.Goal, bool) bool) {
	t.Helper()
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	for {
		g, ok, changed := store.observe(sessionID)
		if cond(g, ok) {
			return
		}
		select {
		case <-changed:
		case <-timer.C:
			t.Fatalf("goal never reached the expected state: %+v (present=%v)", g, ok)
		}
	}
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
	g.RenewLease("lease-active")
	_, _ = store.Save(context.Background(), g, goal.Version{})
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
	started := make(chan struct{}, 1)
	fake := &fakeRuns{t: t, store: store, script: []turn{{outcome: execution.OutcomeCompleted}}, hold: hold, started: started}
	d := goals.NewDriver(store, fake, &fakeSessions{})
	t.Cleanup(func() { _ = d.Close() })

	if _, err := d.Start(context.Background(), "s1", "do it", "p", "m", goal.Budget{}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	select {
	case <-started: // the loop launched the run and is draining its terminal
	case <-time.After(2 * time.Second):
		t.Fatal("goal driver did not launch its first run")
	}

	if _, err := d.Stop(context.Background(), "s1"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	close(hold) // let the in-flight run finish

	if err := d.Close(); err != nil { // join proves no checkpoint remains in flight
		t.Fatalf("Close: %v", err)
	}
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

func TestDriverStopSaveFailureKeepsGoalDriving(t *testing.T) {
	store := newMemStore()
	hold := make(chan struct{})
	started := make(chan struct{}, 1)
	fake := &fakeRuns{t: t, store: store, script: []turn{
		{outcome: execution.OutcomeCompleted},
		{setStatus: goal.StatusComplete},
	}, hold: hold, started: started}
	d := goals.NewDriver(store, fake, &fakeSessions{})
	t.Cleanup(func() { _ = d.Close() })

	if _, err := d.Start(t.Context(), "s1", "do it", "p", "m", goal.Budget{}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("goal driver did not launch its first run")
	}

	stopErr := errors.New("goal store unavailable")
	store.failNextSave(stopErr)
	if _, err := d.Stop(t.Context(), "s1"); !errors.Is(err, stopErr) {
		t.Fatalf("Stop error = %v, want %v", err, stopErr)
	}
	close(hold)

	// Stop's durable pause did not commit, so the active goal must be driven by
	// the replacement loop rather than remaining active with no loop.
	waitGoal(t, store, "s1", func(_ goal.Goal, ok bool) bool { return !ok })
}

func TestSessionMutationFailureLeavesGoalLoopRunning(t *testing.T) {
	store := newMemStore()
	hold := make(chan struct{})
	started := make(chan struct{}, 1)
	fake := &fakeRuns{t: t, store: store, script: []turn{{setStatus: goal.StatusComplete}}, hold: hold, started: started}
	d := goals.NewDriver(store, fake, &fakeSessions{})
	t.Cleanup(func() { _ = d.Close() })

	if _, err := d.Start(t.Context(), "s1", "do it", "p", "m", goal.Budget{}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("goal driver did not launch its first run")
	}

	mutationErr := errors.New("write-set failed")
	err := d.WithSessionMutation(t.Context(), []string{"s1"}, func(context.Context) error { return mutationErr })
	if !errors.Is(err, mutationErr) {
		t.Fatalf("WithSessionMutation error = %v, want %v", err, mutationErr)
	}
	close(hold)
	waitGoal(t, store, "s1", func(_ goal.Goal, ok bool) bool { return !ok })
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
	if err := d.Close(); err != nil { // goal.turn ends before its span is inspected
		t.Fatalf("Close: %v", err)
	}

	var span *tracetest.SpanStub
	for _, s := range exporter.GetSpans() {
		if s.Name == "goal.turn" {
			stub := s
			span = &stub
			break
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

func TestReconcileDegradesActiveAndClearsComplete(t *testing.T) {
	store := newMemStore()
	now := time.Unix(0, 0)
	active, _ := goal.New("live", "obj", "", "", goal.Budget{}, now)
	done, _ := goal.New("done", "obj", "", "", goal.Budget{}, now)
	done.Complete(now)
	paused, _ := goal.New("held", "obj", "", "", goal.Budget{}, now)
	paused.Pause("earlier", now)
	for _, g := range []goal.Goal{active, done, paused} {
		g.RenewLease("lease-" + g.SessionID)
		_, _ = store.Save(context.Background(), g, goal.Version{})
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

func TestStartRefusesMissingSession(t *testing.T) {
	store := newMemStore()
	fake := &fakeRuns{t: t, store: store}
	d := goals.NewDriver(store, fake, &fakeSessions{deleted: map[string]bool{"ghost": true}})
	t.Cleanup(func() { _ = d.Close() })

	if _, err := d.Start(context.Background(), "ghost", "obj", "p", "m", goal.Budget{}); err != goals.ErrNoSession {
		t.Fatalf("Start(missing session) = %v, want ErrNoSession", err)
	}
	if _, ok, _ := store.Get(context.Background(), "ghost"); ok {
		t.Fatal("a goal was created for a nonexistent session")
	}
	fake.mu.Lock()
	calls := fake.calls
	fake.mu.Unlock()
	if calls != 0 {
		t.Fatalf("launched %d runs for a missing session, want 0", calls)
	}
}

func TestReconcileSweepsOrphanGoal(t *testing.T) {
	store := newMemStore()
	now := time.Unix(0, 0)
	orphan, _ := goal.New("gone", "obj", "", "", goal.Budget{}, now) // session deleted while down
	orphan.RenewLease("lease-gone")
	_, _ = store.Save(context.Background(), orphan, goal.Version{})
	kept, _ := goal.New("live", "obj", "", "", goal.Budget{}, now)
	kept.RenewLease("lease-live")
	kept.Pause("earlier", now)
	_, _ = store.Save(context.Background(), kept, goal.Version{})

	d := goals.NewDriver(store, &fakeRuns{t: t, store: store}, &fakeSessions{deleted: map[string]bool{"gone": true}})
	t.Cleanup(func() { _ = d.Close() })
	if err := d.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if _, ok, _ := store.Get(context.Background(), "gone"); ok {
		t.Fatal("orphan goal for a deleted session was not swept")
	}
	if _, ok, _ := store.Get(context.Background(), "live"); !ok {
		t.Fatal("a goal for a live session was wrongly swept")
	}
}

// TestStopThenStartRejectsStragglerWrite is the race-#4 keystone: a run whose
// goal was stopped and replaced by a fresh Start (a new lease) must not
// clobber the new goal when its straggler loop finally drains. The loop's launch
// lease no longer matches, so it stops without writing.
func TestStopThenStartRejectsStragglerWrite(t *testing.T) {
	store := newMemStore()
	hold := make(chan struct{})
	started := make(chan struct{}, 1)
	fake := &fakeRuns{t: t, store: store, script: []turn{{outcome: execution.OutcomeError}}, hold: hold, started: started}
	d := goals.NewDriver(store, fake, &fakeSessions{})
	t.Cleanup(func() { _ = d.Close() })

	if _, err := d.Start(context.Background(), "s1", "objective one", "p", "m", goal.Budget{}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("goal driver did not launch its first run")
	}
	// User stops the goal (paused, lease revoked, loop canceled) then starts a
	// fresh objective. Save the new goal with a new lease exactly as Start would
	// (no second loop launched, to keep the straggler the only writer under test).
	stopped, err := d.Stop(context.Background(), "s1")
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}
	newGoal, _ := goal.New("s1", "objective two", "p", "m", goal.Budget{}, time.Unix(0, 0))
	newGoal.Revision = stopped.Revision
	newGoal.RenewLease("lease-replacement")
	if applied, err := store.Save(context.Background(), newGoal, stopped.Version()); err != nil || !applied {
		t.Fatalf("seed replacement goal: applied=%v err=%v", applied, err)
	}

	close(hold) // release the straggler run; its loop drains and re-reads the goal
	if err := d.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	got, ok, _ := store.Get(context.Background(), "s1")
	if !ok || got.LeaseID != "lease-replacement" || got.Status != goal.StatusActive || got.Objective != "objective two" {
		t.Fatalf("straggler clobbered the replacement goal: %+v", got)
	}
}

// TestStopResumeRaceNeverWedgesActive runs Stop and Resume concurrently on a
// paused goal. The command mutex must serialize them so the goal never ends up
// active with no loop driving it — a wedge would leave it active forever, and
// waitGoal would time out. Run under -race to also catch memory races.
func TestStopResumeRaceNeverWedgesActive(t *testing.T) {
	for i := 0; i < 50; i++ {
		store := newMemStore()
		g, _ := goal.New("s1", "obj", "p", "m", goal.Budget{MaxTurns: 1}, time.Unix(0, 0))
		g.RenewLease("lease-seed")
		g.Pause("seed", time.Unix(0, 0))
		if _, err := store.Save(context.Background(), g, goal.Version{}); err != nil {
			t.Fatalf("seed: %v", err)
		}
		fake := &fakeRuns{t: t, store: store, script: []turn{{outcome: execution.OutcomeCompleted}}}
		d := goals.NewDriver(store, fake, &fakeSessions{})

		var wg sync.WaitGroup
		wg.Go(func() { _, _ = d.Stop(context.Background(), "s1") })
		wg.Go(func() { _, _ = d.Resume(context.Background(), "s1") })
		wg.Wait()

		// Settles non-active: paused (Stop won) or blocked (Resume's loop ran its one
		// budgeted turn). Active-with-no-loop would never leave active.
		waitGoal(t, store, "s1", func(g goal.Goal, ok bool) bool {
			return ok && g.Status != goal.StatusActive
		})
		if err := d.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
	}
}
