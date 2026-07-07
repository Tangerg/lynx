package filter

import (
	"github.com/Tangerg/lynx/core/vectorstore/filter/ast"
	"github.com/Tangerg/lynx/core/vectorstore/filter/token"
)

func logic[L ast.ComputedExpr, R ast.ComputedExpr](l L, r R, op token.Kind) *ast.BinaryExpr {
	return &ast.BinaryExpr{
		Left:  l,
		Op:    newKindToken(op),
		Right: r,
	}
}

// And builds `l AND r`. Both operands must be computed expressions —
// raw literals or identifiers do not satisfy [ast.ComputedExpr].
func And[L ast.ComputedExpr, R ast.ComputedExpr](l L, r R) *ast.BinaryExpr {
	return logic(l, r, token.AND)
}

// Or builds `l OR r`. Both operands must be computed expressions.
func Or[L ast.ComputedExpr, R ast.ComputedExpr](l L, r R) *ast.BinaryExpr {
	return logic(l, r, token.OR)
}

// Not builds `NOT r`. The operand must be a computed expression.
func Not[T ast.ComputedExpr](r T) *ast.UnaryExpr {
	return &ast.UnaryExpr{
		Op:    newKindToken(token.NOT),
		Right: r,
	}
}
