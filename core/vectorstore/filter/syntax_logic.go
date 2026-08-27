package filter

func logic[L Predicate, R Predicate](left L, right R, operator Operator) *BinaryExpr {
	return &BinaryExpr{
		left:     left,
		operator: operator,
		right:    right,
	}
}

// And combines two predicates. Raw literals and selectors do not satisfy
// [Predicate].
func And[L Predicate, R Predicate](left L, right R) *BinaryExpr {
	return logic(left, right, OpAnd)
}

// Or accepts only predicate operands.
func Or[L Predicate, R Predicate](left L, right R) *BinaryExpr {
	return logic(left, right, OpOr)
}

// Not accepts only a predicate operand.
func Not[T Predicate](predicate T) *UnaryExpr {
	return &UnaryExpr{
		operator: OpNot,
		right:    predicate,
	}
}
