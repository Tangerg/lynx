package filter

import (
	"github.com/Tangerg/lynx/core/vectorstore/filter/ast"
	"github.com/Tangerg/lynx/core/vectorstore/filter/token"
	"github.com/Tangerg/lynx/pkg/math"
)

// Index builds `left[index]`. Left can be a name, an existing
// identifier, or a previously built index expression — the latter
// supports nested access like `matrix[1][2]`. Index must be numeric or
// a string.
func Index[L identType | *ast.IndexExpr, I math.NumericType | string | *ast.Literal](left L, index I) *ast.IndexExpr {
	expr := &ast.IndexExpr{
		LBrack: newKindToken(token.LBRACK),
		RBrack: newKindToken(token.RBRACK),
		Index:  NewLiteral(index),
	}

	switch typedL := any(left).(type) {
	case string:
		expr.Left = NewIdent(typedL)
	case *ast.Ident:
		expr.Left = typedL
	case *ast.IndexExpr:
		expr.Left = typedL
	}
	return expr
}
