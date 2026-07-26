package turn

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/accounting"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/hooks"
)

// cleanupTurnOwned releases a terminal turn exactly once. Process persistence
// failures remain observable, but cannot retain a completed turn in memory:
// terminal cleanup has no later runtime owner that could safely retry it.
func (s *memoryDispatcher) cleanupTurnOwned(st *turnState) error {
	if st.released() {
		return nil
	}
	if !st.terminalized() {
		return errors.New("turn: cleanup requested before terminal")
	}
	var cleanupErr error
	if p := st.process(); p != nil {
		if err := discardProcess(st.ctx, p); err != nil {
			recordTurnCleanupError(st, err)
			cleanupErr = err
		}
	}
	if st.span != nil {
		st.span.End()
	}
	s.mu.Lock()
	if s.turns[st.handle.TurnID] == st {
		delete(s.turns, st.handle.TurnID)
	}
	s.mu.Unlock()
	st.markReleased()
	close(st.done)
	return cleanupErr
}

const processDiscardTimeout = 2 * time.Second

func discardProcess(ctx context.Context, process agentexec.TurnProcess) error {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), processDiscardTimeout)
	defer cancel()
	err := process.Discard(cleanupCtx)
	if err != nil {
		return fmt.Errorf("turn: discard process %q: %w", process.ID(), err)
	}
	return nil
}

// finishTurn emits the terminal [TurnEnd] (stamping the elapsed duration)
// and tears the turn down. It serves the emergency-teardown paths —
// Cancel and a failed Resume — where no drive
// goroutine will run [emitTurnEnd]. The clean path goes through
// emitTurnEnd (which carries usage) followed by terminal cleanup in [drive].
func (s *memoryDispatcher) finishTurn(st *turnState, reason execution.Outcome) error {
	return s.completeTurn(st, func() { s.emitFinishedTurn(st, reason) })
}

func (s *memoryDispatcher) finishTurnOwned(st *turnState, reason execution.Outcome) error {
	return s.completeTurnOwned(st, func() { s.emitFinishedTurn(st, reason) })
}

func (s *memoryDispatcher) emitFinishedTurn(st *turnState, reason execution.Outcome) {
	dur := time.Since(st.startedAt)
	finishTurnSpan(st.span, reason, accounting.TokenUsage{}, false, "")
	recordTurnDuration(st.ctx, reason, st.model, dur)
	s.emit(st, runs.TurnEnd{Reason: reason, Duration: dur})
}

// finishFailedTurn closes an emergency error path with one self-contained
// terminal event. The raw error stays local to tracing and stop hooks; the
// EngineEvent contract carries only the stable application problem.
func (s *memoryDispatcher) finishFailedTurn(st *turnState, problem transcript.Problem, err error) error {
	return s.completeTurn(st, func() {
		dur := time.Since(st.startedAt)
		errMsg := "turn failed"
		if err != nil {
			errMsg = err.Error()
		}
		finishTurnSpan(st.span, execution.OutcomeError, accounting.TokenUsage{}, false, errMsg)
		recordTurnDuration(st.ctx, execution.OutcomeError, st.model, dur)
		s.emit(st, runs.TurnEnd{Reason: execution.OutcomeError, Problem: &problem, Duration: dur})
		s.fireStop(st, errMsg)
	})
}

// finishExecutionError resolves an engine operation failure against concurrent
// cancellation before publishing the terminal. Cancellation is the user's
// authoritative intent when it already won the lifecycle transition; otherwise
// the operation's stable problem is surfaced. The returned error is teardown
// only, so synchronous callers can join it with the operation error without
// losing either fact.
func (s *memoryDispatcher) finishExecutionError(st *turnState, problem transcript.Problem, err error) error {
	if st.cancelRequested() {
		return s.finishTurn(st, execution.OutcomeCanceled)
	}
	return s.finishFailedTurn(st, problem, err)
}

func (s *memoryDispatcher) completeTurn(st *turnState, emitTerminal func()) error {
	st.lifecycleMu.Lock()
	defer st.lifecycleMu.Unlock()
	return s.completeTurnOwned(st, emitTerminal)
}

func (s *memoryDispatcher) completeTurnOwned(st *turnState, emitTerminal func()) error {
	if st.beginTerminal() {
		emitTerminal()
		st.closeEvents()
	}
	return s.cleanupTurnOwned(st)
}

// emitTurnEnd maps the process segment's immutable completion onto the
// transport-shape TurnEnd.
func (s *memoryDispatcher) emitTurnEnd(st *turnState, completion agentexec.TurnCompletion, duration time.Duration) {
	plan := planTurnEnd(completion)
	out := completion.Output

	finishTurnSpan(st.span, plan.reason, out.Usage, plan.withUsage, plan.errMsg)
	recordTurnDuration(st.ctx, plan.reason, st.model, duration)
	end := runs.TurnEnd{Reason: plan.reason, Problem: plan.problem, Duration: duration}
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
// don't), and an optional stable problem for an error terminal.
type turnEndPlan struct {
	reason    execution.Outcome
	withUsage bool
	errMsg    string // local tracing and hook diagnostic only
	problem   *transcript.Problem
}

// planTurnEnd maps one joined Agent completion onto the application terminal.
// Every status is handled explicitly; an internally inconsistent completion is
// an error rather than an implicit success.
func planTurnEnd(completion agentexec.TurnCompletion) turnEndPlan {
	completionErr := completion.Error()
	switch completion.Status {
	case core.StatusCompleted:
		if completionErr != nil {
			return failurePlan(completionErr)
		}
		if !completion.HasOutput {
			return failurePlan(errors.New("agent process completed without TurnOutput"))
		}
		return completedPlan(completion.Output)
	case core.StatusKilled, core.StatusTerminated:
		return turnEndPlan{reason: execution.OutcomeCanceled}
	case core.StatusFailed, core.StatusPaused:
		return failurePlan(completionErr)
	case core.StatusStuck:
		problem := problemForFailure(execution.FailureAgentStuck, 0)
		return turnEndPlan{reason: execution.OutcomeError, errMsg: "agent stuck — no forward progress", problem: &problem}
	default:
		return failurePlan(fmt.Errorf("agent process joined in non-terminal status %s", completion.Status))
	}
}

func failurePlan(err error) turnEndPlan {
	if err == nil {
		err = errors.New("agent process failed without an error")
	}
	problem := problemFromError(err)
	return turnEndPlan{reason: execution.OutcomeError, errMsg: err.Error(), problem: &problem}
}

// completedPlan maps a cleanly-completed turn's output to its reason: a budget
// stop is its own reason, otherwise a plain completion.
func completedPlan(out agentexec.TurnOutput) turnEndPlan {
	switch out.StopReason {
	case agentexec.StopReasonSteps:
		return turnEndPlan{reason: execution.OutcomeMaxSteps, withUsage: true}
	case agentexec.StopReasonBudget:
		return turnEndPlan{reason: execution.OutcomeMaxBudget, withUsage: true}
	case agentexec.StopReasonNone:
		return turnEndPlan{reason: execution.OutcomeCompleted, withUsage: true}
	default:
		problem := internalRunProblem()
		return turnEndPlan{reason: execution.OutcomeError, errMsg: fmt.Sprintf("invalid turn stop reason %q", out.StopReason), problem: &problem}
	}
}

func problemFromError(err error) transcript.Problem {
	var failure *execution.Failure
	if errors.As(err, &failure) {
		return problemForFailure(failure.Kind, failure.RetryAfter)
	}
	return internalRunProblem()
}

func problemForFailure(kind execution.FailureKind, retryAfter time.Duration) transcript.Problem {
	problem := transcript.Problem{Scope: transcript.RunProblem}
	switch kind {
	case execution.FailureAgentStuck:
		problem.Kind = transcript.AgentStuckProblem
		problem.Detail = "the agent stopped because it could not make forward progress"
	case execution.FailureRateLimited:
		problem.Kind = transcript.RateLimitedProblem
		problem.Detail = "the model provider rate-limited the request; retry shortly"
		problem.Retryable = true
	case execution.FailureInvalidCredentials:
		problem.Kind = transcript.InvalidAPIKeyProblem
		problem.Detail = "the model provider rejected the credentials; check the provider API key"
	case execution.FailureTimeout:
		problem.Kind = transcript.TimeoutProblem
		problem.Detail = "the model provider request timed out or the connection failed; retry shortly"
		problem.Retryable = true
	case execution.FailureProviderUnavailable:
		problem.Kind = transcript.ProviderUnavailableProblem
		problem.Detail = "the model provider is temporarily unavailable; retry shortly"
		problem.Retryable = true
	case execution.FailureProviderRejected:
		problem.Kind = transcript.ProviderRejectedProblem
		problem.Detail = "the model provider rejected the request as invalid"
	default:
		return internalRunProblem()
	}
	if problem.Retryable && retryAfter > 0 {
		problem.RetryAfterSeconds = int((retryAfter + time.Second - 1) / time.Second)
	}
	return problem
}

func internalRunProblem() transcript.Problem {
	return transcript.Problem{
		Kind: transcript.InternalProblem, Scope: transcript.RunProblem,
		Detail: "the run failed due to an internal error",
	}
}
