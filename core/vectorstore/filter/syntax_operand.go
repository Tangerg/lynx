package filter

import (
	"fmt"

	"github.com/Tangerg/lynx/core/vectorstore/filter/ast"
	"github.com/Tangerg/lynx/core/vectorstore/filter/token"
)

// identType is the input constraint for [NewIdent]: a raw string is
// turned into a fresh [ast.Ident]; an existing [*ast.Ident] passes
// through unchanged.
type identType interface {
	string | *ast.Ident
}

func newIdent(value any) (*ast.Ident, error) {
	switch typed := value.(type) {
	case string:
		return &ast.Ident{
			Token: token.OfIdent(typed, token.NoPosition, token.NoPosition),
			Value: typed,
		}, nil
	case *ast.Ident:
		return typed, nil
	default:
		return nil, fmt.Errorf("filter.newIdent: expected string or *ast.Ident, got %T (%v)",
			value, value)
	}
}

// NewIdent builds an [*ast.Ident] from either a string name or an
// existing identifier node. Position is always zero — these are
// hand-built nodes, not parsed ones.
func NewIdent[T identType](value T) *ast.Ident {
	ident, err := newIdent(value)
	if err != nil {
		// Unreachable while the generic constraint is honored.
		panic(fmt.Errorf("filter.NewIdent: %w", err))
	}
	return ident
}

func identOrIndex(l any) (ast.Expr, error) {
	if ix, ok := l.(*ast.IndexExpr); ok {
		return ix, nil
	}
	return newIdent(l)
}

func leftOperand[L identType | *ast.IndexExpr](l L) ast.Expr {
	expr, _ := identOrIndex(l)
	return expr
}
