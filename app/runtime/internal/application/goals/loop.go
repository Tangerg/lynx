package goals

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/completion"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/goal"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

var errTerminalOutcomeMissing = errors.New("goals: terminal run has no outcome")

const (
	goalRunCleanupTimeout = 5 * time.Second
)

// goalDrive is the local execution of one cross-process-owned active Goal
// incarnation. The durable Goal remains authoritative; this value owns its
// lease, goroutine, cancellation boundary, and completion result.
type goalDrive struct {
	incarnationID string
	cancel        context.CancelFunc
	done          chan struct{}
	lease         DriveLease
	err           error
}

func (g *goalDrive) await(ctx context.Context) error {
	if g == nil {
		return nil
	}
	if err := completion.Wait(ctx, g.done); err != nil {
		return err
	}
	return g.err
}

func (g *goalDrive) completed() bool {
	if g == nil {
		return true
	}
	select {
	case <-g.done:
		return true
	default:
		return false
	}
}

func (g *goalDrive) resultIfCompleted() (bool, error) {
	if !g.completed() {
		return false, nil
	}
	return true, g.err
}

func (g *goalDrive) quiesce() {
	if g != nil && g.cancel != nil {
		g.cancel()
	}
}

// launchLocked attaches exactly one drive to an already-accepted active Goal.
// Callers hold this session's mutation lock; BeginShutdown holds the admission
// write lock, which linearizes the durable active transition with task-group
// admission.
func (d *Driver) launchLocked(
	parent context.Context,
	sessionID, incarnationID string,
	lease DriveLease,
) error {
	if lease == nil {
		var acquired bool
		lease, acquired = d.tryDriveLease(sessionID)
		if !acquired {
			return ErrGoalOwned
		}
	}
	owner, release, ok := d.tasks.Attach(parent)
	if !ok {
		lease.Release()
		return ErrClosed
	}
	ctx, cancel := context.WithCancel(owner)
	drive := &goalDrive{
		incarnationID: incarnationID,
		cancel:        cancel,
		done:          make(chan struct{}),
		lease:         lease,
	}

	d.mutations.launch(sessionID, drive)

	go func() {
		defer release()
		defer drive.lease.Release()
		drive.err = d.drive(ctx, sessionID, incarnationID)
		close(drive.done)
		if drive.err == nil {
			d.mutations.forget(sessionID, drive)
		}
	}()
	return nil
}

// ensureDriveLocked restores the in-process side of an authoritative active
// goal after a command discovers an active row. A failed owner remains
// registered and its error is returned until an explicit lifecycle command
// quiesces it. The caller holds this session's mutation lock.
func (d *Driver) ensureDriveLocked(ctx context.Context, sessionID, incarnationID string) error {
	if d.closed.Load() {
		return ErrClosed
	}
	if drive := d.mutations.activeDrive(sessionID); drive != nil {
		if drive.incarnationID != incarnationID {
			return ErrGoalConflict
		}
		if completed, err := drive.resultIfCompleted(); completed {
			if err != nil {
				return err
			}
			d.mutations.forget(sessionID, drive)
		} else {
			return nil
		}
	}
	if err := d.launchLocked(ctx, sessionID, incarnationID, nil); err != nil {
		if errors.Is(err, ErrGoalOwned) {
			return nil
		}
		return err
	}
	return nil
}

// drive runs autonomous Runs until the goal leaves active. Cancellation (Stop /
// shutdown) leaves the goal's stored status untouched — Stop already paused it;
// a shutdown leaves it active so the boot reconcile degrades it to paused rather
// than resuming and burning budget.
func (d *Driver) drive(ctx context.Context, sessionID, incarnationID string) error {
	for {
		if ctx.Err() != nil {
			return nil
		}
		g, ok, err := d.goals.Get(ctx, sessionID)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		// Stop when the goal is gone or no longer active. The incarnation check is a
		// cheap backstop — a supersession (Stop/Start/Resume) is already caught above
		// by ctx cancellation or by the status leaving active — that guards a future
		// regression where a transition stops canceling the drive. The load-bearing
		// incarnation guard is the re-read in driveRun: it prevents adopting and
		// clobbering a foreign incarnation mid-Run.
		if !ok || g.Status != goal.StatusActive || g.IncarnationID != incarnationID {
			return nil
		}
		disposition, err := d.driveRun(ctx, &g)
		if err != nil {
			return err
		}
		if disposition != dispContinue {
			return nil
		}
	}
}

// driveRun launches one autonomous run, waits for it to finish, folds its usage
// in, and decides what to do next — all under a goal.run span. The returned
// disposition is empty when a cancellation or vanished goal means no Run
// completed, so nothing is metered.
func (d *Driver) driveRun(ctx context.Context, g *goal.Goal) (disposition runDisposition, err error) {
	ctx, span := driverTracer.Start(ctx, "goal.run", trace.WithAttributes(
		attribute.String("goal.session", g.SessionID),
		attribute.Int("goal.run_ordinal", g.Used.Runs+1),
	))
	defer span.End()
	// Meter each Run under its own span (this defer runs before span.End) so the
	// exemplar links to the Run; a "" disposition (canceled / vanished goal) is
	// not a completed Run and is not counted.
	defer func() {
		if disposition != "" {
			recordGoalRun(ctx, disposition)
		}
	}()

	if ctxErr := ctx.Err(); ctxErr != nil {
		return "", nil
	}
	var result runs.StartResult
	for {
		if waitSessionStartableErr := d.runs.WaitSessionStartable(ctx, g.SessionID); waitSessionStartableErr != nil {
			return d.resolveGoalRunStartError(ctx, g, span, waitSessionStartableErr)
		}

		// Waiting is an observation boundary, not a reservation. The Run that
		// released the Session may have charged this same Goal incarnation or
		// reported its terminal outcome. Re-read before every admission attempt so
		// a resumed HITL Run can block/complete the Goal without an extra Run being
		// launched from the pre-wait snapshot.
		owned, refreshOwnedGoalErr := d.refreshOwnedGoal(ctx, g)
		if refreshOwnedGoalErr != nil {
			span.RecordError(refreshOwnedGoalErr)
			return "", refreshOwnedGoalErr
		}
		if !owned {
			return "", nil
		}
		settleDisposition, refreshOwnedGoalErr := d.settleOwned(ctx, g)
		if refreshOwnedGoalErr != nil {
			return "", refreshOwnedGoalErr
		}
		if settleDisposition != dispContinue {
			// This drive did not launch a Run, so leave the metric disposition
			// empty. The Run that changed the Goal owns its own observation.
			return "", nil
		}

		result, refreshOwnedGoalErr = d.runs.Start(ctx, d.command(*g))
		if !errors.Is(refreshOwnedGoalErr, runs.ErrRunAdmissionBusy) {
			if refreshOwnedGoalErr != nil {
				return d.resolveGoalRunStartError(ctx, g, span, refreshOwnedGoalErr)
			}
			break
		}
		// Another Run or working-tree mutation won after the observation. Retry
		// from the real boundary and re-read the Goal again before attempting
		// admission.
	}

	finished := drainRootBoundary(result.Events, result.RunID)
	if ctx.Err() != nil {
		if finished == nil {
			return "", d.cancelRun(ctx, result.RunID)
		}
		return "", nil
	}
	recordTerminalRunAttributes(span, finished)

	// Re-read: the model may have reported completed/blocked mid-Run.
	owned, err := d.refreshOwnedGoal(ctx, g)
	if err != nil {
		span.RecordError(err)
		return "", err
	}
	if !owned {
		return "", nil
	}
	disposition, err = d.settleOwned(ctx, g)
	if err != nil {
		if ctx.Err() != nil {
			return "", nil
		}
		return "", err
	}
	if disposition != dispContinue {
		return disposition, nil
	}
	return d.resolveTerminalRun(ctx, g, span, finished)
}

func (d *Driver) resolveGoalRunStartError(
	ctx context.Context,
	g *goal.Goal,
	span trace.Span,
	startErr error,
) (runDisposition, error) {
	if ctx.Err() != nil {
		return "", nil
	}
	span.RecordError(startErr)
	span.SetStatus(codes.Error, "start run")
	// A start failure is an operational fact already recorded on the span.
	// Persist only its stable cause so Goal status never stores diagnostic
	// details that cannot be recovered consistently.
	disposition, err := d.pauseOwned(ctx, g, goal.ReasonRunStartFailed, "")
	if err != nil && ctx.Err() != nil {
		return "", nil
	}
	return disposition, err
}

func recordTerminalRunAttributes(span trace.Span, finished *run.Run) {
	if finished == nil {
		return
	}
	outcome, err := outcomeOf(finished)
	if err != nil {
		return
	}
	span.SetAttributes(
		attribute.String("run.outcome", outcome.String()),
		attribute.Float64("goal.cost_usd", runCost(finished)),
		attribute.Int("goal.steps", runSteps(finished)),
	)
}

func (d *Driver) refreshOwnedGoal(ctx context.Context, current *goal.Goal) (bool, error) {
	reread, found, err := d.goals.Get(ctx, current.SessionID)
	if err != nil {
		if ctx.Err() != nil {
			return false, nil
		}
		return false, err
	}
	if !found || reread.IncarnationID != current.IncarnationID {
		return false, nil
	}
	*current = reread
	return true, nil
}

func (d *Driver) resolveTerminalRun(
	ctx context.Context,
	g *goal.Goal,
	span trace.Span,
	finished *run.Run,
) (runDisposition, error) {
	if finished == nil {
		// A segment stream must end with its root boundary. Absence cannot prove
		// HITL: child boundaries may have been present and a broken stream may end
		// without any boundary at all. Preserve that distinction as a contract
		// failure instead of inventing a waiting Run.
		disposition, err := d.pauseOwned(ctx, g, goal.ReasonTerminalOutcomeMissing, "")
		if err != nil {
			if ctx.Err() != nil {
				return "", nil
			}
			return "", err
		}
		return disposition, nil
	}
	if finished.State() == run.Waiting {
		// Waiting is a first-class root boundary. The user may resume the Goal
		// drive while resolving the durable interrupt; WaitSessionStartable keeps
		// that drive behind the same parked Run until it terminalizes.
		disposition, err := d.pauseOwned(ctx, g, goal.ReasonAwaitingInput, "")
		if err != nil {
			if ctx.Err() != nil {
				return "", nil
			}
			return "", err
		}
		return disposition, nil
	}

	_, err := outcomeOf(finished)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "malformed terminal run")
		disposition, pauseErr := d.pauseOwned(ctx, g, goal.ReasonTerminalOutcomeMissing, "")
		if pauseErr != nil {
			if ctx.Err() != nil {
				return "", nil
			}
			return "", pauseErr
		}
		return disposition, nil
	}
	// The terminal Run transaction has already recorded this Run's usage (and
	// derived any pause/block) under the Goal incarnation. Do not reconstruct that
	// durable fact from the stream: a failed post-hoc checkpoint can otherwise
	// start an extra run against an undercounted budget.
	return dispContinue, nil
}

// command builds the next autonomous Run under the exact client contract frozen
// on the Goal incarnation. Waiting is therefore a first-class Goal boundary: a
// capable client may answer it and resume the same Goal provenance.
func (d *Driver) command(g goal.Goal) runs.StartCommand {
	return runs.StartCommand{
		SessionID:      g.SessionID,
		ModelSelection: g.ModelSelection,
		Capabilities:   g.Capabilities.Clone(),
		Input: []transcript.ContentBlock{{
			Kind: transcript.TextContent,
			Text: d.instructions(RunInstructionInput{
				Objective:  g.Objective,
				Continuing: g.Used.Runs > 0,
			}),
		}},
		// GoalIncarnationID stamps the run with the incarnation that launched it, so
		// A terminal outcome report only signals THIS Goal: a straggler Run from a superseded
		// goal (stopped, then replaced by a fresh Start) cannot mark the new goal
		// complete/blocked — its incarnation no longer matches.
		GoalIncarnationID: g.IncarnationID,
	}
}

// drainRootBoundary consumes a root run's whole-tree stream to its close and
// returns only that root's final segment boundary. Child SegmentFinished frames
// are valid members of the stream but cannot decide the owning Goal's lifecycle.
// Nil means the stream violated its root-boundary contract; waiting is expressed
// by a non-nil Run whose authoritative state is [run.Waiting].
func drainRootBoundary(events iter.Seq[runs.Event], rootRunID string) *run.Run {
	var finished *run.Run
	for ev := range events {
		if seg, ok := ev.Payload.(runs.SegmentFinished); ok && ev.RunID == rootRunID {
			run := seg.Run
			finished = &run
		}
	}
	return finished
}

// outcomeOf reads a terminal run's outcome. A SegmentFinished without one
// violates the Run contract and must not be treated as a successful Run.
func outcomeOf(run *run.Run) (run.Outcome, error) {
	outcome, terminal := run.Outcome()
	if !terminal {
		return "", errTerminalOutcomeMissing
	}
	return outcome, nil
}

func runCost(run *run.Run) float64 {
	if usage, reported := run.Metrics().Usage(); reported && usage.Total.CostUSD != nil {
		return *usage.Total.CostUSD
	}
	return 0
}

func runSteps(run *run.Run) int { return run.Metrics().Steps() }

// pauseOwned persists a drive-originated pause against the newest revision of
// the incarnation it owns. A CAS miss is resolved from the authoritative row: a
// concurrent complete/blocked transition determines the Run disposition
// instead of being mislabeled as paused.
func (d *Driver) pauseOwned(
	ctx context.Context,
	current *goal.Goal,
	code goal.ReasonCode,
	detail string,
) (runDisposition, error) {
	for current.Status == goal.StatusActive {
		expected := current.Version()
		candidate := *current
		candidate.Pause(code, detail, d.now())
		saved, applied, err := d.goals.Save(ctx, candidate, expected)
		if err != nil {
			return "", err
		}
		if applied {
			*current = saved
			return dispPaused, nil
		}
		reread, ok, err := d.goals.Get(ctx, current.SessionID)
		if err != nil {
			return "", err
		}
		if !ok || reread.IncarnationID != current.IncarnationID {
			return "", nil
		}
		*current = reread
	}
	return d.settleOwned(ctx, current)
}

// settleOwned maps the authoritative state of the current incarnation to the drive
// outcome. Complete is transient, so this method also owns its conditional
// clear and resolves any CAS miss before returning.
func (d *Driver) settleOwned(ctx context.Context, current *goal.Goal) (runDisposition, error) {
	for {
		switch current.Status {
		case goal.StatusActive:
			return dispContinue, nil
		case goal.StatusPaused:
			return dispPaused, nil
		case goal.StatusBlocked:
			return dispBlocked, nil
		case goal.StatusComplete:
			applied, err := d.goals.ClearIf(ctx, current.SessionID, current.Version())
			if err != nil {
				return "", err
			}
			if applied {
				return dispComplete, nil
			}
			reread, ok, err := d.goals.Get(ctx, current.SessionID)
			if err != nil {
				return "", err
			}
			if !ok {
				return dispComplete, nil
			}
			if reread.IncarnationID != current.IncarnationID {
				return "", nil
			}
			*current = reread
		default:
			return "", fmt.Errorf("goals: invalid authoritative status %q", current.Status)
		}
	}
}

func (d *Driver) cancelRun(ctx context.Context, runID string) error {
	cancelCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), goalRunCleanupTimeout)
	defer cancel()
	_, err := d.runs.Cancel(cancelCtx, runs.CancelCommand{
		RunID:  runID,
		Reason: "autonomous goal stopped",
	})
	// Cancellation is cleanup, not a second lifecycle intent. A terminal Run may
	// commit before its root-boundary event reaches this drive; a concurrent Goal
	// command then cancels the stream and the Run owner correctly reports
	// ErrRunFinished. The durable terminal transaction (including Goal accounting)
	// is already authoritative, so both absent and finished mean there is nothing
	// left for this cleanup owner to cancel.
	if errors.Is(err, runs.ErrRunNotFound) || errors.Is(err, runs.ErrRunFinished) {
		return nil
	}
	return err
}
