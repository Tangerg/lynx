package evaluation

import (
	"context"
	"fmt"
	"strings"

	"github.com/Tangerg/lynx/core/chatclient"
	"github.com/Tangerg/lynx/core/chat"
)

const modelResultSchema = `{
  "type": "object",
  "properties": {
    "score": {"type": "number", "minimum": 0, "maximum": 1},
    "feedback": {"type": "string"}
  },
  "required": ["score"],
  "additionalProperties": false
}`

// ModelConfig configures a model-backed evaluator. PromptTemplate is rendered
// over .Query, .Answer, and .Context. A nil Threshold selects
// DefaultPassThreshold; a non-nil value must be in [0, 1].
type ModelConfig struct {
	Model          chat.Model
	PromptTemplate *chatclient.Template
	Threshold      *float64
}

type promptVariables struct {
	Query   string
	Answer  string
	Context string
}

type modelResult struct {
	Score    float64 `json:"score"`
	Feedback string  `json:"feedback,omitzero"`
}

type modelEvaluator struct {
	generation      chatclient.Generation[modelResult]
	prompt          *chatclient.Template
	threshold       float64
	validateRequest func(Request) error
}

func newModelEvaluator(
	config ModelConfig,
	defaultPrompt string,
	validate func(Request) error,
	required ...string,
) (*modelEvaluator, error) {
	if isNilCapability(config.Model) {
		return nil, fmt.Errorf("%w: nil model", ErrInvalidConfig)
	}
	threshold := DefaultPassThreshold
	if config.Threshold != nil {
		threshold = *config.Threshold
	}
	if threshold < 0 || threshold > 1 {
		return nil, fmt.Errorf("%w: threshold must be between 0 and 1", ErrInvalidConfig)
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
	format, err := chatclient.JSONSchema[modelResult]("evaluation_result", []byte(modelResultSchema))
	if err != nil {
		return nil, fmt.Errorf("%w: output format: %w", ErrInvalidConfig, err)
	}
	return &modelEvaluator{
		generation:      client.Output(format),
		prompt:          prompt,
		threshold:       threshold,
		validateRequest: validate,
	}, nil
}

func (e *modelEvaluator) Evaluate(ctx context.Context, request Request) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if err := e.validateRequest(request); err != nil {
		return Result{}, err
	}

	message, err := e.prompt.UserMessage(promptVariables{
		Query: request.Query, Answer: request.Answer, Context: request.contextText(),
	})
	if err != nil {
		return Result{}, fmt.Errorf("evaluation: render prompt: %w", err)
	}
	output, err := e.generation.Call(ctx, &chat.Request{Messages: []chat.Message{message}})
	if err != nil {
		return Result{}, fmt.Errorf("evaluation: generate result: %w", err)
	}
	result := Result{
		Pass:     output.Score >= e.threshold,
		Score:    output.Score,
		Feedback: strings.TrimSpace(output.Feedback),
	}
	if err := result.Validate(); err != nil {
		return Result{}, fmt.Errorf("evaluation: model result: %w", err)
	}
	return result, nil
}
