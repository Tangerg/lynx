package filter

func compare[L IdentifierValue | *IndexExpr, R LiteralValue](l L, r R, op Operator) *BinaryExpr {
	return &BinaryExpr{
		Left:  leftOperand(l),
		Op:    op,
		Right: NewLiteral(r),
	}
}

// EQ builds `l == r` — equality, any literal type. Examples:
// `id == 1`, `name == 'john'`, `arr[0] == 'value'`.
func EQ[L IdentifierValue | *IndexExpr, R LiteralValue](l L, r R) *BinaryExpr {
	return compare(l, r, OpEqual)
}

// NE builds `l != r` — inequality, any literal type.
func NE[L IdentifierValue | *IndexExpr, R LiteralValue](l L, r R) *BinaryExpr {
	return compare(l, r, OpNotEqual)
}

// LT builds `l < r` — strict less-than. Right operand must be numeric.
func LT[L IdentifierValue | *IndexExpr, R Number | *Literal](l L, r R) *BinaryExpr {
	return compare(l, r, OpLess)
}

// LE builds `l <= r` — less-than-or-equal. Right operand must be
// numeric.
func LE[L IdentifierValue | *IndexExpr, R Number | *Literal](l L, r R) *BinaryExpr {
	return compare(l, r, OpLessEqual)
}

// GT builds `l > r` — strict greater-than. Right operand must be
// numeric.
func GT[L IdentifierValue | *IndexExpr, R Number | *Literal](l L, r R) *BinaryExpr {
	return compare(l, r, OpGreater)
}

// GE builds `l >= r` — greater-than-or-equal. Right operand must be
// numeric.
func GE[L IdentifierValue | *IndexExpr, R Number | *Literal](l L, r R) *BinaryExpr {
	return compare(l, r, OpGreaterEqual)
}

// In builds `l IN (...)`. Right operand is converted via
// [NewListLiteral]. Examples: `status IN ('active','pending')`,
// `id IN (1,2,3)`.
func In[L IdentifierValue | *IndexExpr, R ListValue](l L, r R) *BinaryExpr {
	return &BinaryExpr{
		Left:  leftOperand(l),
		Op:    OpIn,
		Right: NewListLiteral(r),
	}
}

// Like builds `l LIKE r`. Right operand must be a string. Examples:
// `name LIKE 'John%'`, `email LIKE '%@gmail.com'`.
func Like[L IdentifierValue | *IndexExpr, R string | *Literal](l L, r R) *BinaryExpr {
	return &BinaryExpr{
		Left:  leftOperand(l),
		Op:    OpLike,
		Right: NewLiteral(r),
	}
}

// IsNull builds `l IS NULL`.
func IsNull[L IdentifierValue | *IndexExpr](l L) *BinaryExpr {
	return &BinaryExpr{Left: leftOperand(l), Op: OpIs, Right: &Literal{Kind: LiteralNull, Value: "null"}}
}

// IsNotNull builds `NOT (l IS NULL)`.
func IsNotNull[L IdentifierValue | *IndexExpr](l L) *UnaryExpr {
	return Not(IsNull(l))
}
