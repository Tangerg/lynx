package filter

import "fmt"

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
	leftNil := isNilExpr(left)
	rightNil := isNilExpr(right)
	if leftNil || rightNil {
		return leftNil && rightNil
	}
	return left.Equal(right)
}

func isNilExpr(expr Expr) bool {
	if expr == nil {
		return true
	}
	switch node := expr.(type) {
	case *Ident:
		return node == nil
	case *Literal:
		return node == nil
	case *ListLiteral:
		return node == nil
	case *UnaryExpr:
		return node == nil
	case *BinaryExpr:
		return node == nil
	case *IndexExpr:
		return node == nil
	default:
		return false
	}
}
