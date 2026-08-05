package core

import (
	"context"

	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/agent/internal/nilvalue"
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

	// EvaluationCost estimates the relative work required to evaluate the condition.
	// Planners resolve unknown evaluator-backed conditions from lower to higher
	// cost so a cheap mismatch can avoid an expensive observation. It does not
	// contribute to action cost or plan ranking.
	EvaluationCost() float64

	Evaluate(ctx context.Context, env *ConditionEnv) Truth
}

// ConditionDescriptor is the immutable, non-executable projection of a
// condition. It carries the planner's static evaluation-cost hint but no
// Evaluate capability.
type ConditionDescriptor struct {
	name           string
	evaluationCost float64
}

// Name returns the condition's identity.
func (d ConditionDescriptor) Name() string { return d.name }

// EvaluationCost returns the condition's static evaluation-cost hint.
func (d ConditionDescriptor) EvaluationCost() float64 { return d.evaluationCost }

// ConditionFunc is the function shape used by NewCondition — exported so
// callers can name parameters in their own code without re-typing the
// signature.
type ConditionFunc func(ctx context.Context, env *ConditionEnv) Truth

// FuncCondition wraps a function — by far the common case.
type FuncCondition struct {
	name           string
	evaluationCost float64
	fn             ConditionFunc
}

// ConditionConfig describes a function-backed condition. EvaluationCost is a relative
// evaluation-cost hint; zero is appropriate for an in-memory predicate.
type ConditionConfig struct {
	Name           string
	EvaluationCost float64
	Evaluate       ConditionFunc
}

// NewCondition constructs a function-backed condition.
func NewCondition(config ConditionConfig) *FuncCondition {
	return &FuncCondition{name: config.Name, evaluationCost: config.EvaluationCost, fn: config.Evaluate}
}

func (c *FuncCondition) Name() string            { return c.name }
func (c *FuncCondition) EvaluationCost() float64 { return c.evaluationCost }

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
// accept any Condition, so a nil side (including an interface holding a typed
// nil) is a real state rather than a caller error: it names itself, contributes
// no cost, and evaluates to Unknown — which is what three-valued logic already
// says about something not known.
//
// The nil check lives here so each combinator can read its sides directly. It is
// a plain field rather than an embedded Condition because an embedded interface
// would promote any method this type does not override, turning a future
// addition to Condition into a nil panic instead of a compile error.
type operand struct{ condition Condition }

func (o operand) Name() string {
	if nilvalue.Is(o.condition) {
		return "<nil>"
	}
	if name := o.condition.Name(); name != "" {
		return name
	}
	return "<unnamed>"
}

func (o operand) EvaluationCost() float64 {
	if nilvalue.Is(o.condition) {
		return 0
	}
	return o.condition.EvaluationCost()
}

func (o operand) Evaluate(ctx context.Context, env *ConditionEnv) Truth {
	if nilvalue.Is(o.condition) {
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

func (c binaryCondition) EvaluationCost() float64 {
	return c.left.EvaluationCost() + c.right.EvaluationCost()
}

// evaluationOrder returns the cheaper operand first. AND and OR are
// commutative, so this preserves their truth semantics while maximizing the
// chance that short-circuiting avoids the more expensive observation. Equal
// costs preserve declaration order.
func (c binaryCondition) evaluationOrder() (operand, operand) {
	if c.right.EvaluationCost() < c.left.EvaluationCost() {
		return c.right, c.left
	}
	return c.left, c.right
}

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
	first, second := c.evaluationOrder()
	result := first.Evaluate(ctx, env)
	if result == False {
		return False
	}
	return result.And(second.Evaluate(ctx, env))
}

type orCondition struct{ binaryCondition }

// Or returns a condition that is true when either operand is true.
func Or(left, right Condition) Condition {
	return &orCondition{newBinaryCondition(left, right)}
}

func (c *orCondition) Name() string { return c.name("OR") }

func (c *orCondition) Evaluate(ctx context.Context, env *ConditionEnv) Truth {
	first, second := c.evaluationOrder()
	result := first.Evaluate(ctx, env)
	if result == True {
		return True
	}
	return result.Or(second.Evaluate(ctx, env))
}

type notCondition struct{ inner operand }

// Not returns the three-valued negation of inner.
func Not(inner Condition) Condition { return &notCondition{operand{condition: inner}} }

func (c *notCondition) Name() string            { return "(NOT " + c.inner.Name() + ")" }
func (c *notCondition) EvaluationCost() float64 { return c.inner.EvaluationCost() }

func (c *notCondition) Evaluate(ctx context.Context, env *ConditionEnv) Truth {
	return c.inner.Evaluate(ctx, env).Not()
}
