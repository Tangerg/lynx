package filter

import (
	"github.com/Tangerg/lynx/core/vectorstore/filter/ast"
)

// ExprBuilder is the AND-by-default fluent builder for filter
// expressions. Each comparison/membership method appends a new
// predicate joined to the running expression with AND; nested
// [ExprBuilder.And] / [ExprBuilder.Or] / [ExprBuilder.Not] take a
// callback that builds a sub-expression. The first error encountered
// is captured and short-circuits subsequent calls — call sites can
// keep chaining and check the error once at [Builder.Build].
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

// EQ appends `l == r` joined with AND. Captures any conversion error.
func (b *ExprBuilder) EQ(l, r any) *ExprBuilder {
	if b.err != nil {
		return b
	}

	literal, err := newLiteral(r)
	if err != nil {
		b.err = err
		return b
	}

	if indexExpr, ok := l.(*ast.IndexExpr); ok {
		b.and(EQ(indexExpr, literal))
		return b
	}

	ident, err := newIdent(l)
	if err != nil {
		b.err = err
		return b
	}

	b.and(EQ(ident, literal))
	return b
}

// NE appends `l != r` joined with AND.
func (b *ExprBuilder) NE(l, r any) *ExprBuilder {
	if b.err != nil {
		return b
	}

	literal, err := newLiteral(r)
	if err != nil {
		b.err = err
		return b
	}

	if indexExpr, ok := l.(*ast.IndexExpr); ok {
		b.and(NE(indexExpr, literal))
		return b
	}

	ident, err := newIdent(l)
	if err != nil {
		b.err = err
		return b
	}

	b.and(NE(ident, literal))
	return b
}

// LT appends `l < r` joined with AND. Right operand must be numeric.
func (b *ExprBuilder) LT(l, r any) *ExprBuilder {
	if b.err != nil {
		return b
	}

	literal, err := newLiteral(r)
	if err != nil {
		b.err = err
		return b
	}

	if indexExpr, ok := l.(*ast.IndexExpr); ok {
		b.and(LT(indexExpr, literal))
		return b
	}

	ident, err := newIdent(l)
	if err != nil {
		b.err = err
		return b
	}

	b.and(LT(ident, literal))
	return b
}

// LE appends `l <= r` joined with AND. Right operand must be numeric.
func (b *ExprBuilder) LE(l, r any) *ExprBuilder {
	if b.err != nil {
		return b
	}

	literal, err := newLiteral(r)
	if err != nil {
		b.err = err
		return b
	}

	if indexExpr, ok := l.(*ast.IndexExpr); ok {
		b.and(LE(indexExpr, literal))
		return b
	}

	ident, err := newIdent(l)
	if err != nil {
		b.err = err
		return b
	}

	b.and(LE(ident, literal))
	return b
}

// GT appends `l > r` joined with AND. Right operand must be numeric.
func (b *ExprBuilder) GT(l, r any) *ExprBuilder {
	if b.err != nil {
		return b
	}

	literal, err := newLiteral(r)
	if err != nil {
		b.err = err
		return b
	}

	if indexExpr, ok := l.(*ast.IndexExpr); ok {
		b.and(GT(indexExpr, literal))
		return b
	}

	ident, err := newIdent(l)
	if err != nil {
		b.err = err
		return b
	}

	b.and(GT(ident, literal))
	return b
}

// GE appends `l >= r` joined with AND. Right operand must be numeric.
func (b *ExprBuilder) GE(l, r any) *ExprBuilder {
	if b.err != nil {
		return b
	}

	literal, err := newLiteral(r)
	if err != nil {
		b.err = err
		return b
	}

	if indexExpr, ok := l.(*ast.IndexExpr); ok {
		b.and(GE(indexExpr, literal))
		return b
	}

	ident, err := newIdent(l)
	if err != nil {
		b.err = err
		return b
	}

	b.and(GE(ident, literal))
	return b
}

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

	if indexExpr, ok := l.(*ast.IndexExpr); ok {
		b.and(In(indexExpr, list))
		return b
	}

	ident, err := newIdent(l)
	if err != nil {
		b.err = err
		return b
	}

	b.and(In(ident, list))
	return b
}

// Like appends `l LIKE r` joined with AND. Right operand must be a
// string.
func (b *ExprBuilder) Like(l, r any) *ExprBuilder {
	if b.err != nil {
		return b
	}

	literal, err := newLiteral(r)
	if err != nil {
		b.err = err
		return b
	}

	if indexExpr, ok := l.(*ast.IndexExpr); ok {
		b.and(Like(indexExpr, literal))
		return b
	}

	ident, err := newIdent(l)
	if err != nil {
		b.err = err
		return b
	}

	b.and(Like(ident, literal))
	return b
}

// And runs fn against a fresh sub-builder and joins the resulting
// expression with AND. Sub-builder errors propagate.
func (b *ExprBuilder) And(fn func(*ExprBuilder)) *ExprBuilder {
	if b.err != nil {
		return b
	}

	sub := NewExprBuilder()
	fn(sub)

	if sub.err != nil {
		b.err = sub.err
		return b
	}

	b.and(sub.expr)
	return b
}

// Or runs fn against a fresh sub-builder and joins the resulting
// expression with OR. Sub-builder errors propagate.
func (b *ExprBuilder) Or(fn func(*ExprBuilder)) *ExprBuilder {
	if b.err != nil {
		return b
	}

	sub := NewExprBuilder()
	fn(sub)

	if sub.err != nil {
		b.err = sub.err
		return b
	}

	b.or(sub.expr)
	return b
}

// Not runs fn against a fresh sub-builder, wraps the resulting
// expression in NOT, and joins it with AND. An empty sub-builder
// (nil expression) is silently skipped.
func (b *ExprBuilder) Not(fn func(*ExprBuilder)) *ExprBuilder {
	if b.err != nil {
		return b
	}

	sub := NewExprBuilder()
	fn(sub)

	if sub.err != nil {
		b.err = sub.err
		return b
	}
	if any(sub.expr) == nil {
		return b
	}

	b.and(Not(sub.expr))
	return b
}

// Builder is the public entry point for fluent construction of filter
// expressions. It thinly wraps [ExprBuilder] and exposes
// [Builder.Build] to collect the final AST plus the first deferred
// error.
type Builder struct {
	exprBuilder *ExprBuilder
}

// NewBuilder returns a fresh [Builder] backed by an empty
// [ExprBuilder].
func NewBuilder() *Builder {
	return &Builder{exprBuilder: NewExprBuilder()}
}

// EQ — see [ExprBuilder.EQ].
func (b *Builder) EQ(l, r any) *Builder {
	b.exprBuilder.EQ(l, r)
	return b
}

// NE — see [ExprBuilder.NE].
func (b *Builder) NE(l, r any) *Builder {
	b.exprBuilder.NE(l, r)
	return b
}

// LT — see [ExprBuilder.LT].
func (b *Builder) LT(l, r any) *Builder {
	b.exprBuilder.LT(l, r)
	return b
}

// LE — see [ExprBuilder.LE].
func (b *Builder) LE(l, r any) *Builder {
	b.exprBuilder.LE(l, r)
	return b
}

// GT — see [ExprBuilder.GT].
func (b *Builder) GT(l, r any) *Builder {
	b.exprBuilder.GT(l, r)
	return b
}

// GE — see [ExprBuilder.GE].
func (b *Builder) GE(l, r any) *Builder {
	b.exprBuilder.GE(l, r)
	return b
}

// In — see [ExprBuilder.In].
func (b *Builder) In(l, r any) *Builder {
	b.exprBuilder.In(l, r)
	return b
}

// Like — see [ExprBuilder.Like].
func (b *Builder) Like(l, r any) *Builder {
	b.exprBuilder.Like(l, r)
	return b
}

// And — see [ExprBuilder.And].
func (b *Builder) And(fn func(*ExprBuilder)) *Builder {
	b.exprBuilder.And(fn)
	return b
}

// Or — see [ExprBuilder.Or].
func (b *Builder) Or(fn func(*ExprBuilder)) *Builder {
	b.exprBuilder.Or(fn)
	return b
}

// Not — see [ExprBuilder.Not].
func (b *Builder) Not(fn func(*ExprBuilder)) *Builder {
	b.exprBuilder.Not(fn)
	return b
}

// Build returns the constructed AST and the first error captured
// during the chain. A nil expression with a nil error means no
// predicate was added.
func (b *Builder) Build() (ast.Expr, error) {
	if b.exprBuilder.err != nil {
		return nil, b.exprBuilder.err
	}
	return b.exprBuilder.expr, nil
}
