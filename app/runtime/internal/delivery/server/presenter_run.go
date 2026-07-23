package server

import (
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

func presentRun(run transcript.Run) protocol.RunRef {
	status := protocol.RunStatusFinished
	if run.State == execution.Running {
		status = protocol.RunStatusRunning
	}
	ref := protocol.RunRef{
		ID: run.ID, SessionID: run.SessionID, SpawnedByItemID: run.SpawnedByItemID,
		Provider: run.Provider, Model: run.Model, Status: status,
		CreatedAt: run.CreatedAt, FinishedAt: run.FinishedAt,
	}
	if run.State != execution.Running {
		outcome := presentOutcome(run)
		ref.Outcome = &outcome
	}
	return ref
}

func presentOutcome(run transcript.Run) protocol.RunOutcome {
	if run.State == execution.Interrupted {
		return protocol.RunOutcome{Type: protocol.OutcomeInterrupt, Interrupts: presentInterrupts(run.Interrupts)}
	}
	kind := protocol.OutcomeCompleted
	if run.Outcome != nil {
		switch *run.Outcome {
		case execution.OutcomeCanceled:
			kind = protocol.OutcomeCanceled
		case execution.OutcomeError:
			kind = protocol.OutcomeError
		case execution.OutcomeMaxBudget:
			kind = protocol.OutcomeMaxBudget
		case execution.OutcomeMaxSteps:
			kind = protocol.OutcomeMaxSteps
		}
	}
	return protocol.RunOutcome{Type: kind, Result: presentRunResult(run.Result), Detail: run.Detail}
}

func presentRunResult(result *transcript.RunResult) *protocol.RunResult {
	if result == nil {
		return nil
	}
	steps := result.Steps
	return &protocol.RunResult{
		Usage: presentUsage(result.Usage), Steps: &steps,
		Error: presentProblem(result.Error), DurationMs: int(result.Duration.Milliseconds()),
	}
}

func presentProgress(progress runs.RunProgress) protocol.RunProgress {
	return protocol.RunProgress{
		Step:  progress.Step,
		Usage: presentUsage(progress.Usage), ContextTokens: progress.ContextTokens,
		Activity: progress.Activity,
	}
}

func presentUsage(usage *transcript.Usage) *protocol.Usage {
	if usage == nil {
		return nil
	}
	out := &protocol.Usage{ModelUsage: presentModelUsage(usage.ModelUsage)}
	if len(usage.ByModel) > 0 {
		out.ByModel = make(map[string]protocol.ModelUsage, len(usage.ByModel))
		for model, modelUsage := range usage.ByModel {
			out.ByModel[model] = presentModelUsage(modelUsage)
		}
	}
	return out
}

func presentModelUsage(usage transcript.ModelUsage) protocol.ModelUsage {
	return protocol.ModelUsage{
		InputTokens: usage.InputTokens, OutputTokens: usage.OutputTokens,
		CacheReadTokens: usage.CacheReadTokens, CacheWriteTokens: usage.CacheWriteTokens,
		ReasoningTokens: usage.ReasoningTokens, CostUSD: usage.CostUSD,
	}
}

func presentProblem(problem *transcript.Problem) *protocol.ProblemData {
	if problem == nil {
		return nil
	}
	kind := protocol.ProblemInternalError
	switch problem.Kind {
	case transcript.RunLostProblem:
		kind = protocol.ProblemRunLost
	case transcript.AgentStuckProblem:
		kind = protocol.ProblemAgentStuck
	case transcript.RateLimitedProblem:
		kind = protocol.ProblemRateLimited
	case transcript.InvalidAPIKeyProblem:
		kind = protocol.ProblemInvalidAPIKey
	case transcript.TimeoutProblem:
		kind = protocol.ProblemTimeout
	case transcript.ProviderUnavailableProblem:
		kind = protocol.ProblemProviderUnavailable
	case transcript.ProviderRejectedProblem:
		kind = protocol.ProblemProviderRejected
	case transcript.DeniedByUserProblem:
		kind = protocol.ProblemDeniedByUser
	case transcript.ToolFailedProblem:
		kind = protocol.ProblemToolFailed
	}
	scope := protocol.ErrorChannelRun
	if problem.Scope == transcript.ToolProblem {
		scope = protocol.ErrorChannelTool
	}
	return &protocol.ProblemData{
		Type: kind, Channel: scope, Detail: problem.Detail, DocURL: problem.DocURL,
		Retryable: problem.Retryable, RetryAfterSeconds: problem.RetryAfterSeconds,
	}
}

func presentInterrupts(interrupts []transcript.Interrupt) []protocol.Interrupt {
	out := make([]protocol.Interrupt, 0, len(interrupts))
	for _, interrupt := range interrupts {
		entry := protocol.Interrupt{ItemID: interrupt.ItemID}
		switch interrupt.Kind {
		case transcript.ApprovalInterrupt:
			if interrupt.Approval == nil {
				continue
			}
			entry.Type = protocol.InterruptApproval
			entry.Payload = &protocol.InterruptPayload{
				Tool:   new(presentTool(interrupt.Approval.Tool)),
				Risk:   presentApprovalRisk(interrupt.Approval.Risk),
				Reason: interrupt.Approval.Reason,
			}
		case transcript.QuestionInterrupt:
			if interrupt.Question == nil {
				continue
			}
			entry.Type = protocol.InterruptQuestion
			entry.Payload = &protocol.InterruptPayload{Question: new(presentQuestion(*interrupt.Question))}
		default:
			continue
		}
		out = append(out, entry)
	}
	return out
}
