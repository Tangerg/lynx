package visitors

import (
	"github.com/Tangerg/lynx/core/vectorstore/filter/internal/ast"
	"github.com/Tangerg/lynx/core/vectorstore/filter/internal/token"
)

// Optimizer rewrites a filter AST into a smaller, semantically identical
// form, stripping dead logic before a backend visitor translates it.
// Run it after [Analyzer] (on a valid tree) and before a backend
// visitor.
//
// Every rewrite is a boolean-algebra identity that holds pointwise for
// any truth value of its operands — including SQL NULL / "unknown"
// (Kleene three-valued logic) — so the set of matching records never
// changes; only redundant structure is removed.
//
// Rewrites (applied bottom-up, reaching a fixed point in one pass since
// each parent sees already-simplified children):
//
//   - Multiple-negation collapse: NOT NOT x → x, NOT NOT NOT x → NOT x.
//   - Idempotent law:             x AND x → x, x OR x → x.
//   - Absorption law:             x AND (x OR y) → x, x OR (x AND y) → x.
//
// Comparison / IN / LIKE / IS nodes and their operands carry no logical
// structure to fold and pass through untouched. The optimizer never
// mutates the input tree: it returns new nodes only where a rewrite
// applied and shares untouched sub-trees.
type Optimizer struct{}

func NewOptimizer() *Optimizer { return &Optimizer{} }

func (o *Optimizer) Optimize(expr ast.Expr) ast.Expr {
	if expr == nil {
		return nil
	}
	return o.rewrite(expr)
}

func (o *Optimizer) rewrite(expr ast.Expr) ast.Expr {
	switch node := expr.(type) {
	case *ast.UnaryExpr:
		return o.rewriteUnary(node)
	case *ast.BinaryExpr:
		return o.rewriteBinary(node)
	default:
		// Idents, literals, list literals, index expressions: no logical
		// structure to fold.
		return expr
	}
}

func (o *Optimizer) rewriteUnary(u *ast.UnaryExpr) ast.Expr {
	right := o.rewrite(u.Right)

	if u.Op.Kind.Is(token.NOT) {
		if inner, ok := right.(*ast.UnaryExpr); ok && inner.Op.Kind.Is(token.NOT) {
			return inner.Right // NOT NOT x → x (inner.Right is already simplified)
		}
	}

	if right == u.Right {
		return u
	}
	// right came from rewriting a ComputedExpr operand, so it stays one.
	if computed, ok := right.(ast.ComputedExpr); ok {
		return &ast.UnaryExpr{Op: u.Op, Right: computed}
	}
	return u
}

func (o *Optimizer) rewriteBinary(b *ast.BinaryExpr) ast.Expr {
	if !b.Op.Kind.IsLogicalOperator() {
		return b
	}

	left := o.rewrite(b.Left)
	right := o.rewrite(b.Right)

	if left.Equal(right) {
		return left
	}

	dual := token.OR
	if b.Op.Kind.Is(token.OR) {
		dual = token.AND
	}
	switch {
	case absorbs(left, right, dual):
		return left
	case absorbs(right, left, dual):
		return right
	}

	if left == b.Left && right == b.Right {
		return b
	}
	return &ast.BinaryExpr{Left: left, Op: b.Op, Right: right}
}

func absorbs(x, outer ast.Expr, op token.Kind) bool {
	bin, ok := outer.(*ast.BinaryExpr)
	if !ok || !bin.Op.Kind.Is(op) {
		return false
	}
	return x.Equal(bin.Left) || x.Equal(bin.Right)
}
