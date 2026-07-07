package turn

import (
	"context"
	"errors"
	"time"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/hooks"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel"
)

// endTurn closes the turn's event channel and removes it from the live
// registry — subsequent Events / Cancel / Resume return ErrTurnNotFound.
// It also ends the turn span: the single teardown point, so the span
// closes exactly once no matter which terminal path (drive / finishTurn)
// reached it. finishTurnSpan has already stamped the outcome.
func (s *inMemory) endTurn(st *turnState) {
	// Release the backing process now that the turn is terminal: free its
	// in-memory registry entry and delete its persisted auto-snapshot, which
	// only matters while a process is PARKED for HITL resume (endTurn never runs
	// on a parked turn — handleWaiting leaves it registered). Without this every
	// run leaks one process_snapshot row. Off a cancel-decoupled ctx so the
	// delete lands even when the turn ctx was canceled, keeping the trace span.
	if p := st.process(); p != nil {
		p.Discard(context.WithoutCancel(st.ctx))
	}
	if st.span != nil {
		st.span.End()
	}
	// close(st.events) is safe only because every emit happens-before it: the
	// run-loop emitters (the tool observer, steerSource) all complete before
	// proc.Done() fires, and drive reads Done() before reaching endTurn; the
	// teardown emitters (Cancel / finishTurn) run on this same goroutine,
	// serialized against a racing Resume by claimPark. The lifecycle listener
	// runs on a DETACHED agent goroutine, so it must only CAPTURE the terminal
	// event, never emit — an emit from any goroutine lacking that happens-before
	// would race this close and panic (send on a closed channel).
	close(st.events)
	s.mu.Lock()
	delete(s.turns, st.handle.TurnID)
	s.mu.Unlock()
}

// finishTurn emits the terminal [TurnEnd] (stamping the elapsed duration)
// and tears the turn down. It serves the emergency-teardown paths —
// [Dispatcher.Cancel] and a failed [Dispatcher.Resume] — where no drive
// goroutine will run [emitTurnEnd]. The clean path goes through
// emitTurnEnd (which carries usage) followed by endTurn in [drive].
func (s *inMemory) finishTurn(st *turnState, reason TurnEndReason) {
	dur := time.Since(st.startedAt)
	finishTurnSpan(st.span, reason, TokenUsage{}, false, "")
	recordTurnDuration(st.ctx, reason, st.model, dur)
	s.emit(st, TurnEnd{Reason: reason, Duration: dur})
	s.endTurn(st)
}

// emitTurnEnd maps the captured agent runtime terminal event onto a
// transport-shape TurnEnd. The lifecycle listener fires terminal
// events authoritatively (ProcessKilled / ProcessFailed /
// ProcessStuck / ProcessTerminated / ProcessCompleted), so we read
// those rather than re-deriving status from the run loop's error.
// The runErr / ctxErr / status fallback covers stub tests where no
// listener fired and any race where Done() returned before the
// platform multicast delivered the terminal event.
func (s *inMemory) emitTurnEnd(st *turnState, proc kernel.TurnProcess, terminal event.Event, runErr error, duration time.Duration, ctxErr error) {
	out, _ := proc.Output()
	plan := planTurnEnd(terminal, out, runErr, ctxErr, proc.Status())

	finishTurnSpan(st.span, plan.reason, out.Usage, plan.withUsage, plan.errMsg)
	recordTurnDuration(st.ctx, plan.reason, st.model, duration)
	if plan.errMsg != "" {
		s.emit(st, ErrorEvent{Message: plan.errMsg, Code: plan.errCode})
	}
	end := TurnEnd{Reason: plan.reason, Duration: duration, MaxBudget: st.maxBudget, MaxCostUSD: st.maxCostUSD, MaxSteps: st.maxSteps}
	if plan.withUsage {
		end.TokenUsage = out.Usage
		end.UsageByModel = out.UsageByModel
		end.CostUSD = out.CostUSD
	}
	s.emit(st, end)
	// Stop hooks (observe-only): fire after the terminal is emitted (the client
	// already saw run.finished) — for notify / chain / cleanup. Bounded by the
	// hook timeout; it precedes only the turn's teardown, not the client signal.
	s.fireStop(st, plan.errMsg)
}

// fireStop runs the Stop lifecycle hooks for a terminated turn (observe-only).
func (s *inMemory) fireStop(st *turnState, detail string) {
	if st.hooks.Empty() {
		return
	}
	_ = st.hooks.Run(st.ctx, hooks.Input{
		Event: hooks.Stop, SessionID: st.handle.SessionID, Cwd: st.cwd, Reason: detail,
	})
}

// turnEndPlan is the decision emitTurnEnd derives before emitting: the
// TurnEnd reason, whether the turn's usage should ride along (only clean
// / budget-stopped completions carry usage; cancellations and errors
// don't), and an optional ErrorEvent to emit first.
type turnEndPlan struct {
	reason    TurnEndReason
	withUsage bool
	errMsg    string // non-empty → emit an ErrorEvent before TurnEnd
	errCode   string
}

// planTurnEnd is the turnEndPlan constructor: it maps the captured
// agent-runtime terminal event (plus the engine output and the run-loop's
// error signals) onto the plan emitTurnEnd executes. The lifecycle listener
// fires terminal events authoritatively (ProcessCompleted / Killed / Failed /
// Stuck / Terminated), so those drive the decision; the default case is the
// fallback for stub tests where no listener fired and the race where Done()
// returned before the platform multicast delivered the event. completedPlan /
// fallbackPlan are the per-branch builders it delegates to.
func planTurnEnd(terminal event.Event, out kernel.TurnOutput, runErr, ctxErr error, status core.AgentProcessStatus) turnEndPlan {
	switch t := terminal.(type) {
	case event.ProcessCompleted:
		return completedPlan(out)
	case event.ProcessKilled, event.ProcessTerminated:
		return turnEndPlan{reason: TurnEndCanceled}
	case event.ProcessFailed:
		msg := "engine error"
		if t.Err != nil {
			msg = t.Err.Error()
		}
		return turnEndPlan{reason: TurnEndErrored, errMsg: msg, errCode: "ENGINE_ERROR"}
	case event.ProcessStuck:
		return turnEndPlan{reason: TurnEndErrored, errMsg: "agent stuck — no forward progress", errCode: "AGENT_STUCK"}
	default:
		return fallbackPlan(out, runErr, ctxErr, status)
	}
}

// completedPlan maps a cleanly-completed turn's output to its reason: a
// budget stop is its own reason, otherwise a plain completion. Shared by
// the ProcessCompleted case and the fallback so the mapping lives in one
// place.
func completedPlan(out kernel.TurnOutput) turnEndPlan {
	switch {
	case out.StoppedOnSteps:
		return turnEndPlan{reason: TurnEndStepsExceeded, withUsage: true}
	case out.StoppedOnBudget:
		return turnEndPlan{reason: TurnEndBudgetExceeded, withUsage: true}
	default:
		return turnEndPlan{reason: TurnEndCompleted, withUsage: true}
	}
}

// fallbackPlan derives the plan from the run-loop signals when no
// terminal event was captured: a run error is a cancellation (ctx
// canceled / killed) or an engine error; no error falls back to the
// same completion mapping the happy path uses.
func fallbackPlan(out kernel.TurnOutput, runErr, ctxErr error, status core.AgentProcessStatus) turnEndPlan {
	if runErr != nil {
		if status == core.StatusKilled || errors.Is(ctxErr, context.Canceled) {
			return turnEndPlan{reason: TurnEndCanceled}
		}
		return turnEndPlan{reason: TurnEndErrored, errMsg: runErr.Error(), errCode: "ENGINE_ERROR"}
	}
	return completedPlan(out)
}
