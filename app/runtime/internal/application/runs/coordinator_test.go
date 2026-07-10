package runs

import (
	"context"
	"fmt"
	"iter"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/kernel/runsegment"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/turn"
)

// --- fakes (drive the Coordinator without the wire or the agent SDK) ---

type fakeExecutor struct {
	events   []turn.Event
	mu       sync.Mutex
	canceled int
	startErr error
}

func (f *fakeExecutor) TurnEvents(ctx context.Context, _ turn.TurnHandle) (iter.Seq[turn.Event], error) {
	if f.startErr != nil {
		return nil, f.startErr
	}
	return func(yield func(turn.Event) bool) {
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

func (f *fakeExecutor) CancelTurn(context.Context, turn.TurnHandle) error {
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

func (p *fakeProjector) Open() []ProjectedEvent                { return p.open }
func (p *fakeProjector) Translate(turn.Event) []ProjectedEvent { return p.translate }
func (p *fakeProjector) SynthesizeTerminal() []ProjectedEvent  { return p.terminal }
func (p *fakeProjector) Abort(msg string)                      { p.aborted = msg }

type fakeEffects struct {
	mu        sync.Mutex
	before    int
	after     int
	finished  bool
	beforeErr error
}

func (e *fakeEffects) BeforeLive(context.Context, runsegment.Event) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.before++
	return e.beforeErr
}

func (e *fakeEffects) AfterLive(context.Context, runsegment.Event) {
	e.mu.Lock()
	e.after++
	e.mu.Unlock()
}

func (e *fakeEffects) Finish(context.Context, runsegment.Finish) {
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

// TestCoordinatorStartStreamsThenTerminates: Start opens a run, streams the
// projector's open + translated events, and — because the executor stream ends
// without a terminal — synthesizes one on teardown; each non-interrupt event is
// committed after publication.
func TestCoordinatorStartStreamsThenTerminates(t *testing.T) {
	exec := &fakeExecutor{events: []turn.Event{turn.TurnStart{}}}
	eff := &fakeEffects{}
	proj := &fakeProjector{
		open:      []ProjectedEvent{{Durable: true, Payload: "started"}},
		translate: []ProjectedEvent{{Durable: true, Payload: "item"}},
		terminal:  []ProjectedEvent{{Durable: true, Terminal: true, Payload: "finished"}},
	}
	c := NewCoordinator(exec, eff, &fakeMinter{})

	events, err := c.Start(context.Background(),
		StartSpec{RunID: "run_1", SessionID: "ses_1", Handle: turn.TurnHandle{SessionID: "ses_1", TurnID: "run_1"}},
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
	c := NewCoordinator(exec, &fakeEffects{}, &fakeMinter{})

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
	c := NewCoordinator(&fakeExecutor{}, &fakeEffects{}, &fakeMinter{})
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
