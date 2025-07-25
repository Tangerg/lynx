// Package ast provides abstract syntax tree definitions for filter expressions.
// It defines the core interfaces and utilities for representing and manipulating
// filter expression trees in the vector store filtering system.
package ast

import (
	"fmt"
	"strconv"

	"github.com/Tangerg/lynx/ai/vectorstore/filter/token"
)

// Expr represents the base interface for all expression nodes in the AST.
// All expression types must implement this interface to be part of the syntax tree.
type Expr interface {
	// Start returns the starting position of this expression in the source
	Start() token.Position
	// End returns the ending position of this expression in the source
	End() token.Position
	expr()
}

// AtomicExpr represents atomic (leaf) expressions in the AST.
// These are the basic building blocks that cannot be further decomposed,
// such as literals, identifiers, or simple values.
type AtomicExpr interface {
	Expr
	atomicExpr()
}

// ComputedExpr represents computed (composite) expressions in the AST.
// These are expressions that involve operations or computations,
// such as unary operations, binary operations, or complex expressions.
type ComputedExpr interface {
	Expr
	computedExpr()
}

// precedenceAble represents expressions that have operator precedence.
type precedenceAble interface {
	ComputedExpr
	// Precedence returns the precedence level of this expression's operator.
	// Higher values indicate higher precedence (evaluated first).
	Precedence() int
}

// Ident represents an identifier node in the AST.
// It holds both the token information (including position) and the string value
// of the identifier. Identifiers are atomic expressions that typically represent
// variable names, field names, or other symbolic references.
type Ident struct {
	Token token.Token // The underlying token containing position and literal information
	Value string      // The string value of the identifier
}

func (i *Ident) expr()       {}
func (i *Ident) atomicExpr() {}

func (i *Ident) Start() token.Position {
	return i.Token.Start
}

func (i *Ident) End() token.Position {
	return i.Token.End
}

// Literal represents a literal value node in the AST.
// It holds both the token information (including position and type) and the string
// representation of the literal value. Literals are atomic expressions that represent
// constant values such as strings, numbers, or boolean values.
type Literal struct {
	Token token.Token // The underlying token containing position and type information
	Value string      // The string representation of the literal value
}

func (l *Literal) expr() {}

func (l *Literal) atomicExpr() {}

func (l *Literal) Start() token.Position {
	return l.Token.Start
}

func (l *Literal) End() token.Position {
	return l.Token.End
}

// IsString checks whether this literal represents a string value
func (l *Literal) IsString() bool {
	return l.Token.Kind.Is(token.STRING)
}

// AsString returns the string value of this literal.
// Returns an error if the literal is not of string type.
func (l *Literal) AsString() (string, error) {
	if !l.IsString() {
		return "", fmt.Errorf("type mismatch: expected STRING literal, got %s with value '%s'",
			l.Token.Kind.Name(), l.Value)
	}
	return l.Value, nil
}

// IsNumber checks whether this literal represents a numeric value
func (l *Literal) IsNumber() bool {
	return l.Token.Kind.Is(token.NUMBER)
}

// AsNumber parses and returns the numeric value of this literal as a float64.
// Returns an error if the literal is not of number type or cannot be parsed.
func (l *Literal) AsNumber() (float64, error) {
	if !l.IsNumber() {
		return 0, fmt.Errorf("type mismatch: expected NUMBER literal, got %s with value '%s'",
			l.Token.Kind.Name(), l.Value)
	}
	num, err := strconv.ParseFloat(l.Value, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse number literal '%s': %w", l.Value, err)
	}
	return num, nil
}

// IsBool checks whether this literal represents a boolean value (true or false)
func (l *Literal) IsBool() bool {
	return l.Token.Kind.Is(token.TRUE) || l.Token.Kind.Is(token.FALSE)
}

// AsBool returns the boolean value of this literal.
// Returns an error if the literal is not of boolean type.
func (l *Literal) AsBool() (bool, error) {
	switch {
	case l.Token.Kind.Is(token.TRUE):
		return true, nil
	case l.Token.Kind.Is(token.FALSE):
		return false, nil
	default:
		return false, fmt.Errorf("type mismatch: expected boolean literal (TRUE or FALSE), got %s with value '%s'",
			l.Token.Kind.Name(), l.Value)
	}
}

// IsSameKind checks if two literals have the same kind.
// Returns true if both literals are of the same basic type (bool, string, or number),
// false otherwise.
func (l *Literal) IsSameKind(other *Literal) bool {
	if other == nil {
		return false
	}

	return (l.IsBool() && other.IsBool()) ||
		(l.IsString() && other.IsString()) ||
		(l.IsNumber() && other.IsNumber())
}

// ListLiteral represents a list literal node in the Abstract Syntax Tree (AST).
// It encapsulates a collection of literal values enclosed in parentheses, such as
// (1, 2, 3), ('a', 'b', 'c'), or (true, false). List literals serve as atomic
// expressions that represent arrays or collections of constant values.
type ListLiteral struct {
	Lparen token.Token // The left parenthesis token '('
	Rparen token.Token // The right parenthesis token ')'
	Values []*Literal  // The literal values contained within the list
}

func (l *ListLiteral) expr()       {}
func (l *ListLiteral) atomicExpr() {}

func (l *ListLiteral) Start() token.Position {
	return l.Lparen.Start
}

func (l *ListLiteral) End() token.Position {
	return l.Rparen.End
}

// UnaryExpr represents a unary expression node in the AST.
// It consists of a unary operator applied to a single operand, such as
// logical negation (NOT condition) or other prefix operators.
// Unary expressions are computed expressions that apply an operation to one sub-expression.
type UnaryExpr struct {
	Op    token.Token  // The unary operator token (NOT, etc.)
	Right ComputedExpr // The operand expression to which the operator is applied
}

func (u *UnaryExpr) expr()         {}
func (u *UnaryExpr) computedExpr() {}

func (u *UnaryExpr) Start() token.Position {
	return u.Op.Start
}

func (u *UnaryExpr) End() token.Position {
	return u.Right.End()
}

func (u *UnaryExpr) Precedence() int {
	return u.Op.Kind.Precedence()
}

// IsRightLower checks whether the right operand has lower precedence than this expression.
// This is used to determine whether the right operand should be evaluated first
// despite having lower precedence in the expression hierarchy.
// Returns true if the right operand is an expression with lower precedence.
func (u *UnaryExpr) IsRightLower() bool {
	rightExpr, ok := u.Right.(precedenceAble)
	if !ok {
		return false
	}
	return rightExpr.Precedence() < u.Precedence()
}

// BinaryExpr represents a binary expression node in the AST.
// It consists of a left operand, an operator, and a right operand, such as
// comparison operations (id == 1, age > 18), logical operations (condition1 AND condition2),
// membership tests (status IN ['active', 'pending']), or pattern matching (name LIKE 'John%').
// Binary expressions are computed expressions that combine two sub-expressions with an operator.
type BinaryExpr struct {
	Left  Expr        // The left operand expression
	Op    token.Token // The binary operator token (==, !=, <, <=, >, >=, AND, OR, IN, LIKE, etc.)
	Right Expr        // The right operand expression
}

func (b *BinaryExpr) expr()         {}
func (b *BinaryExpr) computedExpr() {}

func (b *BinaryExpr) Start() token.Position {
	return b.Left.Start()
}

func (b *BinaryExpr) End() token.Position {
	return b.Right.End()
}

func (b *BinaryExpr) Precedence() int {
	return b.Op.Kind.Precedence()
}

// IsLeftLower checks whether the left operand has lower precedence than this expression.
// This is used to determine whether the left operand should be evaluated first
// despite having lower precedence in the expression hierarchy.
// Returns true if the left operand is an expression with lower precedence.
func (b *BinaryExpr) IsLeftLower() bool {
	leftExpr, ok := b.Left.(precedenceAble)
	if !ok {
		return false
	}
	return leftExpr.Precedence() < b.Precedence()
}

// IsRightLower checks whether the right operand has lower precedence than this expression.
// This is used to determine whether the right operand should be evaluated first
// despite having lower precedence in the expression hierarchy.
// Returns true if the right operand is an expression with lower precedence.
func (b *BinaryExpr) IsRightLower() bool {
	rightExpr, ok := b.Right.(precedenceAble)
	if !ok {
		return false
	}
	return rightExpr.Precedence() < b.Precedence()
}

// IndexExpr represents an index expression node in the AST.
// It represents array/map access operations such as arr[0], obj['key'], or nested access like arr[0][1].
// Index expressions are computed expressions that involve indexing or key-based access operations.
type IndexExpr struct {
	LBrack token.Token // The left bracket token '['
	RBrack token.Token // The right bracket token ']'
	Left   Expr        // The left expression being indexed (identifier or another index expression)
	Index  *Literal    // The index or key literal used for access
}

func (i *IndexExpr) expr()         {}
func (i *IndexExpr) computedExpr() {}

func (i *IndexExpr) Start() token.Position {
	return i.Left.Start()
}

func (i *IndexExpr) End() token.Position {
	return i.RBrack.End
}
