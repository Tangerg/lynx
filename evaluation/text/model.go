package text

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/samber/lo"

	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
	"github.com/Tangerg/scope/evaluation"
)

const (
	modelReportOutputName = "evaluation_report"
	templateInputName     = "Input"
	templateOutputName    = "Output"
	templateContextName   = "Context"
)

// ModelEvaluatorConfig configures the model judge shared by text metrics.
// PromptTemplate is rendered over .Input, .Output, and .Context. A nil
// Threshold selects [evaluation.DefaultThreshold].
type ModelEvaluatorConfig struct {
	Model          chat.Model
	PromptTemplate *chatclient.Template
	Threshold      *evaluation.Score
}

func (config ModelEvaluatorConfig) threshold() (evaluation.Score, error) {
	threshold := evaluation.DefaultThreshold
	if config.Threshold != nil {
		threshold = *config.Threshold
	}
	if err := threshold.Validate(); err != nil {
		return 0, fmt.Errorf("%w: threshold: %w", evaluation.ErrInvalidEvaluatorConfig, err)
	}
	return threshold, nil
}

func (config ModelEvaluatorConfig) prompt(fallback string, required ...string) (*chatclient.Template, error) {
	prompt := config.PromptTemplate
	if prompt == nil {
		var err error
		prompt, err = chatclient.ParseTemplate(fallback)
		if err != nil {
			return nil, fmt.Errorf("%w: default prompt: %w", evaluation.ErrInvalidEvaluatorConfig, err)
		}
	}
	if err := prompt.Require(required...); err != nil {
		return nil, fmt.Errorf("%w: prompt: %w", evaluation.ErrInvalidEvaluatorConfig, err)
	}
	if _, err := prompt.Render(promptVariables{}); err != nil {
		return nil, fmt.Errorf("%w: prompt: %w", evaluation.ErrInvalidEvaluatorConfig, err)
	}
	return prompt, nil
}

type promptVariables struct {
	Input   string
	Output  string
	Context string
}

type modelReport struct {
	Score    evaluation.Score `json:"score" jsonschema:"minimum=0,maximum=1"`
	Feedback string           `json:"feedback,omitzero"`
}

func (output modelReport) report(metric evaluation.Metric, threshold evaluation.Score) (evaluation.Report, error) {
	report := evaluation.Report{
		Metric:   metric,
		Passed:   output.Score.Passes(threshold),
		Score:    output.Score,
		Feedback: strings.TrimSpace(output.Feedback),
	}
	if err := report.Validate(); err != nil {
		return evaluation.Report{}, fmt.Errorf("evaluation/text: model report: %w", err)
	}
	return report, nil
}

type modelEvaluator struct {
	generation chatclient.Generation[modelReport]
	metric     evaluation.Metric
	prompt     *chatclient.Template
	threshold  evaluation.Score
}

func newModelEvaluator(
	config ModelEvaluatorConfig,
	metric evaluation.Metric,
	defaultPrompt string,
	required ...string,
) (*modelEvaluator, error) {
	if lo.IsNil(config.Model) {
		return nil, fmt.Errorf("%w: model is nil", evaluation.ErrInvalidEvaluatorConfig)
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
		return nil, fmt.Errorf("%w: model: %w", evaluation.ErrInvalidEvaluatorConfig, err)
	}
	format, err := chatclient.JSONSchema[modelReport](modelReportOutputName)
	if err != nil {
		return nil, fmt.Errorf("%w: output format: %w", evaluation.ErrInvalidEvaluatorConfig, err)
	}
	return &modelEvaluator{
		generation: client.Output(format), metric: metric, prompt: prompt, threshold: threshold,
	}, nil
}

func (evaluator *modelEvaluator) evaluate(ctx context.Context, sample Sample) (evaluation.Report, error) {
	if err := ctx.Err(); err != nil {
		return evaluation.Report{}, err
	}

	message, err := evaluator.prompt.UserMessage(promptVariables{
		Input: sample.Input, Output: sample.Output, Context: sample.ContextText(),
	})
	if err != nil {
		return evaluation.Report{}, fmt.Errorf("evaluation/text: render prompt: %w", err)
	}
	output, err := evaluator.generation.Call(ctx, &chat.Request{Messages: []chat.Message{message}})
	if err != nil {
		if errors.Is(err, chatclient.ErrInvalidOutput) {
			return evaluation.Report{}, fmt.Errorf("%w: model output: %w", evaluation.ErrInvalidReport, err)
		}
		return evaluation.Report{}, fmt.Errorf("evaluation/text: generate report: %w", err)
	}
	return output.report(evaluator.metric, evaluator.threshold)
}
