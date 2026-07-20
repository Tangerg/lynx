package goals

import (
	"context"
	"errors"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/goal"
)

const (
	// busyBackoff / maxBusyRetries bound the wait for the previous run's pump to
	// finish tearing down and free the session's single admission slot. The pump
	// closes the event stream before it releases the slot, so the loop's drain can
	// return a few milliseconds before a back-to-back launch is admissible. This is
	// waiting for a local resource, keyed on the ErrSessionBusy sentinel — not a
	// transient-error retry layer.
	busyBackoff    = 100 * time.Millisecond
	maxBusyRetries = 20
)

// autonomyNote frames every autonomous turn: the model drives itself and ends
// the loop only through update_goal, never by narrating completion.
const autonomyNote = "\n\n(You are running autonomously toward this goal — you do not need to wait for the user. Take one concrete next step. Call update_goal(status=\"complete\") only when the whole goal is done and verified, or update_goal(status=\"blocked\", reason=\"...\") if you genuinely cannot proceed without the user. Otherwise just keep working.)"

// launch spawns the loop for sessionID, canceling any prior loop for the same
// session first. The loop runs request-detached (task group) so it outlives the
// call that started it and is canceled by Stop or shutdown.
func (d *Driver) launch(parent context.Context, sessionID string) {
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
		d.drive(ctx, sessionID)
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
func (d *Driver) drive(ctx context.Context, sessionID string) {
	for {
		if ctx.Err() != nil {
			return
		}
		g, ok, err := d.goals.Get(ctx, sessionID)
		if err != nil || !ok || g.Status != goal.StatusActive {
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

	result, err := d.startNext(ctx, d.command(*g))
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
	*g = reread
	switch g.Status {
	case goal.StatusComplete:
		d.clear(ctx, g.SessionID) // transient — announce (the model's reply) then clear
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
	outcome := outcomeOf(finished)
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
	d.checkpoint(ctx, *g)
	return dispContinue, true
}

// startNext launches the next run, waiting out the brief window in which the
// previous run's pump is still releasing the session's admission slot.
func (d *Driver) startNext(ctx context.Context, cmd runs.StartCommand) (runs.StartResult, error) {
	if err := ctx.Err(); err != nil {
		// Stop/shutdown landed after the loop's top check: never launch a run the
		// cancellation would immediately abandon.
		return runs.StartResult{}, err
	}
	for retry := 0; ; retry++ {
		result, err := d.runs.Start(ctx, cmd)
		if !errors.Is(err, runs.ErrSessionBusy) || retry >= maxBusyRetries {
			return result, err
		}
		select {
		case <-ctx.Done():
			return runs.StartResult{}, ctx.Err()
		case <-time.After(busyBackoff):
		}
	}
}

// command builds the next autonomous run. It is headless: no InterruptKinds, so a
// tool that would need approval is auto-denied by the run rather than parking a
// loop no client is watching (the user's chosen global approval stance still
// gates tools — yolo runs everything, a stricter stance keeps the agent read-only).
func (d *Driver) command(g goal.Goal) runs.StartCommand {
	message := d.prompt(g)
	return runs.StartCommand{
		SessionID:       g.SessionID,
		Message:         message,
		Provider:        g.Provider,
		Model:           g.Model,
		OpeningUserText: message,
		Input:           []runs.ContentBlock{{Kind: runs.TextContent, Text: message}},
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

// outcomeOf reads a terminal run's outcome. A SegmentFinished always carries a
// non-nil outcome; the nil default is defensive only, and resolves to Completed
// so a malformed terminal lets the loop continue (and hit a real stop — budget
// or a model signal) rather than silently pausing on absent data.
func outcomeOf(run *transcript.Run) execution.Outcome {
	if run.Outcome == nil {
		return execution.OutcomeCompleted
	}
	return *run.Outcome
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
// Stop/shutdown (detached drops cancellation but keeps trace values). Best-effort:
// a failed write leaves the goal for the boot reconcile to degrade, so it can
// never corrupt into a resumed-but-lost loop — but the failure is recorded on the
// turn span rather than dropped.
func (d *Driver) save(ctx context.Context, g goal.Goal) {
	recordSaveError(ctx, d.goals.Save(detached(ctx), g))
}

func (d *Driver) clear(ctx context.Context, sessionID string) {
	recordSaveError(ctx, d.goals.Clear(detached(ctx), sessionID))
}

// checkpoint persists mid-loop usage progress. Unlike the terminal saves it
// HONORS ctx: it re-affirms status=active, so if it detached it could commit
// after a concurrent Stop paused the goal and clobber the pause — wedging the
// goal active with no loop driving it. A Stop cancels this ctx before it writes
// paused, so the checkpoint is skipped (losing at most this turn's usage
// accounting) and the store converges on paused; a cancellation here is that
// expected path, not an error worth recording.
func (d *Driver) checkpoint(ctx context.Context, g goal.Goal) {
	if err := d.goals.Save(ctx, g); err != nil && ctx.Err() == nil {
		recordSaveError(ctx, err)
	}
}

func detached(ctx context.Context) context.Context { return context.WithoutCancel(ctx) }
