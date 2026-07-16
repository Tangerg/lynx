package core

import "context"

type andCondition struct{ left, right Condition }

func And(left, right Condition) Condition { return &andCondition{left, right} }

func (c *andCondition) Name() string {
	return "(" + conditionName(c.left) + " AND " + conditionName(c.right) + ")"
}

func (c *andCondition) Cost() float64 {
	return conditionCost(c.left) + conditionCost(c.right)
}

func (c *andCondition) Evaluate(ctx context.Context, env *ConditionEnv) Truth {
	leftResult := evaluateCondition(ctx, c.left, env)
	if leftResult == False {
		return False
	}
	return leftResult.And(evaluateCondition(ctx, c.right, env))
}

type orCondition struct{ left, right Condition }

func Or(left, right Condition) Condition { return &orCondition{left, right} }

func (c *orCondition) Name() string {
	return "(" + conditionName(c.left) + " OR " + conditionName(c.right) + ")"
}

func (c *orCondition) Cost() float64 {
	return conditionCost(c.left) + conditionCost(c.right)
}

func (c *orCondition) Evaluate(ctx context.Context, env *ConditionEnv) Truth {
	leftResult := evaluateCondition(ctx, c.left, env)
	if leftResult == True {
		return True
	}
	return leftResult.Or(evaluateCondition(ctx, c.right, env))
}

type notCondition struct{ inner Condition }

func Not(inner Condition) Condition { return &notCondition{inner} }

func (c *notCondition) Name() string  { return "(NOT " + conditionName(c.inner) + ")" }
func (c *notCondition) Cost() float64 { return conditionCost(c.inner) }

func (c *notCondition) Evaluate(ctx context.Context, env *ConditionEnv) Truth {
	return evaluateCondition(ctx, c.inner, env).Not()
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

func evaluateCondition(ctx context.Context, condition Condition, env *ConditionEnv) Truth {
	if condition == nil {
		return Unknown
	}
	return condition.Evaluate(ctx, env)
}
