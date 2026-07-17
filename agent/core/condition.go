package core

import (
	"context"

	"github.com/Tangerg/lynx/agent/interaction"
)

// ConditionEnv is the read-only surface a Condition.Evaluate sees. It's
// kept small intentionally: a condition should not need a chat client, an
// LLM, or a publish channel to decide whether a fact holds. (Prompt-driven
// conditions plug in via PromptCondition, which carries its own client.)
//
// Blackboard is typed as [BlackboardReader] so condition implementations
// cannot accidentally mutate state during the OBSERVE phase — the
// compiler enforces the structural contract.
type ConditionEnv struct {
	Process        ProcessView
	Blackboard     BlackboardReader
	RunInteraction func(context.Context, Interaction) (interaction.Result, error)
}

// Condition is a named, evaluable predicate. The planner treats it as a
// world-state probe; multiple cheap conditions can compose into expensive
// gating logic via And/Or/Not.
type Condition interface {
	Name() string

	// Cost is the planner's hint for how expensive evaluation is — composite
	// conditions sum their children's costs, LLM-backed conditions report higher
	// numbers so the planner explores cheaper branches first.
	Cost() float64

	Evaluate(ctx context.Context, env *ConditionEnv) Truth
}

// ConditionFunc is the function shape used by NewCondition — exported so
// callers can name parameters in their own code without re-typing the
// signature.
type ConditionFunc func(ctx context.Context, env *ConditionEnv) Truth

// FuncCondition wraps a function — by far the common case.
type FuncCondition struct {
	name string
	cost float64
	fn   ConditionFunc
}

// NewCondition constructs a function-backed condition with zero cost.
func NewCondition(name string, fn ConditionFunc) *FuncCondition {
	return &FuncCondition{name: name, fn: fn}
}

func (c *FuncCondition) Name() string  { return c.name }
func (c *FuncCondition) Cost() float64 { return c.cost }

func (c *FuncCondition) Evaluate(ctx context.Context, env *ConditionEnv) Truth {
	if c.fn == nil {
		return Unknown
	}
	return c.fn(ctx, env)
}
