package runs

import (
	"errors"
	"fmt"
	"math"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/accounting"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

func (r *reducer) segmentEnd(e SegmentEnded) ([]RunEvent, error) {
	if e.Reason != run.OutcomeFailed && e.Reason != run.OutcomeTimedOut && e.Problem != nil {
		return nil, errors.New("outcome does not allow a problem")
	}
	if e.Usage != nil {
		if err := r.applyUsage(*e.Usage); err != nil {
			return nil, err
		}
	}
	r.segmentDuration = e.Duration
	var failure *transcript.Problem
	detail := ""
	switch e.Reason {
	case run.OutcomeFailed, run.OutcomeTimedOut:
		if e.Problem == nil {
			return nil, errors.New("failure outcome is missing a problem")
		}
		var err error
		failure, err = runResultProblem(*e.Problem)
		if err != nil {
			return nil, err
		}
	case run.OutcomeCanceled:
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

func (r *reducer) runRecord(state run.RunState) transcript.Run {
	// Only a running Run names a segment: the record that parks or ends it clears
	// the identity in the same commit, so nothing can attach to a stream that
	// stopped.
	activeSegment := ""
	if state == run.Running {
		activeSegment = r.cfg.SegmentID
	}
	return transcript.Run{
		SessionID:       r.cfg.SessionID,
		ID:              r.cfg.RunID,
		SpawnedByItemID: r.cfg.Lineage.SpawnedByItemID,
		ParentRunID:     r.cfg.Lineage.ParentRunID,
		RootRunID:       r.cfg.Lineage.RootRunID,
		ModelSelection:  r.cfg.ModelSelection,
		GoalLeaseID:     r.cfg.GoalLeaseID,
		State:           state,
		ActiveSegmentID: activeSegment,
		Metrics:         r.metrics(),
		Limits:          r.cfg.Limits,
		Capabilities:    r.cfg.Capabilities,
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
	metrics := r.cfg.Metrics
	if r.usage != nil {
		metrics.Usage = r.usage
	}
	metrics.Steps = r.step
	metrics.ActiveDuration += r.segmentDuration
	return metrics
}

func (r *reducer) applyUsage(reported SegmentUsage) error {
	if reported.Steps < r.step {
		return fmt.Errorf(
			"cumulative model-call count regressed from %d to %d",
			r.step,
			reported.Steps,
		)
	}
	next, err := validatedSegmentUsage(reported)
	if err != nil {
		return err
	}
	previous := r.cfg.Metrics.Usage
	if r.usage != nil {
		previous = r.usage
	}
	if err := validateUsageMonotonic(previous, next); err != nil {
		return err
	}
	r.usage = next
	r.step = reported.Steps
	return nil
}

func (r *reducer) finishedRun(outcome run.Outcome, failure *transcript.Problem, detail string) (SegmentFinished, error) {
	state, ok := run.Running.Terminate(outcome)
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

func transcriptUsage(reported SegmentUsage) *transcript.Usage {
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
				model.ReasoningTokens,
				model.CacheReadTokens,
				model.CacheWriteTokens,
				model.CostUSD,
			)
		}
	}
	return usage
}

func validatedSegmentUsage(reported SegmentUsage) (*transcript.Usage, error) {
	if reported.Steps < 0 {
		return nil, fmt.Errorf("model-call count %d is negative", reported.Steps)
	}
	total := accounting.ModelUsage{
		Model:      "total",
		TokenUsage: reported.Tokens,
		CostUSD:    reported.CostUSD,
		Calls:      max(reported.Steps, 1),
	}
	if err := total.Validate(); err != nil {
		return nil, fmt.Errorf("total usage: %w", err)
	}
	if reported.Steps == 0 &&
		(reported.Tokens != (accounting.TokenUsage{}) || reported.CostUSD != 0) {
		return nil, errors.New("zero model calls carry non-zero token or cost usage")
	}
	if len(reported.ByModel) > 0 {
		aggregate, err := (accounting.Snapshot{Models: reported.ByModel}).Total()
		if err != nil {
			return nil, fmt.Errorf("per-model usage: %w", err)
		}
		if aggregate.TokenUsage != reported.Tokens ||
			aggregate.Calls != reported.Steps ||
			!sameUsageCost(aggregate.CostUSD, reported.CostUSD) {
			return nil, fmt.Errorf(
				"per-model aggregate {tokens:%+v cost:%g calls:%d} does not match total {tokens:%+v cost:%g calls:%d}",
				aggregate.TokenUsage,
				aggregate.CostUSD,
				aggregate.Calls,
				reported.Tokens,
				reported.CostUSD,
				reported.Steps,
			)
		}
	}
	return transcriptUsage(reported), nil
}

func validateUsageMonotonic(previous, next *transcript.Usage) error {
	if previous == nil {
		return nil
	}
	if next == nil {
		return errors.New("cumulative usage disappeared after it was reported")
	}
	if err := validateModelUsageMonotonic("total", previous.ModelUsage, next.ModelUsage); err != nil {
		return err
	}
	for model, previousModel := range previous.ByModel {
		nextModel, ok := next.ByModel[model]
		if !ok {
			return fmt.Errorf("cumulative usage dropped model %q", model)
		}
		if err := validateModelUsageMonotonic(model, previousModel, nextModel); err != nil {
			return err
		}
	}
	return nil
}

func validateModelUsageMonotonic(label string, previous, next transcript.ModelUsage) error {
	if next.InputTokens < previous.InputTokens ||
		next.OutputTokens < previous.OutputTokens ||
		next.ReasoningTokens < previous.ReasoningTokens ||
		next.CacheReadTokens < previous.CacheReadTokens ||
		next.CacheWriteTokens < previous.CacheWriteTokens ||
		usageCost(next.CostUSD) < usageCost(previous.CostUSD) {
		return fmt.Errorf(
			"cumulative usage for %q regressed from %+v to %+v",
			label,
			previous,
			next,
		)
	}
	return nil
}

func usageCost(cost *float64) float64 {
	if cost == nil {
		return 0
	}
	return *cost
}

func sameUsageCost(left, right float64) bool {
	scale := max(1, math.Abs(left), math.Abs(right))
	return math.Abs(left-right) <= 1e-12*scale
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
