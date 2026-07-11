package runs

import (
	"context"
	"iter"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
)

// pump is the run segment goroutine: it leads with the projector's open events,
// projects each executor event, and — on a terminal or a drained stream — tears
// the run down. It preserves the run stream's ordering exactly: live publication
// generally precedes the durable write (a slow store can't delay the terminal
// reaching subscribers or the teardown), EXCEPT an interrupt, whose durable
// record is committed before its event is published (a client may resume the
// instant it sees the interrupt). A parked run leaves its live turn alive for
// resume; a true terminal cancels it.
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
				committed, err := live.commitInterrupt(func() error {
					if err := c.effects.BeforeLive(ownerCtx, pe.Effect); err != nil {
						return err
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
				c.effects.AfterLive(ownerCtx, pe.Effect)
				continue
			}
			if pe.Terminal {
				finished = true
			}
			// Live first: deliver + retain in the replay backlog before the
			// durable write, so a slow store can't delay the terminal.
			hub.Append(ev)
			c.effects.AfterLive(ownerCtx, pe.Effect)
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
			// balanced. The projector decides error-vs-canceled from its state.
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
		// Release the durable admission slot (§8.2) on a true terminal; a parked
		// run stays non-terminal in the runs table (it is resumable), matching the
		// live turn left alive above. Best-effort: a missed write leaves a running
		// row the next boot's ReconcileOrphans sweeps.
		if !parked && c.runStore != nil {
			_ = c.runStore.Terminalize(ownerCtx, spec.SessionID, pumpOutcome(ctx, abortTurn))
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

// pumpOutcome is the coarse terminal reason the durable run row records at
// teardown: canceled when the run's context was canceled, error when a
// projection/commit aborted the turn, else completed. It is deliberately coarse
// — the precise [execution.Outcome] (maxBudget / maxSteps) rides the terminal
// event's opaque payload; a later atomic EventCommit (§8.1) records it exactly.
func pumpOutcome(runCtx context.Context, aborted bool) string {
	switch {
	case runCtx.Err() != nil:
		return execution.OutcomeCanceled.String()
	case aborted:
		return execution.OutcomeError.String()
	default:
		return execution.OutcomeCompleted.String()
	}
}
