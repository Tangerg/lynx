package filter

func logic[L Predicate, R Predicate](l L, r R, op Operator) *BinaryExpr {
	return &BinaryExpr{
		Left:  l,
		Op:    op,
		Right: r,
	}
}

// And builds `l AND r`. Raw literals and selectors do not satisfy
// [Predicate].
func And[L Predicate, R Predicate](l L, r R) *BinaryExpr {
	return logic(l, r, OpAnd)
}

// Or builds `l OR r`. Both operands must be predicates.
func Or[L Predicate, R Predicate](l L, r R) *BinaryExpr {
	return logic(l, r, OpOr)
}

// Not builds `NOT r`. The operand must be a predicate.
func Not[T Predicate](r T) *UnaryExpr {
	return &UnaryExpr{
		Op:    OpNot,
		Right: r,
	}
}
