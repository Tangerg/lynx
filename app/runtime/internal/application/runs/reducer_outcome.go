package runs

import (
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/accounting"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
)

func (r *reducer) segmentEnd(e SegmentEnded) ([]RunEvent, error) {
	if e.Reason != run.OutcomeFailed && e.Reason != run.OutcomeTimedOut && e.Reason != run.OutcomeLost && e.Failure != nil {
		return nil, errors.New("outcome does not allow a failure")
	}
	if e.Usage != nil {
		if err := r.applyUsage(*e.Usage); err != nil {
			return nil, err
		}
	}
	r.segmentDuration = e.Duration
	var failure *run.Failure
	detail := ""
	switch e.Reason {
	case run.OutcomeFailed, run.OutcomeTimedOut, run.OutcomeLost:
		if e.Failure == nil {
			return nil, errors.New("failure outcome is missing a failure")
		}
		failure = e.Failure
	case run.OutcomeCanceled:
		if r.cfg.CancelReason != nil {
			detail = r.cfg.CancelReason()
		}
	}
	terminal, err := r.finishedRun(e.Reason, failure, detail)
	if err != nil {
		return nil, err
	}
	out, err := r.closeStreaming()
	if err != nil {
		return nil, err
	}
	drained, err := r.drainTools()
	if err != nil {
		return nil, err
	}
	out = append(out, drained...)
	return append(out, terminal), nil
}

func (r *reducer) runRecord(state run.State) (run.Run, error) {
	if state != run.Running && state != run.Waiting {
		return run.Run{}, fmt.Errorf("reducer cannot project non-terminal state %s", state)
	}
	metrics, err := r.metrics()
	if err != nil {
		return run.Run{}, err
	}
	updatedAt := r.now().UTC()
	createdAt := r.cfg.CreatedAt.UTC()
	if createdAt.IsZero() {
		createdAt = updatedAt
	}
	current, err := run.Restore(run.Snapshot{
		SessionID: r.cfg.SessionID, ID: r.cfg.RunID, Lineage: r.cfg.Lineage,
		ModelSelection: r.cfg.ModelSelection, GoalIncarnationID: r.cfg.GoalIncarnationID,
		State: run.Running, ActiveSegmentID: r.cfg.SegmentID,
		Metrics: metrics, Limits: r.cfg.Limits, Capabilities: r.cfg.Capabilities,
		CreatedAt: createdAt, UpdatedAt: updatedAt, MessageMark: run.UnknownMessageMark,
	})
	if err != nil {
		return run.Run{}, fmt.Errorf("project Run: %w", err)
	}
	if state == run.Waiting {
		return current.Suspend(updatedAt)
	}
	return current, nil
}

// metrics is the Run's cumulative consumption as of now: what it brought into
// this segment plus what this segment has reported. Every committed Run record
// goes through here, which is what makes the sequence non-decreasing — the seed
// is fixed for the segment and the segment's own figures only grow.
func (r *reducer) metrics() (run.Metrics, error) {
	usage, reported := r.cfg.Metrics.Usage()
	if r.usage != nil {
		usage, reported = r.usage.Clone(), true
	}
	var usageRef *accounting.Usage
	if reported {
		usageRef = &usage
	}
	activeDuration := r.cfg.Metrics.ActiveDuration()
	if r.segmentDuration < 0 || (r.segmentDuration > 0 && activeDuration > time.Duration(math.MaxInt64)-r.segmentDuration) {
		return run.Metrics{}, errors.New("segment active duration is invalid or overflows")
	}
	return run.NewMetrics(usageRef, r.step, activeDuration+r.segmentDuration)
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
	var previous *accounting.Usage
	if value, reported := r.cfg.Metrics.Usage(); reported {
		previous = &value
	}
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

func (r *reducer) finishedRun(outcome run.Outcome, failure *run.Failure, detail string) (SegmentFinished, error) {
	if _, ok := run.Running.Terminate(outcome); !ok {
		return SegmentFinished{}, fmt.Errorf("outcome %d does not terminate a running run", outcome)
	}
	current, err := r.runRecord(run.Running)
	if err != nil {
		return SegmentFinished{}, err
	}
	terminal, err := current.Terminate(run.Termination{
		Outcome: outcome, Detail: detail, Failure: failure,
		FinishedAt: r.now().UTC(), MessageMark: run.UnknownMessageMark,
	})
	if err != nil {
		return SegmentFinished{}, err
	}
	return SegmentFinished{Run: terminal}, nil
}

func transcriptUsage(reported SegmentUsage) *accounting.Usage {
	usage := &accounting.Usage{Total: modelUsageFrom(
		reported.Tokens.PromptTokens,
		reported.Tokens.CompletionTokens,
		reported.Tokens.ReasoningTokens,
		reported.Tokens.CacheReadTokens,
		reported.Tokens.CacheWriteTokens,
		reported.CostUSD,
	)}
	if len(reported.ByModel) > 0 {
		usage.ByModel = make(map[string]accounting.Totals, len(reported.ByModel))
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

func validatedSegmentUsage(reported SegmentUsage) (*accounting.Usage, error) {
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

func validateUsageMonotonic(previous, next *accounting.Usage) error {
	if previous == nil {
		return nil
	}
	if next == nil {
		return errors.New("cumulative usage disappeared after it was reported")
	}
	if err := validateModelUsageMonotonic("total", previous.Total, next.Total); err != nil {
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

func validateModelUsageMonotonic(label string, previous, next accounting.Totals) error {
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

func modelUsageFrom(prompt, completion, reasoning, cacheRead, cacheWrite int64, cost float64) accounting.Totals {
	return accounting.Totals{
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
