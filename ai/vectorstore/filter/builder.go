package filter

import (
	"github.com/Tangerg/lynx/ai/vectorstore/filter/ast"
)

// buildExpr todo Check grammar and semantics
func buildExpr(b *ExprBuilder) (ast.ComputedExpr, error) {
	return b.expr, nil
}

type ExprBuilder struct {
	err  error
	expr ast.ComputedExpr
}

func NewExprBuilder() *ExprBuilder {
	return &ExprBuilder{}
}

func (b *ExprBuilder) and(expr ast.ComputedExpr) {
	if b.expr == nil {
		b.expr = expr
		return
	}

	andExpr := And(b.expr, expr)
	b.expr = andExpr
}

func (b *ExprBuilder) or(expr ast.ComputedExpr) {
	if b.expr == nil {
		b.expr = expr
	}

	orExpr := Or(b.expr, expr)
	b.expr = orExpr
}

func (b *ExprBuilder) EQ(l, r any) *ExprBuilder {
	if b.err != nil {
		return b
	}

	id, err := newIdent(l)
	if err != nil {
		b.err = err
		return b
	}

	lit, err := newLiteral(r)
	if err != nil {
		b.err = err
		return b
	}

	eqExpr := EQ(id, lit)
	b.and(eqExpr)

	return b
}

func (b *ExprBuilder) NE(l, r any) *ExprBuilder {
	if b.err != nil {
		return b
	}

	id, err := newIdent(l)
	if err != nil {
		b.err = err
		return b
	}

	lit, err := newLiteral(r)
	if err != nil {
		b.err = err
		return b
	}

	neExpr := NE(id, lit)
	b.and(neExpr)

	return b
}

func (b *ExprBuilder) LT(l, r any) *ExprBuilder {
	if b.err != nil {
		return b
	}

	id, err := newIdent(l)
	if err != nil {
		b.err = err
		return b
	}

	lit, err := newLiteral(r)
	if err != nil {
		b.err = err
		return b
	}

	ltExpr := LT(id, lit)
	b.and(ltExpr)

	return b
}

func (b *ExprBuilder) LE(l, r any) *ExprBuilder {
	if b.err != nil {
		return b
	}

	id, err := newIdent(l)
	if err != nil {
		b.err = err
		return b
	}

	lit, err := newLiteral(r)
	if err != nil {
		b.err = err
		return b
	}

	leExpr := LE(id, lit)
	b.and(leExpr)

	return b
}

func (b *ExprBuilder) GT(l, r any) *ExprBuilder {
	if b.err != nil {
		return b
	}

	id, err := newIdent(l)
	if err != nil {
		b.err = err
		return b
	}

	lit, err := newLiteral(r)
	if err != nil {
		b.err = err
		return b
	}

	gtExpr := GT(id, lit)
	b.and(gtExpr)

	return b
}

func (b *ExprBuilder) GE(l, r any) *ExprBuilder {
	if b.err != nil {
		return b
	}

	id, err := newIdent(l)
	if err != nil {
		b.err = err
		return b
	}

	lit, err := newLiteral(r)
	if err != nil {
		b.err = err
		return b
	}

	geExpr := GE(id, lit)
	b.and(geExpr)

	return b
}

func (b *ExprBuilder) In(l, r any) *ExprBuilder {
	if b.err != nil {
		return b
	}

	id, err := newIdent(l)
	if err != nil {
		b.err = err
		return b
	}

	listLit, err := newListLiteral(r)
	if err != nil {
		b.err = err
		return b
	}

	inExpr := In(id, listLit)
	b.and(inExpr)

	return b
}

func (b *ExprBuilder) Like(l, r any) *ExprBuilder {
	if b.err != nil {
		return b
	}

	id, err := newIdent(l)
	if err != nil {
		b.err = err
		return b
	}

	lit, err := newLiteral(r)
	if err != nil {
		b.err = err
		return b
	}

	likeExpr := Like(id, lit)
	b.and(likeExpr)

	return b
}

func (b *ExprBuilder) And(fn func(*ExprBuilder)) *ExprBuilder {
	if b.err != nil {
		return b
	}

	sub := NewExprBuilder()
	fn(sub)

	subExpr, err := buildExpr(sub)
	if err != nil {
		b.err = err
		return b
	}

	b.and(subExpr)

	return b
}

func (b *ExprBuilder) Or(fn func(*ExprBuilder)) *ExprBuilder {
	if b.err != nil {
		return b
	}

	sub := NewExprBuilder()
	fn(sub)

	subExpr, err := buildExpr(sub)
	if err != nil {
		b.err = err
		return b
	}

	b.or(subExpr)

	return b
}

func (b *ExprBuilder) Not(fn func(*ExprBuilder)) *ExprBuilder {
	if b.err != nil {
		return b
	}

	sub := NewExprBuilder()
	fn(sub)

	subExpr, err := buildExpr(sub)
	if err != nil {
		b.err = err
		return b
	}

	notExpr := Not(subExpr)
	b.and(notExpr)

	return b
}

type Builder struct {
	exprBuilder *ExprBuilder
}

func (b *Builder) EQ(l, r any) *Builder {
	b.exprBuilder.EQ(l, r)
	return b
}

func (b *Builder) NE(l, r any) *Builder {
	b.exprBuilder.NE(l, r)
	return b
}

func (b *Builder) LT(l, r any) *Builder {
	b.exprBuilder.LT(l, r)
	return b
}

func (b *Builder) LE(l, r any) *Builder {
	b.exprBuilder.LE(l, r)
	return b
}

func (b *Builder) GT(l, r any) *Builder {
	b.exprBuilder.GT(l, r)
	return b
}

func (b *Builder) GE(l, r any) *Builder {
	b.exprBuilder.GE(l, r)
	return b
}

func (b *Builder) And(fn func(*ExprBuilder)) *Builder {
	b.exprBuilder.And(fn)
	return b
}

func (b *Builder) Or(fn func(*ExprBuilder)) *Builder {
	b.exprBuilder.Or(fn)
	return b
}

func (b *Builder) Not(fn func(*ExprBuilder)) *Builder {
	b.exprBuilder.Not(fn)
	return b
}

func (b *Builder) In(l, r any) *Builder {
	b.exprBuilder.In(l, r)
	return b
}

func (b *Builder) Like(l, r any) *Builder {
	b.exprBuilder.Like(l, r)
	return b
}

func (b *Builder) Build() (ast.Expr, error) {
	if b.exprBuilder.err != nil {
		return nil, b.exprBuilder.err
	}
	return buildExpr(b.exprBuilder)
}

func NewBuilder() *Builder {
	return &Builder{
		exprBuilder: NewExprBuilder(),
	}
}
