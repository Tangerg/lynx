package core

import "context"

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

func (c *andCondition) Name() string  { return "(" + c.left.Name() + " AND " + c.right.Name() + ")" }
func (c *andCondition) Cost() float64 { return c.left.Cost() + c.right.Cost() }

func (c *andCondition) Evaluate(ctx context.Context, oc *OperationContext) Determination {
	leftResult := c.left.Evaluate(ctx, oc)
	if leftResult == False {
		return False
	}
	return leftResult.And(c.right.Evaluate(ctx, oc))
}

type orCondition struct{ left, right Condition }

func Or(left, right Condition) Condition { return &orCondition{left, right} }

func (c *orCondition) Name() string  { return "(" + c.left.Name() + " OR " + c.right.Name() + ")" }
func (c *orCondition) Cost() float64 { return c.left.Cost() + c.right.Cost() }

func (c *orCondition) Evaluate(ctx context.Context, oc *OperationContext) Determination {
	leftResult := c.left.Evaluate(ctx, oc)
	if leftResult == True {
		return True
	}
	return leftResult.Or(c.right.Evaluate(ctx, oc))
}

type notCondition struct{ inner Condition }

func Not(inner Condition) Condition { return &notCondition{inner} }

func (c *notCondition) Name() string  { return "(NOT " + c.inner.Name() + ")" }
func (c *notCondition) Cost() float64 { return c.inner.Cost() }

func (c *notCondition) Evaluate(ctx context.Context, oc *OperationContext) Determination {
	return c.inner.Evaluate(ctx, oc).Not()
}
