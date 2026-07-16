package server

import (
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

func canonicalRunFromWire(sessionID, path string, ref protocol.RunRef, updatedAt time.Time, messageMark int) (transcript.Run, error) {
	if ref.ID == "" {
		return transcript.Run{}, invalidArtifact(path+".id", "is required")
	}
	if ref.SessionID != sessionID {
		return transcript.Run{}, invalidArtifact(path+".sessionId", "must equal artifact.session.id")
	}
	run := transcript.Run{
		SessionID: sessionID, ID: ref.ID, SpawnedByItemID: ref.SpawnedByItemID,
		Provider: ref.Provider, Model: ref.Model, State: execution.Running,
		CreatedAt: ref.CreatedAt, FinishedAt: ref.FinishedAt,
		UpdatedAt: updatedAt, MessageMark: messageMark,
	}
	switch ref.Status {
	case protocol.RunStatusRunning:
		return transcript.Run{}, invalidArtifact(path+".status", "running runs are not portable")
	case protocol.RunStatusFinished:
		if ref.Outcome == nil {
			return transcript.Run{}, invalidArtifact(path+".outcome", "is required while status is finished")
		}
		if ref.FinishedAt.IsZero() {
			return transcript.Run{}, invalidArtifact(path+".finishedAt", "is required while status is finished")
		}
	default:
		return transcript.Run{}, invalidArtifact(path+".status", "unknown value %q", ref.Status)
	}

	if ref.Outcome.Type == protocol.OutcomeInterrupt {
		return transcript.Run{}, invalidArtifact(path+".outcome.type", "interrupted runs are not portable")
	}
	if len(ref.Outcome.Interrupts) != 0 {
		return transcript.Run{}, invalidArtifact(path+".outcome.interrupts", "must be absent for terminal outcome %q", ref.Outcome.Type)
	}
	outcome, err := canonicalOutcome(path+".outcome.type", ref.Outcome.Type)
	if err != nil {
		return transcript.Run{}, err
	}
	state, ok := execution.Running.Terminate(outcome)
	if !ok {
		return transcript.Run{}, invalidArtifact(path+".outcome.type", "cannot terminate a running run")
	}
	result, err := canonicalRunResult(path+".outcome.result", ref.Outcome.Result, outcome)
	if err != nil {
		return transcript.Run{}, err
	}
	run.State = state
	run.Outcome = new(outcome)
	run.Result = result
	run.Detail = ref.Outcome.Detail
	return run, nil
}

func canonicalOutcome(path string, kind protocol.RunOutcomeType) (execution.Outcome, error) {
	switch kind {
	case protocol.OutcomeCompleted:
		return execution.OutcomeCompleted, nil
	case protocol.OutcomeCanceled:
		return execution.OutcomeCanceled, nil
	case protocol.OutcomeError:
		return execution.OutcomeError, nil
	case protocol.OutcomeMaxBudget:
		return execution.OutcomeMaxBudget, nil
	case protocol.OutcomeMaxSteps:
		return execution.OutcomeMaxSteps, nil
	default:
		return 0, invalidArtifact(path, "unknown value %q", kind)
	}
}

func canonicalRunResult(path string, result *protocol.RunResult, outcome execution.Outcome) (*transcript.RunResult, error) {
	if result == nil {
		return nil, invalidArtifact(path, "is required for terminal outcome %q", outcome.String())
	}
	if result.DurationMs < 0 {
		return nil, invalidArtifact(path+".durationMs", "must not be negative")
	}
	steps := 0
	if result.Steps != nil {
		if *result.Steps < 0 {
			return nil, invalidArtifact(path+".steps", "must not be negative")
		}
		steps = *result.Steps
	}
	usage, err := canonicalUsage(path+".usage", result.Usage)
	if err != nil {
		return nil, err
	}
	problem, err := canonicalProblem(path+".error", result.Error, protocol.ErrorChannelRun)
	if err != nil {
		return nil, err
	}
	if outcome == execution.OutcomeError && problem == nil {
		return nil, invalidArtifact(path+".error", "is required for outcome error")
	}
	if outcome != execution.OutcomeError && problem != nil {
		return nil, invalidArtifact(path+".error", "must be absent for outcome %q", outcome.String())
	}
	return &transcript.RunResult{
		Usage: usage, Steps: steps, Error: problem,
		Duration: time.Duration(result.DurationMs) * time.Millisecond,
	}, nil
}

func canonicalUsage(path string, usage *protocol.Usage) (*transcript.Usage, error) {
	if usage == nil {
		return nil, nil
	}
	total, err := canonicalModelUsage(path, usage.ModelUsage)
	if err != nil {
		return nil, err
	}
	out := &transcript.Usage{ModelUsage: total}
	if len(usage.ByModel) > 0 {
		out.ByModel = make(map[string]transcript.ModelUsage, len(usage.ByModel))
		for model, modelUsage := range usage.ByModel {
			if model == "" {
				return nil, invalidArtifact(path+".byModel", "contains an empty model id")
			}
			converted, err := canonicalModelUsage(path+".byModel["+model+"]", modelUsage)
			if err != nil {
				return nil, err
			}
			out.ByModel[model] = converted
		}
	}
	return out, nil
}

func canonicalModelUsage(path string, usage protocol.ModelUsage) (transcript.ModelUsage, error) {
	if usage.InputTokens < 0 || usage.OutputTokens < 0 || usage.CacheReadTokens < 0 || usage.CacheWriteTokens < 0 || usage.ReasoningTokens < 0 {
		return transcript.ModelUsage{}, invalidArtifact(path, "token counts must not be negative")
	}
	if usage.CostUSD != nil && *usage.CostUSD < 0 {
		return transcript.ModelUsage{}, invalidArtifact(path+".costUsd", "must not be negative")
	}
	return transcript.ModelUsage{
		InputTokens: usage.InputTokens, OutputTokens: usage.OutputTokens,
		CacheReadTokens: usage.CacheReadTokens, CacheWriteTokens: usage.CacheWriteTokens,
		ReasoningTokens: usage.ReasoningTokens, CostUSD: usage.CostUSD,
	}, nil
}
