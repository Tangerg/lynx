package runs

import (
	"context"
	"fmt"
	"iter"
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
	finished := false
	parked := false
	abortTurn := false
	commitCtx := ownerCtx

	// publish validates the whole batch before side effects, then drives
	// commit-before-publish. published=false with no error means cancellation won
	// the interrupt-commit race.
	publish := func(reductions []reduction) (published bool, err error) {
		if err := validateReductionBatch(reductions); err != nil {
			return false, err
		}
		if len(reductions) > 0 && reductions[0].Interrupt {
			// Park is a batch boundary, not one event: commit every transcript
			// projection + the open interrupt + Suspend, then publish the complete
			// batch under one reserved boundary. A cancellation therefore observes
			// either no park or the complete park and cancels + joins an in-flight
			// durable commit without waiting on a mutex held across I/O.
			committed, err := live.commitInterrupt(commitCtx, func(interruptCtx context.Context) error {
				if err := c.effects.CommitEvent(interruptCtx, *reductions[0].Commit); err != nil {
					return err
				}
				finished = true
				parked = true
				for _, reduced := range reductions {
					hub.Append(c.event(spec, reduced))
					if reduced.Nudge != nil {
						c.effects.Nudge(reduced.Nudge.Cwd, reduced.Nudge.Paths)
					}
				}
				return nil
			})
			if err != nil {
				return false, fmt.Errorf("runs: commit interrupt: %w", err)
			}
			return committed, nil
		}
		for _, reduced := range reductions {
			// Commit before publish: a durable event's atomic commit (for a terminal,
			// recording the run + terminalizing the run-state) lands before the event
			// is delivered or retained for replay, so a subscriber never observes an
			// event the store doesn't yet back. A commit failure aborts the turn (as
			// the interrupt path does) rather than publishing an unbacked event.
			if reduced.Commit != nil {
				if err := c.effects.CommitEvent(commitCtx, *reduced.Commit); err != nil {
					return false, fmt.Errorf("runs: commit %T: %w", reduced.Event, err)
				}
			}
			if reduced.Event.Terminal() {
				finished = true
			}
			hub.Append(c.event(spec, reduced))
			if reduced.Nudge != nil {
				c.effects.Nudge(reduced.Nudge.Cwd, reduced.Nudge.Paths)
			}
		}
		return true, nil
	}

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
			} else if _, err := publish(reductions); err != nil {
				fail(err)
			}
			cancelTerminal()
		}
		if ctx.Err() != nil || abortTurn {
			teardownCtx, cancelTeardown := context.WithTimeout(context.WithoutCancel(ownerCtx), runCleanupTimeout)
			_ = c.executor.CancelTurn(teardownCtx, spec.Handle)
			cancelTeardown()
		}
		hub.Close()
		if e, ok := c.registry.Close(spec.RunID); ok {
			// A parked run keeps its live turn alive for resume — only cancel +
			// forget on a true terminal.
			if !parked && e.Payload != nil {
				e.Payload.stop()
			}
		}
		// Maintenance may only observe a boundary the store actually committed.
		// In particular, a failed terminal commit must not create a checkpoint or
		// title that falsely implies the run completed. A parked commit sets
		// finished too; Effects deliberately treats it as non-terminal.
		if finished {
			finishCtx, cancelFinish := context.WithTimeout(context.WithoutCancel(ownerCtx), runCleanupTimeout)
			c.effects.Finish(finishCtx, Finish{
				SessionID:       spec.SessionID,
				RunID:           spec.RunID,
				Cwd:             spec.Cwd,
				Parked:          parked,
				OpeningUserText: spec.OpeningUserText,
			})
			cancelFinish()
		}
	}()

	for ev := range inner {
		reductions, err := reducer.reduce(ev)
		if err != nil {
			fail(err)
			return
		}
		published, err := publish(reductions)
		if err != nil {
			fail(err)
			return
		}
		if !published {
			return
		}
		if parked {
			// Interrupt segment done; leave the turn parked for resume.
			return
		}
	}
}
