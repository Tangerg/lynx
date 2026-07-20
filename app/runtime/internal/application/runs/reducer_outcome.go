package runs

import (
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

func (r *reducer) turnEnd(e TurnEnd) ([]RunEvent, error) {
	result := &RunResult{
		Usage: r.turnUsage(e), Steps: r.step, Duration: e.Duration,
	}
	detail := ""
	switch e.Reason {
	case execution.OutcomeError:
		result.Error = r.runProblem()
	case execution.OutcomeMaxBudget:
		detail = budgetDetail(e)
	case execution.OutcomeMaxSteps:
		detail = stepDetail(e)
	case execution.OutcomeCanceled:
		if r.cfg.CancelReason != nil {
			detail = r.cfg.CancelReason()
		}
	}
	terminal, err := r.finishedRun(e.Reason, result, detail)
	if err != nil {
		return nil, err
	}
	out := r.closeStreaming()
	drained, err := r.drainTools()
	if err != nil {
		return nil, err
	}
	out = append(out, drained...)
	return append(out, terminal), nil
}

func (r *reducer) runRecord(state execution.RunState) transcript.Run {
	return transcript.Run{
		SessionID:   r.cfg.SessionID,
		ID:          r.cfg.RunID,
		Provider:    r.cfg.Provider,
		Model:       r.cfg.Model,
		State:       state,
		CreatedAt:   r.cfg.CreatedAt,
		UpdatedAt:   r.now(),
		MessageMark: -1,
	}
}

func (r *reducer) finishedRun(outcome execution.Outcome, result *RunResult, detail string) (SegmentFinished, error) {
	state, ok := execution.Running.Terminate(outcome)
	if !ok {
		return SegmentFinished{}, fmt.Errorf("outcome %d does not terminate a running run", outcome)
	}
	run := r.runRecord(state)
	run.Outcome = &outcome
	run.Result = result
	run.Detail = detail
	run.FinishedAt = r.now()
	return SegmentFinished{Run: run}, nil
}

func budgetDetail(e TurnEnd) string {
	switch {
	case e.MaxCostUSD > 0:
		return fmt.Sprintf("spent $%.2f of $%.2f budget", e.CostUSD, e.MaxCostUSD)
	case e.MaxBudget > 0:
		return fmt.Sprintf("reached the %d-token budget", e.MaxBudget)
	default:
		return "reached the configured budget"
	}
}

func stepDetail(e TurnEnd) string {
	if e.MaxSteps > 0 {
		return fmt.Sprintf("reached the %d-step limit", e.MaxSteps)
	}
	return "reached the configured step limit"
}

func (r *reducer) turnUsage(e TurnEnd) *Usage {
	usage := &Usage{ModelUsage: modelUsageFrom(
		e.TokenUsage.PromptTokens,
		e.TokenUsage.CompletionTokens,
		e.TokenUsage.ReasoningTokens,
		e.TokenUsage.CacheReadTokens,
		e.TokenUsage.CacheWriteTokens,
		e.CostUSD,
	)}
	if len(e.UsageByModel) > 0 {
		usage.ByModel = make(map[string]transcript.ModelUsage, len(e.UsageByModel))
		for _, model := range e.UsageByModel {
			usage.ByModel[model.Model] = modelUsageFrom(
				model.PromptTokens,
				model.CompletionTokens,
				0,
				model.CacheReadTokens,
				model.CacheWriteTokens,
				model.CostUSD,
			)
		}
	}
	return usage
}

func modelUsageFrom(prompt, completion, reasoning, cacheRead, cacheWrite int64, cost float64) ModelUsage {
	return ModelUsage{
		InputTokens: prompt, OutputTokens: completion,
		ReasoningTokens: reasoning, CacheReadTokens: cacheRead,
		CacheWriteTokens: cacheWrite, CostUSD: optCostUSD(cost),
	}
}

func optCostUSD(cost float64) *float64 {
	if cost <= 0 {
		return nil
	}
	return &cost
}
