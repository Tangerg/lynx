package core

import (
	"cmp"
	"context"
	"errors"
	"math"
	"strings"

	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/core/chat"
)

const (
	defaultPromptConditionCost = 1.0
	conditionInteractionPrefix = "condition:"
)

// ConditionInteractionID returns the stable managed-interaction identity for
// a named condition.
func ConditionInteractionID(name string) string {
	return conditionInteractionPrefix + name
}

// PromptFunc builds a condition prompt from the current process state.
type PromptFunc func(context.Context, *ConditionEnv) string

// ParseTruthFunc interprets a model response as three-valued truth.
type ParseTruthFunc func(string) Truth

// PromptConditionConfig configures an LLM-evaluated condition. EvaluationCost defaults
// to one because each evaluation performs a model call; an explicit cost must
// be finite and non-negative.
type PromptConditionConfig struct {
	Name           string
	Prompt         PromptFunc
	Parse          ParseTruthFunc
	EvaluationCost float64
}

// PromptCondition evaluates a named condition with a model call.
type PromptCondition struct {
	name           string
	evaluationCost float64
	prompt         PromptFunc
	parse          ParseTruthFunc
}

// NewPromptCondition validates config and returns an LLM-evaluated condition.
func NewPromptCondition(config PromptConditionConfig) (*PromptCondition, error) {
	if config.Name == "" || strings.TrimSpace(config.Name) != config.Name {
		return nil, errors.New("agent: prompt condition name must be non-empty without surrounding whitespace")
	}
	if config.Prompt == nil {
		return nil, errors.New("agent: prompt condition prompt must not be nil")
	}
	if config.Parse == nil {
		return nil, errors.New("agent: prompt condition parser must not be nil")
	}
	if math.IsNaN(config.EvaluationCost) || math.IsInf(config.EvaluationCost, 0) || config.EvaluationCost < 0 {
		return nil, errors.New("agent: prompt condition evaluation cost must be finite and non-negative")
	}
	evaluationCost := cmp.Or(config.EvaluationCost, defaultPromptConditionCost)
	return &PromptCondition{
		name:           config.Name,
		evaluationCost: evaluationCost,
		prompt:         config.Prompt,
		parse:          config.Parse,
	}, nil
}

// Name implements [Condition].
func (c *PromptCondition) Name() string { return c.name }

// EvaluationCost implements [Condition].
func (c *PromptCondition) EvaluationCost() float64 { return c.evaluationCost }

// Evaluate returns Unknown when the managed model interaction cannot produce a
// model response, keeping an uncertain gate closed without aborting the tick.
func (c *PromptCondition) Evaluate(ctx context.Context, env *ConditionEnv) Truth {
	if env == nil || env.RunInteraction == nil {
		return Unknown
	}
	request, err := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart(c.prompt(ctx, env))))
	if err != nil {
		return Unknown
	}
	result, err := env.RunInteraction(ctx, Interaction{
		ID:      ConditionInteractionID(c.name),
		Request: request,
	})
	if err != nil || result.Final == nil || result.Final.Kind != interaction.EventModelResponse {
		return Unknown
	}
	return c.parse(result.Final.Response.Text())
}

// ParseYesNo interprets the first word of a response as True, False, or
// Unknown. It accepts common boolean and affirmative/negative spellings.
func ParseYesNo(text string) Truth {
	fields := strings.Fields(strings.ToLower(strings.TrimSpace(text)))
	if len(fields) == 0 {
		return Unknown
	}
	first := strings.TrimRight(fields[0], ".,!?:;'\"")
	switch first {
	case "yes", "true", "y", "1", "correct", "affirmative":
		return True
	case "no", "false", "n", "0", "incorrect", "negative":
		return False
	}
	return Unknown
}
