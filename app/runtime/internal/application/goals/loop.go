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

const goalPersistenceTimeout = 5 * time.Second

// launchLocked attaches exactly one loop to an already-accepted active goal.
// Callers hold mutations.commands, which is also held by BeginShutdown; that
// linearizes the durable active transition with task-group admission.
func (d *Driver) launchLocked(parent context.Context, sessionID, leaseID string) {
	owner, release, ok := d.tasks.Attach(parent)
	if !ok {
		panic("goals: accepted an active goal after driver shutdown")
	}
	ctx, cancel := context.WithCancel(owner)
	handle := &loopHandle{
		leaseID: leaseID, cancel: cancel, owner: owner, done: make(chan struct{}),
		loopReconcile: true,
	}

	d.mutations.launch(sessionID, handle)

	go func() {
		defer release()
		handle.err = d.drive(ctx, sessionID, leaseID, handle)
		d.forget(sessionID, handle)
		close(handle.done)
		d.restoreDrive(handle.owner, sessionID, leaseID, handle)
	}()
}

func (d *Driver) forget(sessionID string, handle *loopHandle) {
	d.mutations.forget(sessionID, handle)
}

// ensureDriveLocked restores the in-process side of an authoritative active
// goal. It is used after a command discovers an active row and by Stop's
// failed-write recovery. The caller holds mutations.commands.
func (d *Driver) ensureDriveLocked(ctx context.Context, sessionID, leaseID string) {
	if d.closed || d.mutations.driverLease(sessionID) == leaseID {
		return
	}
	d.launchLocked(ctx, sessionID, leaseID)
}

// restoreDrive closes the gap between a loop ending and a durable state change.
// A loop may lose a CAS or a store write after a run has completed; it must not
// turn the still-active durable intent into an orphan. This performs one fresh
// authoritative read and only attaches a replacement for the same lease. It is
// not a retry policy: a durable non-active transition always wins immediately.
func (d *Driver) restoreDrive(ctx context.Context, sessionID, leaseID string, handle *loopHandle) {
	if !handle.mayReconcile() {
		return
	}
	// Persistence runs outside the command mutex. BeginShutdown must be able to
	// close admission and cancel ctx even when a store is slow; lifecycle
	// commands that win during this read are harmless because a replacement
	// loop revalidates status and lease before starting any Run.
	g, ok, err := d.goals.Get(ctx, sessionID)
	if err != nil || !ok || g.Status != goal.StatusActive || g.LeaseID != leaseID {
		return
	}
	d.mutations.lock()
	defer d.mutations.unlock()
	if d.closed || !handle.mayReconcile() || d.mutations.driverLease(sessionID) != "" {
		return
	}
	// Unlike an explicit command, background recovery may only fill an empty
	// slot. A newer Start can change the lease after the read above and install
	// its driver before this lock is reacquired; recovery must never replace it
	// with the stale lease it observed.
	d.launchLocked(ctx, sessionID, leaseID)
}

// drive runs autonomous turns until the goal leaves active. Cancellation (Stop /
// shutdown) leaves the goal's stored status untouched — Stop already paused it;
// a shutdown leaves it active so the boot reconcile degrades it to paused rather
// than resuming and burning budget.
func (d *Driver) drive(ctx context.Context, sessionID, leaseID string, handle *loopHandle) error {
	for {
		if ctx.Err() != nil {
			return nil
		}
		g, ok, err := d.goals.Get(ctx, sessionID)
		// Stop when the goal is gone or no longer active. The lease check is a
		// cheap backstop — a supersession (Stop/Start/Resume) is already caught above
		// by ctx cancellation or by the status leaving active — that guards a future
		// regression where a transition stops canceling the loop. The load-bearing
		// lease guard is the re-read in runTurn: it prevents adopting and
		// clobbering a foreign incarnation mid-turn.
		if err != nil || !ok || g.Status != goal.StatusActive || g.LeaseID != leaseID {
			return nil
		}
		disposition, err := d.runTurn(ctx, &g, handle)
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
func (d *Driver) runTurn(ctx context.Context, g *goal.Goal, handle *loopHandle) (disposition turnDisposition, err error) {
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
		g.Pause(goal.ReasonRunStartFailed, "", d.now())
		d.save(ctx, *g)
		return dispPaused, nil
	}

	owned := &ownedRun{
		cancel: func(cancelCtx context.Context) error {
			err := d.runs.Cancel(cancelCtx, runs.CancelCommand{
				RunID:  result.RunID,
				Reason: "autonomous goal stopped",
			})
			if errors.Is(err, runs.ErrRunNotFound) {
				return nil
			}
			return err
		},
	}
	if stopped, cancelErr := handle.bindRun(owned); stopped {
		return "", cancelErr
	}
	finished := drainTerminal(result.Events)
	handle.releaseRun(owned)
	if ctx.Err() != nil {
		return "", owned.stop(handle.owner)
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
	reread, ok, err := d.goals.Get(ctx, g.SessionID)
	if err != nil {
		span.RecordError(err)
		return "", nil
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
		d.clear(ctx, *g) // transient — announce (the model's reply) then clear
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
		g.Pause(goal.ReasonAwaitingInput, "", d.now())
		d.save(ctx, *g)
		return dispPaused, nil
	}

	outcome, err := outcomeOf(finished)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "malformed terminal run")
		g.Pause(goal.ReasonTerminalOutcomeMissing, "", d.now())
		d.save(ctx, *g)
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

// save / clear get one bounded persistence window even when ctx was canceled by
// Stop/shutdown. Both are compare-and-swap on the loop's version: a straggler
// whose goal was
// superseded (Stop/Start) or cleared (delete/rollback) simply does not apply —
// it can neither clobber a newer goal nor resurrect a deleted one. A store error
// (not a lost CAS) is recorded on the turn span; the boot reconcile is the
// backstop.
func (d *Driver) save(ctx context.Context, g goal.Goal) {
	expected := g.Version()
	saveCtx, cancel := goalPersistenceContext(ctx)
	_, _, err := d.goals.Save(saveCtx, g, expected)
	recordSaveError(saveCtx, err)
	cancel()
}

func (d *Driver) clear(ctx context.Context, g goal.Goal) {
	clearCtx, cancel := goalPersistenceContext(ctx)
	_, err := d.goals.ClearIf(clearCtx, g.SessionID, g.Version())
	recordSaveError(clearCtx, err)
	cancel()
}

func turnSteps(run *transcript.Run) int {
	if run.Result == nil {
		return 0
	}
	return run.Result.Steps
}

func goalPersistenceContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), goalPersistenceTimeout)
}

func (h *loopHandle) quiesce(ctx context.Context) {
	h.mu.Lock()
	h.stopping = true
	h.stopCtx = ctx
	h.loopReconcile = false
	run := h.run
	h.mu.Unlock()

	if run != nil {
		_ = run.stop(ctx)
	}
	h.cancel()
}

func (h *loopHandle) bindRun(run *ownedRun) (bool, error) {
	h.mu.Lock()
	h.run = run
	stopping := h.stopping
	stopCtx := h.stopCtx
	owner := h.owner
	h.mu.Unlock()
	if !stopping {
		return false, nil
	}
	err := run.stop(stopCtx)
	if err != nil {
		// The component owner performs the durable retry when the command
		// deadline expired. ownedRun retains earlier diagnostics, so this return
		// already contains both attempts without duplicating the first error.
		err = run.stop(owner)
	}
	return true, err
}

func (h *loopHandle) releaseRun(run *ownedRun) {
	h.mu.Lock()
	if h.run == run {
		h.run = nil
	}
	h.mu.Unlock()
}

func (h *loopHandle) mayReconcile() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.loopReconcile
}

func (h *loopHandle) returnReconcile() {
	h.mu.Lock()
	h.loopReconcile = true
	h.mu.Unlock()
}

func (r *ownedRun) stop(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stopped {
		return r.err
	}
	err := r.cancel(ctx)
	if err == nil || errors.Is(err, runs.ErrRunNotFound) {
		r.stopped = true
		return r.err
	}
	r.err = errors.Join(r.err, err)
	return r.err
}
