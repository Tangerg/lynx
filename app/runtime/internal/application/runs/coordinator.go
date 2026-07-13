package runs

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

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
// It is the transport-neutral home of the run lifecycle: reading Start and the
// pump explains a run end to end; delivery only presents its canonical events.
type Coordinator struct {
	executor     SegmentExecutor
	turns        TurnControl
	sessions     SessionLifecycle
	effects      Effects
	now          func() time.Time
	newRunID     func() string
	newSegmentID func() string
	// seq is the process-wide monotonic run-event cursor source (§11.2): the pump
	// stamps every event with the next value, fixed-width so the Journal's lexical
	// replay stays correct. It is an opaque application cursor — the evt_ wire
	// framing is applied by the delivery layer, which owns the protocol format.
	seq      atomic.Uint64
	tasks    taskgroup.Group
	registry Registry[*handle]
}

// Dependencies is the complete collaborator set for the user-visible run use
// cases and the segment supervisor they own.
type Dependencies struct {
	Segments     SegmentExecutor
	Turns        TurnControl
	Sessions     SessionLifecycle
	Effects      Effects
	Now          func() time.Time
	NewRunID     func() string
	NewSegmentID func() string
}

// NewCoordinator builds the single owner of run use cases and live segments.
func NewCoordinator(deps Dependencies) *Coordinator {
	if deps.Now == nil {
		deps.Now = time.Now
	}
	return &Coordinator{
		executor:     deps.Segments,
		turns:        deps.Turns,
		sessions:     deps.Sessions,
		effects:      deps.Effects,
		now:          deps.Now,
		newRunID:     deps.NewRunID,
		newSegmentID: deps.NewSegmentID,
	}
}

// mintCursor returns the next monotonic, fixed-width, lexically-ordered run-event
// cursor. It is prefix-free (the evt_ wire id is applied in delivery, §11.2); the
// fixed width keeps lexical and numeric order in agreement so the Journal can
// replay strictly-after a cursor without knowing its format.
func (c *Coordinator) mintCursor() string {
	return fmt.Sprintf("%011d", c.seq.Add(1))
}

// openSegment attaches an already-prepared executor stream, atomically commits
// admission/resume plus opening projections, registers the live owner, then
// activates a continuation and spawns the pump. The run lifetime is detached
// from the request without losing its trace; request cancellation drops only
// that subscriber.
func (c *Coordinator) openSegment(reqCtx context.Context, spec segmentSpec) (<-chan Event, error) {
	if c.executor == nil {
		return nil, errors.New("runs: executor is required")
	}
	if c.effects == nil {
		return nil, errors.New("runs: effects are required")
	}
	resume := spec.Activate != nil
	taskCtx, release, ok := c.tasks.Attach(reqCtx)
	if !ok {
		if !resume {
			_ = c.cancelTurnAfterAdmissionFailure(reqCtx, spec.Handle)
		}
		return nil, ErrClosed
	}
	runCtx, cancel := context.WithCancel(taskCtx)
	inner, err := c.executor.TurnEvents(runCtx, spec.Handle)
	if err != nil {
		cancel()
		if !resume {
			_ = c.executor.CancelTurn(taskCtx, spec.Handle)
		}
		release()
		return nil, err
	}
	hub := NewJournal[Event]()
	live := &handle{cancel: cancel, owner: taskCtx, hub: hub}
	reducer := newReducer(reducerConfig{
		RunID: spec.RunID, SegmentID: spec.SegmentID, SessionID: spec.SessionID,
		Cwd: spec.Cwd, TurnID: spec.TurnID, Provider: spec.Provider, Model: spec.Model,
		CreatedAt: spec.CreatedAt, UserInput: spec.Input, Pending: spec.Pending,
		Now: c.now, CancelReason: live.CancelReason,
	})
	opening, err := c.commitOpening(reqCtx, spec, reducer)
	if err != nil {
		cancel()
		if !resume {
			_ = c.executor.CancelTurn(taskCtx, spec.Handle)
		}
		release()
		return nil, err
	}
	c.registry.Open(Record{
		ID:        spec.RunID,
		SegmentID: spec.SegmentID,
		SessionID: spec.SessionID,
		Cwd:       spec.Cwd,
		CreatedAt: spec.CreatedAt,
		TurnID:    spec.TurnID,
		Provider:  spec.Provider,
		Model:     spec.Model,
	}, live)
	events, unsubscribe := hub.Subscribe("")
	context.AfterFunc(reqCtx, unsubscribe)
	for _, pe := range opening {
		hub.Append(c.event(spec, pe))
	}
	if spec.Activate != nil {
		if err := spec.Activate(taskCtx); err != nil {
			reducer.abort(err.Error())
			cancel()
		}
	}
	go func() {
		defer release()
		c.pump(runCtx, taskCtx, spec, inner, live, reducer)
	}()
	return events, nil
}

func (c *Coordinator) commitOpening(ctx context.Context, spec segmentSpec, reducer *reducer) ([]reduction, error) {
	projected := reducer.open()
	if len(projected) == 0 {
		return nil, errors.New("runs: reducer produced no opening events")
	}
	opening := OpeningCommit{Events: make([]EventCommit, 0, len(projected))}
	if spec.Activate != nil {
		opening.Resume = &execution.ResumeDraft{RunID: spec.RunID, SessionID: spec.SessionID}
	} else {
		opening.Admit = &execution.RunDraft{
			RunID:     spec.RunID,
			SessionID: spec.SessionID,
			Provider:  spec.Provider,
			Model:     spec.Model,
			CreatedAt: spec.CreatedAt,
		}
	}
	for _, reduced := range projected {
		if reduced.Event.Terminal() || reduced.Interrupt || reduced.Nudge != nil {
			return nil, errors.New("runs: invalid opening event")
		}
		if reduced.Commit != nil {
			opening.Events = append(opening.Events, *reduced.Commit)
		}
	}
	if len(opening.Events) == 0 {
		return nil, errors.New("runs: opening has no durable projection")
	}
	if err := c.effects.CommitOpening(ctx, opening); err != nil {
		return nil, err
	}
	return projected, nil
}

func (c *Coordinator) event(spec segmentSpec, reduced reduction) Event {
	return Event{
		RunID:     spec.RunID,
		SegmentID: spec.SegmentID,
		Seq:       c.mintCursor(),
		Timestamp: c.now().UTC(),
		Payload:   reduced.Event,
	}
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
