package runs

import (
	"context"
	"errors"
	"fmt"
	"iter"

	"go.opentelemetry.io/otel/trace"
)

// pump is the run segment goroutine: Start has already atomically committed and
// published the reducer's opening events; the pump reduces each executor
// event and — on a terminal or a drained stream — tears the run down. It commits
// before it publishes (§7.2): every durable event's
// ATOMIC commit — its projections plus the run-state transition it implies (park
// → interrupted, terminal → terminalized; §8.3) — lands in one transaction
// BEFORE the event reaches subscribers, so a client that acts on an event (reads
// the transcript after a terminal, resumes the instant it sees an interrupt)
// never observes state the store does not yet hold, and a terminal frees the
// session's durable admission slot before it frees the in-memory one. A commit
// failure aborts the turn rather than publishing an event the durable record
// can't back. The interrupt commit additionally linearizes against cancel (a
// cancel that wins the race skips the commit). Non-durable deltas publish
// directly. A parked run leaves its live turn alive for resume; a true terminal
// cancels it.
func (c *Coordinator) pump(ctx, ownerCtx context.Context, spec segmentSpec, inner iter.Seq[EngineEvent], live *handle, reducer *reducer) {
	hub := live.hub
	publisher := segmentPublisher{coordinator: c, spec: spec, live: live}
	finished := false
	parked := false
	abortTurn := false
	commitCtx := ownerCtx

	fail := func(err error) {
		abortTurn = true
		if ctx.Err() == nil && ownerCtx.Err() == nil {
			reducer.abort(err)
		}
	}

	defer func() {
		// Coordinator.Close cancels ownerCtx before joining this pump. Terminal
		// synthesis is a durable cleanup boundary, so it must outlive that signal
		// while remaining bounded; otherwise a graceful shutdown itself leaves a
		// Running transcript/admission row for boot recovery to repair.
		if !finished {
			// The stream ended without a segment.finished (canceled mid-flight /
			// drained iterator, or a failed continuation activation) — synthesize the terminal
			// so the stream ends balanced. The reducer decides error-vs-canceled
			// from its state, and the synthesized terminal's commit terminalizes the
			// run-state, so no separate teardown state write is needed. This commit
			// happens before executor teardown: a slow or broken CancelTurn must never
			// consume the only budget available for the durable terminal boundary.
			terminalCtx, cancelTerminal := context.WithTimeout(context.WithoutCancel(ownerCtx), runCleanupTimeout)
			commitCtx = terminalCtx
			reductions, err := reducer.synthesizeTerminal()
			if err != nil {
				fail(err)
			} else {
				publication, err := publisher.publish(commitCtx, reductions)
				if err != nil {
					fail(err)
				} else {
					finished = publication.finished
					parked = publication.parked
				}
			}
			cancelTerminal()
		}
		if ctx.Err() != nil || abortTurn {
			teardownCtx, cancelTeardown := context.WithTimeout(context.WithoutCancel(ownerCtx), runCleanupTimeout)
			if err := c.executor.CancelTurn(teardownCtx, spec.turnRef()); err != nil && !errors.Is(err, ErrTurnNotLive) {
				recordRunCleanupError(teardownCtx, fmt.Errorf("runs: tear down turn %q: %w", spec.TurnID, err))
			}
			cancelTeardown()
		}
		releaseMaintenance, maintenanceHeld := c.admission.BeginMaintenance(spec.RunID)
		entry, tracked := c.registry.Remove(spec.RunID)
		if tracked {
			// A parked run keeps its live turn alive for resume — only cancel +
			// forget on a true terminal.
			if !parked && entry.handle != nil {
				entry.handle.stop()
			}
		}
		// Maintenance may only observe a boundary the store actually committed.
		// In particular, a failed terminal commit must not create a checkpoint or
		// title that falsely implies the run completed. The session claim above is
		// retained through the synchronous checkpoint fence; title work may detach.
		// A parked commit sets finished too; Effects treats it as non-terminal.
		if finished {
			finishCtx, cancelFinish := context.WithTimeout(context.WithoutCancel(ownerCtx), runCleanupTimeout)
			if err := c.effects.Finish(finishCtx, Finish{
				SessionID:       spec.SessionID,
				RunID:           spec.RunID,
				Cwd:             spec.Cwd,
				Parked:          parked,
				OpeningUserText: spec.OpeningUserText,
			}); err != nil {
				recordRunCleanupError(finishCtx, err)
			}
			cancelFinish()
		}
		if maintenanceHeld {
			releaseMaintenance()
		}
		// Journal closure is the externally observable completion boundary. A
		// consumer that drains it may immediately admit the next segment, so the
		// synchronous maintenance fence and its admission claim must be gone first.
		hub.Close()
	}()

	for ev := range inner {
		reductions, err := reducer.reduce(ev)
		if err != nil {
			fail(err)
			return
		}
		publication, err := publisher.publish(commitCtx, reductions)
		if err != nil {
			fail(err)
			return
		}
		finished = finished || publication.finished
		parked = parked || publication.parked
		if !publication.published {
			return
		}
		if parked {
			// Interrupt segment done; leave the turn parked for resume.
			return
		}
		if finished {
			// A committed terminal is the last event this run can durably back: stop
			// before consuming any further buffered event. A cancel that races a turn
			// in the act of parking can emit a TurnInterrupted after the terminal
			// TurnEnd; processing it would try to Suspend an already-terminalized run.
			// The pump owns durable run-state integrity, so it enforces "nothing after
			// a terminal" here rather than trusting the upstream event ordering.
			return
		}
	}
}

type reductionPublication struct {
	published bool
	finished  bool
	parked    bool
}

// segmentPublisher owns the one batch boundary between canonical reductions
// and their durable/live projections. It returns the lifecycle state the pump
// must carry into the next executor event and final cleanup.
type segmentPublisher struct {
	coordinator *Coordinator
	spec        segmentSpec
	live        *handle
}

// publish validates a complete batch before any side effect, then commits every
// durable fact before appending its event. published=false without an error
// means cancellation won the interrupt-commit race.
func (p segmentPublisher) publish(ctx context.Context, reductions []reduction) (reductionPublication, error) {
	if err := validateReductionBatch(reductions); err != nil {
		return reductionPublication{}, err
	}
	if len(reductions) > 0 && reductions[0].Interrupt {
		return p.publishInterrupt(ctx, reductions)
	}
	publication := reductionPublication{published: true}
	for _, reduced := range reductions {
		// Commit before publish: a durable event's atomic commit (for a terminal,
		// recording the run + terminalizing the run-state) lands before the event
		// is delivered or retained for replay, so a subscriber never observes an
		// event the store doesn't yet back. A commit failure aborts the turn (as
		// the interrupt path does) rather than publishing an unbacked event.
		if reduced.Commit != nil {
			if err := p.coordinator.effects.CommitEvent(ctx, *reduced.Commit); err != nil {
				return reductionPublication{}, fmt.Errorf("runs: commit %T: %w", reduced.Event, err)
			}
		}
		if reduced.Event.Terminal() {
			publication.finished = true
		}
		p.append(reduced)
	}
	return publication, nil
}

func (p segmentPublisher) publishInterrupt(ctx context.Context, reductions []reduction) (reductionPublication, error) {
	// Park is a batch boundary, not one event: commit every transcript
	// projection + the open interrupt + Suspend, then publish the complete
	// batch under one reserved boundary. A cancellation therefore observes
	// either no park or the complete park and cancels + joins an in-flight
	// durable commit without waiting on a mutex held across I/O.
	committed, err := p.live.commitInterrupt(ctx, func(interruptCtx context.Context) error {
		if err := p.coordinator.effects.CommitEvent(interruptCtx, *reductions[0].Commit); err != nil {
			return err
		}
		for _, reduced := range reductions {
			p.append(reduced)
		}
		return nil
	})
	if err != nil {
		return reductionPublication{}, fmt.Errorf("runs: commit interrupt: %w", err)
	}
	return reductionPublication{published: committed, finished: committed, parked: committed}, nil
}

func (p segmentPublisher) append(reduced reduction) {
	p.live.hub.Append(p.coordinator.event(p.spec, reduced))
	if reduced.Nudge != nil {
		p.coordinator.effects.Nudge(reduced.Nudge.Cwd, reduced.Nudge.Paths)
	}
}

func recordRunCleanupError(ctx context.Context, err error) {
	if err != nil {
		trace.SpanFromContext(ctx).RecordError(err)
	}
}
