package filter

import (
	"github.com/Tangerg/lynx/ai/vectorstore/filter/ast"
)

// ExprBuilder provides a fluent API for building complex filter expressions.
// It uses AND logic by default to combine expressions and maintains error state
// for deferred error handling throughout the building process.
type ExprBuilder struct {
	err  error
	expr ast.ComputedExpr
}

// NewExprBuilder creates a new expression builder with empty state.
func NewExprBuilder() *ExprBuilder {
	return &ExprBuilder{}
}

// and combines the given expression with the current expression using AND logic.
// If the current expression is nil, the given expression becomes the root.
// Nil expressions are ignored to prevent invalid AST nodes.
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

// or combines the given expression with the current expression using OR logic.
// If the current expression is nil, the given expression becomes the root.
// Nil expressions are ignored to prevent invalid AST nodes.
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

// EQ creates an equality comparison expression and adds it using AND logic.
// Supports both identifier and index expressions as the left operand.
// Returns the builder for method chaining with error propagation.
func (b *ExprBuilder) EQ(l, r any) *ExprBuilder {
	if b.err != nil {
		return b
	}

	literal, err := newLiteral(r)
	if err != nil {
		b.err = err
		return b
	}

	indexExpr, ok := l.(*ast.IndexExpr)
	if ok {
		eqExpr := EQ(indexExpr, literal)
		b.and(eqExpr)
		return b
	}

	ident, err := newIdent(l)
	if err != nil {
		b.err = err
		return b
	}

	eqExpr := EQ(ident, literal)
	b.and(eqExpr)

	return b
}

// NE creates an inequality comparison expression and adds it using AND logic.
// Supports both identifier and index expressions as the left operand.
// Returns the builder for method chaining with error propagation.
func (b *ExprBuilder) NE(l, r any) *ExprBuilder {
	if b.err != nil {
		return b
	}

	literal, err := newLiteral(r)
	if err != nil {
		b.err = err
		return b
	}

	indexExpr, ok := l.(*ast.IndexExpr)
	if ok {
		neExpr := NE(indexExpr, literal)
		b.and(neExpr)
		return b
	}

	ident, err := newIdent(l)
	if err != nil {
		b.err = err
		return b
	}

	neExpr := NE(ident, literal)
	b.and(neExpr)

	return b
}

// LT creates a less-than comparison expression and adds it using AND logic.
// Supports both identifier and index expressions as the left operand.
// Returns the builder for method chaining with error propagation.
func (b *ExprBuilder) LT(l, r any) *ExprBuilder {
	if b.err != nil {
		return b
	}

	literal, err := newLiteral(r)
	if err != nil {
		b.err = err
		return b
	}

	indexExpr, ok := l.(*ast.IndexExpr)
	if ok {
		ltExpr := LT(indexExpr, literal)
		b.and(ltExpr)
		return b
	}

	ident, err := newIdent(l)
	if err != nil {
		b.err = err
		return b
	}

	ltExpr := LT(ident, literal)
	b.and(ltExpr)

	return b
}

// LE creates a less-than-or-equal comparison expression and adds it using AND logic.
// Supports both identifier and index expressions as the left operand.
// Returns the builder for method chaining with error propagation.
func (b *ExprBuilder) LE(l, r any) *ExprBuilder {
	if b.err != nil {
		return b
	}

	literal, err := newLiteral(r)
	if err != nil {
		b.err = err
		return b
	}

	indexExpr, ok := l.(*ast.IndexExpr)
	if ok {
		leExpr := LE(indexExpr, literal)
		b.and(leExpr)
		return b
	}

	ident, err := newIdent(l)
	if err != nil {
		b.err = err
		return b
	}

	leExpr := LE(ident, literal)
	b.and(leExpr)

	return b
}

// GT creates a greater-than comparison expression and adds it using AND logic.
// Supports both identifier and index expressions as the left operand.
// Returns the builder for method chaining with error propagation.
func (b *ExprBuilder) GT(l, r any) *ExprBuilder {
	if b.err != nil {
		return b
	}

	literal, err := newLiteral(r)
	if err != nil {
		b.err = err
		return b
	}

	indexExpr, ok := l.(*ast.IndexExpr)
	if ok {
		gtExpr := GT(indexExpr, literal)
		b.and(gtExpr)
		return b
	}

	ident, err := newIdent(l)
	if err != nil {
		b.err = err
		return b
	}

	gtExpr := GT(ident, literal)
	b.and(gtExpr)

	return b
}

// GE creates a greater-than-or-equal comparison expression and adds it using AND logic.
// Supports both identifier and index expressions as the left operand.
// Returns the builder for method chaining with error propagation.
func (b *ExprBuilder) GE(l, r any) *ExprBuilder {
	if b.err != nil {
		return b
	}

	literal, err := newLiteral(r)
	if err != nil {
		b.err = err
		return b
	}

	indexExpr, ok := l.(*ast.IndexExpr)
	if ok {
		geExpr := GE(indexExpr, literal)
		b.and(geExpr)
		return b
	}

	ident, err := newIdent(l)
	if err != nil {
		b.err = err
		return b
	}

	geExpr := GE(ident, literal)
	b.and(geExpr)

	return b
}

// In creates a membership test expression and adds it using AND logic.
// The right operand should be a slice or list that gets converted to a list literal.
// Supports both identifier and index expressions as the left operand.
// Returns the builder for method chaining with error propagation.
func (b *ExprBuilder) In(l, r any) *ExprBuilder {
	if b.err != nil {
		return b
	}

	listLiteral, err := newListLiteral(r)
	if err != nil {
		b.err = err
		return b
	}

	indexExpr, ok := l.(*ast.IndexExpr)
	if ok {
		inExpr := In(indexExpr, listLiteral)
		b.and(inExpr)
		return b
	}

	ident, err := newIdent(l)
	if err != nil {
		b.err = err
		return b
	}

	inExpr := In(ident, listLiteral)
	b.and(inExpr)

	return b
}

// Like creates a pattern matching expression and adds it using AND logic.
// Typically used for string pattern matching with wildcards (e.g., "John%").
// Supports both identifier and index expressions as the left operand.
// Returns the builder for method chaining with error propagation.
func (b *ExprBuilder) Like(l, r any) *ExprBuilder {
	if b.err != nil {
		return b
	}

	literal, err := newLiteral(r)
	if err != nil {
		b.err = err
		return b
	}

	indexExpr, ok := l.(*ast.IndexExpr)
	if ok {
		likeExpr := Like(indexExpr, literal)
		b.and(likeExpr)
		return b
	}

	ident, err := newIdent(l)
	if err != nil {
		b.err = err
		return b
	}

	likeExpr := Like(ident, literal)
	b.and(likeExpr)

	return b
}

// And creates a nested AND expression using a sub-builder function.
// The sub-expression is combined with the current expression using AND logic.
// Enables complex nested structures like (a AND b) AND (c OR d).
func (b *ExprBuilder) And(fn func(*ExprBuilder)) *ExprBuilder {
	if b.err != nil {
		return b
	}

	subExpr := NewExprBuilder()
	fn(subExpr)

	if subExpr.err != nil {
		b.err = subExpr.err
		return b
	}

	b.and(subExpr.expr)
	return b
}

// Or creates a nested OR expression using a sub-builder function.
// The sub-expression is combined with the current expression using OR logic.
// Enables complex nested structures like (a AND b) OR (c AND d).
func (b *ExprBuilder) Or(fn func(*ExprBuilder)) *ExprBuilder {
	if b.err != nil {
		return b
	}

	subExpr := NewExprBuilder()
	fn(subExpr)

	if subExpr.err != nil {
		b.err = subExpr.err
		return b
	}

	b.or(subExpr.expr)
	return b
}

// Not creates a negation expression using a sub-builder function.
// The sub-expression is negated and combined with the current expression using AND logic.
// Enables expressions like expr AND NOT(sub-expr).
func (b *ExprBuilder) Not(fn func(*ExprBuilder)) *ExprBuilder {
	if b.err != nil {
		return b
	}

	subExpr := NewExprBuilder()
	fn(subExpr)

	if subExpr.err != nil {
		b.err = subExpr.err
		return b
	}

	notExpr := Not(subExpr.expr)
	b.and(notExpr)
	return b
}

// Builder provides a clean public interface wrapping ExprBuilder functionality.
// It delegates all operations to an internal ExprBuilder and provides the Build()
// method to retrieve the constructed expression or any accumulated errors.
type Builder struct {
	exprBuilder *ExprBuilder
}

// EQ creates an equality comparison and returns the Builder for method chaining.
func (b *Builder) EQ(l, r any) *Builder {
	b.exprBuilder.EQ(l, r)
	return b
}

// NE creates an inequality comparison and returns the Builder for method chaining.
func (b *Builder) NE(l, r any) *Builder {
	b.exprBuilder.NE(l, r)
	return b
}

// LT creates a less-than comparison and returns the Builder for method chaining.
func (b *Builder) LT(l, r any) *Builder {
	b.exprBuilder.LT(l, r)
	return b
}

// LE creates a less-than-or-equal comparison and returns the Builder for method chaining.
func (b *Builder) LE(l, r any) *Builder {
	b.exprBuilder.LE(l, r)
	return b
}

// GT creates a greater-than comparison and returns the Builder for method chaining.
func (b *Builder) GT(l, r any) *Builder {
	b.exprBuilder.GT(l, r)
	return b
}

// GE creates a greater-than-or-equal comparison and returns the Builder for method chaining.
func (b *Builder) GE(l, r any) *Builder {
	b.exprBuilder.GE(l, r)
	return b
}

// In creates a membership test expression and returns the Builder for method chaining.
func (b *Builder) In(l, r any) *Builder {
	b.exprBuilder.In(l, r)
	return b
}

// Like creates a pattern matching expression and returns the Builder for method chaining.
func (b *Builder) Like(l, r any) *Builder {
	b.exprBuilder.Like(l, r)
	return b
}

// And creates a nested AND expression and returns the Builder for method chaining.
func (b *Builder) And(fn func(*ExprBuilder)) *Builder {
	b.exprBuilder.And(fn)
	return b
}

// Or creates a nested OR expression and returns the Builder for method chaining.
func (b *Builder) Or(fn func(*ExprBuilder)) *Builder {
	b.exprBuilder.Or(fn)
	return b
}

// Not creates a negation expression and returns the Builder for method chaining.
func (b *Builder) Not(fn func(*ExprBuilder)) *Builder {
	b.exprBuilder.Not(fn)
	return b
}

// Build finalizes the expression construction and returns the built AST expression.
// Returns any error that occurred during the building process, enabling deferred
// error handling after a chain of operations.
func (b *Builder) Build() (ast.Expr, error) {
	if b.exprBuilder.err != nil {
		return nil, b.exprBuilder.err
	}
	return b.exprBuilder.expr, nil
}

// NewBuilder creates a new Builder instance with a fresh ExprBuilder.
// This is the entry point for constructing filter expressions using the fluent API.
func NewBuilder() *Builder {
	return &Builder{
		exprBuilder: NewExprBuilder(),
	}
}
