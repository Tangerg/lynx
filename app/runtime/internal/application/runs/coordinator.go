package runs

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"time"

	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/lynx/app/runtime/internal/application/admission"
	"github.com/Tangerg/lynx/app/runtime/internal/application/change"
	"github.com/Tangerg/lynx/app/runtime/internal/component/replaycursor"
	"github.com/Tangerg/lynx/app/runtime/internal/component/taskgroup"
	rundomain "github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

// ErrClosed is returned by [Coordinator.Start] once the Coordinator is closing:
// it accepts no new run segments.
var ErrClosed = errors.New("runs: coordinator closed")

// Coordinator owns the live run segments, the per-run event [Journal], the
// segment pump that drives a run from Start to terminal, and the
// request-detached task group that keeps runs alive across client disconnects
// and cancels + joins them during shutdown.
//
// Reading Start and the pump explains a Run end to end through canonical
// application events.
type Coordinator struct {
	executor     SegmentExecutor
	control      ExecutionControl
	sessions     SessionLifecycle
	effects      Effects
	isolation    IsolationProvider // resolves an isolated session's sandbox copy; nil = isolation off
	now          func() time.Time
	newRunID     func() string
	newSegmentID func() string
	// runs reads the durable run record. A subscribe or a steer that cannot be
	// served has to say WHY — waiting, finished, a child, or a segment that has
	// been replaced — and only the durable projection knows: the live registry
	// holds running segments, so every one of those looks identical there.
	runs RunProjection
	// items resolves the exact parent tool projection a waiting child
	// cancellation replaces.
	items ItemProjection
	// epoch identifies this Coordinator's event streams. Every cursor it mints
	// carries it, so a cursor from a previous process names a buffer that no
	// longer exists instead of resolving against a live one. It is minted once
	// per Coordinator, which is what makes "a restart changes it" structural
	// rather than remembered.
	epoch     string
	retention Retention
	tasks     taskgroup.Group
	registry  registry
	admission *admission.Gate
	// changed tells clients that are NOT following this run that its lifecycle
	// moved. The run's own stream carries the events themselves; this is the
	// invalidation for everyone else, published only after the durable commit the
	// event stands on. nil publishes nothing.
	changed change.Publish
}

// Dependencies is the complete collaborator set for the user-visible run use
// cases and the segment supervisor they own.
type Dependencies struct {
	Segments   SegmentExecutor
	Control    ExecutionControl
	Sessions   SessionLifecycle
	Effects    Effects
	Runs       RunProjection
	Items      ItemProjection
	Admissions *admission.Gate
	Isolation  IsolationProvider // nil disables isolated sessions (their start is refused)
	Now        func() time.Time
	// Retention bounds every segment's replay window. Zero takes
	// [DefaultRetention], which is also what discovery advertises.
	Retention    Retention
	NewRunID     func() string
	NewSegmentID func() string
	// Changed publishes run/session/interrupt/state invalidations for clients that
	// are not following the run. nil disables them (no runtime change stream wired).
	Changed change.Publish
}

// NewCoordinator builds the single owner of run use cases and live segments.
func NewCoordinator(deps Dependencies) *Coordinator {
	if deps.Now == nil {
		deps.Now = time.Now
	}
	if deps.Retention == (Retention{}) {
		deps.Retention = DefaultRetention()
	}
	return &Coordinator{
		executor:     deps.Segments,
		control:      deps.Control,
		sessions:     deps.Sessions,
		effects:      deps.Effects,
		runs:         deps.Runs,
		items:        deps.Items,
		isolation:    deps.Isolation,
		now:          deps.Now,
		newRunID:     deps.NewRunID,
		newSegmentID: deps.NewSegmentID,
		epoch:        replaycursor.NewEpoch(),
		retention:    deps.Retention,
		admission:    deps.Admissions,
		changed:      deps.Changed,
	}
}

// ReplayRetention is the window this Coordinator enforces. Discovery publishes
// it from here rather than from a constant of its own: a number the client is
// told and a number the runtime evicts by must be the same number.
func (c *Coordinator) ReplayRetention() Retention { return c.retention }

// WaitSessionStartable lets an application-owned continuation wait for the
// current Session Run and working-tree mutation boundaries before attempting
// its own Start. It does not reserve either resource; Start remains the
// authority that acquires them.
func (c *Coordinator) WaitSessionStartable(ctx context.Context, sessionID string) error {
	if c == nil || c.admission == nil || c.sessions == nil {
		return errors.New("runs: admission gate is unavailable")
	}
	sess, err := c.sessions.Get(ctx, sessionID)
	if err != nil {
		return err
	}
	return c.admission.WaitRunStartable(ctx, sess.ID, sess.CWD)
}

// openSegment attaches an already-prepared executor stream, atomically commits
// admission/resume plus opening projections, registers the live owner, then
// activates a continuation and spawns the pump. The run lifetime is detached
// from the request without losing its trace; request cancellation drops only
// that subscriber.
func (c *Coordinator) openSegment(reqCtx context.Context, spec segmentSpec) (iter.Seq[Event], error) {
	if c.executor == nil {
		return nil, errors.New("runs: executor is required")
	}
	if c.effects == nil {
		return nil, errors.New("runs: effects are required")
	}
	resume := spec.Continuation != nil
	taskCtx, release, ok := c.tasks.Attach(reqCtx)
	if !ok {
		if !resume {
			return nil, c.rejectUnadmittedExecution(reqCtx, spec.executorRef(), ErrClosed)
		}
		return nil, ErrClosed
	}
	runCtx, cancel := context.WithCancel(taskCtx)
	inner, err := c.executor.Events(runCtx, spec.executorRef())
	if err != nil {
		cancel()
		if !resume {
			err = c.rejectUnadmittedExecution(taskCtx, spec.executorRef(), err)
		}
		release()
		return nil, err
	}
	hub := newJournal(streamScope{
		Epoch: c.epoch, RunID: spec.RunID, SegmentID: spec.SegmentID,
	}, c.retention)
	live := &handle{cancel: cancel, owner: taskCtx, hub: hub, done: make(chan struct{})}
	routes, err := c.openingRoutes(spec, live.CancelReasonFor)
	if err != nil {
		cancel()
		if !resume {
			err = c.rejectUnadmittedExecution(taskCtx, spec.executorRef(), err)
		}
		release()
		return nil, err
	}
	for _, route := range routes.admissionOrder {
		if route.source.ProcessID == "" {
			continue
		}
		if err := live.bindExecutorProcess(route.runID, route.source.ProcessID); err != nil {
			cancel()
			if !resume {
				err = c.rejectUnadmittedExecution(taskCtx, spec.executorRef(), err)
			}
			release()
			return nil, err
		}
	}
	openings, err := c.commitOpening(reqCtx, spec, routes)
	if err != nil {
		cancel()
		if !resume {
			err = c.rejectUnadmittedExecution(taskCtx, spec.executorRef(), err)
		}
		release()
		return nil, err
	}
	if spec.admission != nil && !spec.admission.Admit(spec.RunID) {
		panic("runs: committed opening without a pending admission")
	}
	c.registry.Open(Record{
		ID:             spec.RunID,
		SegmentID:      spec.SegmentID,
		SessionID:      spec.SessionID,
		CWD:            spec.CWD,
		CreatedAt:      spec.CreatedAt,
		ExecutorID:     spec.ExecutorID,
		ModelSelection: spec.ModelSelection,
		Capabilities:   spec.effectiveCapabilities(),
	}, live)
	// The opening subscription attaches before any event is appended, so tail-only
	// and "from the beginning" are the same stream here — there is no beginning yet.
	opened := hub.Tail()
	stopUnsubscribe := context.AfterFunc(reqCtx, opened.Cancel)
	seq := func(yield func(Event) bool) {
		defer stopUnsubscribe()
		opened.Events(yield)
	}
	for _, opening := range openings {
		for _, reduced := range opening.batch.events {
			hub.Append(c.event(opening.route.runID, opening.route.segmentID, reduced))
		}
	}
	segmentStartedAt := c.now().UTC()
	for _, route := range routes.admissionOrder {
		route.segmentStartedAt = segmentStartedAt
	}
	if spec.Activate != nil {
		if err := spec.Activate(taskCtx); err != nil {
			trace.SpanFromContext(taskCtx).RecordError(fmt.Errorf("runs: activate segment: %w", err))
			routes.abortUnfinished()
			cancel()
		}
	}
	go func() {
		defer release()
		c.pump(runCtx, taskCtx, spec, inner, live, routes)
	}()
	return seq, nil
}

type routeOpening struct {
	route *executorRoute
	batch reductionBatch
}

func (c *Coordinator) commitOpening(ctx context.Context, spec segmentSpec, routes *executorRoutes) ([]routeOpening, error) {
	ordered, err := routes.unfinishedInPostorder()
	if err != nil {
		return nil, fmt.Errorf("runs: order opening tree: %w", err)
	}
	opening := OpeningCommit{}
	if spec.Continuation != nil {
		opening.Resume = &rundomain.TreeResumeDraft{
			RootRunID: spec.RunID,
			SessionID: spec.SessionID,
			ResumedAt: c.now().UTC(),
			Runs:      make([]rundomain.RunResumeDraft, 0, len(ordered)),
		}
	} else {
		opening.Admit = &rundomain.RunDraft{
			RunID:          spec.RunID,
			SessionID:      spec.SessionID,
			SegmentID:      spec.SegmentID,
			ModelSelection: spec.ModelSelection,
			GoalLeaseID:    spec.GoalLeaseID,
			Limits:         spec.Limits,
			Capabilities:   spec.Capabilities,
			CreatedAt:      spec.CreatedAt,
		}
		opening.ScheduledSession = spec.ScheduledSession
		opening.SessionModel = spec.SessionModel
		opening.ScheduleFiring = spec.ScheduleFiring
	}
	openings := make([]routeOpening, 0, len(ordered))
	for _, route := range ordered {
		projected, err := route.reducer.open()
		if err != nil {
			return nil, fmt.Errorf("runs: reduce opening for Run %q: %w", route.runID, err)
		}
		if len(projected.events) == 0 {
			return nil, fmt.Errorf("runs: reducer for Run %q produced no opening events", route.runID)
		}
		if projected.parkCommit != nil {
			return nil, fmt.Errorf("runs: opening for Run %q cannot park", route.runID)
		}
		if err := validateRouteReductionBatch(route, spec.SessionID, projected); err != nil {
			return nil, err
		}
		for _, reduced := range projected.events {
			if reduced.Event.Terminal() || reduced.Nudge != nil {
				return nil, fmt.Errorf("runs: invalid opening event for Run %q", route.runID)
			}
			if reduced.Commit != nil {
				opening.Events = append(opening.Events, *reduced.Commit)
			}
		}
		if opening.Resume != nil {
			opening.Resume.Runs = append(opening.Resume.Runs, rundomain.RunResumeDraft{
				RunID:     route.runID,
				SegmentID: route.segmentID,
			})
		}
		openings = append(openings, routeOpening{route: route, batch: projected})
	}
	// An opening may carry no item commits at all — a resumed segment that only
	// delivers an approval has nothing to append. Its durable projection is the
	// admission or resume above, which is what makes the Run exist.
	commitOpening := c.effects.CommitOpening
	if spec.CommitOpening != nil {
		commitOpening = spec.CommitOpening
	}
	if err := commitOpening(ctx, opening); err != nil {
		return nil, err
	}
	return openings, nil
}

// event builds the envelope for one reduced payload. Its stream position is NOT
// set here — the Journal assigns it while publishing, so sequence order and
// publication order are the same order by construction.
func (c *Coordinator) event(runID, segmentID string, reduced reduction) Event {
	return Event{
		RunID:     runID,
		SegmentID: segmentID,
		Timestamp: c.now().UTC(),
		Payload:   reduced.Event,
	}
}

// rejectUnadmittedExecution tears down a fresh execution that failed before its opening
// write-set committed. The rejection cause and teardown failure are both
// preserved: hiding the latter would report a clean rejection while leaking an
// executor the application never admitted.
func (c *Coordinator) rejectUnadmittedExecution(ctx context.Context, ref ExecutorRef, cause error) error {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), runCleanupTimeout)
	defer cancel()
	if err := c.executor.CancelExecution(cleanupCtx, ref); err != nil {
		cleanupErr := fmt.Errorf("runs: cancel unadmitted executor %q: %w", ref.ExecutorID, err)
		return errors.Join(cause, cleanupErr)
	}
	return cause
}

// Subscribe attaches to the Segment the request names. The record and Journal
// are captured from the same registry entry, so a caller cannot receive a
// Segment id from one entry and subscribe to a replacement or removed entry. The
// subscription is dropped when ctx ends or the consumer stops ranging.
//
// Refusals name what the caller should do instead: [ErrRunNotFound],
// [transcript.ErrNotRoot], [ErrRunWaiting], [ErrRunFinished],
// [ErrStaleSegment], [ErrReplayCursorInvalid], [ErrReplayUnavailable], and
// [rundomain.InsufficientCapabilities] when the caller could not follow what this run
// publishes — the same rule a resume applies, kept here rather than at each
// caller so the two entry points into an existing Run cannot disagree about it.
func (c *Coordinator) Subscribe(ctx context.Context, req SubscribeRequest) (Subscription, error) {
	live, err := c.addressLiveSegment(ctx, req.RunID, req.SegmentID)
	if err != nil {
		return Subscription{}, err
	}
	if gap := live.record.Capabilities.MissingFrom(req.CallerCapabilities); !gap.IsEmpty() {
		return Subscription{}, &rundomain.InsufficientCapabilities{RunID: req.RunID, Missing: gap}
	}
	attached, err := live.handle.hub.attach(req.Cursor)
	if err != nil {
		return Subscription{}, err
	}
	stopUnsubscribe := context.AfterFunc(ctx, attached.Cancel)
	return Subscription{
		Record:     live.record,
		HeadCursor: attached.HeadCursor,
		Events: func(yield func(Event) bool) {
			defer stopUnsubscribe()
			attached.Events(yield)
		},
	}, nil
}

// addressLiveSegment resolves the run and segment a control command addresses,
// refusing with the reason the caller can act on.
//
// The durable record is the authority for the refusal, not the live registry:
// the registry holds only running segments, so a run that is waiting, finished,
// or a child all look the same there — one indistinguishable "not found" for
// three situations whose remedies are answering an interrupt, reading the
// transcript, and following rootRunId.
//
// Both entry points into an executing run (subscribe and steer) resolve here, so
// they cannot come to different conclusions about the same run.
func (c *Coordinator) addressLiveSegment(ctx context.Context, runID, segmentID string) (liveSegment, error) {
	if c.runs == nil {
		return liveSegment{}, errors.New("runs: run projection is required")
	}
	run, ok, err := c.runs.Run(ctx, runID)
	if err != nil {
		return liveSegment{}, fmt.Errorf("runs: read run %q: %w", runID, err)
	}
	if !ok {
		return liveSegment{}, ErrRunNotFound
	}
	if run.Lineage().IsChild() {
		return liveSegment{}, fmt.Errorf("%w: %q", transcript.ErrNotRoot, runID)
	}
	switch status := run.State.Status(); status {
	case rundomain.StatusWaiting:
		return liveSegment{}, fmt.Errorf("%w: %q", ErrRunWaiting, runID)
	case rundomain.StatusFinished:
		return liveSegment{}, fmt.Errorf("%w: %q", ErrRunFinished, runID)
	case rundomain.StatusRunning:
	default:
		return liveSegment{}, fmt.Errorf("runs: run %q has unknown status %d", runID, status)
	}
	if run.ActiveSegmentID != segmentID {
		return liveSegment{}, fmt.Errorf("%w: run %q is executing %q", ErrStaleSegment, runID, run.ActiveSegmentID)
	}
	live, ok := c.registry.Get(runID)
	if !ok || live.handle == nil || live.handle.hub == nil {
		// A Running record whose segment this process does not own. Restart recovery
		// terminalizes orphans before the runtime serves, so this is a broken
		// invariant rather than a state a client can act on — reporting it as one
		// would teach the client a lie about the run.
		return liveSegment{}, fmt.Errorf("runs: run %q is running segment %q with no live stream", runID, segmentID)
	}
	return live, nil
}

// BeginShutdown prevents new runs and cancels every in-flight pump.
func (c *Coordinator) BeginShutdown() { c.tasks.Cancel() }

// AwaitShutdown joins every in-flight pump after [BeginShutdown].
func (c *Coordinator) AwaitShutdown(ctx context.Context) error { return c.tasks.Wait(ctx) }
