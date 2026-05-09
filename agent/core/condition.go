package core

import (
	"context"
	"strings"

	"github.com/Tangerg/lynx/core/model/chat"
)

// OperationContext is the read-only surface a Condition.Evaluate sees. It's
// kept small intentionally: a condition should not need a chat client, an
// LLM, or a publish channel to decide whether a fact holds. (Prompt-driven
// conditions plug in via PromptCondition, which carries its own client.)
//
// Blackboard is typed as [BlackboardReader] so condition implementations
// cannot accidentally mutate state during the OBSERVE phase — the
// compiler enforces the structural contract.
type OperationContext struct {
	Process    Process
	Blackboard BlackboardReader
}

// Condition is a named, evaluable predicate. The planner treats it as a
// world-state probe; multiple cheap conditions can compose into expensive
// gating logic via And/Or/Not.
type Condition interface {
	Name() string

	// Cost is the planner's hint for how expensive evaluation is — composite
	// conditions average their children, LLM-backed conditions report higher
	// numbers so the planner explores cheaper branches first.
	Cost() float64

	Evaluate(ctx context.Context, oc *OperationContext) Determination
}

// ConditionFunc is the function shape used by NewCondition — exported so
// callers can name parameters in their own code without re-typing the
// signature.
type ConditionFunc func(ctx context.Context, oc *OperationContext) Determination

// ComputedCondition wraps a function — by far the common case.
type ComputedCondition struct {
	name string
	cost float64
	fn   ConditionFunc
}

// NewCondition constructs a function-backed condition with zero cost.
func NewCondition(name string, fn ConditionFunc) *ComputedCondition {
	return &ComputedCondition{name: name, fn: fn}
}

func (c *ComputedCondition) Name() string  { return c.name }
func (c *ComputedCondition) Cost() float64 { return c.cost }

func (c *ComputedCondition) Evaluate(ctx context.Context, oc *OperationContext) Determination {
	if c.fn == nil {
		return Unknown
	}
	return c.fn(ctx, oc)
}

// --- LLM-driven condition -------------------------------------------------

// PromptBuilder is the user-supplied function that turns the current
// operation context into the LLM user prompt. Pulled into the
// condition's [Evaluate] every tick so prompts can reference live
// blackboard state.
type PromptBuilder func(ctx context.Context, oc *OperationContext) string

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
//   - prompt: builds the user prompt from the live OperationContext.
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
) *PromptCondition {
	if client == nil {
		panic("core.NewPromptCondition: client must not be nil")
	}
	if prompt == nil {
		panic("core.NewPromptCondition: prompt builder must not be nil")
	}
	if parser == nil {
		panic("core.NewPromptCondition: parser must not be nil")
	}
	return &PromptCondition{
		name:   name,
		cost:   1.0,
		client: client,
		prompt: prompt,
		parser: parser,
	}
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
func (c *PromptCondition) Evaluate(ctx context.Context, oc *OperationContext) Determination {
	text, _, err := c.client.
		Chat().
		WithUserPrompt(c.prompt(ctx, oc)).
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
// — we accept the first word and ignore the rest. For stricter
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

// --- Boolean composition --------------------------------------------------
//
// These mirror Kotlin's operator overloads via plain functions. They
// short-circuit so an expensive child is never evaluated when a cheap
// sibling has already determined the outcome.

type andCondition struct{ left, right Condition }

func And(left, right Condition) Condition { return &andCondition{left, right} }

func (c *andCondition) Name() string {
	return "(" + conditionName(c.left) + " AND " + conditionName(c.right) + ")"
}

func (c *andCondition) Cost() float64 {
	return conditionCost(c.left) + conditionCost(c.right)
}

func (c *andCondition) Evaluate(ctx context.Context, oc *OperationContext) Determination {
	leftResult := evaluateCondition(ctx, c.left, oc)
	if leftResult == False {
		return False
	}
	return leftResult.And(evaluateCondition(ctx, c.right, oc))
}

type orCondition struct{ left, right Condition }

func Or(left, right Condition) Condition { return &orCondition{left, right} }

func (c *orCondition) Name() string {
	return "(" + conditionName(c.left) + " OR " + conditionName(c.right) + ")"
}

func (c *orCondition) Cost() float64 {
	return conditionCost(c.left) + conditionCost(c.right)
}

func (c *orCondition) Evaluate(ctx context.Context, oc *OperationContext) Determination {
	leftResult := evaluateCondition(ctx, c.left, oc)
	if leftResult == True {
		return True
	}
	return leftResult.Or(evaluateCondition(ctx, c.right, oc))
}

type notCondition struct{ inner Condition }

func Not(inner Condition) Condition { return &notCondition{inner} }

func (c *notCondition) Name() string  { return "(NOT " + conditionName(c.inner) + ")" }
func (c *notCondition) Cost() float64 { return conditionCost(c.inner) }

func (c *notCondition) Evaluate(ctx context.Context, oc *OperationContext) Determination {
	return evaluateCondition(ctx, c.inner, oc).Not()
}

func conditionName(condition Condition) string {
	if condition == nil {
		return "<nil>"
	}
	if name := condition.Name(); name != "" {
		return name
	}
	return "<unnamed>"
}

func conditionCost(condition Condition) float64 {
	if condition == nil {
		return 0
	}
	return condition.Cost()
}

func evaluateCondition(ctx context.Context, condition Condition, oc *OperationContext) Determination {
	if condition == nil {
		return Unknown
	}
	return condition.Evaluate(ctx, oc)
}
