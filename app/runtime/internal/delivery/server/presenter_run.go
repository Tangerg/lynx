package server

import (
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

// presentRunStatus publishes a lifecycle position. Which state IS which position
// is the domain's answer ([execution.RunState.Status]) — the durable status filter
// reads the same one, so a run selected as waiting cannot be published as
// finished; this only spells it for the wire.
func presentRunStatus(status execution.RunStatus) protocol.RunStatus {
	switch status {
	case execution.StatusRunning:
		return protocol.RunStatusRunning
	case execution.StatusWaiting:
		return protocol.RunStatusWaiting
	case execution.StatusFinished:
		return protocol.RunStatusFinished
	default:
		panic("server: unknown run status")
	}
}

// presentRunSummary maps the identity and lifecycle half — what a cold read hands
// back in bulk.
func presentRunSummary(run transcript.Run) protocol.RunSummary {
	summary := protocol.RunSummary{
		ID: run.ID, SessionID: run.SessionID, SpawnedByItemID: run.SpawnedByItemID,
		ParentRunID: run.ParentRunID, RootRunID: run.RootRunID,
		Provider: run.ModelSelection.Provider(), Model: run.ModelSelection.Model(),
		Status:    presentRunStatus(run.State.Status()),
		CreatedAt: run.CreatedAt, FinishedAt: run.FinishedAt,
	}
	// A waiting run has no outcome: what it is waiting on is the answer, and the
	// interrupts carry that.
	if run.State.IsTerminal() {
		outcome := presentOutcome(run)
		summary.Outcome = &outcome
	}
	return summary
}

func presentRun(run transcript.Run) protocol.RunRef {
	return protocol.RunRef{
		RunSummary:      presentRunSummary(run),
		ActiveSegmentID: run.ActiveSegmentID,
		Metrics:         presentMetrics(run.Metrics),
		Limits:          presentLimits(run.Limits),
		ProtocolProfile: presentProtocolProfile(run.ProtocolProfile),
	}
}

func presentCancelResult(result runs.CancelResult) *protocol.CancelRunResponse {
	run := presentRun(result.Run)
	if result.RootRun == nil {
		if result.Run.Lineage().IsChild() {
			panic("server: child cancel result has no root run")
		}
		return &protocol.CancelRunResponse{Type: protocol.CancelRunRoot, Run: run}
	}
	if result.Run.Lineage().IsRoot() {
		panic("server: root cancel result unexpectedly carries a root run")
	}
	root := presentRun(*result.RootRun)
	return &protocol.CancelRunResponse{
		Type: protocol.CancelRunChild, Run: run, RootRun: &root,
	}
}

// presentProtocolProfile publishes the contract the run was created under. Both
// sets are allocated even when empty: an empty profile is the Minimal Profile —
// a meaning — and `null` would report the run's known contract as unknown.
func presentProtocolProfile(profile execution.RunProtocolProfile) protocol.RunProtocolProfile {
	out := protocol.RunProtocolProfile{
		RequiredFeatures: make([]string, 0, len(profile.RequiredFeatures)),
		InterruptTypes:   make([]protocol.InterruptType, 0, len(profile.InterruptKinds)),
	}
	out.RequiredFeatures = append(out.RequiredFeatures, profile.RequiredFeatures...)
	for _, kind := range profile.InterruptKinds {
		out.InterruptTypes = append(out.InterruptTypes, presentInterruptType(kind))
	}
	return out
}

// presentSegmentFinished maps the run record a segment ended with onto the pair
// the event publishes: why the segment stopped, and what the run has consumed.
func presentSegmentFinished(run transcript.Run) (protocol.SegmentOutcome, protocol.RunMetrics) {
	metrics := presentMetrics(run.Metrics)
	if run.State == execution.Interrupted {
		return protocol.SegmentOutcome{
			Type:       protocol.SegmentInterrupt,
			Interrupts: presentInterrupts(run.Interrupts),
		}, metrics
	}
	// `suspended` has no producer while features.subagents is off: it describes a
	// run stopped by ANOTHER run's interrupt barrier, and a session runs one root
	// run with no children. It is published anyway, because a client folding
	// segment outcomes exhaustively must already know the tag when subtrees arrive.
	terminal := presentOutcome(run)
	return protocol.SegmentOutcome{
		Type:   protocol.SegmentOutcomeType(terminal.Type),
		Error:  terminal.Error,
		Detail: terminal.Detail,
	}, metrics
}

func presentOutcome(run transcript.Run) protocol.RunOutcome {
	if run.Outcome == nil {
		panic("server: terminal run has no outcome")
	}
	var kind protocol.RunOutcomeType
	switch *run.Outcome {
	case execution.OutcomeCompleted:
		kind = protocol.OutcomeCompleted
	case execution.OutcomeCanceled:
		kind = protocol.OutcomeCanceled
	case execution.OutcomeError:
		kind = protocol.OutcomeError
	case execution.OutcomeMaxBudget:
		kind = protocol.OutcomeMaxBudget
	case execution.OutcomeMaxSteps:
		kind = protocol.OutcomeMaxSteps
	default:
		panic("server: unknown run outcome")
	}
	return protocol.RunOutcome{Type: kind, Error: presentProblem(run.Error), Detail: run.Detail}
}

func presentMetrics(metrics transcript.RunMetrics) protocol.RunMetrics {
	return protocol.RunMetrics{
		Usage:            presentUsage(metrics.Usage),
		Steps:            metrics.Steps,
		ActiveDurationMs: metrics.ActiveDuration.Milliseconds(),
	}
}

func presentLimits(limits execution.RunLimits) *protocol.RunLimits {
	if limits.IsZero() {
		return nil
	}
	return &protocol.RunLimits{MaxSteps: limits.MaxSteps, MaxBudgetUSD: limits.MaxBudgetUSD}
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
	var kind string
	switch problem.Kind {
	case transcript.InternalProblem:
		kind = protocol.ProblemInternalError
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
	default:
		panic("server: unknown transcript problem kind")
	}
	// The problem's scope is not published: where the frame LANDS already says it —
	// a run's outcome or a tool call's error — and a field restating that is a second
	// answer a client could find disagreeing with the first. The domain keeps its own
	// scope, which is what stops a run problem being stored in a tool slot.
	return &protocol.ProblemData{
		Type: kind, Detail: problem.Detail, DocURL: problem.DocURL,
		RetryAfterSeconds: problem.RetryAfterSeconds,
	}
}

func presentInterrupts(interrupts []transcript.Interrupt) []protocol.Interrupt {
	out := make([]protocol.Interrupt, 0, len(interrupts))
	for _, interrupt := range interrupts {
		entry := protocol.Interrupt{ItemID: interrupt.ItemID, RunID: interrupt.RunID}
		entry.Type = presentInterruptType(interrupt.Kind)
		switch interrupt.Kind {
		case execution.ApprovalInterrupt:
			if interrupt.Approval == nil {
				panic("server: approval interrupt has no approval payload")
			}
			entry.Payload = &protocol.InterruptPayload{
				Tool:         new(presentTool(interrupt.Approval.Tool)),
				Risk:         presentApprovalRisk(interrupt.Approval.Risk),
				Reason:       interrupt.Approval.Reason,
				Rememberable: interrupt.Approval.Rememberable,
			}
		case execution.QuestionInterrupt:
			if interrupt.Question == nil {
				panic("server: question interrupt has no question payload")
			}
			entry.Payload = &protocol.InterruptPayload{Question: new(presentQuestion(*interrupt.Question))}
		}
		out = append(out, entry)
	}
	return out
}
