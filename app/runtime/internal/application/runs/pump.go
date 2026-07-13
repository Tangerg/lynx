package runs

import (
	"context"
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

	// publish stamps each reduced event with a cursor and drives
	// commit-before-publish. It returns false to stop the pump: an aborted
	// projection, or a cancel that won the interrupt-commit race.
	publish := func(reductions []reduction) bool {
		if len(reductions) > 0 && reductions[0].Interrupt {
			// Park is a batch boundary, not one event: commit every transcript
			// projection + the open interrupt + Suspend, then publish the complete
			// batch while requestCancel is excluded by the same handle lock. A
			// cancellation therefore observes either no park or the complete park;
			// it can never interleave between interrupt items and segment.finished.
			if reductions[0].Commit == nil {
				panic("runs: interrupt batch has no durable commit")
			}
			committed, err := live.commitInterrupt(func() error {
				if err := c.effects.CommitEvent(commitCtx, *reductions[0].Commit); err != nil {
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
				if ctx.Err() == nil && ownerCtx.Err() == nil {
					reducer.abort(err.Error())
				}
				abortTurn = true
				return false
			}
			return committed
		}
		for _, reduced := range reductions {
			if reduced.Abort {
				abortTurn = true
				return false
			}
			ev := c.event(spec, reduced)
			if reduced.Interrupt {
				panic("runs: interrupt batch marker must be first")
			}
			// Commit before publish: a durable event's atomic commit (for a terminal,
			// recording the run + terminalizing the run-state) lands before the event
			// is delivered or retained for replay, so a subscriber never observes an
			// event the store doesn't yet back. A commit failure aborts the turn (as
			// the interrupt path does) rather than publishing an unbacked event.
			if reduced.Commit != nil {
				if err := c.effects.CommitEvent(commitCtx, *reduced.Commit); err != nil {
					if ctx.Err() == nil && ownerCtx.Err() == nil {
						reducer.abort(err.Error())
					}
					abortTurn = true
					return false
				}
			}
			if reduced.Event.Terminal() {
				finished = true
			}
			hub.Append(ev)
			if reduced.Nudge != nil {
				c.effects.Nudge(reduced.Nudge.Cwd, reduced.Nudge.Paths)
			}
		}
		return true
	}

	defer func() {
		// Coordinator.Close cancels ownerCtx before joining this pump. Terminal
		// synthesis is a durable cleanup boundary, so it must outlive that signal
		// while remaining bounded; otherwise a graceful shutdown itself leaves a
		// Running transcript/admission row for boot recovery to repair.
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ownerCtx), runCleanupTimeout)
		defer cancel()
		if ctx.Err() != nil || abortTurn {
			_ = c.executor.CancelTurn(cleanupCtx, spec.Handle)
		}
		if !finished {
			// The stream ended without a segment.finished (canceled mid-flight /
			// drained iterator, or a failed continuation activation) — synthesize the terminal
			// so the stream ends balanced. The reducer decides error-vs-canceled
			// from its state, and the synthesized terminal's commit terminalizes the
			// run-state, so no separate teardown state write is needed.
			commitCtx = cleanupCtx
			_ = publish(reducer.synthesizeTerminal())
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
			c.effects.Finish(cleanupCtx, Finish{
				SessionID:       spec.SessionID,
				RunID:           spec.RunID,
				Cwd:             spec.Cwd,
				Parked:          parked,
				OpeningUserText: spec.OpeningUserText,
			})
		}
	}()

	for ev := range inner {
		if !publish(reducer.reduce(ev)) {
			return
		}
		if parked {
			// Interrupt segment done; leave the turn parked for resume.
			return
		}
	}
}
