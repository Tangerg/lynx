package text

import (
	"fmt"

	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
	"github.com/Tangerg/scope/evaluation"
	"github.com/Tangerg/scope/evaluation/judge"
)

const (
	templateInputName     = "Input"
	templateOutputName    = "Output"
	templateEvidenceName  = "Evidence"
	templateReferenceName = "Reference"
)

// ModelEvaluatorConfig configures model-backed text metrics. Each evaluator
// exposes only the prompt variables its own sample contains. Samples greater
// than one use the median judge score.
type ModelEvaluatorConfig struct {
	Model          chat.Model
	PromptTemplate *chatclient.Template
	Threshold      *evaluation.Score
	Samples        int
}

func buildPrompt[Variables any](config ModelEvaluatorConfig, fallback string, required ...string) (*chatclient.Template, error) {
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
	if _, err := prompt.Render(*new(Variables)); err != nil {
		return nil, fmt.Errorf("%w: prompt: %w", evaluation.ErrInvalidEvaluatorConfig, err)
	}
	return prompt, nil
}

func newModelEvaluator[Subject, Variables any](
	config ModelEvaluatorConfig,
	metric evaluation.Metric,
	defaultPrompt string,
	variables func(Subject) Variables,
	required ...string,
) (evaluation.Evaluator[Subject], error) {
	prompt, err := buildPrompt[Variables](config, defaultPrompt, required...)
	if err != nil {
		return nil, err
	}
	return judge.NewEvaluator(judge.Config[Subject]{
		Model: config.Model, Metric: metric, Threshold: config.Threshold, Samples: config.Samples,
		Prompt: func(subject Subject) (chat.Message, error) {
			return prompt.UserMessage(variables(subject))
		},
	})
}
