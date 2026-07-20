package turn

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/accounting"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/hooks"
)

// endTurn closes the turn's event channel and removes it from the live
// registry. The first Events call can still drain the handle-owned buffered
// stream; subsequent Events and all Cancel / Resume calls return ErrTurnNotFound.
// It also ends the turn span: the single teardown point, so the span
// closes exactly once no matter which terminal path (drive / finishTurn)
// reached it. finishTurnSpan has already stamped the outcome.
func (s *memoryDispatcher) endTurn(st *turnState) {
	// Release the backing process now that the turn is terminal: free its
	// in-memory registry entry and delete its persisted auto-snapshot, which
	// only matters while a process is PARKED for HITL resume (endTurn never runs
	// on a parked turn — handleWaiting leaves it registered). Without this every
	// run leaks one process_snapshot row. Off a cancel-decoupled ctx so the
	// delete gets a short independent deadline even when the turn ctx was
	// canceled, keeping the trace span without letting best-effort cleanup wedge
	// terminal delivery or component shutdown.
	if p := st.process(); p != nil {
		recordTurnCleanupError(st, discardProcess(st.ctx, p))
	}
	if st.span != nil {
		st.span.End()
	}
	st.closeEvents()
	s.mu.Lock()
	delete(s.turns, st.handle.TurnID)
	s.mu.Unlock()
	close(st.done)
}

const processDiscardTimeout = 2 * time.Second

func discardProcess(ctx context.Context, process agentexec.TurnProcess) error {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), processDiscardTimeout)
	defer cancel()
	if err := process.Discard(cleanupCtx); err != nil {
		return fmt.Errorf("turn: discard process %q: %w", process.ID(), err)
	}
	return nil
}

// finishTurn emits the terminal [TurnEnd] (stamping the elapsed duration)
// and tears the turn down. It serves the emergency-teardown paths —
// [Dispatcher.Cancel] and a failed [Dispatcher.Resume] — where no drive
// goroutine will run [emitTurnEnd]. The clean path goes through
// emitTurnEnd (which carries usage) followed by endTurn in [drive].
func (s *memoryDispatcher) finishTurn(st *turnState, reason execution.Outcome) {
	s.completeTurn(st, func() {
		dur := time.Since(st.startedAt)
		finishTurnSpan(st.span, reason, accounting.TokenUsage{}, false, "")
		recordTurnDuration(st.ctx, reason, st.model, dur)
		s.emit(st, TurnEnd{Reason: reason, Duration: dur})
	})
}

func (s *memoryDispatcher) completeTurn(st *turnState, emitTerminal func()) {
	st.terminalOnce.Do(func() {
		emitTerminal()
		s.endTurn(st)
	})
}

// emitTurnEnd maps the captured agent runtime terminal event onto a
// transport-shape TurnEnd. The lifecycle listener fires terminal
// events authoritatively (ProcessKilled / ProcessFailed /
// ProcessStuck / ProcessTerminated / ProcessCompleted), so we read
// those rather than re-deriving status from the run loop's error.
// The runErr / ctxErr / status fallback covers stub tests where no
// listener fired and any race where Done() returned before the
// engine multicast delivered the terminal event.
func (s *memoryDispatcher) emitTurnEnd(st *turnState, process agentexec.TurnProcess, terminal event.Event, runErr error, duration time.Duration, ctxErr error) {
	out, _ := process.Output()
	plan := planTurnEnd(terminal, out, runErr, ctxErr, process.Status())

	finishTurnSpan(st.span, plan.reason, out.Usage, plan.withUsage, plan.errMsg)
	recordTurnDuration(st.ctx, plan.reason, st.model, duration)
	if plan.errMsg != "" {
		s.emit(st, ErrorEvent{Message: plan.errMsg, Code: plan.errCode, Problem: plan.problem})
	}
	end := TurnEnd{Reason: plan.reason, Duration: duration, MaxBudget: st.maxBudget, MaxCostUSD: st.maxCostUSD, MaxSteps: st.maxSteps}
	if plan.withUsage {
		end.TokenUsage = out.Usage
		end.UsageByModel = out.UsageByModel
		end.CostUSD = out.CostUSD
	}
	s.emit(st, end)
	// Stop hooks (observe-only): fire after the terminal is emitted (the client
	// already saw segment.finished) — for notify / chain / cleanup. Bounded by the
	// hook timeout; it precedes only the turn's teardown, not the client signal.
	s.fireStop(st, plan.errMsg)
}

// fireStop runs the Stop lifecycle hooks for a terminated turn (observe-only).
func (s *memoryDispatcher) fireStop(st *turnState, detail string) {
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
	reason    execution.Outcome
	withUsage bool
	errMsg    string // non-empty → emit an ErrorEvent before TurnEnd
	errCode   ErrorCode
	problem   runs.Problem
}

// planTurnEnd is the turnEndPlan constructor: it maps the captured
// agent-runtime terminal event (plus the engine output and the run-loop's
// error signals) onto the plan emitTurnEnd executes. The lifecycle listener
// fires terminal events authoritatively (ProcessCompleted / Killed / Failed /
// Stuck / Terminated), so those drive the decision; the default case is the
// fallback for stub tests where no listener fired and the race where Done()
// returned before the engine multicast delivered the event. completedPlan /
// fallbackPlan are the per-branch builders it delegates to.
func planTurnEnd(terminal event.Event, out agentexec.TurnOutput, runErr, ctxErr error, status core.ProcessStatus) turnEndPlan {
	switch t := terminal.(type) {
	case event.ProcessCompleted:
		return completedPlan(out)
	case event.ProcessKilled, event.ProcessTerminated:
		return turnEndPlan{reason: execution.OutcomeCanceled}
	case event.ProcessFailed:
		msg := "engine error"
		if t.Err != nil {
			msg = t.Err.Error()
		}
		return turnEndPlan{
			reason: execution.OutcomeError, errMsg: msg, errCode: ErrorCodeEngine,
			problem: problemFromError(t.Err),
		}
	case event.ProcessStuck:
		return turnEndPlan{
			reason: execution.OutcomeError, errMsg: "agent stuck — no forward progress", errCode: ErrorCodeAgentStuck,
			problem: problemForFailure(execution.FailureAgentStuck, 0),
		}
	default:
		return fallbackPlan(out, runErr, ctxErr, status)
	}
}

// completedPlan maps a cleanly-completed turn's output to its reason: a
// budget stop is its own reason, otherwise a plain completion. Shared by
// the ProcessCompleted case and the fallback so the mapping lives in one
// place.
func completedPlan(out agentexec.TurnOutput) turnEndPlan {
	switch out.StopReason {
	case agentexec.StopReasonSteps:
		return turnEndPlan{reason: execution.OutcomeMaxSteps, withUsage: true}
	case agentexec.StopReasonBudget:
		return turnEndPlan{reason: execution.OutcomeMaxBudget, withUsage: true}
	case agentexec.StopReasonNone:
		return turnEndPlan{reason: execution.OutcomeCompleted, withUsage: true}
	default:
		return turnEndPlan{
			reason:  execution.OutcomeError,
			errMsg:  fmt.Sprintf("invalid turn stop reason %q", out.StopReason),
			errCode: ErrorCodeEngine,
			problem: internalRunProblem(),
		}
	}
}

// fallbackPlan derives the plan from the run-loop signals when no
// terminal event was captured: a run error is a cancellation (ctx
// canceled / killed) or an engine error; no error falls back to the
// same completion mapping the happy path uses.
func fallbackPlan(out agentexec.TurnOutput, runErr, ctxErr error, status core.ProcessStatus) turnEndPlan {
	if runErr != nil {
		if status == core.StatusKilled || errors.Is(ctxErr, context.Canceled) {
			return turnEndPlan{reason: execution.OutcomeCanceled}
		}
		return turnEndPlan{
			reason: execution.OutcomeError, errMsg: runErr.Error(), errCode: ErrorCodeEngine,
			problem: problemFromError(runErr),
		}
	}
	return completedPlan(out)
}

func problemFromError(err error) runs.Problem {
	var failure *execution.Failure
	if errors.As(err, &failure) {
		return problemForFailure(failure.Kind, failure.RetryAfter)
	}
	return internalRunProblem()
}

func problemForFailure(kind execution.FailureKind, retryAfter time.Duration) runs.Problem {
	problem := runs.Problem{Scope: runs.RunProblem}
	switch kind {
	case execution.FailureAgentStuck:
		problem.Kind = runs.AgentStuckProblem
		problem.Detail = "the agent stopped because it could not make forward progress"
	case execution.FailureRateLimited:
		problem.Kind = runs.RateLimitedProblem
		problem.Detail = "the model provider rate-limited the request; retry shortly"
		problem.Retryable = true
	case execution.FailureInvalidCredentials:
		problem.Kind = runs.InvalidAPIKeyProblem
		problem.Detail = "the model provider rejected the credentials; check the provider API key"
	case execution.FailureTimeout:
		problem.Kind = runs.TimeoutProblem
		problem.Detail = "the model provider request timed out or the connection failed; retry shortly"
		problem.Retryable = true
	case execution.FailureProviderUnavailable:
		problem.Kind = runs.ProviderUnavailableProblem
		problem.Detail = "the model provider is temporarily unavailable; retry shortly"
		problem.Retryable = true
	case execution.FailureProviderRejected:
		problem.Kind = runs.ProviderRejectedProblem
		problem.Detail = "the model provider rejected the request as invalid"
	default:
		return internalRunProblem()
	}
	if problem.Retryable && retryAfter > 0 {
		problem.RetryAfterSeconds = int((retryAfter + time.Second - 1) / time.Second)
	}
	return problem
}

func internalRunProblem() runs.Problem {
	return runs.Problem{
		Kind: runs.InternalProblem, Scope: runs.RunProblem,
		Detail: "the run failed due to an internal error",
	}
}
