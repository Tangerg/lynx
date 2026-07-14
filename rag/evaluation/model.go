package evaluation

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"text/template"

	"github.com/Tangerg/lynx/core/chat"
)

var scorePattern = regexp.MustCompile(`\d*\.\d+|\d+`)

// ModelConfig configures a model-backed evaluator. Prompt is an optional Go
// text/template over .Query, .Answer, and .Context. Threshold zero selects
// DefaultPassThreshold; other values must be in [0, 1].
type ModelConfig struct {
	Model     chat.Model
	Prompt    string
	Threshold float64
}

type promptData struct {
	Query   string
	Answer  string
	Context string
}

type modelEvaluator struct {
	model     chat.Model
	prompt    *template.Template
	threshold float64
	validate  func(Request) error
}

func newModelEvaluator(config ModelConfig, defaultPrompt string, validate func(Request) error) (*modelEvaluator, error) {
	if config.Model == nil {
		return nil, fmt.Errorf("%w: nil model", ErrInvalidConfig)
	}
	if config.Threshold < 0 || config.Threshold > 1 {
		return nil, fmt.Errorf("%w: threshold must be between 0 and 1", ErrInvalidConfig)
	}
	if config.Threshold == 0 {
		config.Threshold = DefaultPassThreshold
	}
	if config.Prompt == "" {
		config.Prompt = defaultPrompt
	}
	prompt, err := template.New("evaluation").Option("missingkey=error").Parse(config.Prompt)
	if err != nil {
		return nil, fmt.Errorf("%w: parse prompt: %w", ErrInvalidConfig, err)
	}
	var check bytes.Buffer
	if err := prompt.Execute(&check, promptData{}); err != nil {
		return nil, fmt.Errorf("%w: validate prompt: %w", ErrInvalidConfig, err)
	}
	return &modelEvaluator{
		model: config.Model, prompt: prompt, threshold: config.Threshold, validate: validate,
	}, nil
}

func (e *modelEvaluator) Evaluate(ctx context.Context, request Request) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if err := e.validate(request); err != nil {
		return Result{}, err
	}

	var prompt bytes.Buffer
	if err := e.prompt.Execute(&prompt, promptData{
		Query: request.Query, Answer: request.Answer, Context: request.contextText(),
	}); err != nil {
		return Result{}, fmt.Errorf("evaluation: render prompt: %w", err)
	}
	modelRequest, err := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart(prompt.String())))
	if err != nil {
		return Result{}, fmt.Errorf("evaluation: build model request: %w", err)
	}
	response, err := e.model.Call(ctx, modelRequest)
	if err != nil {
		return Result{}, fmt.Errorf("evaluation: model call: %w", err)
	}
	return parseScore(response.Text(), e.threshold)
}

func parseScore(text string, threshold float64) (Result, error) {
	for _, span := range scorePattern.FindAllStringIndex(text, -1) {
		token := text[span[0]:span[1]]
		score, err := strconv.ParseFloat(token, 64)
		if err != nil || score < 0 || score > 1 {
			continue
		}
		return Result{
			Pass: score >= threshold, Score: score, Feedback: strings.TrimSpace(text[span[1]:]),
		}, nil
	}
	return Result{}, fmt.Errorf("%w: %q", ErrNoScore, text)
}
