package evaluation

import (
	"context"
	"fmt"
	"strings"

	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/chatclient"
	"github.com/samber/lo"
)

const modelReportSchema = `{
  "type": "object",
  "properties": {
    "score": {"type": "number", "minimum": 0, "maximum": 1},
    "feedback": {"type": "string"}
  },
  "required": ["score"],
  "additionalProperties": false
}`

// ModelConfig configures a model-backed evaluator. PromptTemplate is rendered
// over .Input, .Output, and .Context. A nil Threshold selects
// [DefaultThreshold]; a non-nil value must be in [0, 1].
type ModelConfig struct {
	Model          chat.Model
	PromptTemplate *chatclient.Template
	Threshold      *Score
}

type promptVariables struct {
	Input   string
	Output  string
	Context string
}

type modelReport struct {
	Score    Score  `json:"score"`
	Feedback string `json:"feedback,omitzero"`
}

type modelEvaluator struct {
	generation     chatclient.Generation[modelReport]
	metric         Metric
	prompt         *chatclient.Template
	threshold      Score
	validateSample func(TextSample) error
}

func newModelEvaluator(
	config ModelConfig,
	metric Metric,
	defaultPrompt string,
	validate func(TextSample) error,
	required ...string,
) (*modelEvaluator, error) {
	if lo.IsNil(config.Model) {
		return nil, fmt.Errorf("%w: nil model", ErrInvalidConfig)
	}
	threshold := DefaultThreshold
	if config.Threshold != nil {
		threshold = *config.Threshold
	}
	if err := threshold.Validate(); err != nil {
		return nil, fmt.Errorf("%w: threshold: %w", ErrInvalidConfig, err)
	}
	prompt := config.PromptTemplate
	if prompt == nil {
		var err error
		prompt, err = chatclient.ParseTemplate(defaultPrompt)
		if err != nil {
			return nil, fmt.Errorf("%w: default prompt: %w", ErrInvalidConfig, err)
		}
	}
	if err := prompt.Require(required...); err != nil {
		return nil, fmt.Errorf("%w: prompt: %w", ErrInvalidConfig, err)
	}
	if _, err := prompt.Render(promptVariables{}); err != nil {
		return nil, fmt.Errorf("%w: prompt: %w", ErrInvalidConfig, err)
	}
	client, err := chatclient.New(config.Model, chatclient.Config{})
	if err != nil {
		return nil, fmt.Errorf("%w: model: %w", ErrInvalidConfig, err)
	}
	format, err := chatclient.JSONSchema[modelReport]("evaluation_report", []byte(modelReportSchema))
	if err != nil {
		return nil, fmt.Errorf("%w: output format: %w", ErrInvalidConfig, err)
	}
	return &modelEvaluator{
		generation:     client.Output(format),
		metric:         metric,
		prompt:         prompt,
		threshold:      threshold,
		validateSample: validate,
	}, nil
}

func (m *modelEvaluator) Evaluate(ctx context.Context, sample TextSample) (Report, error) {
	if err := ctx.Err(); err != nil {
		return Report{}, err
	}
	if err := m.validateSample(sample); err != nil {
		return Report{}, err
	}

	message, err := m.prompt.UserMessage(promptVariables{
		Input: sample.Input, Output: sample.Output, Context: sample.ContextText(),
	})
	if err != nil {
		return Report{}, fmt.Errorf("evaluation: render prompt: %w", err)
	}
	output, err := m.generation.Call(ctx, &chat.Request{Messages: []chat.Message{message}})
	if err != nil {
		return Report{}, fmt.Errorf("evaluation: generate report: %w", err)
	}
	report := Report{
		Metric:   m.metric,
		Passed:   output.Score.Passes(m.threshold),
		Score:    output.Score,
		Feedback: strings.TrimSpace(output.Feedback),
	}
	if err := report.Validate(); err != nil {
		return Report{}, fmt.Errorf("evaluation: model report: %w", err)
	}
	return report, nil
}
