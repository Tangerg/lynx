package runs

import (
	"context"
	"iter"
	"time"
)

// pump is the run segment goroutine: it leads with the projector's open events,
// projects each executor event, and — on a terminal or a drained stream — tears
// the run down. It preserves the run stream's ordering exactly: live publication
// generally precedes the durable commit (a slow store can't delay the terminal
// reaching subscribers or the teardown), EXCEPT an interrupt, whose durable
// record is committed before its event is published (a client may resume the
// instant it sees the interrupt). Each event's durable commit is ATOMIC: its
// projections and the run-state transition it implies (park → interrupted,
// terminal) land in one transaction (§8.3), so a crash never leaves a parked run
// with no admission mark or a terminal transcript with a running row. A parked
// run leaves its live turn alive for resume; a true terminal cancels it.
func (c *Coordinator) pump(ctx, ownerCtx context.Context, spec StartSpec, inner iter.Seq[EngineEvent], live *handle, projector Projector) {
	hub := live.hub
	finished := false
	parked := false
	abortTurn := false

	// publish stamps each projected event with a cursor and drives
	// commit-before-publish. It returns false to stop the pump: an aborted
	// projection, or a cancel that won the interrupt-commit race.
	publish := func(pes []ProjectedEvent) bool {
		for _, pe := range pes {
			if pe.Abort {
				abortTurn = true
				return false
			}
			ev := Event{
				RunID:     spec.RunID,
				Seq:       c.minter.Mint(),
				Timestamp: time.Now().UTC(),
				IsDurable: pe.Durable,
				IsTerm:    pe.Terminal,
				Payload:   pe.Payload,
			}
			if pe.Interrupt {
				// Park: the atomic commit ({open interrupt + suspend the run-state},
				// §8.3) must land before the event is published, so a client can't
				// resume ahead of the durable record. Linearized vs cancel: a cancel
				// that won the race skips the commit (committed=false).
				committed, err := live.commitInterrupt(func() error {
					if pe.Commit != nil {
						if err := c.effects.CommitEvent(ownerCtx, *pe.Commit); err != nil {
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
						projector.Abort(err.Error())
					}
					abortTurn = true
					return false
				}
				if !committed {
					return false
				}
				continue
			}
			if pe.Terminal {
				finished = true
			}
			// Live first: deliver + retain in the replay backlog before the durable
			// commit, so a slow store can't delay the terminal. The commit (which for
			// a terminal atomically records the run + terminalizes the run-state) is
			// best-effort — a lost projection self-heals, a stranded admission row is
			// swept by the next boot's ReconcileOrphans.
			hub.Append(ev)
			if pe.Commit != nil {
				_ = c.effects.CommitEvent(ownerCtx, *pe.Commit)
			}
			if pe.Nudge != nil {
				c.effects.Nudge(pe.Nudge.Cwd, pe.Nudge.Paths)
			}
		}
		return true
	}

	// run.started leads every segment, before the teardown defer is armed — a
	// failure here (which the run.started class can't actually trigger) means no
	// teardown, matching the pre-rewrite pump.
	if !publish(projector.Open()) {
		return
	}

	defer func() {
		if ctx.Err() != nil || abortTurn {
			_ = c.executor.CancelTurn(ownerCtx, spec.Handle)
		}
		if !finished {
			// The stream ended without a run.finished (canceled mid-flight /
			// drained iterator) — synthesize the terminal so the stream ends
			// balanced. The projector decides error-vs-canceled from its state, and
			// the synthesized terminal's commit terminalizes the run-state, so no
			// separate teardown state write is needed.
			_ = publish(projector.SynthesizeTerminal())
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
		if !publish(projector.Translate(ev)) {
			return
		}
		if parked {
			// Interrupt segment done; leave the turn parked for resume.
			return
		}
	}
}
