package core

import (
	"context"

	"github.com/Tangerg/lynx/agent/interaction"
)

// ConditionEnv is the read-only surface a Condition.Evaluate sees. It's
// kept small intentionally: a condition should not need a chat client, an
// LLM, or a publish channel to decide whether a fact holds. Prompt-driven
// conditions use the same runtime-managed interaction path as actions.
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

// ConditionDescriptor is the immutable, non-executable projection of a
// condition. It carries the planner's static evaluation-cost hint but no
// Evaluate capability.
type ConditionDescriptor struct {
	name string
	cost float64
}

// Name returns the condition's identity.
func (d ConditionDescriptor) Name() string { return d.name }

// Cost returns the condition's static evaluation-cost hint.
func (d ConditionDescriptor) Cost() float64 { return d.cost }

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

// Evaluate reports the condition's truth. A condition built without a function
// is Unknown rather than False: three-valued logic already distinguishes "not
// known" from "known false", and collapsing the two would let a planner treat an
// unwired condition as a satisfied negation.
func (c *FuncCondition) Evaluate(ctx context.Context, env *ConditionEnv) Truth {
	if c.fn == nil {
		return Unknown
	}
	return c.fn(ctx, env)
}

// operand is one side of a condition combinator. The combinators are public and
// accept any Condition, so a nil side is a real state rather than a caller
// error: it names itself, contributes no cost, and evaluates to Unknown — which
// is what three-valued logic already says about something not known.
//
// The nil check lives here so each combinator can read its sides directly. It is
// a plain field rather than an embedded Condition because an embedded interface
// would promote any method this type does not override, turning a future
// addition to Condition into a nil panic instead of a compile error.
type operand struct{ condition Condition }

func (o operand) Name() string {
	if o.condition == nil {
		return "<nil>"
	}
	if name := o.condition.Name(); name != "" {
		return name
	}
	return "<unnamed>"
}

func (o operand) Cost() float64 {
	if o.condition == nil {
		return 0
	}
	return o.condition.Cost()
}

func (o operand) Evaluate(ctx context.Context, env *ConditionEnv) Truth {
	if o.condition == nil {
		return Unknown
	}
	return o.condition.Evaluate(ctx, env)
}

// binaryCondition is the half And and Or share: two sides, a parenthesized name
// around an operator, and a cost that is the sum of both sides. Evaluate stays
// with each of them, since that is the only part where they actually differ —
// which Truth short-circuits, and how the two results fold.
type binaryCondition struct{ left, right operand }

func newBinaryCondition(left, right Condition) binaryCondition {
	return binaryCondition{left: operand{condition: left}, right: operand{condition: right}}
}

func (c binaryCondition) Cost() float64 { return c.left.Cost() + c.right.Cost() }

func (c binaryCondition) name(operator string) string {
	return "(" + c.left.Name() + " " + operator + " " + c.right.Name() + ")"
}

type andCondition struct{ binaryCondition }

// And returns a condition that is true only when both operands are true.
func And(left, right Condition) Condition {
	return &andCondition{newBinaryCondition(left, right)}
}

func (c *andCondition) Name() string { return c.name("AND") }

func (c *andCondition) Evaluate(ctx context.Context, env *ConditionEnv) Truth {
	left := c.left.Evaluate(ctx, env)
	if left == False {
		return False
	}
	return left.And(c.right.Evaluate(ctx, env))
}

type orCondition struct{ binaryCondition }

// Or returns a condition that is true when either operand is true.
func Or(left, right Condition) Condition {
	return &orCondition{newBinaryCondition(left, right)}
}

func (c *orCondition) Name() string { return c.name("OR") }

func (c *orCondition) Evaluate(ctx context.Context, env *ConditionEnv) Truth {
	left := c.left.Evaluate(ctx, env)
	if left == True {
		return True
	}
	return left.Or(c.right.Evaluate(ctx, env))
}

type notCondition struct{ inner operand }

// Not returns the three-valued negation of inner.
func Not(inner Condition) Condition { return &notCondition{operand{condition: inner}} }

func (c *notCondition) Name() string  { return "(NOT " + c.inner.Name() + ")" }
func (c *notCondition) Cost() float64 { return c.inner.Cost() }

func (c *notCondition) Evaluate(ctx context.Context, env *ConditionEnv) Truth {
	return c.inner.Evaluate(ctx, env).Not()
}
