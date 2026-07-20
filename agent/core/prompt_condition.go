package core

import (
	"context"
	"errors"
	"strings"

	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/core/chat"
)

const defaultPromptConditionCost = 1.0

// PromptFunc builds a condition prompt from the current process state.
type PromptFunc func(context.Context, *ConditionEnv) string

// ParseTruthFunc interprets a model response as three-valued truth.
type ParseTruthFunc func(string) Truth

// PromptConditionConfig configures an LLM-evaluated condition. Cost defaults
// to one because each evaluation performs a model call.
type PromptConditionConfig struct {
	Name   string
	Model  chat.Model
	Prompt PromptFunc
	Parse  ParseTruthFunc
	Cost   float64
}

// PromptCondition evaluates a named condition with a model call.
type PromptCondition struct {
	name   string
	cost   float64
	model  chat.Model
	prompt PromptFunc
	parse  ParseTruthFunc
}

// NewPromptCondition validates config and returns an LLM-evaluated condition.
func NewPromptCondition(config PromptConditionConfig) (*PromptCondition, error) {
	if config.Name == "" {
		return nil, errors.New("agent: prompt condition name must not be empty")
	}
	if config.Model == nil {
		return nil, errors.New("agent: prompt condition model must not be nil")
	}
	if config.Prompt == nil {
		return nil, errors.New("agent: prompt condition prompt must not be nil")
	}
	if config.Parse == nil {
		return nil, errors.New("agent: prompt condition parser must not be nil")
	}
	if config.Cost < 0 {
		return nil, errors.New("agent: prompt condition cost must not be negative")
	}
	cost := config.Cost
	if cost == 0 {
		cost = defaultPromptConditionCost
	}
	return &PromptCondition{
		name:   config.Name,
		cost:   cost,
		model:  config.Model,
		prompt: config.Prompt,
		parse:  config.Parse,
	}, nil
}

// Name implements [Condition].
func (c *PromptCondition) Name() string { return c.name }

// Cost implements [Condition].
func (c *PromptCondition) Cost() float64 { return c.cost }

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
		ID:      "condition:" + c.name,
		Model:   c.model,
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
