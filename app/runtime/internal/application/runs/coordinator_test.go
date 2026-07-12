package runs

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
)

// --- fakes (drive the Coordinator without the wire or the agent SDK) ---

// stubEngineEvent is a placeholder executor event: the pump forwards it and the
// fakeProjector ignores its content. It satisfies the engine event's classifying
// contract ([execution.Event]) as a plain, non-terminal mid-run signal.
type stubEngineEvent struct{}

func (stubEngineEvent) Terminal() (execution.Outcome, bool) { return 0, false }
func (stubEngineEvent) Interrupt() bool                     { return false }

type fakeExecutor struct {
	events   []EngineEvent
	block    bool // when set, the turn blocks on ctx instead of emitting — a live run
	mu       sync.Mutex
	canceled int
	startErr error
}

func (f *fakeExecutor) TurnEvents(ctx context.Context, _ Handle) (iter.Seq[EngineEvent], error) {
	if f.startErr != nil {
		return nil, f.startErr
	}
	return func(yield func(EngineEvent) bool) {
		if f.block {
			<-ctx.Done()
			return
		}
		for _, e := range f.events {
			if ctx.Err() != nil {
				return
			}
			if !yield(e) {
				return
			}
		}
	}, nil
}

func (f *fakeExecutor) CancelTurn(context.Context, Handle) error {
	f.mu.Lock()
	f.canceled++
	f.mu.Unlock()
	return nil
}

func (f *fakeExecutor) cancels() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.canceled
}

type fakeProjector struct {
	open      []ProjectedEvent
	translate []ProjectedEvent
	terminal  []ProjectedEvent
	view      SegmentView
	aborted   string
}

func (p *fakeProjector) Open() []ProjectedEvent                 { return p.open }
func (p *fakeProjector) Translate(EngineEvent) []ProjectedEvent { return p.translate }
func (p *fakeProjector) SynthesizeTerminal() []ProjectedEvent   { return p.terminal }
func (p *fakeProjector) Abort(msg string)                       { p.aborted = msg }

// fakeEffects records the atomic event commits + nudges the pump drives: it
// commits an interrupt before publishing (checking the error) and every other
// event after (best-effort). commitErr fails a commit — the interrupt-abort path.
type fakeEffects struct {
	mu        sync.Mutex
	commits   []execution.EventCommit
	nudges    int
	finished  bool
	commitErr error
}

func (e *fakeEffects) CommitEvent(_ context.Context, c execution.EventCommit) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.commitErr != nil {
		return e.commitErr
	}
	e.commits = append(e.commits, c)
	return nil
}

func (e *fakeEffects) Nudge(string, []string) {
	e.mu.Lock()
	e.nudges++
	e.mu.Unlock()
}

func (e *fakeEffects) Finish(context.Context, Finish) {
	e.mu.Lock()
	e.finished = true
	e.mu.Unlock()
}

func (e *fakeEffects) commitCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.commits)
}

func (e *fakeEffects) didFinish() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.finished
}

// terminalized reports whether a terminalizing commit landed for sessionID — the
// terminal run-state transition now rides CommitEvent, not the admission store.
func (e *fakeEffects) terminalized(sessionID string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, c := range e.commits {
		if c.State == execution.StateTerminalize && c.SessionID == sessionID {
			return true
		}
	}
	return false
}

// fakeRunStore is the durable admission backstop under test. It enforces the
// real one-non-terminal-run-per-session invariant (Admit rejects a session that
// already holds a non-terminal run) so a continuation that wrongly re-Admits is
// caught, and records the admitted drafts + resumes. The park/terminal state
// transitions ride the atomic CommitEvent (see [fakeEffects]), not this store.
type fakeRunStore struct {
	mu       sync.Mutex
	admitted []execution.RunDraft
	active   map[string]bool // sessions holding a non-terminal run
	resumed  []string
	admitErr error
}

func newFakeRunStore() *fakeRunStore {
	return &fakeRunStore{active: map[string]bool{}}
}

func (f *fakeRunStore) Admit(_ context.Context, d execution.RunDraft) error {
	if f.admitErr != nil {
		return f.admitErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.active[d.SessionID] {
		return execution.ErrSessionBusy
	}
	f.active[d.SessionID] = true
	f.admitted = append(f.admitted, d)
	return nil
}

func (f *fakeRunStore) Resume(_ context.Context, sessionID string) error {
	f.mu.Lock()
	f.resumed = append(f.resumed, sessionID)
	f.mu.Unlock()
	return nil
}

func (f *fakeRunStore) ReconcileOrphans(context.Context) (int, error) { return 0, nil }

func (f *fakeRunStore) admits() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.admitted)
}

func (f *fakeRunStore) resumes() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.resumed)
}

// TestCoordinatorStartRejectsDurablyBusySession: when the durable backstop
// rejects admission (the session already holds a non-terminal run across a
// restart the in-memory claim can't see), Start surfaces ErrSessionBusy and
// tears the created turn down, registering no run.
func TestCoordinatorStartRejectsDurablyBusySession(t *testing.T) {
	exec := &fakeExecutor{}
	store := &fakeRunStore{admitErr: execution.ErrSessionBusy}
	c := NewCoordinator(exec, &fakeEffects{}, store)

	_, err := c.Start(context.Background(),
		StartSpec{RunID: "run_1", SessionID: "ses_1", TurnID: "run_1"},
		func(SegmentView) Projector { return &fakeProjector{} })
	if !errors.Is(err, execution.ErrSessionBusy) {
		t.Fatalf("Start err = %v, want ErrSessionBusy", err)
	}
	if exec.cancels() != 1 {
		t.Fatalf("CancelTurn calls = %d, want 1 (turn torn down on durable busy)", exec.cancels())
	}
	if c.Contains("run_1") {
		t.Fatal("a durably-rejected start must register no run")
	}
}

// TestCoordinatorStartAdmitsAndTerminalizes: a run records a durable admission on
// Start and, on its true (non-parked) terminal, commits the terminalizing state
// transition atomically through the event committer (not the admission store).
func TestCoordinatorStartAdmitsAndTerminalizes(t *testing.T) {
	exec := &fakeExecutor{}
	store := newFakeRunStore()
	eff := &fakeEffects{}
	proj := &fakeProjector{
		open: []ProjectedEvent{{Durable: true, Payload: "started"}},
		terminal: []ProjectedEvent{{
			Durable: true, Terminal: true, Payload: "finished",
			Commit: &execution.EventCommit{SessionID: "ses_1", State: execution.StateTerminalize},
		}},
	}
	c := NewCoordinator(exec, eff, store)

	events, err := c.Start(context.Background(),
		StartSpec{RunID: "run_1", SessionID: "ses_1", TurnID: "run_1"},
		func(v SegmentView) Projector { proj.view = v; return proj })
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	// Admission is synchronous in Start.
	if store.admits() != 1 {
		t.Fatalf("admits = %d, want 1", store.admits())
	}
	for range events {
	}
	if !eff.terminalized("ses_1") {
		t.Fatal("pump never committed the terminalizing state transition for ses_1")
	}
}

// TestCoordinatorResumeReusesDurableSlot: a continuation of a parked run (a spec
// carrying a ParentRunID) transitions the session's EXISTING durable row back to
// running rather than admitting a second row — so a resume does not trip the
// one-non-terminal-run-per-session guard the parent's still-open row would trip.
func TestCoordinatorResumeReusesDurableSlot(t *testing.T) {
	exec := &fakeExecutor{}
	store := newFakeRunStore()
	// The parent run parked: its durable row is still non-terminal (active).
	store.active["ses_1"] = true
	proj := &fakeProjector{
		open:     []ProjectedEvent{{Durable: true, Payload: "started"}},
		terminal: []ProjectedEvent{{Durable: true, Terminal: true, Payload: "finished"}},
	}
	c := NewCoordinator(exec, &fakeEffects{}, store)

	events, err := c.Start(context.Background(),
		StartSpec{RunID: "cont_1", ParentRunID: "run_1", SessionID: "ses_1", TurnID: "run_1"},
		func(v SegmentView) Projector { proj.view = v; return proj })
	if err != nil {
		t.Fatalf("resume Start must not re-admit a durably-busy session: %v", err)
	}
	if store.admits() != 0 {
		t.Fatalf("a continuation must not Admit (got %d) — it reuses the parent's row", store.admits())
	}
	if store.resumes() != 1 {
		t.Fatalf("resumes = %d, want 1", store.resumes())
	}
	for range events {
	}
}

// TestCoordinatorStartStreamsThenTerminates: Start opens a run, streams the
// projector's open + translated events, and — because the executor stream ends
// without a terminal — synthesizes one on teardown; each durable event is
// committed before publication (§7.2).
func TestCoordinatorStartStreamsThenTerminates(t *testing.T) {
	exec := &fakeExecutor{events: []EngineEvent{stubEngineEvent{}}}
	eff := &fakeEffects{}
	commit := &execution.EventCommit{SessionID: "ses_1"}
	proj := &fakeProjector{
		open:      []ProjectedEvent{{Durable: true, Payload: "started", Commit: commit}},
		translate: []ProjectedEvent{{Durable: true, Payload: "item", Commit: commit}},
		terminal:  []ProjectedEvent{{Durable: true, Terminal: true, Payload: "finished", Commit: commit}},
	}
	c := NewCoordinator(exec, eff, nil)

	events, err := c.Start(context.Background(),
		StartSpec{RunID: "run_1", SessionID: "ses_1", TurnID: "run_1"},
		func(v SegmentView) Projector { proj.view = v; return proj })
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	var payloads []any
	var cursors []string
	for e := range events {
		payloads = append(payloads, e.Payload)
		cursors = append(cursors, e.Seq)
	}

	want := []any{"started", "item", "finished"}
	if fmt.Sprint(payloads) != fmt.Sprint(want) {
		t.Fatalf("payloads = %v, want %v", payloads, want)
	}
	// Cursors are monotonic and minted per event.
	if cursors[0] >= cursors[1] || cursors[1] >= cursors[2] {
		t.Fatalf("cursors not monotonic: %v", cursors)
	}
	// Every durable event committed (before publication, §7.2 — proven
	// deterministically by TestCoordinatorItemPersistFailureAborts).
	if got := eff.commitCount(); got != 3 {
		t.Fatalf("CommitEvent calls = %d, want 3", got)
	}
	if proj.view == nil {
		t.Fatal("projector never received its segment view")
	}
}

// TestCoordinatorStartExecutorError: a turn that fails to start returns the
// error and tears the created turn down (cancels it), registering no run.
func TestCoordinatorStartExecutorError(t *testing.T) {
	exec := &fakeExecutor{startErr: fmt.Errorf("boom")}
	c := NewCoordinator(exec, &fakeEffects{}, nil)

	_, err := c.Start(context.Background(),
		StartSpec{RunID: "run_1", SessionID: "ses_1"},
		func(SegmentView) Projector { return &fakeProjector{} })
	if err == nil {
		t.Fatal("Start must surface the executor error")
	}
	if exec.cancels() != 1 {
		t.Fatalf("CancelTurn calls = %d, want 1", exec.cancels())
	}
	if c.Contains("run_1") {
		t.Fatal("a failed start must register no run")
	}
}

// TestCoordinatorAdmission: the Coordinator is the session single-writer — a
// claim blocks a second claim and an active run, until released/closed.
func TestCoordinatorAdmission(t *testing.T) {
	c := NewCoordinator(&fakeExecutor{}, &fakeEffects{}, nil)
	if !c.ClaimSession("ses_1") {
		t.Fatal("first claim must succeed")
	}
	if c.ClaimSession("ses_1") {
		t.Fatal("second claim on the same session must fail")
	}
	if !c.ActiveSession("ses_1") {
		t.Fatal("a claimed session reads as active")
	}
	c.ReleaseSession("ses_1")
	if c.ActiveSession("ses_1") {
		t.Fatal("a released session is no longer active")
	}
}

// TestCoordinatorStartAfterClose: once closed, Start admits no new run and tears
// down the turn that was already created.
func TestCoordinatorStartAfterClose(t *testing.T) {
	exec := &fakeExecutor{block: true}
	c := NewCoordinator(exec, &fakeEffects{}, nil)
	c.Close()

	_, err := c.Start(context.Background(),
		StartSpec{RunID: "run_1", SessionID: "ses_1"},
		func(SegmentView) Projector { return &fakeProjector{} })
	if !errors.Is(err, ErrClosed) {
		t.Fatalf("Start after Close = %v, want ErrClosed", err)
	}
	if exec.cancels() != 1 {
		t.Fatalf("CancelTurn calls = %d, want 1 (created turn torn down)", exec.cancels())
	}
}

// TestCoordinatorCloseCancelsAndJoins: Close cancels an in-flight blocking run
// and joins its pump; the run's stream drains to close.
func TestCoordinatorCloseCancelsAndJoins(t *testing.T) {
	exec := &fakeExecutor{block: true}
	proj := &fakeProjector{
		open:     []ProjectedEvent{{Durable: true, Payload: "started"}},
		terminal: []ProjectedEvent{{Durable: true, Terminal: true, Payload: "canceled"}},
	}
	c := NewCoordinator(exec, &fakeEffects{}, nil)
	events, err := c.Start(context.Background(),
		StartSpec{RunID: "run_1", SessionID: "ses_1"},
		func(SegmentView) Projector { return proj })
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if e := <-events; e.Payload != "started" { // run is live
		t.Fatalf("first event = %v, want started", e.Payload)
	}

	done := make(chan struct{})
	go func() { c.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not cancel + join the blocking run")
	}
	for range events { // teardown synthesized the terminal, then closed the stream
	}
}

// TestCoordinatorInterruptPersistFailureAborts: when the interrupt's durable
// commit fails, the pump aborts — it never publishes the interrupt, cancels the
// turn, and terminalizes as error.
func TestCoordinatorInterruptPersistFailureAborts(t *testing.T) {
	exec := &fakeExecutor{events: []EngineEvent{stubEngineEvent{}}}
	eff := &fakeEffects{commitErr: fmt.Errorf("store down")}
	proj := &fakeProjector{
		open: []ProjectedEvent{{Durable: true, Payload: "started"}},
		translate: []ProjectedEvent{{
			Durable: true, Terminal: true, Interrupt: true, Payload: "interrupt",
			Commit: &execution.EventCommit{SessionID: "ses_1", State: execution.StateSuspend},
		}},
		terminal: []ProjectedEvent{{Durable: true, Terminal: true, Payload: "error"}},
	}
	c := NewCoordinator(exec, eff, nil)
	events, err := c.Start(context.Background(),
		StartSpec{RunID: "run_1", SessionID: "ses_1", TurnID: "run_1"},
		func(SegmentView) Projector { return proj })
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	var payloads []any
	for e := range events {
		payloads = append(payloads, e.Payload)
	}
	if len(payloads) == 0 || payloads[len(payloads)-1] != "error" {
		t.Fatalf("payloads = %v, want ending in an error terminal (interrupt never published)", payloads)
	}
	if exec.cancels() != 1 {
		t.Fatalf("CancelTurn calls = %d, want 1 (aborted turn canceled)", exec.cancels())
	}
	if proj.aborted == "" {
		t.Fatal("projector Abort was not called with the commit error")
	}
}

// TestCoordinatorItemPersistFailureAborts proves commit-before-publish for a
// durable non-terminal event (§7.2): when the item's atomic commit fails, the
// pump aborts BEFORE publishing it — the item never reaches the stream, the turn
// is canceled, and the run terminalizes as error. Under a live-first order the
// item would already be on the stream (backed by no durable record); this test
// would fail then, so it pins the ordering.
func TestCoordinatorItemPersistFailureAborts(t *testing.T) {
	exec := &fakeExecutor{events: []EngineEvent{stubEngineEvent{}}}
	eff := &fakeEffects{commitErr: fmt.Errorf("store down")}
	proj := &fakeProjector{
		open:      []ProjectedEvent{{Durable: true, Payload: "started"}},
		translate: []ProjectedEvent{{Durable: true, Payload: "item", Commit: &execution.EventCommit{SessionID: "ses_1"}}},
		terminal:  []ProjectedEvent{{Durable: true, Terminal: true, Payload: "error"}},
	}
	c := NewCoordinator(exec, eff, nil)
	events, err := c.Start(context.Background(),
		StartSpec{RunID: "run_1", SessionID: "ses_1", TurnID: "run_1"},
		func(SegmentView) Projector { return proj })
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	var payloads []any
	for e := range events {
		payloads = append(payloads, e.Payload)
	}
	for _, p := range payloads {
		if p == "item" {
			t.Fatalf("payloads = %v, want the item NEVER published (commit-before-publish)", payloads)
		}
	}
	if len(payloads) == 0 || payloads[len(payloads)-1] != "error" {
		t.Fatalf("payloads = %v, want ending in an error terminal", payloads)
	}
	if exec.cancels() != 1 {
		t.Fatalf("CancelTurn calls = %d, want 1 (aborted turn canceled)", exec.cancels())
	}
	if proj.aborted == "" {
		t.Fatal("projector Abort was not called with the commit error")
	}
}

// TestCoordinatorOpeningCommitFailureTearsDown proves the teardown defer is armed
// BEFORE the opening run.started publish (§8.2 correctness): when that first
// durable commit fails, the run is still torn down — the turn is canceled, the
// registry entry removed (the session freed), and the stream drained to close —
// rather than stranding a client that acked start but can never reach a terminal.
func TestCoordinatorOpeningCommitFailureTearsDown(t *testing.T) {
	exec := &fakeExecutor{}
	eff := &fakeEffects{commitErr: fmt.Errorf("store down")}
	proj := &fakeProjector{
		open: []ProjectedEvent{{Durable: true, Payload: "started", Commit: &execution.EventCommit{SessionID: "ses_1"}}},
	}
	c := NewCoordinator(exec, eff, nil)

	events, err := c.Start(context.Background(),
		StartSpec{RunID: "run_1", SessionID: "ses_1", TurnID: "run_1"},
		func(v SegmentView) Projector { proj.view = v; return proj })
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	for range events { // the stream must drain to close, not hang
	}
	c.Close() // join the pump goroutine so the post-hub-close teardown is race-free

	if exec.cancels() != 1 {
		t.Fatalf("CancelTurn calls = %d, want 1 (opening-commit failure must cancel the turn)", exec.cancels())
	}
	if c.Contains("run_1") {
		t.Fatal("opening-commit failure must remove the registry entry, not strand a busy session")
	}
	if !eff.didFinish() {
		t.Fatal("teardown Finish never ran after the opening-commit failure")
	}
	if proj.aborted == "" {
		t.Fatal("projector Abort was not called with the opening-commit error")
	}
}

// TestCoordinatorBeginCancelCleanupSurvivesRequest: the run outlives the request
// that started it, so BeginCancel's cleanup context (rooted on the run's owner)
// stays alive even after the request context is canceled.
func TestCoordinatorBeginCancelCleanupSurvivesRequest(t *testing.T) {
	exec := &fakeExecutor{block: true}
	c := NewCoordinator(exec, &fakeEffects{}, nil)
	reqCtx, cancelReq := context.WithCancel(context.Background())
	events, err := c.Start(reqCtx,
		StartSpec{RunID: "run_1", SessionID: "ses_1"},
		func(SegmentView) Projector {
			return &fakeProjector{open: []ProjectedEvent{{Durable: true, Payload: "started"}}}
		})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	<-events    // run is live
	cancelReq() // the request ends; the run must survive it

	binding, cleanupCtx, cancel, ok := c.BeginCancel(context.Background(), "run_1", "stop")
	if !ok {
		t.Fatal("BeginCancel must find the live run")
	}
	defer cancel()
	if cleanupCtx.Err() != nil {
		t.Fatalf("cleanup context canceled despite the run outliving the request: %v", cleanupCtx.Err())
	}
	if binding.SessionID != "ses_1" {
		t.Fatalf("binding = %+v, want SessionID ses_1", binding)
	}
	c.Close()
	for range events {
	}
}
