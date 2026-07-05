package core

import (
	"context"
	"errors"
	"strings"

	"github.com/Tangerg/lynx/core/model/chat"
)

// PromptBuilder is the user-supplied function that turns the current
// operation context into the LLM user prompt. Pulled into the
// condition's [Evaluate] every tick so prompts can reference live
// blackboard state.
type PromptBuilder func(ctx context.Context, env *ConditionEnv) string

// ConditionParser converts the LLM's raw text reply into a
// [Determination]. [ParseYesNoDetermination] is the canonical default
// for "yes / no / unsure" prompts; users can supply their own parser
// for richer reply shapes (JSON / structured).
type ConditionParser func(text string) Determination

// PromptCondition is the LLM-as-judge variant of [Condition]: each
// Evaluate call asks an LLM via the supplied [chat.Client] and parses
// the reply into a Determination. Use for "is this draft acceptable?"
// / "did the search return a relevant result?" style gates that need
// natural-language reasoning rather than a pure-function predicate.
//
// Cost defaults to 1.0 — LLM-backed conditions are expensive and the
// planner's heuristic should reflect that. Override via [PromptCondition.WithCost]
// when integrating with measured cost models.
type PromptCondition struct {
	name   string
	cost   float64
	client *chat.Client
	prompt PromptBuilder
	parser ConditionParser
}

// NewPromptCondition wires an LLM call as a [Condition].
//
// Parameters:
//   - name: condition key the planner uses (e.g., "draft_acceptable").
//   - client: the chat client; nil panics — LLM-driven conditions
//     without a model don't have a meaningful default.
//   - prompt: builds the user prompt from the live ConditionEnv.
//   - parser: maps the LLM reply to True/False/Unknown. Pass
//     [ParseYesNoDetermination] for the common yes/no shape.
//
// The resulting Condition's Evaluate is best-effort: an LLM error or
// missing parser yields Unknown (which the planner treats as
// "doesn't satisfy"), so a flaky model degrades to "the gate stays
// closed" rather than crashing the tick.
func NewPromptCondition(
	name string,
	client *chat.Client,
	prompt PromptBuilder,
	parser ConditionParser,
) (*PromptCondition, error) {
	if client == nil {
		return nil, errors.New("agent.NewPromptCondition: client must not be nil")
	}
	if prompt == nil {
		return nil, errors.New("agent.NewPromptCondition: prompt builder must not be nil")
	}
	if parser == nil {
		return nil, errors.New("agent.NewPromptCondition: parser must not be nil")
	}
	return &PromptCondition{
		name:   name,
		cost:   1.0,
		client: client,
		prompt: prompt,
		parser: parser,
	}, nil
}

// WithCost overrides the planner cost hint (default 1.0).
func (c *PromptCondition) WithCost(cost float64) *PromptCondition {
	c.cost = cost
	return c
}

// Name implements [Condition].
func (c *PromptCondition) Name() string { return c.name }

// Cost implements [Condition].
func (c *PromptCondition) Cost() float64 { return c.cost }

// Evaluate calls the LLM with a freshly-built user prompt and parses
// the reply. Returns [Unknown] on LLM error / empty reply, so the
// planner falls back to "doesn't satisfy" rather than tripping on a
// transient model issue.
func (c *PromptCondition) Evaluate(ctx context.Context, env *ConditionEnv) Determination {
	text, _, err := c.client.
		Chat().
		WithUserPrompt(c.prompt(ctx, env)).
		Call().
		Text(ctx)
	if err != nil {
		return Unknown
	}
	return c.parser(text)
}

// ParseYesNoDetermination is the canonical [ConditionParser]: looks
// at the first non-empty word of text and maps yes/true/y/1/correct/
// affirmative → [True], no/false/n/0/incorrect/negative → [False],
// anything else → [Unknown]. Punctuation around the first word is
// trimmed so "Yes." / "No," still classify cleanly.
//
// The leniency is intentional: LLMs often answer "Yes, because ..."
// — the first word is accepted and the rest ignored. For stricter
// shapes (structured JSON / scored Feedback) write a custom parser.
func ParseYesNoDetermination(text string) Determination {
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
