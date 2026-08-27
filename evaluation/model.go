package evaluation

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/samber/lo"

	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
)

const modelReportOutputName = "evaluation_report"

// ModelEvaluatorConfig configures a model-backed evaluator. PromptTemplate is
// rendered over .Input, .Output, and .Context. A nil Threshold selects
// [DefaultThreshold].
type ModelEvaluatorConfig struct {
	Model          chat.Model
	PromptTemplate *chatclient.Template
	Threshold      *Score
}

func (config ModelEvaluatorConfig) threshold() (Score, error) {
	threshold, err := config.Threshold.valueOr(DefaultThreshold)
	if err != nil {
		return 0, fmt.Errorf("%w: threshold: %w", ErrInvalidEvaluatorConfig, err)
	}
	return threshold, nil
}

func (config ModelEvaluatorConfig) prompt(fallback string, required ...string) (*chatclient.Template, error) {
	prompt := config.PromptTemplate
	if prompt == nil {
		var err error
		prompt, err = chatclient.ParseTemplate(fallback)
		if err != nil {
			return nil, fmt.Errorf("%w: default prompt: %w", ErrInvalidEvaluatorConfig, err)
		}
	}
	if err := prompt.Require(required...); err != nil {
		return nil, fmt.Errorf("%w: prompt: %w", ErrInvalidEvaluatorConfig, err)
	}
	if _, err := prompt.Render(promptVariables{}); err != nil {
		return nil, fmt.Errorf("%w: prompt: %w", ErrInvalidEvaluatorConfig, err)
	}
	return prompt, nil
}

type promptVariables struct {
	Input   string
	Output  string
	Context string
}

type modelReport struct {
	Score    Score  `json:"score" jsonschema:"minimum=0,maximum=1"`
	Feedback string `json:"feedback,omitzero"`
}

func (output modelReport) report(metric Metric, threshold Score) (Report, error) {
	report := Report{
		Metric:   metric,
		Passed:   output.Score.Passes(threshold),
		Score:    output.Score,
		Feedback: strings.TrimSpace(output.Feedback),
	}
	if err := report.Validate(); err != nil {
		return Report{}, fmt.Errorf("evaluation: model report: %w", err)
	}
	return report, nil
}

type modelEvaluator struct {
	generation chatclient.Generation[modelReport]
	metric     Metric
	prompt     *chatclient.Template
	threshold  Score
}

func newModelEvaluator(
	config ModelEvaluatorConfig,
	metric Metric,
	defaultPrompt string,
	required ...string,
) (*modelEvaluator, error) {
	if lo.IsNil(config.Model) {
		return nil, fmt.Errorf("%w: model is nil", ErrInvalidEvaluatorConfig)
	}
	threshold, err := config.threshold()
	if err != nil {
		return nil, err
	}
	prompt, err := config.prompt(defaultPrompt, required...)
	if err != nil {
		return nil, err
	}
	client, err := chatclient.New(config.Model, chatclient.Config{})
	if err != nil {
		return nil, fmt.Errorf("%w: model: %w", ErrInvalidEvaluatorConfig, err)
	}
	format, err := chatclient.JSONSchema[modelReport](modelReportOutputName)
	if err != nil {
		return nil, fmt.Errorf("%w: output format: %w", ErrInvalidEvaluatorConfig, err)
	}
	return &modelEvaluator{
		generation: client.Output(format),
		metric:     metric,
		prompt:     prompt,
		threshold:  threshold,
	}, nil
}

func (evaluator *modelEvaluator) evaluate(ctx context.Context, sample TextSample) (Report, error) {
	if err := ctx.Err(); err != nil {
		return Report{}, err
	}

	message, err := evaluator.prompt.UserMessage(promptVariables{
		Input: sample.Input, Output: sample.Output, Context: sample.ContextText(),
	})
	if err != nil {
		return Report{}, fmt.Errorf("evaluation: render prompt: %w", err)
	}
	output, err := evaluator.generation.Call(ctx, &chat.Request{Messages: []chat.Message{message}})
	if err != nil {
		if errors.Is(err, chatclient.ErrInvalidOutput) {
			return Report{}, fmt.Errorf("%w: model output: %w", ErrInvalidReport, err)
		}
		return Report{}, fmt.Errorf("evaluation: generate report: %w", err)
	}
	return output.report(evaluator.metric, evaluator.threshold)
}
