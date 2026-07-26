package goals

import (
	"context"
	"errors"
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
	goalStoreAttemptTimeout = 5 * time.Second
	goalStoreRetryInitial   = 50 * time.Millisecond
	goalStoreRetryMax       = time.Second
	goalRunCleanupTimeout   = 5 * time.Second
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
		d.forget(sessionID, handle)
		close(handle.released)
	}()
	return true
}

func (d *Driver) forget(sessionID string, handle *loopHandle) {
	d.mutations.forget(sessionID, handle)
}

// ensureDriveLocked restores the in-process side of an authoritative active
// goal after a command discovers an active row. The caller holds this session's
// mutation lock.
func (d *Driver) ensureDriveLocked(ctx context.Context, sessionID, leaseID string) {
	if d.closed.Load() || d.mutations.driverLease(sessionID) == leaseID {
		return
	}
	if !d.launchLocked(ctx, sessionID, leaseID) {
		panic("goals: command crossed the shutdown admission boundary")
	}
}

// drive runs autonomous turns until the goal leaves active. Cancellation (Stop /
// shutdown) leaves the goal's stored status untouched — Stop already paused it;
// a shutdown leaves it active so the boot reconcile degrades it to paused rather
// than resuming and burning budget.
func (d *Driver) drive(ctx context.Context, sessionID, leaseID string) error {
	for {
		if ctx.Err() != nil {
			return nil
		}
		g, ok, err := d.loadGoal(ctx, sessionID)
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
		// lease guard is the re-read in runTurn: it prevents adopting and
		// clobbering a foreign incarnation mid-turn.
		if !ok || g.Status != goal.StatusActive || g.LeaseID != leaseID {
			return nil
		}
		disposition, err := d.runTurn(ctx, &g)
		if err != nil {
			return err
		}
		if disposition != dispContinue {
			return nil
		}
	}
}

// runTurn launches one autonomous run, waits for it to finish, folds its usage
// in, and decides what to do next — all under a goal.turn span. The returned
// disposition is empty when a cancellation or vanished goal means no turn
// completed, so nothing is metered.
func (d *Driver) runTurn(ctx context.Context, g *goal.Goal) (disposition turnDisposition, err error) {
	ctx, span := driverTracer.Start(ctx, "goal.turn", trace.WithAttributes(
		attribute.String("goal.session", g.SessionID),
		attribute.Int("goal.turn", g.Used.Turns+1),
	))
	defer span.End()
	// Meter each turn under its own span (this defer runs before span.End) so the
	// exemplar links to the turn; a "" disposition (canceled / vanished goal) is
	// not a completed turn and is not counted.
	defer func() {
		if disposition != "" {
			recordGoalTurn(ctx, disposition)
		}
	}()

	if err := ctx.Err(); err != nil {
		return "", nil
	}
	result, err := d.runs.Start(ctx, d.command(*g))
	if err != nil {
		if ctx.Err() != nil {
			return "", nil // Stop/shutdown — the state is handled by Stop / reconcile
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, "start run")
		// A start failure is an operational fact already recorded on the span.
		// Persist only its stable cause so goal status cannot become a transport
		// for adapter diagnostics.
		if err := d.pauseOwned(ctx, g, goal.ReasonRunStartFailed, ""); err != nil {
			if ctx.Err() != nil {
				return "", nil
			}
			return "", err
		}
		return dispPaused, nil
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

	// Re-read: the model may have set complete/blocked mid-turn via update_goal.
	reread, ok, err := d.loadGoal(ctx, g.SessionID)
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
	switch g.Status {
	case goal.StatusComplete:
		if err := d.clearOwned(ctx, g); err != nil {
			if ctx.Err() != nil {
				return "", nil
			}
			return "", err
		}
		return dispComplete, nil
	case goal.StatusBlocked:
		return dispBlocked, nil // the model declared blocked
	case goal.StatusPaused:
		return "", nil // a concurrent Stop already recorded its intent
	}

	if finished == nil {
		// The run parked for HITL and produced no terminal (rare — autonomous runs
		// are headless, so an unanswerable interrupt auto-denies rather than
		// parking). Wait for the user, who resolves it and can resume the goal.
		if err := d.pauseOwned(ctx, g, goal.ReasonAwaitingInput, ""); err != nil {
			if ctx.Err() != nil {
				return "", nil
			}
			return "", err
		}
		return dispPaused, nil
	}

	outcome, err := outcomeOf(finished)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "malformed terminal run")
		if err := d.pauseOwned(ctx, g, goal.ReasonTerminalOutcomeMissing, ""); err != nil {
			if ctx.Err() != nil {
				return "", nil
			}
			return "", err
		}
		return dispPaused, nil
	}
	span.SetAttributes(
		attribute.String("run.outcome", outcome.String()),
		attribute.Float64("goal.cost_usd", turnCost(finished)),
		attribute.Int("goal.steps", turnSteps(finished)),
	)
	// The terminal Run transaction has already recorded this turn's usage (and
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
				Continuing: g.Used.Turns > 0,
			}),
		}},
		// GoalLeaseID stamps the run with the incarnation that launched it, so
		// update_goal only signals THIS goal: a straggler run from a superseded
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
// violates the Run contract and must not be treated as a successful turn.
func outcomeOf(run *transcript.Run) (execution.Outcome, error) {
	if run.Outcome == nil {
		return 0, errTerminalOutcomeMissing
	}
	return *run.Outcome, nil
}

func turnCost(run *transcript.Run) float64 {
	if run.Result == nil {
		return 0
	}
	if run.Result.Usage != nil && run.Result.Usage.CostUSD != nil {
		return *run.Result.Usage.CostUSD
	}
	return 0
}

func turnSteps(run *transcript.Run) int {
	if run.Result == nil {
		return 0
	}
	return run.Result.Steps
}

// pauseOwned persists a loop-originated pause against the newest revision of
// the lease it owns. Store failures retry in this same supervisor goroutine;
// CAS misses re-read and reapply only while the same active incarnation remains
// authoritative.
func (d *Driver) pauseOwned(ctx context.Context, current *goal.Goal, cause goal.ReasonCause, detail string) error {
	for current.Status == goal.StatusActive {
		expected := current.Version()
		candidate := *current
		candidate.Pause(cause, detail, d.now())
		saved, applied, err := d.saveGoal(ctx, candidate, expected)
		if err != nil {
			return err
		}
		if applied {
			*current = saved
			return nil
		}
		reread, ok, err := d.loadGoal(ctx, current.SessionID)
		if err != nil {
			return err
		}
		if !ok || reread.LeaseID != current.LeaseID {
			return nil
		}
		*current = reread
	}
	return nil
}

// clearOwned removes the transient complete state without abandoning the
// driver on a store fault. A newer revision of the same completed lease is
// retried; a changed lease or lifecycle state wins.
func (d *Driver) clearOwned(ctx context.Context, current *goal.Goal) error {
	for current.Status == goal.StatusComplete {
		applied, err := d.clearGoal(ctx, current.SessionID, current.Version())
		if err != nil {
			return err
		}
		if applied {
			return nil
		}
		reread, ok, err := d.loadGoal(ctx, current.SessionID)
		if err != nil {
			return err
		}
		if !ok || reread.LeaseID != current.LeaseID {
			return nil
		}
		*current = reread
	}
	return nil
}

func (d *Driver) loadGoal(ctx context.Context, sessionID string) (loaded goal.Goal, ok bool, err error) {
	err = retryGoalStore(ctx, func(attempt context.Context) error {
		var loadErr error
		loaded, ok, loadErr = d.goals.Get(attempt, sessionID)
		return loadErr
	})
	return loaded, ok, err
}

func (d *Driver) saveGoal(ctx context.Context, candidate goal.Goal, expected goal.Version) (saved goal.Goal, applied bool, err error) {
	err = retryGoalStore(ctx, func(attempt context.Context) error {
		var saveErr error
		saved, applied, saveErr = d.goals.Save(attempt, candidate, expected)
		return saveErr
	})
	return saved, applied, err
}

func (d *Driver) clearGoal(ctx context.Context, sessionID string, expected goal.Version) (applied bool, err error) {
	err = retryGoalStore(ctx, func(attempt context.Context) error {
		var clearErr error
		applied, clearErr = d.goals.ClearIf(attempt, sessionID, expected)
		return clearErr
	})
	return applied, err
}

func retryGoalStore(ctx context.Context, operation func(context.Context) error) error {
	if ctx == nil {
		ctx = context.Background()
	}
	delay := goalStoreRetryInitial
	for {
		attempt, cancel := context.WithTimeout(ctx, goalStoreAttemptTimeout)
		err := operation(attempt)
		cancel()
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		recordGoalStoreRetry(ctx, err)

		timer := time.NewTimer(delay)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		}
		delay = min(delay*2, goalStoreRetryMax)
	}
}

func (d *Driver) cancelRun(ctx context.Context, runID string) error {
	cancelCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), goalRunCleanupTimeout)
	defer cancel()
	err := d.runs.Cancel(cancelCtx, runs.CancelCommand{
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
