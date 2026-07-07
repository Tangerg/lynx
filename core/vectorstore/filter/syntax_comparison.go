package filter

import (
	"github.com/Tangerg/lynx/core/vectorstore/filter/ast"
	"github.com/Tangerg/lynx/core/vectorstore/filter/token"
	"github.com/Tangerg/lynx/pkg/math"
)

func compare[L identType | *ast.IndexExpr, R literalType](l L, r R, op token.Kind) *ast.BinaryExpr {
	return &ast.BinaryExpr{
		Left:  leftOperand(l),
		Op:    newKindToken(op),
		Right: NewLiteral(r),
	}
}

// EQ builds `l == r` — equality, any literal type. Examples:
// `id == 1`, `name == 'john'`, `arr[0] == 'value'`.
func EQ[L identType | *ast.IndexExpr, R literalType](l L, r R) *ast.BinaryExpr {
	return compare(l, r, token.EQ)
}

// NE builds `l != r` — inequality, any literal type.
func NE[L identType | *ast.IndexExpr, R literalType](l L, r R) *ast.BinaryExpr {
	return compare(l, r, token.NE)
}

// LT builds `l < r` — strict less-than. Right operand must be numeric.
func LT[L identType | *ast.IndexExpr, R math.NumericType | *ast.Literal](l L, r R) *ast.BinaryExpr {
	return compare(l, r, token.LT)
}

// LE builds `l <= r` — less-than-or-equal. Right operand must be
// numeric.
func LE[L identType | *ast.IndexExpr, R math.NumericType | *ast.Literal](l L, r R) *ast.BinaryExpr {
	return compare(l, r, token.LE)
}

// GT builds `l > r` — strict greater-than. Right operand must be
// numeric.
func GT[L identType | *ast.IndexExpr, R math.NumericType | *ast.Literal](l L, r R) *ast.BinaryExpr {
	return compare(l, r, token.GT)
}

// GE builds `l >= r` — greater-than-or-equal. Right operand must be
// numeric.
func GE[L identType | *ast.IndexExpr, R math.NumericType | *ast.Literal](l L, r R) *ast.BinaryExpr {
	return compare(l, r, token.GE)
}

// In builds `l IN (...)`. Right operand is converted via
// [NewListLiteral]. Examples: `status IN ('active','pending')`,
// `id IN (1,2,3)`.
func In[L identType | *ast.IndexExpr, R listLiteralType](l L, r R) *ast.BinaryExpr {
	return &ast.BinaryExpr{
		Left:  leftOperand(l),
		Op:    newKindToken(token.IN),
		Right: NewListLiteral(r),
	}
}

// Like builds `l LIKE r`. Right operand must be a string. Examples:
// `name LIKE 'John%'`, `email LIKE '%@gmail.com'`.
func Like[L identType | *ast.IndexExpr, R string | *ast.Literal](l L, r R) *ast.BinaryExpr {
	return &ast.BinaryExpr{
		Left:  leftOperand(l),
		Op:    newKindToken(token.LIKE),
		Right: NewLiteral(r),
	}
}
