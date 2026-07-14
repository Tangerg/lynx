package filter

// ExprBuilder is the AND-by-default fluent builder for filter
// expressions. Each comparison/membership method appends a new
// predicate joined to the running expression with AND; nested
// [ExprBuilder.And] / [ExprBuilder.Or] / [ExprBuilder.Not] take a
// callback that builds a sub-expression. The first error encountered
// is captured and short-circuits subsequent calls — call sites can
// keep chaining and check the error once at [ExprBuilder.Build].
type ExprBuilder struct {
	err  error
	expr ComputedExpr
}

func NewExprBuilder() *ExprBuilder {
	return &ExprBuilder{}
}

func (b *ExprBuilder) and(expr ComputedExpr) {
	if expr == nil {
		return
	}

	if b.expr == nil {
		b.expr = expr
		return
	}

	b.expr = And(b.expr, expr)
}

func (b *ExprBuilder) or(expr ComputedExpr) {
	if expr == nil {
		return
	}

	if b.expr == nil {
		b.expr = expr
		return
	}

	b.expr = Or(b.expr, expr)
}

func (b *ExprBuilder) appendBinary(l, r any, op Operator) *ExprBuilder {
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

	b.and(&BinaryExpr{Left: left, Op: op, Right: literal})
	return b
}

func (b *ExprBuilder) EQ(l, r any) *ExprBuilder { return b.appendBinary(l, r, OpEqual) }

func (b *ExprBuilder) NE(l, r any) *ExprBuilder { return b.appendBinary(l, r, OpNotEqual) }

func (b *ExprBuilder) LT(l, r any) *ExprBuilder { return b.appendBinary(l, r, OpLess) }

func (b *ExprBuilder) LE(l, r any) *ExprBuilder { return b.appendBinary(l, r, OpLessEqual) }

func (b *ExprBuilder) GT(l, r any) *ExprBuilder { return b.appendBinary(l, r, OpGreater) }

func (b *ExprBuilder) GE(l, r any) *ExprBuilder { return b.appendBinary(l, r, OpGreaterEqual) }

func (b *ExprBuilder) Like(l, r any) *ExprBuilder { return b.appendBinary(l, r, OpLike) }

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

	b.and(&BinaryExpr{Left: left, Op: OpIn, Right: list})
	return b
}

func (b *ExprBuilder) subExpr(fn func(*ExprBuilder)) (ComputedExpr, bool) {
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

// Build returns the constructed expression and the first construction error.
// Call [Validate] before sending a programmatically built tree to a provider.
func (b *ExprBuilder) Build() (Expr, error) {
	if b.err != nil {
		return nil, b.err
	}
	return b.expr, nil
}
