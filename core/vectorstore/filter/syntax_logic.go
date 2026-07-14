package filter

func logic[L ComputedExpr, R ComputedExpr](l L, r R, op Operator) *BinaryExpr {
	return &BinaryExpr{
		Left:  l,
		Op:    op,
		Right: r,
	}
}

// And builds `l AND r`. Both operands must be computed expressions —
// raw literals or identifiers do not satisfy [ComputedExpr].
func And[L ComputedExpr, R ComputedExpr](l L, r R) *BinaryExpr {
	return logic(l, r, OpAnd)
}

// Or builds `l OR r`. Both operands must be computed expressions.
func Or[L ComputedExpr, R ComputedExpr](l L, r R) *BinaryExpr {
	return logic(l, r, OpOr)
}

// Not builds `NOT r`. The operand must be a computed expression.
func Not[T ComputedExpr](r T) *UnaryExpr {
	return &UnaryExpr{
		Op:    OpNot,
		Right: r,
	}
}
