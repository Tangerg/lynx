package turn

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/accounting"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/hooks"
)

// cleanupTurnOwned releases a terminal turn exactly once after the Agent
// runtime confirms that it relinquished process ownership.
func (s *controller) cleanupTurnOwned(ctx context.Context, st *turnState) error {
	if st.released() {
		return nil
	}
	if !st.terminalized() {
		return errors.New("turn: cleanup requested before terminal")
	}
	if p := st.process(); p != nil {
		if err := discardProcess(ctx, p); err != nil {
			recordTurnCleanupError(st, err)
			return err
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
	return nil
}

func discardProcess(ctx context.Context, process agentexec.TurnProcess) error {
	err := process.Discard(ctx)
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
func (s *controller) finishTurn(st *turnState, reason execution.Outcome) error {
	return s.completeTurn(st, func() { s.emitFinishedTurn(st, reason) })
}

func (s *controller) finishTurnOwned(ctx context.Context, st *turnState, reason execution.Outcome) error {
	return s.completeTurnOwned(ctx, st, func() { s.emitFinishedTurn(st, reason) })
}

func (s *controller) emitFinishedTurn(st *turnState, reason execution.Outcome) {
	dur := st.segmentElapsed()
	finishTurnSpan(st.span, reason, accounting.TokenUsage{}, false, "")
	recordTurnDuration(st.ctx, reason, st.model, dur)
	s.emitRootEvent(st, runs.SegmentEnded{Reason: reason, Duration: dur})
}

// finishFailedTurn closes an emergency error path with one self-contained
// terminal event. The raw error stays local to tracing and stop hooks; the
// EngineEvent contract carries only the stable application problem.
func (s *controller) finishFailedTurn(st *turnState, problem transcript.Problem, err error) error {
	return s.completeTurn(st, func() {
		dur := st.segmentElapsed()
		errMsg := "turn failed"
		if err != nil {
			errMsg = err.Error()
		}
		finishTurnSpan(st.span, execution.OutcomeError, accounting.TokenUsage{}, false, errMsg)
		recordTurnDuration(st.ctx, execution.OutcomeError, st.model, dur)
		s.emitRootEvent(st, runs.SegmentEnded{Reason: execution.OutcomeError, Problem: &problem, Duration: dur})
		s.fireStop(st, errMsg)
	})
}

// finishExecutionError resolves an engine operation failure against concurrent
// cancellation before publishing the terminal. Cancellation is the user's
// authoritative intent when it already won the lifecycle transition; otherwise
// the operation's stable problem is surfaced. The returned error is teardown
// only, so synchronous callers can join it with the operation error without
// losing either fact.
func (s *controller) finishExecutionError(st *turnState, problem transcript.Problem, err error) error {
	if st.cancelRequested() {
		return s.finishTurn(st, execution.OutcomeCanceled)
	}
	return s.finishFailedTurn(st, problem, err)
}

func (s *controller) completeTurn(st *turnState, emitTerminal func()) error {
	st.lifecycleMu.Lock()
	defer st.lifecycleMu.Unlock()
	return s.completeTurnOwned(st.ctx, st, emitTerminal)
}

func (s *controller) completeTurnOwned(ctx context.Context, st *turnState, emitTerminal func()) error {
	if st.beginTerminal() {
		emitTerminal()
		st.closeEvents()
		if !s.cleanupTasks.Start(st.ctx, func(ctx context.Context) {
			_ = s.cleanupTurn(ctx, st)
		}) {
			// Shutdown already owns the turn set and will perform cleanup through
			// this lifecycle command using its caller-bounded context.
			return s.cleanupTurnOwned(ctx, st)
		}
	}
	return nil
}

func (s *controller) cleanupTurn(ctx context.Context, st *turnState) error {
	st.lifecycleMu.Lock()
	defer st.lifecycleMu.Unlock()
	return s.cleanupTurnOwned(ctx, st)
}

// emitTurnEnd maps the process segment's immutable completion onto the
// application-owned TurnEnd event.
func (s *controller) emitTurnEnd(st *turnState, completion agentexec.TurnCompletion, duration time.Duration) {
	plan := planTurnEnd(completion)
	out := completion.Output

	finishTurnSpan(st.span, plan.reason, out.Usage, plan.withUsage, plan.errMsg)
	recordTurnDuration(st.ctx, plan.reason, st.model, duration)
	end := runs.SegmentEnded{Reason: plan.reason, Problem: plan.problem, Duration: duration}
	if plan.withUsage {
		end.Usage = &runs.SegmentUsage{
			Tokens:  out.Usage,
			ByModel: out.UsageByModel,
			CostUSD: out.CostUSD,
			Steps:   out.Steps,
		}
	}
	s.emitRootEvent(st, end)
	// Stop hooks (observe-only): fire after the terminal is emitted (the client
	// already saw segment.finished) — for notify / chain / cleanup. Bounded by the
	// hook timeout; it precedes only the turn's teardown, not the client signal.
	s.fireStop(st, plan.errMsg)
}

// OnChildProcessEnd projects a durably admitted Agent child onto its own
// Segment terminal. The root and child share one executor channel, but the
// source identity keeps their reducers and accounting boundaries independent.
func (t *turnObserver) OnChildProcessEnd(completion agentexec.ChildCompletion) {
	if t == nil || !t.projects(completion.Process.ProcessRef) {
		return
	}
	plan := planChildTurnEnd(completion)
	duration := max(completion.CompletedAt.Sub(completion.Process.StartedAt), 0)
	end := runs.SegmentEnded{
		Reason:   plan.reason,
		Problem:  plan.problem,
		Duration: duration,
	}
	if plan.withUsage || len(completion.UsageByModel) > 0 {
		end.Usage = &runs.SegmentUsage{
			Tokens:  completion.Usage,
			ByModel: completion.UsageByModel,
			CostUSD: completion.CostUSD,
			Steps:   completion.Steps,
		}
	}
	t.controller.emitProcessEvent(t.st, completion.Process.ProcessRef, end)
}

func planChildTurnEnd(completion agentexec.ChildCompletion) turnEndPlan {
	if completion.Process.ID == "" ||
		completion.Process.ParentID == "" ||
		completion.Process.SpawnCallID == "" ||
		completion.Process.StartedAt.IsZero() ||
		completion.CompletedAt.IsZero() {
		return failurePlan(errors.Join(
			completion.Err,
			errors.New("delegated child completion has incomplete identity or timestamps"),
		))
	}
	if completion.Err != nil {
		// ProcessFailed carries its execution error here. For every other
		// terminal status a non-nil value is a projection failure (for example,
		// an accounting invariant), which must not be disguised as cancellation.
		return failurePlan(completion.Err)
	}
	return planTurnEnd(agentexec.TurnCompletion{
		Status: completion.Status,
		Output: agentexec.TurnOutput{
			Usage:        completion.Usage,
			UsageByModel: completion.UsageByModel,
			CostUSD:      completion.CostUSD,
			Steps:        completion.Steps,
			StopReason:   completion.StopReason,
		},
		HasOutput: completion.Status == core.StatusCompleted,
	})
}

// fireStop runs the Stop lifecycle hooks for a terminated turn (observe-only).
func (s *controller) fireStop(st *turnState, detail string) {
	if st.hooks.Empty() {
		return
	}
	_ = st.hooks.Run(st.ctx, hooks.Input{
		Event: hooks.Stop, SessionID: st.handle.SessionID, Cwd: st.cwd, Reason: detail,
	})
}

// turnEndPlan is the decision emitTurnEnd derives before emitting: the TurnEnd
// reason, whether a normal completion requires usage, and an optional stable
// problem for an error terminal. Child projection may additionally attach an
// authoritative subtree report to a cancellation or failure.
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
	completionErr := completion.Err
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

// completedPlan maps a cleanly-completed turn's output to its reason. This is
// the one place the framework's account of which bound stopped the interaction
// becomes Runtime's own run outcome.
func completedPlan(out agentexec.TurnOutput) turnEndPlan {
	switch out.StopReason {
	case agent.InteractionStopModelCalls:
		return turnEndPlan{reason: execution.OutcomeMaxSteps, withUsage: true}
	case agent.InteractionStopBudget:
		return turnEndPlan{reason: execution.OutcomeMaxBudget, withUsage: true}
	case agent.InteractionStopNone:
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
	// A provider's backoff hint only means something for the failures that
	// waiting can clear. Naming those kinds here is what a transient/permanent
	// flag was standing in for.
	acceptsBackoff := false
	switch kind {
	case execution.FailureAgentStuck:
		problem.Kind = transcript.AgentStuckProblem
		problem.Detail = "the agent stopped because it could not make forward progress"
	case execution.FailureRateLimited:
		problem.Kind = transcript.RateLimitedProblem
		problem.Detail = "the model provider rate-limited the request; retry shortly"
		acceptsBackoff = true
	case execution.FailureInvalidCredentials:
		problem.Kind = transcript.InvalidAPIKeyProblem
		problem.Detail = "the model provider rejected the credentials; check the provider API key"
	case execution.FailureTimeout:
		problem.Kind = transcript.TimeoutProblem
		problem.Detail = "the model provider request timed out or the connection failed; retry shortly"
		acceptsBackoff = true
	case execution.FailureProviderUnavailable:
		problem.Kind = transcript.ProviderUnavailableProblem
		problem.Detail = "the model provider is temporarily unavailable; retry shortly"
		acceptsBackoff = true
	case execution.FailureProviderRejected:
		problem.Kind = transcript.ProviderRejectedProblem
		problem.Detail = "the model provider rejected the request as invalid"
	default:
		return internalRunProblem()
	}
	if acceptsBackoff && retryAfter > 0 {
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
