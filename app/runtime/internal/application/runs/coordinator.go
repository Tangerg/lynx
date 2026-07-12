package runs

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/Tangerg/lynx/app/runtime/internal/component/taskgroup"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
)

// ErrClosed is returned by [Coordinator.Start] once the Coordinator is closing:
// it accepts no new run segments.
var ErrClosed = errors.New("runs: coordinator closed")

// Coordinator owns the live run segments: admission (one writer per session, via
// the embedded [Registry]), the per-run event [Journal], the segment pump that
// drives a run from Start to terminal, and the request-detached task group that
// keeps runs alive across client disconnects and cancels + joins them on Close.
//
// It is the transport-neutral home of the run lifecycle: reading Start + the
// pump explains a run end to end, with the wire shape confined to the injected
// [Projector].
type Coordinator struct {
	executor Executor
	effects  Effects
	// seq is the process-wide monotonic run-event cursor source (§11.2): the pump
	// stamps every event with the next value, fixed-width so the Journal's lexical
	// replay stays correct. It is an opaque application cursor — the evt_ wire
	// framing is applied by the delivery layer, which owns the protocol format.
	seq atomic.Uint64
	// runStore is the durable admission backstop (§8.2): Start records the run as
	// the session's active run, the pump terminalizes it. nil disables it — the
	// in-memory registry claim still guards admission within a single process.
	runStore RunStore
	tasks    taskgroup.Group
	registry Registry[*handle]
}

// NewCoordinator builds a Coordinator over the executor it drives, the durable
// effects it commits through, and the durable run-admission backstop (nil to run
// in-memory-only).
func NewCoordinator(executor Executor, effects Effects, runStore RunStore) *Coordinator {
	return &Coordinator{executor: executor, effects: effects, runStore: runStore}
}

// mintCursor returns the next monotonic, fixed-width, lexically-ordered run-event
// cursor. It is prefix-free (the evt_ wire id is applied in delivery, §11.2); the
// fixed width keeps lexical and numeric order in agreement so the Journal can
// replay strictly-after a cursor without knowing its format.
func (c *Coordinator) mintCursor() string {
	return fmt.Sprintf("%011d", c.seq.Add(1))
}

// Start opens a run segment: it detaches the run from the request (so it
// outlives the request without losing the trace), subscribes to the executor's
// event stream, registers the live run, and spawns the pump. It returns the
// run's transport-neutral event channel; the caller drops its subscription when
// its request ends (the run keeps running and stays resumable). newProjector
// builds the per-segment projector, given the segment view it reads at terminal.
func (c *Coordinator) Start(reqCtx context.Context, spec StartSpec, newProjector func(SegmentView) Projector) (<-chan Event, error) {
	taskCtx, release, ok := c.tasks.Attach(reqCtx)
	if !ok {
		_ = c.cancelTurnAfterAdmissionFailure(reqCtx, spec.Handle)
		return nil, ErrClosed
	}
	runCtx, cancel := context.WithCancel(taskCtx)
	inner, err := c.executor.TurnEvents(runCtx, spec.Handle)
	if err != nil {
		cancel()
		_ = c.executor.CancelTurn(taskCtx, spec.Handle)
		release()
		return nil, err
	}
	if err := c.admitDurable(reqCtx, spec); err != nil {
		cancel()
		_ = c.executor.CancelTurn(taskCtx, spec.Handle)
		release()
		return nil, err
	}
	hub := NewJournal[Event]()
	live := &handle{cancel: cancel, owner: taskCtx, hub: hub}
	c.registry.Open(Record{
		ID:          spec.RunID,
		SessionID:   spec.SessionID,
		Cwd:         spec.Cwd,
		CreatedAt:   spec.CreatedAt,
		TurnID:      spec.TurnID,
		ParentRunID: spec.ParentRunID,
		Provider:    spec.Provider,
		Model:       spec.Model,
	}, live)
	events, unsubscribe := hub.Subscribe("")
	context.AfterFunc(reqCtx, unsubscribe)
	projector := newProjector(live)
	go func() {
		defer release()
		c.pump(runCtx, taskCtx, spec, inner, live, projector)
	}()
	return events, nil
}

// admitDurable records the segment's Run in the durable admission table (§8.2)
// as the LAST gate before the pump. A fresh root run (no parent) is Admitted:
// its INSERT is the gate — a rejection means the session already holds a
// non-terminal run (a race the in-memory claim missed, or a run left over across
// restart), and since nothing durable was written, tearing the turn down needs
// no compensation. A continuation of a parked run (spec.ParentRunID set) is
// already gated by the in-memory resume claim, so it does not admit a second row
// for the session; it transitions the session's existing durable row back to
// running (best-effort — the resume proceeds regardless). A nil store disables
// the durable backstop (the in-memory claim still guards within one process).
func (c *Coordinator) admitDurable(ctx context.Context, spec StartSpec) error {
	if c.runStore == nil {
		return nil
	}
	if spec.ParentRunID != "" {
		_ = c.runStore.Resume(ctx, spec.SessionID)
		return nil
	}
	return c.runStore.Admit(ctx, execution.RunDraft{
		RunID:     spec.RunID,
		SessionID: spec.SessionID,
		Provider:  spec.Provider,
		Model:     spec.Model,
		ProcessID: spec.TurnID,
		CreatedAt: spec.CreatedAt,
	})
}

// cancelTurnAfterAdmissionFailure tears down a turn that was created but never
// admitted (the Coordinator closed between turn start and Attach).
func (c *Coordinator) cancelTurnAfterAdmissionFailure(ctx context.Context, handle Handle) error {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), runCleanupTimeout)
	defer cancel()
	return c.executor.CancelTurn(cleanupCtx, handle)
}

// CancelBinding identifies the durable run/turn a cancel must act on.
type CancelBinding struct {
	SessionID string
	TurnID    string
}

// BeginCancel marks a live run for cancellation and returns the binding + a
// bounded cleanup context the caller uses to drive the durable cancel. It
// records the reason on both the live handle (read by a pump that may synthesize
// the canceled terminal before the registry update lands) and the registry
// snapshot, in that order, so a cancel can't delete an interrupt the pump is
// about to recreate. ok=false when the run isn't live (caller falls back to the
// parked-cancel path). The returned cancel func must be called when done.
func (c *Coordinator) BeginCancel(ctx context.Context, runID, reason string) (CancelBinding, context.Context, context.CancelFunc, bool) {
	e, ok := c.registry.Get(runID)
	if !ok {
		return CancelBinding{}, nil, nil, false
	}
	if e.Payload != nil {
		e.Payload.requestCancel(reason)
	}
	c.registry.MarkCancel(runID, reason)
	cleanupCtx, cancel := e.Payload.cleanupContext(ctx)
	return CancelBinding{SessionID: e.Record.SessionID, TurnID: e.Record.TurnID}, cleanupCtx, cancel, true
}

// Subscribe attaches a fresh subscriber to a live run's Journal, replaying the
// durable backlog after fromCursor then tailing live, and drops the
// subscription when ctx ends. ok=false when the run isn't actively streaming.
func (c *Coordinator) Subscribe(ctx context.Context, runID, fromCursor string) (<-chan Event, bool) {
	e, ok := c.registry.Get(runID)
	if !ok || e.Payload == nil || e.Payload.hub == nil {
		return nil, false
	}
	events, unsubscribe := e.Payload.hub.Subscribe(fromCursor)
	context.AfterFunc(ctx, unsubscribe)
	return events, true
}

// LiveRun returns a live segment's record, or false when the run isn't actively
// tracked (finished / parked / unknown).
func (c *Coordinator) LiveRun(runID string) (Record, bool) {
	e, ok := c.registry.Get(runID)
	if !ok {
		return Record{}, false
	}
	return e.Record, true
}

// Contains reports whether a run is actively tracked.
func (c *Coordinator) Contains(runID string) bool { return c.registry.Contains(runID) }

// List snapshots the records of the currently-live runs.
func (c *Coordinator) List() []Record {
	entries := c.registry.List()
	out := make([]Record, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Record)
	}
	return out
}

// ActiveSession reports whether the session has a run in flight (open or an
// in-progress admission claim) — the session-busy guard.
func (c *Coordinator) ActiveSession(sessionID string) bool {
	return c.registry.ActiveSession(sessionID)
}

// ActiveSessionWithCwd returns the session id of a live run whose canonical
// working tree is cwd, or "" — the cwd-sharing busy guard a file restore needs.
func (c *Coordinator) ActiveSessionWithCwd(cwd string) string {
	return c.registry.ActiveSessionWithCwd(cwd)
}

// ActiveSessions snapshots the session ids with a live run or admission claim.
func (c *Coordinator) ActiveSessions() map[string]bool { return c.registry.ActiveSessions() }

// ClaimSession and ReleaseSession are the single-writer admission slot; the
// Coordinator satisfies the lifecycle session-claimer port the runtime consumes.
func (c *Coordinator) ClaimSession(sessionID string) bool { return c.registry.ClaimSession(sessionID) }
func (c *Coordinator) ReleaseSession(sessionID string)    { c.registry.ReleaseSession(sessionID) }

// Close stops accepting new runs and cancels + joins the in-flight pumps.
func (c *Coordinator) Close() { c.tasks.Close() }
