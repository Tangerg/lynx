package runs

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/application/admission"
	"github.com/Tangerg/lynx/app/runtime/internal/component/taskgroup"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
)

// ErrClosed is returned by [Coordinator.Start] once the Coordinator is closing:
// it accepts no new run segments.
var ErrClosed = errors.New("runs: coordinator closed")

// Coordinator owns the live run segments, the per-run event [Journal], the segment pump that
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
	isolation    IsolationProvider // resolves an isolated session's sandbox copy; nil = isolation off
	now          func() time.Time
	newRunID     func() string
	newSegmentID func() string
	// seq is the process-wide monotonic run-event cursor source (§11.2): the pump
	// stamps every event with the next value, fixed-width so the Journal's lexical
	// replay stays correct. It is an opaque application cursor — the evt_ wire
	// framing is applied by the delivery layer, which owns the protocol format.
	seq       atomic.Uint64
	tasks     taskgroup.Group
	registry  registry
	admission AdmissionGate
}

// AdmissionGate is the Run use case's view of the application-wide session
// admission invariant. Sessions consume their own narrow view of the same gate.
type AdmissionGate interface {
	AcquireSession(sessionID string) (release func(), ok bool)
	OpenRun(runID, sessionID, cwd string)
	BeginMaintenance(runID string) (release func(), ok bool)
	ActiveSession(sessionID string) bool
	ActiveSessionWithCwd(cwd string) string
	ActiveSessions() map[string]bool
}

// Dependencies is the complete collaborator set for the user-visible run use
// cases and the segment supervisor they own.
type Dependencies struct {
	Segments     SegmentExecutor
	Turns        TurnControl
	Sessions     SessionLifecycle
	Effects      Effects
	Admissions   AdmissionGate
	Isolation    IsolationProvider // nil disables isolated sessions (their start is refused)
	Now          func() time.Time
	NewRunID     func() string
	NewSegmentID func() string
}

// NewCoordinator builds the single owner of run use cases and live segments.
func NewCoordinator(deps Dependencies) *Coordinator {
	if deps.Now == nil {
		deps.Now = time.Now
	}
	if deps.Admissions == nil {
		deps.Admissions = &admission.Gate{}
	}
	return &Coordinator{
		executor:     deps.Segments,
		turns:        deps.Turns,
		sessions:     deps.Sessions,
		effects:      deps.Effects,
		isolation:    deps.Isolation,
		now:          deps.Now,
		newRunID:     deps.NewRunID,
		newSegmentID: deps.NewSegmentID,
		admission:    deps.Admissions,
	}
}

// mintCursor returns the next monotonic, fixed-width, lexically-ordered run-event
// cursor. It is prefix-free (the evt_ wire id is applied in delivery, §11.2); the
// fixed width keeps lexical and numeric order in agreement so the Journal can
// replay strictly-after a cursor without knowing its format.
func (c *Coordinator) mintCursor() string {
	// Pad to the full width of a uint64 (20 digits) so the width can never be
	// exceeded: a shorter min-width (e.g. %011d) would let a value past 10^11 mint a
	// wider string that sorts lexically before the narrower ones, breaking the
	// lexical==numeric agreement the Journal replays on.
	return fmt.Sprintf("%020d", c.seq.Add(1))
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
	resume := spec.Pending != nil
	taskCtx, release, ok := c.tasks.Attach(reqCtx)
	if !ok {
		if !resume {
			return nil, c.rejectUnadmittedTurn(reqCtx, spec.turnRef(), ErrClosed)
		}
		return nil, ErrClosed
	}
	runCtx, cancel := context.WithCancel(taskCtx)
	inner, err := c.executor.TurnEvents(runCtx, spec.turnRef())
	if err != nil {
		cancel()
		if !resume {
			err = c.rejectUnadmittedTurn(taskCtx, spec.turnRef(), err)
		}
		release()
		return nil, err
	}
	hub := NewJournal()
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
			err = c.rejectUnadmittedTurn(taskCtx, spec.turnRef(), err)
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
	c.admission.OpenRun(spec.RunID, spec.SessionID, spec.Cwd)
	events, unsubscribe := hub.Subscribe("")
	context.AfterFunc(reqCtx, unsubscribe)
	for _, pe := range opening {
		hub.Append(c.event(spec, pe))
	}
	if spec.Activate != nil {
		if err := spec.Activate(taskCtx); err != nil {
			reducer.abort(fmt.Errorf("runs: activate segment: %w", err))
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
	projected, err := reducer.open()
	if err != nil {
		return nil, fmt.Errorf("runs: reduce opening: %w", err)
	}
	if len(projected) == 0 {
		return nil, errors.New("runs: reducer produced no opening events")
	}
	opening := OpeningCommit{Events: make([]EventCommit, 0, len(projected))}
	if spec.Pending != nil {
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

// rejectUnadmittedTurn tears down a fresh turn that failed before its opening
// write-set committed. The rejection cause and teardown failure are both
// preserved: hiding the latter would report a clean rejection while leaking an
// executor turn the application never admitted.
func (c *Coordinator) rejectUnadmittedTurn(ctx context.Context, ref TurnRef, cause error) error {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), runCleanupTimeout)
	defer cancel()
	if err := c.executor.CancelTurn(cleanupCtx, ref); err != nil {
		cleanupErr := fmt.Errorf("runs: cancel unadmitted turn %q: %w", ref.TurnID, err)
		return errors.Join(cause, cleanupErr)
	}
	return cause
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
	if e.handle != nil {
		e.handle.requestCancel(reason)
	}
	c.registry.MarkCancel(runID, reason)
	cleanupCtx, cancel := e.handle.cleanupContext(ctx)
	return CancelBinding{SessionID: e.record.SessionID, TurnID: e.record.TurnID}, cleanupCtx, cancel, true
}

// SubscribeLive opens a coherent subscription to a live run. The record and
// Journal are captured from the same registry entry, so Delivery cannot return
// a segment id from one entry and subscribe to a replacement or removed entry.
// The subscription is dropped when ctx ends. ok=false when the run is not
// actively streaming.
func (c *Coordinator) SubscribeLive(ctx context.Context, runID, fromCursor string) (Record, <-chan Event, bool) {
	e, ok := c.registry.Get(runID)
	if !ok || e.handle == nil || e.handle.hub == nil {
		return Record{}, nil, false
	}
	events, unsubscribe := e.handle.hub.Subscribe(fromCursor)
	context.AfterFunc(ctx, unsubscribe)
	return e.record, events, true
}

// Contains reports whether a run is actively tracked.
func (c *Coordinator) Contains(runID string) bool { return c.registry.Contains(runID) }

// List snapshots the records of the currently-live runs.
func (c *Coordinator) List() []Record {
	return c.registry.List()
}

// ActiveSession reports whether the session has a run in flight (open or an
// in-progress admission claim) — the session-busy guard.
func (c *Coordinator) ActiveSession(sessionID string) bool {
	return c.admission.ActiveSession(sessionID)
}

// ActiveSessionWithCwd returns the session id of a live run whose canonical
// working tree is cwd, or "" — the cwd-sharing busy guard a file restore needs.
func (c *Coordinator) ActiveSessionWithCwd(cwd string) string {
	return c.admission.ActiveSessionWithCwd(cwd)
}

// ActiveSessions snapshots the session ids with a live run or admission claim.
func (c *Coordinator) ActiveSessions() map[string]bool { return c.admission.ActiveSessions() }

// AcquireSession reserves the single-writer admission slot and returns its
// ownership-bound release. The Coordinator satisfies the lifecycle
// session-claimer port the runtime consumes.
func (c *Coordinator) AcquireSession(sessionID string) (func(), bool) {
	return c.admission.AcquireSession(sessionID)
}

// Close stops accepting new runs and cancels + joins the in-flight pumps.
func (c *Coordinator) Close() { c.tasks.Close() }
