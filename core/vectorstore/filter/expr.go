package filter

import (
	"fmt"

	"github.com/samber/lo"
)

// Expr is the stable root of the immutable filter expression tree.
type Expr interface {
	Start() Position
	End() Position
	Equal(Expr) bool
	expr()
}

// Selector identifies a metadata value. Ident selects a top-level field;
// IndexExpr selects a nested field or array element.
type Selector interface {
	Expr
	Path() ([]string, error)
	selector()
}

// Predicate is a complete boolean expression. Constructed and parsed trees are
// immutable; behavior stays on the tree instead of in extraction helpers.
type Predicate interface {
	Expr
	fmt.Stringer
	Validate() error
	Accept(Visitor) error
	predicate()
}

func equalExpr(left, right Expr) bool {
	leftNil := lo.IsNil(left)
	rightNil := lo.IsNil(right)
	if leftNil || rightNil {
		return leftNil && rightNil
	}
	return left.Equal(right)
}
