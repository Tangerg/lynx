package trajectory

import (
	"bytes"
	"context"
	"fmt"

	"github.com/Tangerg/scope/core/metadata"
	"github.com/Tangerg/scope/eval"
)

const (
	metricNamespace                       = "agent"
	metricMaximumKey                      = "maximum"
	metricUnitCount                       = "count"
	metricUnitToken                       = "token"
	metricUnitSecond                      = "s"
	MetricTrajectory      eval.MetricName = "trajectory"
	MetricTaskSuccess     eval.MetricName = "task_success"
	MetricToolCalls       eval.MetricName = "tool_calls"
	MetricConsistency     eval.MetricName = "consistency"
	MetricCommittedSteps  eval.MetricName = "committed_steps"
	MetricPreparedEffects eval.MetricName = "prepared_effects"
	MetricAcceptedSignals eval.MetricName = "accepted_signals"
	MetricDroppedDeltas   eval.MetricName = "dropped_deltas"
	MetricTotalTokens     eval.MetricName = "total_tokens"
	MetricDuration        eval.MetricName = "duration"
)

// Evaluator deterministically checks terminal success, exact Tool behavior,
// replay consistency, and configured resource regressions for one Sample.
// Its zero value is ready to use because all case-specific policy belongs to
// the typed Sample rather than mutable evaluator configuration.
type Evaluator struct{}

func (Evaluator) Evaluate(ctx context.Context, sample Sample) (eval.Report, error) {
	if err := ctx.Err(); err != nil {
		return eval.Report{}, err
	}
	if err := sample.Validate(); err != nil {
		return eval.Report{}, err
	}
	details := make([]eval.Report, 0, 9)
	task, err := taskReport(sample)
	if err != nil {
		return eval.Report{}, err
	}
	details = append(details, task)
	if sample.Expected.Tools != nil {
		tools, toolErr := toolReport(sample.Actual.toolCalls, sample.Expected.Tools.Calls)
		if toolErr != nil {
			return eval.Report{}, toolErr
		}
		details = append(details, tools)
	}
	if sample.Expected.Baseline != nil {
		consistency, consistencyErr := consistencyReport(sample.Actual, *sample.Expected.Baseline)
		if consistencyErr != nil {
			return eval.Report{}, consistencyErr
		}
		details = append(details, consistency)
	}
	resourceReports, err := limitReports(sample.Actual, sample.Expected.Limits)
	if err != nil {
		return eval.Report{}, err
	}
	details = append(details, resourceReports...)
	metric, err := eval.NewMetric(eval.MetricConfig{Namespace: metricNamespace, Name: MetricTrajectory})
	if err != nil {
		return eval.Report{}, err
	}
	verdict := eval.VerdictPass
	for _, detail := range details {
		if detail.Verdict == eval.VerdictFail {
			verdict = eval.VerdictFail
			break
		}
	}
	report := eval.Report{Metric: metric, Verdict: verdict, Details: details}
	if err := report.Validate(); err != nil {
		return eval.Report{}, err
	}
	return report, nil
}

func taskReport(sample Sample) (eval.Report, error) {
	passed := sample.Actual.termination.Status() == sample.Expected.Status
	feedback := "terminal status matched"
	if !passed {
		feedback = fmt.Sprintf(
			"terminal status was %s, expected %s",
			sample.Actual.termination.Status(), sample.Expected.Status,
		)
	}
	if passed && sample.Expected.Output != nil {
		passed = sample.Actual.output != nil &&
			bytes.Equal(sample.Actual.output.JSON(), sample.Expected.Output.JSON())
		if passed {
			feedback = "terminal status and output matched"
		} else {
			feedback = "terminal output differed from the expected value"
		}
	}
	return binaryReport(MetricTaskSuccess, passed, feedback)
}

func toolReport(actual []ToolCall, expected []ToolExpectation) (eval.Report, error) {
	passed := len(actual) == len(expected)
	feedback := fmt.Sprintf("observed the expected %d Tool calls", len(expected))
	if !passed {
		feedback = fmt.Sprintf("observed %d Tool calls, expected %d", len(actual), len(expected))
	}
	for index := 0; passed && index < len(expected); index++ {
		matches, err := toolMatches(actual[index], expected[index])
		if err != nil {
			return eval.Report{}, err
		}
		if !matches {
			passed = false
			feedback = fmt.Sprintf("Tool call %d did not match its expected name, arguments, or outcome", index)
		}
	}
	return binaryReport(MetricToolCalls, passed, feedback)
}

func toolMatches(actual ToolCall, expected ToolExpectation) (bool, error) {
	if actual.Call.Name != expected.Name {
		return false, nil
	}
	if expected.Arguments != nil {
		actualArguments, err := canonicalArguments(actual.Call.Arguments)
		if err != nil {
			return false, err
		}
		expectedArguments, err := canonicalArguments(string(*expected.Arguments))
		if err != nil {
			return false, err
		}
		if !bytes.Equal(actualArguments, expectedArguments) {
			return false, nil
		}
	}
	return expected.Outcome == ToolOutcomeInvalid || actual.Outcome == expected.Outcome, nil
}

func consistencyReport(actual, baseline Trajectory) (eval.Report, error) {
	actualDigest, err := actual.BehaviorDigest()
	if err != nil {
		return eval.Report{}, err
	}
	baselineDigest, err := baseline.BehaviorDigest()
	if err != nil {
		return eval.Report{}, err
	}
	passed := actualDigest == baselineDigest
	feedback := "semantic trajectory matched the replay baseline"
	if !passed {
		feedback = "semantic trajectory differed from the replay baseline"
	}
	return binaryReport(MetricConsistency, passed, feedback)
}

func limitReports(actual Trajectory, limits Limits) ([]eval.Report, error) {
	reports := make([]eval.Report, 0, 6)
	if limits.CommittedSteps != nil {
		report, err := measurementReport(
			MetricCommittedSteps, metricUnitCount,
			float64(actual.usage.CommittedSteps), actual.usage.CommittedSteps <= *limits.CommittedSteps,
			*limits.CommittedSteps,
		)
		if err != nil {
			return nil, err
		}
		reports = append(reports, report)
	}
	if limits.PreparedEffects != nil {
		report, err := measurementReport(
			MetricPreparedEffects, metricUnitCount,
			float64(actual.usage.PreparedEffects), actual.usage.PreparedEffects <= *limits.PreparedEffects,
			*limits.PreparedEffects,
		)
		if err != nil {
			return nil, err
		}
		reports = append(reports, report)
	}
	if limits.AcceptedSignals != nil {
		report, err := measurementReport(
			MetricAcceptedSignals, metricUnitCount,
			float64(actual.usage.AcceptedSignals), actual.usage.AcceptedSignals <= *limits.AcceptedSignals,
			*limits.AcceptedSignals,
		)
		if err != nil {
			return nil, err
		}
		reports = append(reports, report)
	}
	if limits.DroppedDeltas != nil {
		report, err := measurementReport(
			MetricDroppedDeltas, metricUnitCount,
			float64(actual.usage.DroppedDeltas), actual.usage.DroppedDeltas <= *limits.DroppedDeltas,
			*limits.DroppedDeltas,
		)
		if err != nil {
			return nil, err
		}
		reports = append(reports, report)
	}
	if limits.TotalTokens != nil {
		tokens, err := actual.TotalTokens()
		if err != nil {
			return nil, err
		}
		report, err := measurementReport(
			MetricTotalTokens, metricUnitToken,
			float64(tokens), tokens <= *limits.TotalTokens, *limits.TotalTokens,
		)
		if err != nil {
			return nil, err
		}
		reports = append(reports, report)
	}
	if limits.Duration != nil {
		report, err := measurementReport(
			MetricDuration, metricUnitSecond,
			actual.duration.Seconds(), actual.duration <= *limits.Duration,
			limits.Duration.Seconds(),
		)
		if err != nil {
			return nil, err
		}
		reports = append(reports, report)
	}
	return reports, nil
}

func binaryReport(name eval.MetricName, passed bool, feedback string) (eval.Report, error) {
	metric, err := eval.NewMetric(eval.MetricConfig{Namespace: metricNamespace, Name: name})
	if err != nil {
		return eval.Report{}, err
	}
	score := eval.Score(0)
	verdict := eval.VerdictFail
	if passed {
		score = 1
		verdict = eval.VerdictPass
	}
	report := eval.Report{Metric: metric, Verdict: verdict, Score: &score, Feedback: feedback}
	if err := report.Validate(); err != nil {
		return eval.Report{}, err
	}
	return report, nil
}

func measurementReport[Maximum uint64 | int64 | float64](
	name eval.MetricName,
	unit string,
	measurement float64,
	passed bool,
	maximum Maximum,
) (eval.Report, error) {
	parameters := metadata.Map{}
	if err := parameters.Set(metricMaximumKey, maximum); err != nil {
		return eval.Report{}, fmt.Errorf("eval/trajectory: metric maximum: %w", err)
	}
	metric, err := eval.NewMetric(eval.MetricConfig{
		Namespace: metricNamespace, Name: name, Unit: unit,
		Direction: eval.DirectionLowerIsBetter, Parameters: parameters,
	})
	if err != nil {
		return eval.Report{}, err
	}
	verdict := eval.VerdictFail
	if passed {
		verdict = eval.VerdictPass
	}
	report := eval.Report{Metric: metric, Verdict: verdict, Measurement: &measurement}
	if err := report.Validate(); err != nil {
		return eval.Report{}, err
	}
	return report, nil
}

var _ eval.Evaluator[Sample] = Evaluator{}
