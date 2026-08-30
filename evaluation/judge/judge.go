// Package judge evaluates arbitrary subjects with a structured-output chat
// model. Subject validation and prompt construction remain owned by callers.
package judge

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/samber/lo"

	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
	"github.com/Tangerg/scope/core/metadata"
	"github.com/Tangerg/scope/evaluation"
)

const (
	metricJudgeConfigurationKey = "judge"
	reportSampleScoresKey       = "sample_scores"
)

const outputName = "evaluation_report"

type aggregation string

const aggregationMedian aggregation = "median"

type Prompt[T any] func(T) (chat.Message, error)

type Config[T any] struct {
	Model  chat.Model
	Metric evaluation.Metric
	Prompt Prompt[T]
	// Threshold is optional. Without one, evaluation produces a score without
	// inventing a pass/fail decision.
	Threshold *evaluation.Score
	Samples   int
}

type modelReport struct {
	Score    evaluation.Score `json:"score" jsonschema:"minimum=0,maximum=1"`
	Feedback string           `json:"feedback,omitzero"`
}

type metricConfiguration struct {
	Aggregation aggregation       `json:"aggregation"`
	Samples     int               `json:"samples"`
	Threshold   *evaluation.Score `json:"threshold,omitzero"`
}

type Evaluator[T any] struct {
	generation chatclient.Generation[modelReport]
	metric     evaluation.Metric
	prompt     Prompt[T]
	threshold  *evaluation.Score
	samples    int
}

func NewEvaluator[T any](config Config[T]) (*Evaluator[T], error) {
	if lo.IsNil(config.Model) {
		return nil, fmt.Errorf("%w: model is nil", evaluation.ErrInvalidEvaluatorConfig)
	}
	if err := config.Metric.Validate(); err != nil {
		return nil, fmt.Errorf("%w: metric: %w", evaluation.ErrInvalidEvaluatorConfig, err)
	}
	if config.Prompt == nil {
		return nil, fmt.Errorf("%w: prompt is nil", evaluation.ErrInvalidEvaluatorConfig)
	}
	var threshold *evaluation.Score
	if config.Threshold != nil {
		value := *config.Threshold
		if err := value.Validate(); err != nil {
			return nil, fmt.Errorf("%w: threshold: %w", evaluation.ErrInvalidEvaluatorConfig, err)
		}
		threshold = &value
	}
	if config.Samples < 0 {
		return nil, fmt.Errorf("%w: samples must not be negative", evaluation.ErrInvalidEvaluatorConfig)
	}
	samples := config.Samples
	if samples == 0 {
		samples = 1
	}
	metric, err := configuredMetric(config.Metric, threshold, samples)
	if err != nil {
		return nil, fmt.Errorf("%w: metric configuration: %w", evaluation.ErrInvalidEvaluatorConfig, err)
	}
	client, err := chatclient.New(config.Model, chatclient.Config{})
	if err != nil {
		return nil, fmt.Errorf("%w: model: %w", evaluation.ErrInvalidEvaluatorConfig, err)
	}
	format, err := chatclient.JSONSchema[modelReport](outputName)
	if err != nil {
		return nil, fmt.Errorf("%w: output format: %w", evaluation.ErrInvalidEvaluatorConfig, err)
	}
	return &Evaluator[T]{
		generation: client.Output(format), metric: metric, prompt: config.Prompt,
		threshold: threshold, samples: samples,
	}, nil
}

func configuredMetric(metric evaluation.Metric, threshold *evaluation.Score, samples int) (evaluation.Metric, error) {
	parameters := metric.Parameters.Clone()
	if err := parameters.Set(metricJudgeConfigurationKey, metricConfiguration{
		Aggregation: aggregationMedian, Samples: samples, Threshold: threshold,
	}); err != nil {
		return evaluation.Metric{}, err
	}
	return evaluation.NewMetric(evaluation.MetricConfig{
		Namespace:  metric.Namespace,
		Name:       metric.Name,
		Unit:       metric.Unit,
		Direction:  metric.Direction,
		Parameters: parameters,
	})
}

func (evaluator *Evaluator[T]) Evaluate(ctx context.Context, subject T) (evaluation.Report, error) {
	if err := ctx.Err(); err != nil {
		return evaluation.Report{}, err
	}
	message, err := evaluator.prompt(subject)
	if err != nil {
		return evaluation.Report{}, fmt.Errorf("evaluation/judge: build prompt: %w", err)
	}
	if err := message.Validate(); err != nil {
		return evaluation.Report{}, fmt.Errorf("evaluation/judge: prompt: %w", err)
	}

	outputs := make([]modelReport, evaluator.samples)
	for index := range evaluator.samples {
		output, callErr := evaluator.generation.Call(ctx, &chat.Request{Messages: []chat.Message{message.Clone()}})
		if callErr != nil {
			if errors.Is(callErr, chatclient.ErrInvalidOutput) {
				return evaluation.Report{}, fmt.Errorf("%w: model output: %w", evaluation.ErrInvalidReport, callErr)
			}
			return evaluation.Report{}, fmt.Errorf("evaluation/judge: sample %d: %w", index, callErr)
		}
		if err := output.Score.Validate(); err != nil {
			return evaluation.Report{}, fmt.Errorf("%w: sample %d score: %w", evaluation.ErrInvalidReport, index, err)
		}
		outputs[index] = output
	}
	return evaluator.aggregate(outputs)
}

func (evaluator *Evaluator[T]) aggregate(outputs []modelReport) (evaluation.Report, error) {
	slices.SortFunc(outputs, func(a, b modelReport) int {
		if a.Score < b.Score {
			return -1
		}
		if a.Score > b.Score {
			return 1
		}
		return 0
	})
	middle := len(outputs) / 2
	score := outputs[middle].Score
	feedback := strings.TrimSpace(outputs[middle].Feedback)
	if len(outputs)%2 == 0 {
		score = (outputs[middle-1].Score + outputs[middle].Score) / 2
		feedback = strings.TrimSpace(strings.Join([]string{outputs[middle-1].Feedback, outputs[middle].Feedback}, "\n\n"))
	}
	reportMetadata := metadata.Map{}
	if len(outputs) > 1 {
		scores := make([]evaluation.Score, len(outputs))
		for index := range outputs {
			scores[index] = outputs[index].Score
		}
		if err := reportMetadata.Set(reportSampleScoresKey, scores); err != nil {
			return evaluation.Report{}, fmt.Errorf("evaluation/judge: sample metadata: %w", err)
		}
	}
	verdict := evaluation.VerdictUnspecified
	if evaluator.threshold != nil {
		decided, err := score.Verdict(*evaluator.threshold)
		if err != nil {
			return evaluation.Report{}, fmt.Errorf("evaluation/judge: verdict: %w", err)
		}
		verdict = decided
	}
	report := evaluation.Report{
		Metric: evaluator.metric.Clone(), Verdict: verdict, Score: &score,
		Feedback: feedback, Metadata: reportMetadata,
	}
	if err := report.Validate(); err != nil {
		return evaluation.Report{}, fmt.Errorf("evaluation/judge: report: %w", err)
	}
	return report, nil
}

var _ evaluation.Evaluator[struct{}] = (*Evaluator[struct{}])(nil)
