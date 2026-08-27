package filter

import (
	"fmt"

	"github.com/samber/lo"
)

// Expr is the stable root of the immutable filter expression tree.
type Expr interface {
	// Start returns the inclusive source position of the first token. Trees built
	// programmatically use a zero position.
	Start() Position
	// End returns the exclusive source position after the last token. Trees built
	// programmatically use a zero position.
	End() Position
	// Equal compares semantic tree shape and values, ignoring pointer identity.
	// It must be total for every valid Expr implementation.
	Equal(Expr) bool
	// expr seals the expression algebra so validation and traversal can remain
	// exhaustive when new syntax is introduced.
	expr()
}

// Selector identifies a metadata value. Ident selects a top-level field;
// IndexExpr selects a nested field or array element.
type Selector interface {
	Expr
	// Path returns an independently owned metadata path. It fails when the
	// selector contains an invalid identifier or index expression.
	Path() ([]string, error)
	// selector prevents predicate-only nodes from entering operand positions.
	selector()
}

// Predicate is a complete boolean expression. Constructed and parsed trees are
// immutable; behavior stays on the tree instead of in extraction helpers.
type Predicate interface {
	Expr
	fmt.Stringer
	// Validate proves the complete subtree is structurally legal before a
	// visitor, formatter, or backend compiler observes it.
	Validate() error
	// Accept dispatches the complete validated predicate to visitor. Traversal
	// order belongs to the visitor; nil visitors and invalid trees are rejected.
	Accept(Visitor) error
	// predicate seals the boolean-expression subset of Expr.
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
