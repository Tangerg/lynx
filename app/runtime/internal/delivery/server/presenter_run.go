package server

import (
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/accounting"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupt"
	rundomain "github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	"github.com/Tangerg/lynx/app/runtime/protocol"
)

// presentRunStatus publishes a lifecycle position. Which state IS which position
// is the domain's answer ([rundomain.State.Status]) — the durable status filter
// reads the same one, so a run selected as waiting cannot be published as
// finished; this only spells it for the wire.
func presentRunStatus(status rundomain.Status) protocol.RunStatus {
	switch status {
	case rundomain.StatusRunning:
		return protocol.RunStatusRunning
	case rundomain.StatusWaiting:
		return protocol.RunStatusWaiting
	case rundomain.StatusFinished:
		return protocol.RunStatusFinished
	default:
		panic("server: unknown run status")
	}
}

// presentRunSummary maps the identity and lifecycle half — what a cold read hands
// back in bulk.
func presentRunSummary(run rundomain.Run) protocol.RunSummary {
	summary := protocol.RunSummary{
		ID: run.ID(), SessionID: run.SessionID(), SpawnedByItemID: run.Lineage().SpawnedByItemID,
		ParentRunID: run.Lineage().ParentRunID, RootRunID: run.Lineage().RootRunID,
		Provider: run.ModelSelection().Provider(), Model: run.ModelSelection().Model(),
		Status:    presentRunStatus(run.State().Status()),
		CreatedAt: run.CreatedAt(), FinishedAt: run.FinishedAt(),
	}
	// A waiting run has no outcome: what it is waiting on is the answer, and the
	// interrupts carry that.
	if run.State().IsTerminal() {
		outcome := presentOutcome(run)
		summary.Outcome = &outcome
	}
	return summary
}

func presentRun(run rundomain.Run) protocol.RunRef {
	return protocol.RunRef{
		RunSummary:      presentRunSummary(run),
		ActiveSegmentID: run.ActiveSegmentID(),
		Metrics:         presentMetrics(run.Metrics()),
		Limits:          presentLimits(run.Limits()),
		ProtocolProfile: presentRunProtocolProfile(run.Capabilities()),
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

// presentRunProtocolProfile maps the Run's capabilities to the external
// protocol contract. Both sets are allocated even when empty: the Minimal
// Profile is known, not null.
func presentRunProtocolProfile(capabilities rundomain.Capabilities) protocol.RunProtocolProfile {
	out := protocol.RunProtocolProfile{
		RequiredFeatures: make([]protocol.RunProtocolFeature, 0, 1),
		InterruptTypes:   make([]protocol.InterruptType, 0, len(capabilities.InterruptKinds)),
	}
	if capabilities.ChildRuns {
		out.RequiredFeatures = append(out.RequiredFeatures, protocol.RunProtocolFeatureSubagents)
	}
	for _, kind := range capabilities.InterruptKinds {
		out.InterruptTypes = append(out.InterruptTypes, presentInterruptType(kind))
	}
	return out
}

// presentSegmentFinished maps the run record a segment ended with onto the pair
// the event publishes: why the segment stopped, and what the run has consumed.
func presentSegmentFinished(run rundomain.Run, interrupts []transcript.Interrupt) (protocol.SegmentOutcome, protocol.RunMetrics) {
	metrics := presentMetrics(run.Metrics())
	if run.State() == rundomain.Waiting {
		if len(interrupts) == 0 {
			return protocol.SegmentOutcome{Type: protocol.SegmentSuspended}, metrics
		}
		return protocol.SegmentOutcome{
			Type:       protocol.SegmentInterrupt,
			Interrupts: presentInterrupts(interrupts),
		}, metrics
	}
	terminal := presentOutcome(run)
	return protocol.SegmentOutcome{
		Type:   protocol.SegmentOutcomeType(terminal.Type),
		Error:  terminal.Error,
		Detail: terminal.Detail,
	}, metrics
}

func presentOutcome(run rundomain.Run) protocol.RunOutcome {
	outcome, terminal := run.Outcome()
	if !terminal {
		panic("server: terminal run has no outcome")
	}
	var kind protocol.RunOutcomeType
	switch outcome {
	case rundomain.OutcomeCompleted:
		kind = protocol.OutcomeCompleted
	case rundomain.OutcomeCanceled:
		kind = protocol.OutcomeCanceled
	case rundomain.OutcomeTimedOut:
		kind = protocol.OutcomeTimedOut
	case rundomain.OutcomeFailed:
		kind = protocol.OutcomeFailed
	case rundomain.OutcomeMaxBudget:
		kind = protocol.OutcomeMaxBudget
	case rundomain.OutcomeMaxSteps:
		kind = protocol.OutcomeMaxSteps
	case rundomain.OutcomeLost:
		kind = protocol.OutcomeLost
	default:
		panic("server: unknown run outcome")
	}
	failure, failed := run.Failure()
	var problem *protocol.ProblemData
	if failed {
		problem = presentRunFailure(&failure)
	}
	return protocol.RunOutcome{Type: kind, Error: problem, Detail: run.Detail()}
}

func presentMetrics(metrics rundomain.Metrics) protocol.RunMetrics {
	usage, reported := metrics.Usage()
	var usageRef *accounting.Usage
	if reported {
		usageRef = &usage
	}
	return protocol.RunMetrics{
		Usage:                presentUsage(usageRef),
		Steps:                metrics.Steps(),
		ActiveDurationMillis: metrics.ActiveDuration().Milliseconds(),
	}
}

func presentLimits(limits rundomain.Limits) *protocol.RunLimits {
	if limits.IsZero() {
		return nil
	}
	return &protocol.RunLimits{
		MaxTotalTokens: limits.MaxTotalTokens, MaxSteps: limits.MaxSteps, MaxBudgetUSD: limits.MaxBudgetUSD,
	}
}

func presentProgress(progress runs.RunProgress) protocol.RunProgress {
	return protocol.RunProgress{
		Step:  progress.Step,
		Usage: presentUsage(progress.Usage), ContextTokens: progress.ContextTokens,
		Activity: progress.Activity,
	}
}

func presentUsage(usage *accounting.Usage) *protocol.Usage {
	if usage == nil {
		return nil
	}
	out := &protocol.Usage{ModelUsage: presentModelUsage(usage.Total)}
	if len(usage.ByModel) > 0 {
		out.ByModel = make(map[string]protocol.ModelUsage, len(usage.ByModel))
		for model, modelUsage := range usage.ByModel {
			out.ByModel[model] = presentModelUsage(modelUsage)
		}
	}
	return out
}

func presentModelUsage(usage accounting.Totals) protocol.ModelUsage {
	return protocol.ModelUsage{
		InputTokens: usage.InputTokens, OutputTokens: usage.OutputTokens,
		CacheReadTokens: usage.CacheReadTokens, CacheWriteTokens: usage.CacheWriteTokens,
		ReasoningTokens: usage.ReasoningTokens, CostUSD: usage.CostUSD,
	}
}

func presentRunFailure(problem *rundomain.Failure) *protocol.ProblemData {
	if problem == nil {
		return nil
	}
	var kind string
	switch problem.Kind {
	case rundomain.FailureInternal:
		kind = protocol.ProblemInternalError
	case rundomain.FailureLost:
		kind = protocol.ProblemRunLost
	case rundomain.FailureAgentStuck:
		kind = protocol.ProblemAgentStuck
	case rundomain.FailureRateLimited:
		kind = protocol.ProblemRateLimited
	case rundomain.FailureInvalidCredentials:
		kind = protocol.ProblemInvalidAPIKey
	case rundomain.FailureTimeout:
		kind = protocol.ProblemTimeout
	case rundomain.FailureProviderUnavailable:
		kind = protocol.ProblemProviderUnavailable
	case rundomain.FailureProviderRejected:
		kind = protocol.ProblemProviderRejected
	default:
		panic("server: unknown run failure kind")
	}
	// The problem's scope is not published: where the frame LANDS already says it —
	// a run's outcome or a tool call's error — and a field restating that is a second
	// answer a client could find disagreeing with the first. The domain keeps its own
	// scope, which is what stops a run problem being stored in a tool slot.
	return &protocol.ProblemData{
		Type: kind, Detail: problem.Detail, DocURL: problem.DocURL,
		RetryAfterSeconds: int(problem.RetryAfter.Seconds()),
	}
}

func presentToolFailure(failure *tool.Failure) *protocol.ProblemData {
	if failure == nil {
		return nil
	}
	var kind string
	switch failure.Kind {
	case tool.FailureInternal:
		kind = protocol.ProblemInternalError
	case tool.FailureDenied:
		kind = protocol.ProblemDeniedByUser
	case tool.FailureExecution:
		kind = protocol.ProblemToolFailed
	case tool.FailureChildRunCanceled:
		kind = protocol.ProblemChildRunCanceled
	default:
		panic("server: unknown tool failure kind")
	}
	return &protocol.ProblemData{
		Type: kind, Detail: failure.Detail, DocURL: failure.DocURL,
		RetryAfterSeconds: int(failure.RetryAfter.Seconds()),
	}
}

func presentInterrupts(interrupts []transcript.Interrupt) []protocol.Interrupt {
	out := make([]protocol.Interrupt, 0, len(interrupts))
	for _, request := range interrupts {
		entry := protocol.Interrupt{ItemID: request.ItemID, RunID: request.RunID}
		entry.Type = presentInterruptType(request.Kind)
		switch request.Kind {
		case interrupt.Approval:
			if request.Approval == nil {
				panic("server: approval interrupt has no approval payload")
			}
			entry.Payload = &protocol.InterruptPayload{
				Tool:         new(presentTool(request.Approval.Tool)),
				Risk:         presentApprovalRisk(request.Approval.Risk),
				Reason:       request.Approval.Reason,
				Rememberable: request.Approval.Rememberable,
			}
		case interrupt.Question:
			if request.Question == nil {
				panic("server: question interrupt has no question payload")
			}
			entry.Payload = &protocol.InterruptPayload{Question: new(presentQuestion(*request.Question))}
		}
		out = append(out, entry)
	}
	return out
}
