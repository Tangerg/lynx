package runs

import (
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

func (r *reducer) turnEnd(e TurnEnd) ([]RunEvent, error) {
	if e.Reason != execution.OutcomeError && e.Problem != nil {
		return nil, fmt.Errorf("non-error outcome carries a problem")
	}
	if e.Usage != nil {
		r.usage = turnUsage(*e.Usage)
	}
	r.segmentDuration = e.Duration
	var failure *transcript.Problem
	detail := ""
	switch e.Reason {
	case execution.OutcomeError:
		if e.Problem == nil {
			return nil, fmt.Errorf("error outcome is missing a problem")
		}
		var err error
		failure, err = runResultProblem(*e.Problem)
		if err != nil {
			return nil, err
		}
	case execution.OutcomeCanceled:
		if r.cfg.CancelReason != nil {
			detail = r.cfg.CancelReason()
		}
	}
	terminal, err := r.finishedRun(e.Reason, failure, detail)
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
	// Only a running Run names a segment: the record that parks or ends it clears
	// the identity in the same commit, so nothing can attach to a stream that
	// stopped.
	activeSegment := ""
	if state == execution.Running {
		activeSegment = r.cfg.SegmentID
	}
	return transcript.Run{
		SessionID:       r.cfg.SessionID,
		ID:              r.cfg.RunID,
		ModelSelection:  r.cfg.ModelSelection,
		State:           state,
		ActiveSegmentID: activeSegment,
		Metrics:         r.metrics(),
		Limits:          r.cfg.Limits,
		CreatedAt:       r.cfg.CreatedAt,
		UpdatedAt:       r.now(),
		MessageMark:     transcript.UnknownMessageMark,
	}
}

// metrics is the Run's cumulative consumption as of now: what it brought into
// this segment plus what this segment has reported. Every committed Run record
// goes through here, which is what makes the sequence non-decreasing — the seed
// is fixed for the segment and the segment's own figures only grow.
func (r *reducer) metrics() transcript.RunMetrics {
	return r.cfg.Metrics.Plus(transcript.RunMetrics{
		Usage:          r.usage,
		Steps:          r.step,
		ActiveDuration: r.segmentDuration,
	})
}

func (r *reducer) finishedRun(outcome execution.Outcome, failure *transcript.Problem, detail string) (SegmentFinished, error) {
	state, ok := execution.Running.Terminate(outcome)
	if !ok {
		return SegmentFinished{}, fmt.Errorf("outcome %d does not terminate a running run", outcome)
	}
	run := r.runRecord(state)
	run.Outcome = &outcome
	run.Error = failure
	run.Detail = detail
	run.FinishedAt = r.now()
	return SegmentFinished{Run: run}, nil
}

func turnUsage(reported TurnUsage) *transcript.Usage {
	usage := &transcript.Usage{ModelUsage: modelUsageFrom(
		reported.Tokens.PromptTokens,
		reported.Tokens.CompletionTokens,
		reported.Tokens.ReasoningTokens,
		reported.Tokens.CacheReadTokens,
		reported.Tokens.CacheWriteTokens,
		reported.CostUSD,
	)}
	if len(reported.ByModel) > 0 {
		usage.ByModel = make(map[string]transcript.ModelUsage, len(reported.ByModel))
		for _, model := range reported.ByModel {
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

func modelUsageFrom(prompt, completion, reasoning, cacheRead, cacheWrite int64, cost float64) transcript.ModelUsage {
	return transcript.ModelUsage{
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
