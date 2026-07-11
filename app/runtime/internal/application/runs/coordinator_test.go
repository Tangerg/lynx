package runs

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
)

// --- fakes (drive the Coordinator without the wire or the agent SDK) ---

// stubEngineEvent is an opaque placeholder executor event: the pump forwards it
// verbatim and the fakeProjector ignores its content.
type stubEngineEvent struct{}

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

type fakeEffects struct {
	mu        sync.Mutex
	before    int
	after     int
	finished  bool
	beforeErr error
}

func (e *fakeEffects) BeforeLive(context.Context, any) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.before++
	return e.beforeErr
}

func (e *fakeEffects) AfterLive(context.Context, any) {
	e.mu.Lock()
	e.after++
	e.mu.Unlock()
}

func (e *fakeEffects) Finish(context.Context, Finish) {
	e.mu.Lock()
	e.finished = true
	e.mu.Unlock()
}

func (e *fakeEffects) afters() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.after
}

type fakeMinter struct{ n atomic.Uint64 }

func (m *fakeMinter) Mint() string { return fmt.Sprintf("evt_%011d", m.n.Add(1)) }

// fakeRunStore is the durable admission backstop under test: it records the
// admitted drafts and signals each terminalize on a channel so a test can
// observe the pump's teardown write (which runs after the event channel closes).
type fakeRunStore struct {
	mu       sync.Mutex
	admitted []execution.RunDraft
	admitErr error
	term     chan string
}

func newFakeRunStore() *fakeRunStore { return &fakeRunStore{term: make(chan string, 4)} }

func (f *fakeRunStore) Admit(_ context.Context, d execution.RunDraft) error {
	if f.admitErr != nil {
		return f.admitErr
	}
	f.mu.Lock()
	f.admitted = append(f.admitted, d)
	f.mu.Unlock()
	return nil
}

func (f *fakeRunStore) Terminalize(_ context.Context, sessionID, _ string) error {
	f.term <- sessionID
	return nil
}

func (f *fakeRunStore) ReconcileOrphans(context.Context) (int, error) { return 0, nil }

func (f *fakeRunStore) admits() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.admitted)
}

// TestCoordinatorStartRejectsDurablyBusySession: when the durable backstop
// rejects admission (the session already holds a non-terminal run across a
// restart the in-memory claim can't see), Start surfaces ErrSessionBusy and
// tears the created turn down, registering no run.
func TestCoordinatorStartRejectsDurablyBusySession(t *testing.T) {
	exec := &fakeExecutor{}
	store := &fakeRunStore{admitErr: execution.ErrSessionBusy}
	c := NewCoordinator(exec, &fakeEffects{}, &fakeMinter{}, store)

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
// Start and, on its true (non-parked) terminal, releases the durable slot.
func TestCoordinatorStartAdmitsAndTerminalizes(t *testing.T) {
	exec := &fakeExecutor{}
	store := newFakeRunStore()
	proj := &fakeProjector{
		open:     []ProjectedEvent{{Durable: true, Payload: "started"}},
		terminal: []ProjectedEvent{{Durable: true, Terminal: true, Payload: "finished"}},
	}
	c := NewCoordinator(exec, &fakeEffects{}, &fakeMinter{}, store)

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
	select {
	case s := <-store.term:
		if s != "ses_1" {
			t.Fatalf("terminalized session = %q, want ses_1", s)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("pump never terminalized the durable run slot")
	}
}

// TestCoordinatorStartStreamsThenTerminates: Start opens a run, streams the
// projector's open + translated events, and — because the executor stream ends
// without a terminal — synthesizes one on teardown; each non-interrupt event is
// committed after publication.
func TestCoordinatorStartStreamsThenTerminates(t *testing.T) {
	exec := &fakeExecutor{events: []EngineEvent{stubEngineEvent{}}}
	eff := &fakeEffects{}
	proj := &fakeProjector{
		open:      []ProjectedEvent{{Durable: true, Payload: "started"}},
		translate: []ProjectedEvent{{Durable: true, Payload: "item"}},
		terminal:  []ProjectedEvent{{Durable: true, Terminal: true, Payload: "finished"}},
	}
	c := NewCoordinator(exec, eff, &fakeMinter{}, nil)

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
	// Every non-interrupt event committed after publication (AfterLive precedes
	// the terminal's hub.Close, so this read is ordered by the channel close).
	if got := eff.afters(); got != 3 {
		t.Fatalf("AfterLive calls = %d, want 3", got)
	}
	if proj.view == nil {
		t.Fatal("projector never received its segment view")
	}
}

// TestCoordinatorStartExecutorError: a turn that fails to start returns the
// error and tears the created turn down (cancels it), registering no run.
func TestCoordinatorStartExecutorError(t *testing.T) {
	exec := &fakeExecutor{startErr: fmt.Errorf("boom")}
	c := NewCoordinator(exec, &fakeEffects{}, &fakeMinter{}, nil)

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
	c := NewCoordinator(&fakeExecutor{}, &fakeEffects{}, &fakeMinter{}, nil)
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
	c := NewCoordinator(exec, &fakeEffects{}, &fakeMinter{}, nil)
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
	c := NewCoordinator(exec, &fakeEffects{}, &fakeMinter{}, nil)
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
	eff := &fakeEffects{beforeErr: fmt.Errorf("store down")}
	proj := &fakeProjector{
		open:      []ProjectedEvent{{Durable: true, Payload: "started"}},
		translate: []ProjectedEvent{{Durable: true, Terminal: true, Interrupt: true, Payload: "interrupt"}},
		terminal:  []ProjectedEvent{{Durable: true, Terminal: true, Payload: "error"}},
	}
	c := NewCoordinator(exec, eff, &fakeMinter{}, nil)
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

// TestCoordinatorBeginCancelCleanupSurvivesRequest: the run outlives the request
// that started it, so BeginCancel's cleanup context (rooted on the run's owner)
// stays alive even after the request context is canceled.
func TestCoordinatorBeginCancelCleanupSurvivesRequest(t *testing.T) {
	exec := &fakeExecutor{block: true}
	c := NewCoordinator(exec, &fakeEffects{}, &fakeMinter{}, nil)
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
