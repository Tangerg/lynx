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
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/goal"
)

var errTerminalOutcomeMissing = errors.New("goals: terminal run has no outcome")

const (
	goalRunCleanupTimeout = 5 * time.Second
)

// launchLocked attaches exactly one loop to an already-accepted active goal.
// Callers hold this session's mutation lock; BeginShutdown holds the admission
// write lock, which linearizes the durable active transition with task-group
// admission.
func (d *Driver) launchLocked(parent context.Context, sessionID, leaseID string) bool {
	owner, release, ok := d.tasks.Attach(parent)
	if !ok {
		return false
	}
	ctx, cancel := context.WithCancel(owner)
	handle := &loopHandle{
		leaseID:  leaseID,
		cancel:   cancel,
		released: make(chan struct{}),
	}

	d.mutations.launch(sessionID, handle)

	go func() {
		defer release()
		handle.err = d.drive(ctx, sessionID, leaseID)
		close(handle.released)
		if handle.err == nil {
			d.mutations.forget(sessionID, handle)
		}
	}()
	return true
}

// ensureDriveLocked restores the in-process side of an authoritative active
// goal after a command discovers an active row. A failed owner remains
// registered and its error is returned until an explicit lifecycle command
// quiesces it. The caller holds this session's mutation lock.
func (d *Driver) ensureDriveLocked(ctx context.Context, sessionID, leaseID string) error {
	if d.closed.Load() {
		return ErrClosed
	}
	if handle := d.mutations.driver(sessionID); handle != nil {
		if handle.leaseID != leaseID {
			return ErrGoalConflict
		}
		if err, finished := handle.outcome(); finished {
			if err != nil {
				return err
			}
			d.mutations.forget(sessionID, handle)
		} else {
			return nil
		}
	}
	if !d.launchLocked(ctx, sessionID, leaseID) {
		panic("goals: command crossed the shutdown admission boundary")
	}
	return nil
}

// drive runs autonomous Runs until the goal leaves active. Cancellation (Stop /
// shutdown) leaves the goal's stored status untouched — Stop already paused it;
// a shutdown leaves it active so the boot reconcile degrades it to paused rather
// than resuming and burning budget.
func (d *Driver) drive(ctx context.Context, sessionID, leaseID string) error {
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
		// Stop when the goal is gone or no longer active. The lease check is a
		// cheap backstop — a supersession (Stop/Start/Resume) is already caught above
		// by ctx cancellation or by the status leaving active — that guards a future
		// regression where a transition stops canceling the loop. The load-bearing
		// lease guard is the re-read in driveRun: it prevents adopting and
		// clobbering a foreign incarnation mid-Run.
		if !ok || g.Status != goal.StatusActive || g.LeaseID != leaseID {
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

	if err := ctx.Err(); err != nil {
		return "", nil
	}
	var result runs.StartResult
	if err := d.runs.WaitSessionStartable(ctx, g.SessionID); err != nil {
		if ctx.Err() != nil {
			return "", nil
		}
		return "", err
	}
	for {
		result, err = d.runs.Start(ctx, d.command(*g))
		if !errors.Is(err, runs.ErrRunAdmissionBusy) {
			break
		}
		// WaitSessionStartable is intentionally not a reservation. Another Run
		// or working-tree mutation may win between the observation and Start;
		// wait for its real boundary and retry without spending a Goal Run.
		if err := d.runs.WaitSessionStartable(ctx, g.SessionID); err != nil {
			if ctx.Err() != nil {
				return "", nil
			}
			return "", err
		}
	}
	if err != nil {
		if ctx.Err() != nil {
			return "", nil // Stop/shutdown — the state is handled by Stop / reconcile
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, "start run")
		// A start failure is an operational fact already recorded on the span.
		// Persist only its stable cause so Goal status never stores diagnostic
		// details that cannot be recovered consistently.
		disposition, err := d.pauseOwned(ctx, g, goal.ReasonRunStartFailed, "")
		if err != nil {
			if ctx.Err() != nil {
				return "", nil
			}
			return "", err
		}
		return disposition, nil
	}

	finished := drainTerminal(result.Events)
	if ctx.Err() != nil {
		if finished == nil {
			return "", d.cancelRun(ctx, result.RunID)
		}
		return "", nil
	}
	if finished != nil {
		if outcome, terminalErr := outcomeOf(finished); terminalErr == nil {
			span.SetAttributes(
				attribute.String("run.outcome", outcome.String()),
				attribute.Float64("goal.cost_usd", turnCost(finished)),
				attribute.Int("goal.steps", turnSteps(finished)),
			)
		}
	}

	// Re-read: the model may have reported completed/blocked mid-Run.
	reread, ok, err := d.goals.Get(ctx, g.SessionID)
	if err != nil {
		if ctx.Err() != nil {
			return "", nil
		}
		span.RecordError(err)
		return "", err
	}
	if !ok {
		return "", nil
	}
	// If the lease changed, a Stop/Start/Resume superseded this loop's goal
	// while the run was in flight. Adopting the re-read (a different incarnation,
	// maybe a whole new objective) and saving to it would clobber a goal this
	// loop no longer owns; stop instead. This keeps g at the launch lease, so the
	// terminal saves below CAS on the incarnation the loop actually drove.
	if reread.LeaseID != g.LeaseID {
		return "", nil
	}
	*g = reread
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

	if finished == nil {
		// The run parked for HITL and produced no terminal (rare — autonomous runs
		// are headless, so an unanswerable interrupt auto-denies rather than
		// parking). Wait for the user, who resolves it and can resume the goal.
		disposition, err := d.pauseOwned(ctx, g, goal.ReasonAwaitingInput, "")
		if err != nil {
			if ctx.Err() != nil {
				return "", nil
			}
			return "", err
		}
		return disposition, nil
	}

	outcome, err := outcomeOf(finished)
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
	span.SetAttributes(
		attribute.String("run.outcome", outcome.String()),
		attribute.Float64("goal.cost_usd", turnCost(finished)),
		attribute.Int("goal.steps", turnSteps(finished)),
	)
	// The terminal Run transaction has already recorded this Run's usage (and
	// derived any pause/block) under the goal lease. Do not reconstruct that
	// durable fact from the stream: a failed post-hoc checkpoint can otherwise
	// start an extra run against an undercounted budget.
	return dispContinue, nil
}

// command builds the next autonomous run. It is headless: no InterruptKinds, so a
// tool that would need approval is auto-denied by the run rather than parking a
// loop no client is watching (the user's chosen global approval stance still
// gates tools — yolo runs everything, a stricter stance keeps the agent read-only).
func (d *Driver) command(g goal.Goal) runs.StartCommand {
	return runs.StartCommand{
		SessionID:      g.SessionID,
		ModelSelection: g.ModelSelection,
		Input: []transcript.ContentBlock{{
			Kind: transcript.TextContent,
			Text: d.prompt(PromptInput{
				Objective:  g.Objective,
				Continuing: g.Used.Runs > 0,
			}),
		}},
		// GoalLeaseID stamps the run with the incarnation that launched it, so
		// report_goal_outcome only signals THIS Goal: a straggler Run from a superseded
		// goal (stopped, then replaced by a fresh Start) cannot mark the new goal
		// complete/blocked — its lease no longer matches.
		GoalLeaseID: g.LeaseID,
	}
}

// drainTerminal consumes a run's event stream to its close and returns the run's
// terminal record, or nil when the stream closed without one (the run parked).
func drainTerminal(events iter.Seq[runs.Event]) *transcript.Run {
	var finished *transcript.Run
	for ev := range events {
		if seg, ok := ev.Payload.(runs.SegmentFinished); ok {
			run := seg.Run
			finished = &run
		}
	}
	return finished
}

// outcomeOf reads a terminal run's outcome. A SegmentFinished without one
// violates the Run contract and must not be treated as a successful Run.
func outcomeOf(run *transcript.Run) (execution.Outcome, error) {
	if run.Outcome == nil {
		return 0, errTerminalOutcomeMissing
	}
	return *run.Outcome, nil
}

func turnCost(run *transcript.Run) float64 {
	if run.Metrics.Usage != nil && run.Metrics.Usage.CostUSD != nil {
		return *run.Metrics.Usage.CostUSD
	}
	return 0
}

func turnSteps(run *transcript.Run) int { return run.Metrics.Steps }

// pauseOwned persists a loop-originated pause against the newest revision of
// the lease it owns. A CAS miss is resolved from the authoritative row: a
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
		if !ok || reread.LeaseID != current.LeaseID {
			return "", nil
		}
		*current = reread
	}
	return d.settleOwned(ctx, current)
}

// settleOwned maps the authoritative state of the current lease to the loop
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
			if reread.LeaseID != current.LeaseID {
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
	if errors.Is(err, runs.ErrRunNotFound) {
		return nil
	}
	return err
}

func (h *loopHandle) quiesce() {
	h.cancel()
}
