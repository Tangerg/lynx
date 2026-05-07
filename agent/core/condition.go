package core

import "context"

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
