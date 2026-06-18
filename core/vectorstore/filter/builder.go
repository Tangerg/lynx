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

func NewExprBuilder() *ExprBuilder {
	return &ExprBuilder{}
}

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

func (b *ExprBuilder) EQ(l, r any) *ExprBuilder { return b.appendBinary(l, r, token.EQ) }

func (b *ExprBuilder) NE(l, r any) *ExprBuilder { return b.appendBinary(l, r, token.NE) }

func (b *ExprBuilder) LT(l, r any) *ExprBuilder { return b.appendBinary(l, r, token.LT) }

func (b *ExprBuilder) LE(l, r any) *ExprBuilder { return b.appendBinary(l, r, token.LE) }

func (b *ExprBuilder) GT(l, r any) *ExprBuilder { return b.appendBinary(l, r, token.GT) }

func (b *ExprBuilder) GE(l, r any) *ExprBuilder { return b.appendBinary(l, r, token.GE) }

func (b *ExprBuilder) Like(l, r any) *ExprBuilder { return b.appendBinary(l, r, token.LIKE) }

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

func (b *ExprBuilder) And(fn func(*ExprBuilder)) *ExprBuilder {
	if expr, ok := b.subExpr(fn); ok {
		b.and(expr)
	}
	return b
}

func (b *ExprBuilder) Or(fn func(*ExprBuilder)) *ExprBuilder {
	if expr, ok := b.subExpr(fn); ok {
		b.or(expr)
	}
	return b
}

func (b *ExprBuilder) Not(fn func(*ExprBuilder)) *ExprBuilder {
	if expr, ok := b.subExpr(fn); ok && expr != nil {
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
