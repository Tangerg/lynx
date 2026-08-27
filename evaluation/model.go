package evaluation

import (
	"context"
	"fmt"
	"strings"

	"github.com/samber/lo"

	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/chatclient"
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

const modelReportOutputName = "evaluation_report"

// ModelConfig configures a model-backed evaluator. PromptTemplate is rendered
// over .Input, .Output, and .Context. A nil Threshold selects
// [DefaultThreshold]; a non-nil value must be in [0, 1].
type ModelConfig struct {
	Model          chat.Model
	PromptTemplate *chatclient.Template
	Threshold      *Score
}

func (c ModelConfig) threshold() (Score, error) {
	threshold, err := resolveThreshold(c.Threshold)
	if err != nil {
		return 0, fmt.Errorf("%w: threshold: %w", ErrInvalidConfig, err)
	}
	return threshold, nil
}

func (c ModelConfig) prompt(fallback string, required ...string) (*chatclient.Template, error) {
	prompt := c.PromptTemplate
	if prompt == nil {
		var err error
		prompt, err = chatclient.ParseTemplate(fallback)
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
	return prompt, nil
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

func (m modelReport) report(metric Metric, threshold Score) (Report, error) {
	report := Report{
		Metric:   metric,
		Passed:   m.Score.Passes(threshold),
		Score:    m.Score,
		Feedback: strings.TrimSpace(m.Feedback),
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
	config ModelConfig,
	metric Metric,
	defaultPrompt string,
	required ...string,
) (*modelEvaluator, error) {
	if lo.IsNil(config.Model) {
		return nil, fmt.Errorf("%w: nil model", ErrInvalidConfig)
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
		return nil, fmt.Errorf("%w: model: %w", ErrInvalidConfig, err)
	}
	format, err := chatclient.JSONSchema[modelReport](modelReportOutputName, []byte(modelReportSchema))
	if err != nil {
		return nil, fmt.Errorf("%w: output format: %w", ErrInvalidConfig, err)
	}
	return &modelEvaluator{
		generation: client.Output(format),
		metric:     metric,
		prompt:     prompt,
		threshold:  threshold,
	}, nil
}

func (m *modelEvaluator) evaluate(ctx context.Context, sample TextSample) (Report, error) {
	if err := ctx.Err(); err != nil {
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
	return output.report(m.metric, m.threshold)
}
