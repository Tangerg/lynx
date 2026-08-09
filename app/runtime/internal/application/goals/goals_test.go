package goals_test

import (
	"context"
	"errors"
	"iter"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/Tangerg/lynx/app/runtime/internal/application/goals"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/goal"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

// memStore is an in-memory goals.Store.
type memStore struct {
	mu                sync.Mutex
	goals             map[string]goal.Goal
	changed           chan struct{}
	failSave          func(goal.Goal) error
	runs              map[string]struct{}
	afterContextCheck func() // test hook: called before the context-aware lock
}

var (
	goalTraceOnce     sync.Once
	goalTraceExporter *notifyingSpanExporter
)

func installGoalTraceCapture(t *testing.T) *notifyingSpanExporter {
	t.Helper()
	goalTraceOnce.Do(func() {
		goalTraceExporter = &notifyingSpanExporter{
			InMemoryExporter: tracetest.NewInMemoryExporter(),
		}
		tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(goalTraceExporter))
		otel.SetTracerProvider(tp)
	})
	goalTraceExporter.Reset()
	goalTraceExporter.exported = make(chan struct{}, 1)
	t.Cleanup(goalTraceExporter.Reset)
	return goalTraceExporter
}

type faultingGoalStore struct {
	*memStore
	gets   atomic.Int32
	failAt int32
	err    error
	failed chan struct{}
}

func (s *faultingGoalStore) Get(ctx context.Context, sessionID string) (goal.Goal, bool, error) {
	if s.gets.Add(1) == s.failAt {
		close(s.failed)
		return goal.Goal{}, false, s.err
	}
	return s.memStore.Get(ctx, sessionID)
}

type pauseCompletionRaceStore struct {
	*memStore
	won atomic.Bool
}

func (s *pauseCompletionRaceStore) Save(
	ctx context.Context,
	candidate goal.Goal,
	expected goal.Version,
) (goal.Goal, bool, error) {
	if candidate.Reason.Code != goal.ReasonRunStartFailed || !s.won.CompareAndSwap(false, true) {
		return s.memStore.Save(ctx, candidate, expected)
	}
	if err := s.lock(ctx); err != nil {
		return goal.Goal{}, false, err
	}
	defer s.mu.Unlock()
	current, ok := s.goals[candidate.SessionID]
	if !ok || current.Version() != expected {
		return goal.Goal{}, false, nil
	}
	current.Complete(time.Now())
	current.Revision++
	s.goals[current.SessionID] = current
	s.notifyLocked()
	return goal.Goal{}, false, nil
}

func newMemStore() *memStore {
	return &memStore{
		goals:   map[string]goal.Goal{},
		runs:    map[string]struct{}{},
		changed: make(chan struct{}),
	}
}

// lock observes cancellation before and after waiting for the fake's mutex. The
// second check matters: a loop can be canceled while blocked on a concurrent
// store operation, and must not mutate state after it finally acquires the lock.
func (s *memStore) lock(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.afterContextCheck != nil {
		s.afterContextCheck()
	}
	s.mu.Lock()
	if err := ctx.Err(); err != nil {
		s.mu.Unlock()
		return err
	}
	return nil
}

// The store methods honor ctx cancellation to model the production sqlite store.
// This is load-bearing for the goal loop: a superseded straggler whose ctx was
// canceled by Stop bails at its next store operation instead of racing recovery.
func (s *memStore) Get(ctx context.Context, id string) (goal.Goal, bool, error) {
	if err := s.lock(ctx); err != nil {
		return goal.Goal{}, false, err
	}
	defer s.mu.Unlock()
	g, ok := s.goals[id]
	return g, ok, nil
}
func (s *memStore) Save(ctx context.Context, g goal.Goal, expected goal.Version) (goal.Goal, bool, error) {
	if err := s.lock(ctx); err != nil {
		return goal.Goal{}, false, err
	}
	defer s.mu.Unlock()
	if s.failSave != nil {
		if err := s.failSave(g); err != nil {
			s.failSave = nil
			return goal.Goal{}, false, err
		}
	}
	cur, ok := s.goals[g.SessionID]
	switch {
	case expected == (goal.Version{}):
		if ok {
			return goal.Goal{}, false, nil
		}
		g.Revision = 1
	case !ok || cur.Version() != expected:
		return goal.Goal{}, false, nil
	default:
		g.Revision = expected.Revision + 1
	}
	s.goals[g.SessionID] = g
	s.notifyLocked()
	return g, true, nil
}

func (s *memStore) failNextStopSave(err error) {
	s.mu.Lock()
	s.failSave = func(g goal.Goal) error {
		if g.Reason.Code == goal.ReasonStoppedByUser {
			return err
		}
		return nil
	}
	s.mu.Unlock()
}
func (s *memStore) Clear(ctx context.Context, id string) error {
	if err := s.lock(ctx); err != nil {
		return err
	}
	defer s.mu.Unlock()
	delete(s.goals, id)
	s.notifyLocked()
	return nil
}
func (s *memStore) ClearIf(ctx context.Context, id string, expected goal.Version) (bool, error) {
	if err := s.lock(ctx); err != nil {
		return false, err
	}
	defer s.mu.Unlock()
	cur, ok := s.goals[id]
	if !ok || cur.Version() != expected {
		return false, nil
	}
	delete(s.goals, id)
	s.notifyLocked()
	return true, nil
}
func (s *memStore) List(ctx context.Context) ([]goal.Goal, error) {
	if err := s.lock(ctx); err != nil {
		return nil, err
	}
	defer s.mu.Unlock()
	out := make([]goal.Goal, 0, len(s.goals))
	for _, g := range s.goals {
		out = append(out, g)
	}
	return out, nil
}

// RecordRun models the terminal Run transaction: its idempotency identity and
// the Goal aggregate mutation appear together before the loop sees the event.
func (s *memStore) RecordRun(ctx context.Context, record goal.RunRecord) error {
	if err := s.lock(ctx); err != nil {
		return err
	}
	defer s.mu.Unlock()
	if _, exists := s.runs[record.RunID]; exists {
		return nil
	}
	s.runs[record.RunID] = struct{}{}
	g, exists := s.goals[record.SessionID]
	if !exists || g.LeaseID != record.LeaseID {
		return nil
	}
	g.RecordRun(record)
	g.Revision++
	s.goals[g.SessionID] = g
	s.notifyLocked()
	return nil
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

// Run scripts one autonomous Run's outcome. setStatus simulates the model
// reporting a terminal goal outcome mid-Run (the driver re-reads the store
// after the run).
type scriptedRun struct {
	setStatus      goal.Status
	reason         string
	outcome        run.Outcome
	missingOutcome bool
	cost           float64
	steps          int
	park           bool // close the stream without a terminal
}

type fakeRuns struct {
	t             *testing.T
	store         *memStore
	script        []scriptedRun
	hold          chan struct{} // when non-nil, a run holds its terminal until this closes
	started       chan struct{}
	startErr      error
	startErrs     []error
	cancelStarted chan struct{}
	cancelRelease <-chan struct{}
	idleWaitStart chan struct{}
	idleRelease   <-chan struct{}
	mu            sync.Mutex
	calls         int
	startedRuns   int
	cancels       map[string]chan struct{}
	runDone       map[string]chan struct{}
	runGoals      map[string]goal.RunRecord
	canceled      int
}

func (f *fakeRuns) WaitSessionStartable(ctx context.Context, _ string) error {
	if f.idleWaitStart != nil {
		select {
		case f.idleWaitStart <- struct{}{}:
		default:
		}
	}
	if f.idleRelease == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-f.idleRelease:
		return nil
	}
}

// eventSequence preserves the fake's hold-then-yield-terminal timing and the
// production subscription's caller-cancellation behavior behind its iter.Seq
// contract.
func eventSequence(ctx context.Context, events <-chan runs.Event) iter.Seq[runs.Event] {
	return func(yield func(runs.Event) bool) {
		for {
			select {
			case <-ctx.Done():
				return
			case event, open := <-events:
				if !open || !yield(event) {
					return
				}
			}
		}
	}
}

func (f *fakeRuns) nextScriptedRun() (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	attempt := f.calls
	f.calls++
	var startErr error
	if attempt < len(f.startErrs) {
		startErr = f.startErrs[attempt]
	} else {
		startErr = f.startErr
	}
	runIndex := f.startedRuns
	if startErr == nil {
		f.startedRuns++
	}
	return runIndex, startErr
}

func (f *fakeRuns) notifyRunStarted() {
	if f.started != nil {
		select {
		case f.started <- struct{}{}:
		default:
		}
	}
}

func (f *fakeRuns) applyScriptedGoalStatus(
	ctx context.Context,
	cmd runs.StartCommand,
	script scriptedRun,
) {
	if script.setStatus != "" {
		// Simulate the model reporting a terminal goal outcome mid-Run: a CAS on
		// the current version while retaining the loop's lease.
		g, _, _ := f.store.Get(ctx, cmd.SessionID)
		g.Status = script.setStatus
		if script.setStatus == goal.StatusBlocked {
			g.Reason = goal.Reason{Code: goal.ReasonBlockedByModel, Detail: script.reason}
		}
		expected := g.Version()
		_, _, _ = f.store.Save(ctx, g, expected)
	}
}

func (f *fakeRuns) registerScriptedRun(
	cmd runs.StartCommand,
	runID string,
) (cancelRequested chan struct{}, runFinished chan struct{}) {
	cancelRequested = make(chan struct{})
	runFinished = make(chan struct{})
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.cancels == nil {
		f.cancels = map[string]chan struct{}{}
		f.runDone = map[string]chan struct{}{}
		f.runGoals = map[string]goal.RunRecord{}
	}
	f.cancels[runID] = cancelRequested
	f.runDone[runID] = runFinished
	f.runGoals[runID] = goal.RunRecord{
		SessionID: cmd.SessionID,
		LeaseID:   cmd.GoalLeaseID,
		RunID:     runID,
	}
	return cancelRequested, runFinished
}

func (f *fakeRuns) emitScriptedRun(
	ctx context.Context,
	cmd runs.StartCommand,
	runID string,
	runIndex int,
	script scriptedRun,
	cancelRequested <-chan struct{},
	runFinished chan<- struct{},
	events chan<- runs.Event,
) {
	go func() {
		defer close(events)
		defer close(runFinished)
		defer func() {
			f.mu.Lock()
			delete(f.cancels, runID)
			delete(f.runDone, runID)
			delete(f.runGoals, runID)
			f.mu.Unlock()
		}()
		if f.hold != nil && runIndex == 0 {
			select {
			case <-f.hold:
			case <-cancelRequested:
				return
			}
		}
		if !script.park {
			cost := script.cost
			var outcome *run.Outcome
			if !script.missingOutcome {
				outcome = &script.outcome
			}
			finishedRun := transcript.Run{
				SessionID: cmd.SessionID,
				ID:        runID,
				Outcome:   outcome,
				Metrics:   transcript.RunMetrics{Steps: script.steps, Usage: &transcript.Usage{ModelUsage: transcript.ModelUsage{CostUSD: &cost}}},
			}
			if outcome != nil && cmd.GoalLeaseID != "" {
				if err := f.store.RecordRun(context.WithoutCancel(ctx), goal.RunRecord{
					SessionID: cmd.SessionID, LeaseID: cmd.GoalLeaseID, RunID: runID,
					Outcome: *outcome, CostUSD: cost, Steps: script.steps, CompletedAt: time.Now(),
				}); err != nil {
					f.t.Errorf("record terminal Goal Run: %v", err)
				}
			}
			events <- runs.Event{Payload: runs.SegmentFinished{Run: finishedRun}}
		}
	}()
}

func (f *fakeRuns) Start(ctx context.Context, cmd runs.StartCommand) (runs.StartResult, error) {
	runIndex, startErr := f.nextScriptedRun()
	if startErr != nil {
		return runs.StartResult{}, startErr
	}
	f.notifyRunStarted()

	events := make(chan runs.Event, 2)
	if runIndex >= len(f.script) {
		f.t.Errorf("unexpected extra run (call %d, script has %d)", runIndex, len(f.script))
		close(events)
		return runs.StartResult{SessionID: cmd.SessionID, Events: eventSequence(ctx, events)}, nil
	}
	script := f.script[runIndex]
	f.applyScriptedGoalStatus(ctx, cmd, script)
	runID := "run_" + strconv.Itoa(runIndex)
	cancelRequested, runFinished := f.registerScriptedRun(cmd, runID)
	f.emitScriptedRun(ctx, cmd, runID, runIndex, script, cancelRequested, runFinished, events)
	return runs.StartResult{RunID: runID, SessionID: cmd.SessionID, Events: eventSequence(ctx, events)}, nil
}

func (f *fakeRuns) Cancel(_ context.Context, cmd runs.CancelCommand) (runs.CancelResult, error) {
	if f.cancelStarted != nil {
		select {
		case f.cancelStarted <- struct{}{}:
		default:
		}
	}
	if f.cancelRelease != nil {
		<-f.cancelRelease
	}
	f.mu.Lock()
	cancel := f.cancels[cmd.RunID]
	done := f.runDone[cmd.RunID]
	record := f.runGoals[cmd.RunID]
	if cancel != nil {
		delete(f.cancels, cmd.RunID)
		close(cancel)
		f.canceled++
	}
	f.mu.Unlock()
	if cancel == nil {
		return runs.CancelResult{}, runs.ErrRunNotFound
	}
	<-done
	record.Outcome = run.OutcomeCanceled
	record.CompletedAt = time.Now()
	if err := f.store.RecordRun(context.Background(), record); err != nil {
		return runs.CancelResult{}, err
	}
	outcome := run.OutcomeCanceled
	return runs.CancelResult{Run: transcript.Run{
		ID: cmd.RunID, State: run.Canceled, Outcome: &outcome,
	}}, nil
}

// fakeSessions is the driver's session-existence check; sessions exist unless
// listed in deleted (nil map = all exist).
type fakeSessions struct{ deleted map[string]bool }

func (f *fakeSessions) Exists(_ context.Context, id string) (bool, error) {
	return !f.deleted[id], nil
}

type terminalRaceRuns struct {
	store   *memStore
	started chan struct{}
	session string
	lease   string
}

func (*terminalRaceRuns) WaitSessionStartable(context.Context, string) error { return nil }

func (f *terminalRaceRuns) Start(ctx context.Context, cmd runs.StartCommand) (runs.StartResult, error) {
	f.session = cmd.SessionID
	f.lease = cmd.GoalLeaseID
	close(f.started)
	return runs.StartResult{
		RunID: "run_terminal_race", SessionID: cmd.SessionID,
		Events: func(func(runs.Event) bool) { <-ctx.Done() },
	}, nil
}

func (f *terminalRaceRuns) Cancel(context.Context, runs.CancelCommand) (runs.CancelResult, error) {
	err := f.store.RecordRun(context.Background(), goal.RunRecord{
		SessionID: f.session, LeaseID: f.lease, RunID: "run_terminal_race",
		Outcome: run.OutcomeCompleted, CompletedAt: time.Now(),
	})
	return runs.CancelResult{}, err
}

func newDriver(t *testing.T, store *memStore, script ...scriptedRun) *goals.Driver {
	t.Helper()
	d := goals.NewDriver(store, &fakeRuns{t: t, store: store, script: script}, &fakeSessions{}, goals.NewSessionMutations(), testPrompt)
	cleanupDriver(t, d)
	return d
}

func shutdownDriver(d *goals.Driver) error {
	d.BeginShutdown()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return d.AwaitShutdown(ctx)
}

func cleanupDriver(t testing.TB, d *goals.Driver) {
	t.Helper()
	t.Cleanup(func() {
		if err := shutdownDriver(d); err != nil {
			t.Errorf("shutdown goal driver: %v", err)
		}
	})
}

func testPrompt(input goals.RunInstructionInput) string { return input.Objective }

func testGoalModelSelection() modelref.Selection {
	selection, err := modelref.New("p", "m")
	if err != nil {
		panic(err)
	}
	return selection
}

// waitTestSessionGoal blocks until the test session's goal satisfies cond.
func waitTestSessionGoal(t *testing.T, store *memStore, cond func(goal.Goal, bool) bool) {
	t.Helper()
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	for {
		g, ok, changed := store.observe("s1")
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
	d := newDriver(t, store, scriptedRun{setStatus: goal.StatusComplete})
	if _, err := d.Start(context.Background(), "s1", "do it", testGoalModelSelection(), goal.Budget{}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitTestSessionGoal(t, store, func(_ goal.Goal, ok bool) bool { return !ok }) // completed → cleared
}

func TestDriverWaitsForCurrentSessionRunBeforeFirstGoalRun(t *testing.T) {
	store := newMemStore()
	release := make(chan struct{})
	waiting := make(chan struct{}, 1)
	runUseCases := &fakeRuns{
		t: t, store: store,
		script:        []scriptedRun{{setStatus: goal.StatusComplete}},
		idleWaitStart: waiting,
		idleRelease:   release,
	}
	d := goals.NewDriver(store, runUseCases, &fakeSessions{}, goals.NewSessionMutations(), testPrompt)
	cleanupDriver(t, d)
	if _, err := d.Start(t.Context(), "s1", "do it", modelref.Selection{}, goal.Budget{}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	select {
	case <-waiting:
	case <-time.After(time.Second):
		t.Fatal("Goal driver did not wait for session idle")
	}
	runUseCases.mu.Lock()
	callsBeforeRelease := runUseCases.calls
	runUseCases.mu.Unlock()
	if callsBeforeRelease != 0 {
		t.Fatalf("Run Start calls before session idle = %d", callsBeforeRelease)
	}
	close(release)
	waitTestSessionGoal(t, store, func(_ goal.Goal, ok bool) bool { return !ok })
}

func TestDriverBlocksOnRunBudget(t *testing.T) {
	store := newMemStore()
	// Two completed Runs; MaxRuns=2 blocks after the second.
	d := newDriver(t, store, scriptedRun{outcome: run.OutcomeCompleted}, scriptedRun{outcome: run.OutcomeCompleted})
	if _, err := d.Start(context.Background(), "s1", "do it", testGoalModelSelection(), goal.Budget{MaxRuns: 2}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitTestSessionGoal(t, store, func(g goal.Goal, ok bool) bool { return ok && g.Status == goal.StatusBlocked })
	g, _, _ := store.Get(context.Background(), "s1")
	if g.Used.Runs != 2 {
		t.Fatalf("used Runs = %d, want 2", g.Used.Runs)
	}
}

func TestDriverAccountsModelBlockedTerminalRun(t *testing.T) {
	store := newMemStore()
	d := newDriver(t, store, scriptedRun{
		setStatus: goal.StatusBlocked,
		reason:    "needs credentials",
		outcome:   run.OutcomeCompleted,
		cost:      0.75,
		steps:     2,
	})
	if _, err := d.Start(t.Context(), "s1", "do it", testGoalModelSelection(), goal.Budget{}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitTestSessionGoal(t, store, func(g goal.Goal, ok bool) bool {
		return ok && g.Status == goal.StatusBlocked && g.Used == (goal.Usage{Runs: 1, CostUSD: 0.75, Steps: 2})
	})

	g, _, _ := store.Get(t.Context(), "s1")
	if g.Reason != (goal.Reason{Code: goal.ReasonBlockedByModel, Detail: "needs credentials"}) {
		t.Fatalf("blocked goal reason = %+v", g.Reason)
	}
}

func TestDriverPausesOnRunError(t *testing.T) {
	store := newMemStore()
	d := newDriver(t, store, scriptedRun{outcome: run.OutcomeFailed})
	if _, err := d.Start(context.Background(), "s1", "do it", testGoalModelSelection(), goal.Budget{}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitTestSessionGoal(t, store, func(g goal.Goal, ok bool) bool { return ok && g.Status == goal.StatusPaused })
}

func TestDriverPausesOnMalformedTerminal(t *testing.T) {
	store := newMemStore()
	d := newDriver(t, store, scriptedRun{missingOutcome: true})
	if _, err := d.Start(t.Context(), "s1", "do it", testGoalModelSelection(), goal.Budget{}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitTestSessionGoal(t, store, func(g goal.Goal, ok bool) bool {
		return ok && g.Status == goal.StatusPaused
	})

	g, _, _ := store.Get(t.Context(), "s1")
	if g.Reason != (goal.Reason{Code: goal.ReasonTerminalOutcomeMissing}) {
		t.Fatalf("pause reason = %+v", g.Reason)
	}
	if g.Used.Runs != 0 {
		t.Fatalf("malformed terminal recorded %d Runs, want 0", g.Used.Runs)
	}
}

func TestPauseCASUsesAuthoritativeCompleteOutcome(t *testing.T) {
	base := newMemStore()
	store := &pauseCompletionRaceStore{memStore: base}
	fake := &fakeRuns{t: t, store: base, startErr: runs.ErrSessionBusy}
	d := goals.NewDriver(store, fake, &fakeSessions{}, goals.NewSessionMutations(), testPrompt)
	cleanupDriver(t, d)
	if _, err := d.Start(t.Context(), "s1", "do it", testGoalModelSelection(), goal.Budget{}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitTestSessionGoal(t, base, func(_ goal.Goal, ok bool) bool { return !ok })
	fake.mu.Lock()
	calls := fake.calls
	fake.mu.Unlock()
	if calls != 1 {
		t.Fatalf("run start calls = %d, want one", calls)
	}
}

func TestDriverRetriesSessionBusyAdmissionRace(t *testing.T) {
	store := newMemStore()
	fake := &fakeRuns{
		t:         t,
		store:     store,
		startErrs: []error{runs.ErrRunAdmissionBusy},
		script: []scriptedRun{{
			setStatus: goal.StatusComplete,
			outcome:   run.OutcomeCompleted,
		}},
	}
	d := goals.NewDriver(store, fake, &fakeSessions{}, goals.NewSessionMutations(), testPrompt)
	cleanupDriver(t, d)
	if _, err := d.Start(t.Context(), "s1", "do it", testGoalModelSelection(), goal.Budget{}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitTestSessionGoal(t, store, func(_ goal.Goal, ok bool) bool { return !ok })
	fake.mu.Lock()
	calls := fake.calls
	fake.mu.Unlock()
	if calls != 2 {
		t.Fatalf("run start calls = %d, want one lost race plus one accepted run", calls)
	}
}

func TestDriverAccumulatesCostBudget(t *testing.T) {
	store := newMemStore()
	// Each Run costs 0.5; MaxCostUSD 1.0 blocks after the second (used 1.0).
	d := newDriver(t, store,
		scriptedRun{outcome: run.OutcomeCompleted, cost: 0.5},
		scriptedRun{outcome: run.OutcomeCompleted, cost: 0.5})
	if _, err := d.Start(context.Background(), "s1", "do it", testGoalModelSelection(), goal.Budget{MaxCostUSD: 1.0}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitTestSessionGoal(t, store, func(g goal.Goal, ok bool) bool { return ok && g.Status == goal.StatusBlocked })
	g, _, _ := store.Get(context.Background(), "s1")
	if g.Used.CostUSD != 1.0 {
		t.Fatalf("used cost = %v, want 1.0", g.Used.CostUSD)
	}
}

func TestDriverRefusesConcurrentStart(t *testing.T) {
	store := newMemStore()
	g, _ := goal.New("s1", "obj", modelref.Selection{}, goal.Budget{}, "lease-active", time.Unix(0, 0))
	_, _, _ = store.Save(context.Background(), g, goal.Version{})
	// Refusing also restores the in-process drive for the active row it found, so
	// the fake has to have a Run to serve. Scripting none asserted the opposite
	// — that adoption never happens — and passed only when the assertion outran
	// the loop the refusal had just launched.
	started := make(chan struct{}, 1)
	fake := &fakeRuns{
		t: t, store: store, script: []scriptedRun{{outcome: run.OutcomeCompleted}},
		hold: make(chan struct{}), started: started,
	}
	d := goals.NewDriver(store, fake, &fakeSessions{}, goals.NewSessionMutations(), testPrompt)
	cleanupDriver(t, d)

	if _, err := d.Start(context.Background(), "s1", "obj2", modelref.Selection{}, goal.Budget{}); err != goals.ErrGoalActive {
		t.Fatalf("Start on active goal = %v, want ErrGoalActive", err)
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("Start on an active goal did not restore its drive")
	}
}

// TestDriverStopPausesRunningGoal stops a goal while a run is in flight and
// asserts it settles on paused without launching another run — the loop must
// honor the pause, never re-affirm active over it (the checkpoint-vs-Stop race).
func TestDriverStopPausesRunningGoal(t *testing.T) {
	store := newMemStore()
	hold := make(chan struct{})
	started := make(chan struct{}, 1)
	fake := &fakeRuns{t: t, store: store, script: []scriptedRun{{outcome: run.OutcomeCompleted}}, hold: hold, started: started}
	d := goals.NewDriver(store, fake, &fakeSessions{}, goals.NewSessionMutations(), testPrompt)
	cleanupDriver(t, d)

	if _, err := d.Start(context.Background(), "s1", "do it", testGoalModelSelection(), goal.Budget{}); err != nil {
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
	fake.mu.Lock()
	canceled := fake.canceled
	fake.mu.Unlock()
	if canceled != 1 {
		t.Fatalf("Stop canceled %d owned runs, want 1", canceled)
	}
	if err := shutdownDriver(d); err != nil { // join proves no checkpoint remains in flight
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

func TestDriverStopFoldsTerminalRaceBeforePausing(t *testing.T) {
	store := newMemStore()
	fake := &terminalRaceRuns{store: store, started: make(chan struct{})}
	d := goals.NewDriver(store, fake, &fakeSessions{}, goals.NewSessionMutations(), testPrompt)
	cleanupDriver(t, d)

	if _, err := d.Start(t.Context(), "s1", "do it", testGoalModelSelection(), goal.Budget{}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	select {
	case <-fake.started:
	case <-time.After(time.Second):
		t.Fatal("goal run did not start")
	}

	stopped, err := d.Stop(t.Context(), "s1")
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if stopped.Status != goal.StatusPaused || stopped.Reason.Code != goal.ReasonStoppedByUser {
		t.Fatalf("stopped goal = status %q reason %+v", stopped.Status, stopped.Reason)
	}
	if stopped.Used.Runs != 1 {
		t.Fatalf("terminal race accounting = %d Runs, want 1", stopped.Used.Runs)
	}
}

func TestDriverFreshStartReplacesStoppedGoal(t *testing.T) {
	store := newMemStore()
	hold := make(chan struct{})
	started := make(chan struct{}, 1)
	fake := &fakeRuns{t: t, store: store, hold: hold, started: started, script: []scriptedRun{
		{outcome: run.OutcomeCompleted},
		{setStatus: goal.StatusComplete, outcome: run.OutcomeCompleted},
	}}
	d := goals.NewDriver(store, fake, &fakeSessions{}, goals.NewSessionMutations(), testPrompt)
	cleanupDriver(t, d)

	if _, err := d.Start(t.Context(), "s1", "first", testGoalModelSelection(), goal.Budget{}); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	<-started
	stopped, err := d.Stop(t.Context(), "s1")
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}
	fresh, err := d.Start(t.Context(), "s1", "second", testGoalModelSelection(), goal.Budget{})
	if err != nil {
		t.Fatalf("fresh Start: %v", err)
	}
	if fresh.Objective != "second" || fresh.Revision != stopped.Revision+1 || fresh.LeaseID == stopped.LeaseID {
		t.Fatalf("fresh goal = %+v, stopped = %+v", fresh, stopped)
	}
	waitTestSessionGoal(t, store, func(_ goal.Goal, ok bool) bool { return !ok })
}

func TestDriverStoreFailureRemainsAddressableUntilStop(t *testing.T) {
	base := newMemStore()
	storeErr := errors.New("goal store read failed")
	store := &faultingGoalStore{
		memStore: base,
		failAt:   4,
		err:      storeErr,
		failed:   make(chan struct{}),
	}
	fake := &fakeRuns{t: t, store: base, script: []scriptedRun{
		{outcome: run.OutcomeCompleted},
		{setStatus: goal.StatusComplete, outcome: run.OutcomeCompleted},
	}}
	d := goals.NewDriver(store, fake, &fakeSessions{}, goals.NewSessionMutations(), testPrompt)
	cleanupDriver(t, d)

	if _, err := d.Start(t.Context(), "s1", "old objective", testGoalModelSelection(), goal.Budget{}); err != nil {
		t.Fatalf("start old goal: %v", err)
	}
	select {
	case <-store.failed:
	case <-time.After(2 * time.Second):
		t.Fatal("goal supervisor did not encounter the store failure")
	}

	if _, err := d.Start(t.Context(), "s1", "replacement", testGoalModelSelection(), goal.Budget{}); !errors.Is(err, storeErr) {
		t.Fatalf("Start while supervisor is faulted = %v, want store error", err)
	}
	stopped, err := d.Stop(t.Context(), "s1")
	if !errors.Is(err, storeErr) {
		t.Fatalf("Stop faulted supervisor = %v, want store error", err)
	}
	if stopped.Status != goal.StatusPaused {
		t.Fatalf("stopped status = %s, want paused", stopped.Status)
	}
	if _, err := d.Resume(t.Context(), "s1"); err != nil {
		t.Fatalf("Resume after observing supervisor fault: %v", err)
	}
	waitTestSessionGoal(t, base, func(_ goal.Goal, ok bool) bool { return !ok })
	fake.mu.Lock()
	calls := fake.calls
	fake.mu.Unlock()
	if calls != 2 {
		t.Fatalf("run starts = %d, want one before and one after recovery", calls)
	}
}

func TestDriverStopSaveFailureDoesNotPublishUserStop(t *testing.T) {
	store := newMemStore()
	hold := make(chan struct{})
	started := make(chan struct{}, 1)
	fake := &fakeRuns{t: t, store: store, script: []scriptedRun{
		{outcome: run.OutcomeCompleted},
	}, hold: hold, started: started}
	d := goals.NewDriver(store, fake, &fakeSessions{}, goals.NewSessionMutations(), testPrompt)
	cleanupDriver(t, d)

	if _, err := d.Start(t.Context(), "s1", "do it", testGoalModelSelection(), goal.Budget{}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("goal driver did not launch its first run")
	}

	stopErr := errors.New("goal store unavailable")
	store.failNextStopSave(stopErr)
	if _, err := d.Stop(t.Context(), "s1"); !errors.Is(err, stopErr) {
		t.Fatalf("Stop error = %v, want %v", err, stopErr)
	}
	// The Run cancellation may already have produced its own durable pause, but
	// the failed user-stop write must not be reported as committed.
	got, ok, err := store.Get(t.Context(), "s1")
	if err != nil || !ok {
		t.Fatalf("goal after failed Stop = present=%v err=%v", ok, err)
	}
	if got.Status == goal.StatusActive {
		t.Fatal("failed Stop left an active goal without a driver")
	}
	if got.Reason.Code == goal.ReasonStoppedByUser {
		t.Fatal("failed Stop published the uncommitted user-stop reason")
	}
}

func TestDriverContinuesAfterTerminalAccounting(t *testing.T) {
	store := newMemStore()
	hold := make(chan struct{})
	started := make(chan struct{}, 1)
	fake := &fakeRuns{t: t, store: store, script: []scriptedRun{
		{outcome: run.OutcomeCompleted},
		{setStatus: goal.StatusComplete},
	}, hold: hold, started: started}
	d := goals.NewDriver(store, fake, &fakeSessions{}, goals.NewSessionMutations(), testPrompt)
	cleanupDriver(t, d)

	if _, err := d.Start(t.Context(), "s1", "do it", testGoalModelSelection(), goal.Budget{}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("goal driver did not launch its first run")
	}

	close(hold)
	waitTestSessionGoal(t, store, func(_ goal.Goal, ok bool) bool { return !ok })

	fake.mu.Lock()
	calls := fake.calls
	fake.mu.Unlock()
	if calls != 2 {
		t.Fatalf("run starts = %d, want one supervisor to start 2 Runs", calls)
	}
}

func TestDriverRejectsCommandsAfterShutdown(t *testing.T) {
	store := newMemStore()
	d := newDriver(t, store)
	d.BeginShutdown()

	if _, err := d.Start(t.Context(), "s1", "do it", testGoalModelSelection(), goal.Budget{}); !errors.Is(err, goals.ErrClosed) {
		t.Fatalf("Start after shutdown = %v, want ErrClosed", err)
	}
	if _, ok, err := store.Get(t.Context(), "s1"); err != nil || ok {
		t.Fatalf("shutdown start persisted goal: present=%v err=%v", ok, err)
	}
}

func TestDriverShutdownJoinsRunCancellation(t *testing.T) {
	store := newMemStore()
	hold := make(chan struct{})
	started := make(chan struct{}, 1)
	cancelStarted := make(chan struct{}, 1)
	cancelRelease := make(chan struct{})
	fake := &fakeRuns{
		t:             t,
		store:         store,
		script:        []scriptedRun{{outcome: run.OutcomeCompleted}},
		hold:          hold,
		started:       started,
		cancelStarted: cancelStarted,
		cancelRelease: cancelRelease,
	}
	d := goals.NewDriver(store, fake, &fakeSessions{}, goals.NewSessionMutations(), testPrompt)

	if _, err := d.Start(t.Context(), "s1", "do it", testGoalModelSelection(), goal.Budget{}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	<-started
	d.BeginShutdown()
	<-cancelStarted

	waitCtx, cancelWait := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancelWait()
	if err := d.AwaitShutdown(waitCtx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("AwaitShutdown before Run cancellation completed = %v, want deadline", err)
	}

	close(cancelRelease)
	if err := d.AwaitShutdown(t.Context()); err != nil {
		t.Fatalf("AwaitShutdown after Run cancellation completed: %v", err)
	}
}

func TestMemStoreRejectsCancellationWhileWaitingForLock(t *testing.T) {
	store := newMemStore()
	checked := make(chan struct{})
	continueLock := make(chan struct{})
	store.afterContextCheck = func() {
		close(checked)
		<-continueLock
	}

	store.mu.Lock()
	ctx, cancel := context.WithCancel(t.Context())
	result := make(chan error, 1)
	go func() {
		_, _, err := store.Save(ctx, goal.Goal{SessionID: "s"}, goal.Version{})
		result <- err
	}()
	<-checked // the first cancellation check has passed; Save is about to wait
	cancel()
	close(continueLock)
	store.mu.Unlock()

	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("Save error = %v, want context.Canceled", err)
	}
}

// TestDriverEmitsRunSpan proves the observability is real (not just no-op): a
// goal.run span carries the session, Run ordinal, and the run's outcome/usage.
func TestDriverEmitsRunSpan(t *testing.T) {
	exporter := installGoalTraceCapture(t)

	store := newMemStore()
	// One completed Run; MaxRuns=1 blocks after it, so the span has run.outcome.
	d := newDriver(t, store, scriptedRun{outcome: run.OutcomeCompleted, cost: 0.3, steps: 2})
	if _, err := d.Start(context.Background(), "s1", "do it", testGoalModelSelection(), goal.Budget{MaxRuns: 1}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitTestSessionGoal(t, store, func(g goal.Goal, ok bool) bool { return ok && g.Status == goal.StatusBlocked })
	select {
	case <-exporter.exported:
	case <-time.After(2 * time.Second):
		t.Fatal("goal.run span did not finish")
	}
	if err := shutdownDriver(d); err != nil { // goal.run ends before its span is inspected
		t.Fatalf("Close: %v", err)
	}

	var span *tracetest.SpanStub
	for _, s := range exporter.GetSpans() {
		if s.Name == "goal.run" {
			stub := s
			span = &stub
			break
		}
	}
	if span == nil {
		t.Fatal("no goal.run span was emitted")
	}
	attrs := map[string]string{}
	for _, a := range span.Attributes {
		attrs[string(a.Key)] = a.Value.String()
	}
	if attrs["goal.session"] != "s1" || attrs["goal.run_ordinal"] != "1" || attrs["run.outcome"] != "completed" {
		t.Fatalf("goal.run span attributes = %v, want session s1 / Run ordinal 1 / outcome completed", attrs)
	}
}

type notifyingSpanExporter struct {
	*tracetest.InMemoryExporter
	exported chan struct{}
}

func (e *notifyingSpanExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	err := e.InMemoryExporter.ExportSpans(ctx, spans)
	select {
	case e.exported <- struct{}{}:
	default:
	}
	return err
}

func TestReconcileDegradesActiveAndClearsComplete(t *testing.T) {
	store := newMemStore()
	now := time.Unix(0, 0)
	active, _ := goal.New("live", "obj", modelref.Selection{}, goal.Budget{}, "lease-live", now)
	done, _ := goal.New("done", "obj", modelref.Selection{}, goal.Budget{}, "lease-done", now)
	done.Complete(now)
	paused, _ := goal.New("held", "obj", modelref.Selection{}, goal.Budget{}, "lease-held", now)
	paused.Pause(goal.ReasonAwaitingInput, "", now)
	for _, g := range []goal.Goal{active, done, paused} {
		_, _, _ = store.Save(context.Background(), g, goal.Version{})
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
	if g, _, _ := store.Get(context.Background(), "held"); g.Status != goal.StatusPaused || g.Reason != (goal.Reason{Code: goal.ReasonAwaitingInput}) {
		t.Fatalf("paused goal was disturbed: %+v", g)
	}
}

func TestStartRefusesMissingSession(t *testing.T) {
	store := newMemStore()
	fake := &fakeRuns{t: t, store: store}
	d := goals.NewDriver(store, fake, &fakeSessions{deleted: map[string]bool{"ghost": true}}, goals.NewSessionMutations(), testPrompt)
	cleanupDriver(t, d)

	if _, err := d.Start(context.Background(), "ghost", "obj", testGoalModelSelection(), goal.Budget{}); err != goals.ErrNoSession {
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
	orphan, _ := goal.New("gone", "obj", modelref.Selection{}, goal.Budget{}, "lease-gone", now) // session deleted while down
	_, _, _ = store.Save(context.Background(), orphan, goal.Version{})
	kept, _ := goal.New("live", "obj", modelref.Selection{}, goal.Budget{}, "lease-live", now)
	kept.Pause(goal.ReasonAwaitingInput, "", now)
	_, _, _ = store.Save(context.Background(), kept, goal.Version{})

	d := goals.NewDriver(store, &fakeRuns{t: t, store: store}, &fakeSessions{deleted: map[string]bool{"gone": true}}, goals.NewSessionMutations(), testPrompt)
	cleanupDriver(t, d)
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
	fake := &fakeRuns{t: t, store: store, script: []scriptedRun{{outcome: run.OutcomeFailed}}, hold: hold, started: started}
	d := goals.NewDriver(store, fake, &fakeSessions{}, goals.NewSessionMutations(), testPrompt)
	cleanupDriver(t, d)

	if _, err := d.Start(context.Background(), "s1", "objective one", testGoalModelSelection(), goal.Budget{}); err != nil {
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
	newGoal, _ := goal.New("s1", "objective two", testGoalModelSelection(), goal.Budget{}, "lease-replacement", time.Unix(0, 0))
	if _, applied, err := store.Save(context.Background(), newGoal, stopped.Version()); err != nil || !applied {
		t.Fatalf("seed replacement goal: applied=%v err=%v", applied, err)
	}

	close(hold) // release the straggler run; its loop drains and re-reads the goal
	if err := shutdownDriver(d); err != nil {
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
		g, _ := goal.New("s1", "obj", testGoalModelSelection(), goal.Budget{MaxRuns: 1}, "lease-seed", time.Unix(0, 0))
		g.Pause(goal.ReasonAwaitingInput, "", time.Unix(0, 0))
		if _, _, err := store.Save(context.Background(), g, goal.Version{}); err != nil {
			t.Fatalf("seed: %v", err)
		}
		fake := &fakeRuns{t: t, store: store, script: []scriptedRun{{outcome: run.OutcomeCompleted}}}
		d := goals.NewDriver(store, fake, &fakeSessions{}, goals.NewSessionMutations(), testPrompt)

		var wg sync.WaitGroup
		wg.Go(func() { _, _ = d.Stop(context.Background(), "s1") })
		wg.Go(func() { _, _ = d.Resume(context.Background(), "s1") })
		wg.Wait()

		// Settles non-active: paused (Stop won) or blocked (Resume's loop ran its one
		// budgeted Run). Active-with-no-loop would never leave active.
		waitTestSessionGoal(t, store, func(g goal.Goal, ok bool) bool {
			return ok && g.Status != goal.StatusActive
		})

		if err := shutdownDriver(d); err != nil {
			t.Fatalf("Close: %v", err)
		}
	}
}
