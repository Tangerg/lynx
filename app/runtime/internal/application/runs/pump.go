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

	// publish stamps each reduced event with a cursor and drives
	// commit-before-publish. It returns false to stop the pump: an aborted
	// projection, or a cancel that won the interrupt-commit race.
	publish := func(reductions []reduction) bool {
		for _, reduced := range reductions {
			if reduced.Abort {
				abortTurn = true
				return false
			}
			ev := c.event(spec, reduced)
			if reduced.Interrupt {
				// Park: the atomic commit ({open interrupt + suspend the run-state},
				// §8.3) must land before the event is published, so a client can't
				// resume ahead of the durable record. Linearized vs cancel: a cancel
				// that won the race skips the commit (committed=false).
				committed, err := live.commitInterrupt(func() error {
					if reduced.Commit != nil {
						if err := c.effects.CommitEvent(ownerCtx, *reduced.Commit); err != nil {
							return err
						}
					}
					finished = true
					parked = true
					hub.Append(ev)
					return nil
				})
				if err != nil {
					if ctx.Err() == nil && ownerCtx.Err() == nil {
						reducer.abort(err.Error())
					}
					abortTurn = true
					return false
				}
				if !committed {
					return false
				}
				continue
			}
			// Commit before publish: a durable event's atomic commit (for a terminal,
			// recording the run + terminalizing the run-state) lands before the event
			// is delivered or retained for replay, so a subscriber never observes an
			// event the store doesn't yet back. A commit failure aborts the turn (as
			// the interrupt path does) rather than publishing an unbacked event.
			if reduced.Commit != nil {
				if err := c.effects.CommitEvent(ownerCtx, *reduced.Commit); err != nil {
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
		if ctx.Err() != nil || abortTurn {
			_ = c.executor.CancelTurn(ownerCtx, spec.Handle)
		}
		if !finished {
			// The stream ended without a segment.finished (canceled mid-flight /
			// drained iterator, or a failed continuation activation) — synthesize the terminal
			// so the stream ends balanced. The reducer decides error-vs-canceled
			// from its state, and the synthesized terminal's commit terminalizes the
			// run-state, so no separate teardown state write is needed.
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
		// Terminal boundary maintenance stays off the critical path (async /
		// best-effort inside Effects). A parked run is resumable, not a boundary.
		c.effects.Finish(ownerCtx, Finish{
			SessionID:       spec.SessionID,
			RunID:           spec.RunID,
			Parked:          parked,
			OpeningUserText: spec.OpeningUserText,
		})
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
