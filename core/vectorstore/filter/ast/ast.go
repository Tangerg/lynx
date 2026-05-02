// Package ast holds the abstract-syntax-tree types for the
// vector-store filter mini-language. The parser builds an AST out of
// these types; analyzers and visitors traverse it via the [Visitor]
// interface.
package ast

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/Tangerg/lynx/core/vectorstore/filter/token"
)

// Expr is the root interface every AST node implements. The unexported
// expr() method seals the interface — only types in this package can
// satisfy it, so traversal switches stay exhaustive.
type Expr interface {
	// Start returns the source position of the first byte of the node.
	Start() token.Position

	// End returns the source position one past the last byte of the
	// node.
	End() token.Position

	expr()
}

// AtomicExpr is the marker interface for leaf nodes that cannot be
// decomposed further: identifiers, literals, list literals.
type AtomicExpr interface {
	Expr
	atomicExpr()
}

// ComputedExpr is the marker interface for composite nodes that
// combine sub-expressions: unary, binary, and index expressions.
type ComputedExpr interface {
	Expr
	computedExpr()
}

// precedenceAble is the unexported interface for nodes whose operator
// has an integer precedence. Used by [UnaryExpr.IsRightLower] /
// [BinaryExpr.IsLeftLower] to decide whether parentheses are needed
// when re-rendering an AST.
type precedenceAble interface {
	ComputedExpr

	// Precedence returns the operator's precedence level. Higher
	// values bind tighter.
	Precedence() int
}

// Ident is a name reference — a column / metadata field the filter
// language compares against literals.
type Ident struct {
	// Token is the underlying source token (position + literal text).
	Token token.Token

	// Value is the identifier text.
	Value string
}

func (i *Ident) expr()       {}
func (i *Ident) atomicExpr() {}

func (i *Ident) Start() token.Position { return i.Token.Start }
func (i *Ident) End() token.Position   { return i.Token.End }

// Literal is a constant value: a number, a string, or a boolean.
type Literal struct {
	// Token is the underlying source token (position + kind).
	Token token.Token

	// Value is the lexeme as written in the source.
	Value string
}

func (l *Literal) expr()       {}
func (l *Literal) atomicExpr() {}

func (l *Literal) Start() token.Position { return l.Token.Start }
func (l *Literal) End() token.Position   { return l.Token.End }

// IsString reports whether this literal is a string token.
func (l *Literal) IsString() bool { return l.Token.Kind.Is(token.STRING) }

// AsString returns the string value. Errors when the literal is not a
// string.
func (l *Literal) AsString() (string, error) {
	if !l.IsString() {
		return "", fmt.Errorf("ast.Literal.AsString: expected STRING, got %s with value %q",
			l.Token.Kind.Name(), l.Value)
	}
	return l.Value, nil
}

// IsNumber reports whether this literal is a number token.
func (l *Literal) IsNumber() bool { return l.Token.Kind.Is(token.NUMBER) }

// AsNumber parses the literal as float64. Errors when the literal is
// not a number or fails to parse.
func (l *Literal) AsNumber() (float64, error) {
	if !l.IsNumber() {
		return 0, fmt.Errorf("ast.Literal.AsNumber: expected NUMBER, got %s with value %q",
			l.Token.Kind.Name(), l.Value)
	}

	num, err := strconv.ParseFloat(l.Value, 64)
	if err != nil {
		return 0, fmt.Errorf("ast.Literal.AsNumber: parse %q: %w", l.Value, err)
	}
	return num, nil
}

// IsBool reports whether this literal is a TRUE or FALSE token.
func (l *Literal) IsBool() bool {
	return l.Token.Kind.Is(token.TRUE) || l.Token.Kind.Is(token.FALSE)
}

// AsBool returns the boolean value. Errors when the literal is not a
// boolean or when the lexeme contradicts the token kind (a defensive
// check — the parser should never produce this).
func (l *Literal) AsBool() (bool, error) {
	parsed, err := strconv.ParseBool(l.Value)
	if err != nil {
		return false, fmt.Errorf("ast.Literal.AsBool: parse %q: %w", l.Value, err)
	}

	switch {
	case l.Token.Kind.Is(token.TRUE):
		if !parsed {
			return true, errors.New("ast.Literal.AsBool: token kind TRUE but value parses to false")
		}
		return true, nil
	case l.Token.Kind.Is(token.FALSE):
		if parsed {
			return false, errors.New("ast.Literal.AsBool: token kind FALSE but value parses to true")
		}
		return false, nil
	default:
		return false, fmt.Errorf("ast.Literal.AsBool: expected TRUE or FALSE, got %s with value %q",
			l.Token.Kind.Name(), l.Value)
	}
}

// IsSameKind reports whether two literals share a basic type
// (string / number / boolean). nil receivers / arguments yield false.
func (l *Literal) IsSameKind(other *Literal) bool {
	if other == nil {
		return false
	}

	return (l.IsBool() && other.IsBool()) ||
		(l.IsString() && other.IsString()) ||
		(l.IsNumber() && other.IsNumber())
}

// ListLiteral is a parenthesized list of literals: (1, 2, 3),
// ('a', 'b'), (true, false). Used by IN expressions.
type ListLiteral struct {
	// Lparen is the '(' token.
	Lparen token.Token

	// Rparen is the ')' token.
	Rparen token.Token

	// Values are the list members in source order.
	Values []*Literal
}

func (l *ListLiteral) expr()       {}
func (l *ListLiteral) atomicExpr() {}

func (l *ListLiteral) Start() token.Position { return l.Lparen.Start }
func (l *ListLiteral) End() token.Position   { return l.Rparen.End }

// UnaryExpr is one prefix operator applied to a sub-expression — the
// only unary operator the filter language supports today is logical
// NOT.
type UnaryExpr struct {
	// Op is the operator token (NOT).
	Op token.Token

	// Right is the operand.
	Right ComputedExpr
}

func (u *UnaryExpr) expr()         {}
func (u *UnaryExpr) computedExpr() {}

func (u *UnaryExpr) Start() token.Position { return u.Op.Start }
func (u *UnaryExpr) End() token.Position   { return u.Right.End() }

// Precedence returns the unary operator's precedence.
func (u *UnaryExpr) Precedence() int { return u.Op.Kind.Precedence() }

// IsRightLower reports whether the operand binds looser than this
// expression — informs parenthesization when re-rendering.
func (u *UnaryExpr) IsRightLower() bool {
	right, ok := u.Right.(precedenceAble)
	if !ok {
		return false
	}
	return right.Precedence() < u.Precedence()
}

// BinaryExpr is one operator with two operands — comparisons (==, !=,
// <, <=, >, >=), logical ops (AND, OR), membership (IN), pattern match
// (LIKE). Binary expressions cover most of the filter language.
type BinaryExpr struct {
	// Left is the left operand.
	Left Expr

	// Op is the operator token.
	Op token.Token

	// Right is the right operand.
	Right Expr
}

func (b *BinaryExpr) expr()         {}
func (b *BinaryExpr) computedExpr() {}

func (b *BinaryExpr) Start() token.Position { return b.Left.Start() }
func (b *BinaryExpr) End() token.Position   { return b.Right.End() }

// Precedence returns the binary operator's precedence.
func (b *BinaryExpr) Precedence() int { return b.Op.Kind.Precedence() }

// IsLeftLower reports whether the left operand binds looser than this
// expression — informs parenthesization when re-rendering.
func (b *BinaryExpr) IsLeftLower() bool {
	left, ok := b.Left.(precedenceAble)
	if !ok {
		return false
	}
	return left.Precedence() < b.Precedence()
}

// IsRightLower reports whether the right operand binds looser than
// this expression — informs parenthesization when re-rendering.
func (b *BinaryExpr) IsRightLower() bool {
	right, ok := b.Right.(precedenceAble)
	if !ok {
		return false
	}
	return right.Precedence() < b.Precedence()
}

// IndexExpr is an array / map access — arr[0], obj['key'], or nested
// forms like arr[0][1].
type IndexExpr struct {
	// LBrack is the '[' token.
	LBrack token.Token

	// RBrack is the ']' token.
	RBrack token.Token

	// Left is the value being indexed (an identifier or a nested
	// index expression).
	Left Expr

	// Index is the key/index literal.
	Index *Literal
}

func (i *IndexExpr) expr()         {}
func (i *IndexExpr) computedExpr() {}

func (i *IndexExpr) Start() token.Position { return i.Left.Start() }
func (i *IndexExpr) End() token.Position   { return i.RBrack.End }
