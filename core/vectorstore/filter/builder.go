package filter

import (
	"github.com/Tangerg/lynx/core/vectorstore/filter/ast"
	"github.com/Tangerg/lynx/core/vectorstore/filter/token"
)

// ExprBuilder is the AND-by-default fluent builder for filter
// expressions. Each comparison/membership method appends a new
// predicate joined to the running expression with AND; nested
// [ExprBuilder.And] / [ExprBuilder.Or] / [ExprBuilder.Not] take a
// callback that builds a sub-expression. The first error encountered
// is captured and short-circuits subsequent calls — call sites can
// keep chaining and check the error once at [ExprBuilder.Build].
type ExprBuilder struct {
	err  error
	expr ast.ComputedExpr
}

// NewExprBuilder returns an empty [ExprBuilder].
func NewExprBuilder() *ExprBuilder {
	return &ExprBuilder{}
}

// and joins expr to the running expression with AND. nil is treated as
// a no-op so empty sub-builders don't introduce phantom nodes.
func (b *ExprBuilder) and(expr ast.ComputedExpr) {
	if expr == nil {
		return
	}

	if b.expr == nil {
		b.expr = expr
		return
	}

	b.expr = And(b.expr, expr)
}

// or joins expr to the running expression with OR. nil is treated as a
// no-op (see [ExprBuilder.and]).
func (b *ExprBuilder) or(expr ast.ComputedExpr) {
	if expr == nil {
		return
	}

	if b.expr == nil {
		b.expr = expr
		return
	}

	b.expr = Or(b.expr, expr)
}

// appendBinary is the shared body of every comparison/match builder
// method (EQ, NE, LT, LE, GT, GE, Like). It resolves the left operand
// (identifier or pre-built index expression), builds a literal from
// the right operand, and joins the resulting `left op right` to the
// running expression with AND. Errors short-circuit the chain.
func (b *ExprBuilder) appendBinary(l, r any, op token.Kind) *ExprBuilder {
	if b.err != nil {
		return b
	}

	literal, err := newLiteral(r)
	if err != nil {
		b.err = err
		return b
	}

	left, err := identOrIndex(l)
	if err != nil {
		b.err = err
		return b
	}

	b.and(&ast.BinaryExpr{Left: left, Op: newKindToken(op), Right: literal})
	return b
}

// EQ appends `l == r` joined with AND.
func (b *ExprBuilder) EQ(l, r any) *ExprBuilder { return b.appendBinary(l, r, token.EQ) }

// NE appends `l != r` joined with AND.
func (b *ExprBuilder) NE(l, r any) *ExprBuilder { return b.appendBinary(l, r, token.NE) }

// LT appends `l < r` joined with AND. Right operand must be numeric.
func (b *ExprBuilder) LT(l, r any) *ExprBuilder { return b.appendBinary(l, r, token.LT) }

// LE appends `l <= r` joined with AND. Right operand must be numeric.
func (b *ExprBuilder) LE(l, r any) *ExprBuilder { return b.appendBinary(l, r, token.LE) }

// GT appends `l > r` joined with AND. Right operand must be numeric.
func (b *ExprBuilder) GT(l, r any) *ExprBuilder { return b.appendBinary(l, r, token.GT) }

// GE appends `l >= r` joined with AND. Right operand must be numeric.
func (b *ExprBuilder) GE(l, r any) *ExprBuilder { return b.appendBinary(l, r, token.GE) }

// Like appends `l LIKE r` joined with AND. Right operand must be a
// string.
func (b *ExprBuilder) Like(l, r any) *ExprBuilder { return b.appendBinary(l, r, token.LIKE) }

// In appends `l IN (...)` joined with AND. Right operand is any slice
// type accepted by [newListLiteral].
func (b *ExprBuilder) In(l, r any) *ExprBuilder {
	if b.err != nil {
		return b
	}

	list, err := newListLiteral(r)
	if err != nil {
		b.err = err
		return b
	}

	left, err := identOrIndex(l)
	if err != nil {
		b.err = err
		return b
	}

	b.and(&ast.BinaryExpr{Left: left, Op: newKindToken(token.IN), Right: list})
	return b
}

// subExpr runs fn against a fresh sub-builder and returns the result,
// propagating any sub-error to b. The bool reports whether the caller
// should proceed (false on prior error or sub-error).
func (b *ExprBuilder) subExpr(fn func(*ExprBuilder)) (ast.ComputedExpr, bool) {
	if b.err != nil {
		return nil, false
	}
	sub := NewExprBuilder()
	fn(sub)
	if sub.err != nil {
		b.err = sub.err
		return nil, false
	}
	return sub.expr, true
}

// And runs fn against a fresh sub-builder and joins the resulting
// expression with AND. Sub-builder errors propagate.
func (b *ExprBuilder) And(fn func(*ExprBuilder)) *ExprBuilder {
	if expr, ok := b.subExpr(fn); ok {
		b.and(expr)
	}
	return b
}

// Or runs fn against a fresh sub-builder and joins the resulting
// expression with OR. Sub-builder errors propagate.
func (b *ExprBuilder) Or(fn func(*ExprBuilder)) *ExprBuilder {
	if expr, ok := b.subExpr(fn); ok {
		b.or(expr)
	}
	return b
}

// Not runs fn against a fresh sub-builder, wraps the resulting
// expression in NOT, and joins it with AND. An empty sub-builder
// (nil expression) is silently skipped.
func (b *ExprBuilder) Not(fn func(*ExprBuilder)) *ExprBuilder {
	if expr, ok := b.subExpr(fn); ok && any(expr) != nil {
		b.and(Not(expr))
	}
	return b
}

// Build returns the constructed AST and the first error captured
// during the chain. A nil expression with a nil error means no
// predicate was added.
func (b *ExprBuilder) Build() (ast.Expr, error) {
	if b.err != nil {
		return nil, b.err
	}
	return b.expr, nil
}
