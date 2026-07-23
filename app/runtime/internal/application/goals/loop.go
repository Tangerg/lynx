package goals

import (
	"context"
	"errors"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/goal"
)

var errTerminalOutcomeMissing = errors.New("goals: terminal run has no outcome")

// autonomyNote frames every autonomous turn: the model drives itself and ends
// the loop only through update_goal, never by narrating completion.
const autonomyNote = "\n\n(You are running autonomously toward this goal — you do not need to wait for the user. Take one concrete next step. Call update_goal(status=\"complete\") only when the whole goal is done and verified, or update_goal(status=\"blocked\", reason=\"...\") if you genuinely cannot proceed without the user. Otherwise just keep working.)"

// launch spawns the loop for sessionID, canceling any prior loop for the same
// session first. The loop runs request-detached (task group) so it outlives the
// call that started it and is canceled by Stop or shutdown.
func (d *Driver) launch(parent context.Context, sessionID, leaseID string) {
	ctx, release, ok := d.tasks.Attach(parent)
	if !ok {
		return // shutting down
	}
	ctx, cancel := context.WithCancel(ctx)
	handle := &loopHandle{cancel: cancel}

	d.mu.Lock()
	if prior := d.running[sessionID]; prior != nil {
		prior.cancel()
	}
	d.running[sessionID] = handle
	d.mu.Unlock()

	go func() {
		defer release()
		defer d.forget(sessionID, handle)
		d.drive(ctx, sessionID, leaseID)
	}()
}

func (d *Driver) forget(sessionID string, handle *loopHandle) {
	d.mu.Lock()
	if d.running[sessionID] == handle {
		delete(d.running, sessionID)
	}
	d.mu.Unlock()
}

// drive runs autonomous turns until the goal leaves active. Cancellation (Stop /
// shutdown) leaves the goal's stored status untouched — Stop already paused it;
// a shutdown leaves it active so the boot reconcile degrades it to paused rather
// than resuming and burning budget.
func (d *Driver) drive(ctx context.Context, sessionID, leaseID string) {
	for {
		if ctx.Err() != nil {
			return
		}
		g, ok, err := d.goals.Get(ctx, sessionID)
		// Stop when the goal is gone or no longer active. The lease check is a
		// cheap backstop — a supersession (Stop/Start/Resume) is already caught above
		// by ctx cancellation or by the status leaving active — that guards a future
		// regression where a transition stops canceling the loop. The load-bearing
		// lease guard is the re-read in runTurn: it prevents adopting and
		// clobbering a foreign incarnation mid-turn.
		if err != nil || !ok || g.Status != goal.StatusActive || g.LeaseID != leaseID {
			return
		}
		if _, keepGoing := d.runTurn(ctx, &g); !keepGoing {
			return
		}
	}
}

// runTurn launches one autonomous run, waits for it to finish, folds its usage
// in, and decides what to do next — all under a goal.turn span. It returns the
// turn's disposition (empty when a cancellation or a vanished goal means no turn
// completed, so nothing is metered) and whether the loop should keep going.
func (d *Driver) runTurn(ctx context.Context, g *goal.Goal) (disposition string, keepGoing bool) {
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
		return "", false
	}
	result, err := d.runs.Start(ctx, d.command(*g))
	if err != nil {
		if ctx.Err() != nil {
			return "", false // Stop/shutdown — the state is handled by Stop / reconcile
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, "start run")
		g.Pause("could not start the next run: "+err.Error(), d.now())
		d.save(ctx, *g)
		return dispPaused, false
	}

	finished := drainTerminal(result.Events)

	// Re-read: the model may have set complete/blocked mid-turn via update_goal.
	reread, ok, err := d.goals.Get(ctx, g.SessionID)
	if err != nil {
		span.RecordError(err)
		return "", false
	}
	if !ok {
		return "", false
	}
	// If the lease changed, a Stop/Start/Resume superseded this loop's goal
	// while the run was in flight. Adopting the re-read (a different incarnation,
	// maybe a whole new objective) and saving to it would clobber a goal this
	// loop no longer owns; stop instead. This keeps g at the launch lease, so the
	// terminal saves below CAS on the incarnation the loop actually drove.
	if reread.LeaseID != g.LeaseID {
		return "", false
	}
	*g = reread
	switch g.Status {
	case goal.StatusComplete:
		d.clear(ctx, *g) // transient — announce (the model's reply) then clear
		return dispComplete, false
	case goal.StatusBlocked:
		return dispBlocked, false // the model declared blocked
	case goal.StatusPaused:
		return "", false // a concurrent Stop already recorded its intent
	}

	if finished == nil {
		// The run parked for HITL and produced no terminal (rare — autonomous runs
		// are headless, so an unanswerable interrupt auto-denies rather than
		// parking). Wait for the user, who resolves it and can resume the goal.
		g.Pause("the run is waiting for your input", d.now())
		d.save(ctx, *g)
		return dispPaused, false
	}

	cost, steps := turnUsage(finished)
	outcome, err := outcomeOf(finished)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "malformed terminal run")
		g.Pause("the run ended without a terminal outcome", d.now())
		d.save(ctx, *g)
		return dispPaused, false
	}
	span.SetAttributes(
		attribute.String("run.outcome", outcome.String()),
		attribute.Float64("goal.cost_usd", cost),
		attribute.Int("goal.steps", steps),
	)
	g.AddTurn(cost, steps, d.now())

	if outcome != execution.OutcomeCompleted {
		g.Pause("the run ended ("+outcome.String()+")", d.now())
		d.save(ctx, *g)
		return dispPaused, false
	}
	if axis, over := g.Budget.Exceeded(g.Used); over {
		g.Block("reached the "+axis+" budget", d.now())
		d.save(ctx, *g)
		return dispBlocked, false
	}
	if !d.checkpoint(ctx, *g) {
		// The goal was superseded or cleared out from under this loop (a
		// Stop/Start or a session delete/rollback revoked the lease); stop
		// rather than drive a goal we no longer own.
		return "", false
	}
	return dispContinue, true
}

// command builds the next autonomous run. It is headless: no InterruptKinds, so a
// tool that would need approval is auto-denied by the run rather than parking a
// loop no client is watching (the user's chosen global approval stance still
// gates tools — yolo runs everything, a stricter stance keeps the agent read-only).
func (d *Driver) command(g goal.Goal) runs.StartCommand {
	message := d.prompt(g)
	return runs.StartCommand{
		SessionID: g.SessionID,
		Provider:  g.Provider,
		Model:     g.Model,
		Input:     []runs.ContentBlock{{Kind: runs.TextContent, Text: message}},
		// GoalLeaseID stamps the run with the incarnation that launched it, so
		// update_goal only signals THIS goal: a straggler run from a superseded
		// goal (stopped, then replaced by a fresh Start) cannot mark the new goal
		// complete/blocked — its lease no longer matches.
		GoalLeaseID: g.LeaseID,
	}
}

func (d *Driver) prompt(g goal.Goal) string {
	if g.Used.Turns == 0 {
		return g.Objective + autonomyNote
	}
	return "Continue toward the goal: " + g.Objective + autonomyNote
}

// drainTerminal consumes a run's event stream to its close and returns the run's
// terminal record, or nil when the stream closed without one (the run parked).
func drainTerminal(events <-chan runs.Event) *transcript.Run {
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

func turnUsage(run *transcript.Run) (costUSD float64, steps int) {
	if run.Result == nil {
		return 0, 0
	}
	if run.Result.Usage != nil && run.Result.Usage.CostUSD != nil {
		costUSD = *run.Result.Usage.CostUSD
	}
	return costUSD, run.Result.Steps
}

// save / clear persist the loop's TERMINAL state even when ctx was canceled by
// Stop/shutdown (detached drops cancellation but keeps trace values). Both are
// compare-and-swap on the loop's version: a straggler whose goal was
// superseded (Stop/Start) or cleared (delete/rollback) simply does not apply —
// it can neither clobber a newer goal nor resurrect a deleted one. A store error
// (not a lost CAS) is recorded on the turn span; the boot reconcile is the
// backstop.
func (d *Driver) save(ctx context.Context, g goal.Goal) {
	expected := g.Version()
	g.AdvanceRevision()
	_, err := d.goals.Save(detached(ctx), g, expected)
	recordSaveError(ctx, err)
}

func (d *Driver) clear(ctx context.Context, g goal.Goal) {
	_, err := d.goals.ClearIf(detached(ctx), g.SessionID, g.Version())
	recordSaveError(ctx, err)
}

// checkpoint persists mid-loop usage progress and reports whether the loop still
// owns the goal. It HONORS ctx and CAS-guards on the loop's version: a
// concurrent Stop/Start (lease renewal) or session delete (goal cleared) makes
// the CAS not apply, and the caller stops driving. A ctx cancellation here is the
// expected Stop path (the loop is being torn down), not an error worth recording.
func (d *Driver) checkpoint(ctx context.Context, g goal.Goal) (owned bool) {
	expected := g.Version()
	g.AdvanceRevision()
	applied, err := d.goals.Save(ctx, g, expected)
	if err != nil {
		if ctx.Err() == nil {
			recordSaveError(ctx, err)
		}
		return false
	}
	return applied
}

func detached(ctx context.Context) context.Context { return context.WithoutCancel(ctx) }
