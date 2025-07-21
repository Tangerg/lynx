// Package ast provides abstract syntax tree definitions for filter expressions.
package ast

import (
	"github.com/Tangerg/lynx/ai/vectorstore/filter/token"
)

// BinaryExpr represents a binary expression node in the AST.
// It consists of a left operand, an operator, and a right operand, such as:
// comparison operations (id == 1, age > 18), logical operations (condition1 AND condition2),
// membership tests (status IN ['active', 'pending']), or pattern matching (name LIKE 'John%').
// Binary expressions are computed expressions that combine two sub-expressions with an operator.
type BinaryExpr struct {
	Left  Expr        // The left operand expression
	Op    token.Token // The binary operator token (==, !=, <, >, AND, OR, IN, LIKE, etc.)
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

// Precedence returns the precedence level of this binary expression's operator.
// Higher values indicate higher precedence (evaluated first).
func (b *BinaryExpr) Precedence() int {
	return b.Op.Kind.Precedence()
}

// IsLeftLower checks if the left operand has lower precedence than this expression.
// This is used to determine if the left operand needs parentheses when formatting.
// Returns true if the left operand is a binary or unary expression with lower precedence.
func (b *BinaryExpr) IsLeftLower() bool {
	leftBinaryExpr, ok := b.Left.(*BinaryExpr)
	if ok {
		return leftBinaryExpr.Precedence() < b.Precedence()
	}

	leftUnaryExpr, ok := b.Left.(*UnaryExpr)
	if ok {
		return leftUnaryExpr.Precedence() < b.Precedence()
	}

	return false
}

// IsRightLower checks if the right operand has lower precedence than this expression.
// This is used to determine if the right operand needs parentheses when formatting.
// Returns true if the right operand is a binary or unary expression with lower precedence.
func (b *BinaryExpr) IsRightLower() bool {
	rightBinaryExpr, ok := b.Right.(*BinaryExpr)
	if ok {
		return rightBinaryExpr.Precedence() < b.Precedence()
	}

	rightUnaryExpr, ok := b.Right.(*UnaryExpr)
	if ok {
		return rightUnaryExpr.Precedence() < b.Precedence()
	}

	return false
}

// Compare creates a binary comparison expression between an identifier and a literal value.
// This is a generic helper function for building comparison operations.
// Note: This function only constructs the AST node and does not perform validation.
// Type checking and semantic validation occur in later processing stages.
// Parameters:
//   - l: the left operand (identifier)
//   - r: the right operand (literal value)
//   - op: the comparison operator kind
//
// Returns:
//   - a pointer to a new BinaryExpr representing the comparison
func Compare[L identType, R literalType](l L, r R, op token.Kind) *BinaryExpr {
	return &BinaryExpr{
		Left:  NewIdent(l),
		Op:    newKindToken(op),
		Right: NewLiteral(r),
	}
}

// EQ creates an equality comparison expression.
// Supports comparisons like: id == 1, age == 18, name == 'tom', email == 'tom@gmail.com', active == true
// Note: This function only constructs the AST node and does not perform type validation.
func EQ[L identType, R literalType](l L, r R) *BinaryExpr {
	return Compare(l, r, token.EQ)
}

// NE creates a not-equal comparison expression.
// Supports comparisons like: id != 1, age != 18, name != 'tom', email != 'tom@gmail.com', active != false
// Note: This function only constructs the AST node and does not perform type validation.
func NE[L identType, R literalType](l L, r R) *BinaryExpr {
	return Compare(l, r, token.NE)
}

// LT creates a less-than comparison expression.
// Supports numeric comparisons like: age < 18, price < 100.50
// Note: This function only constructs the AST node and does not perform type validation.
func LT[L identType, R numericType | *Literal](l L, r R) *BinaryExpr {
	return Compare(l, r, token.LT)
}

// LE creates a less-than-or-equal comparison expression.
// Supports numeric comparisons like: age <= 18, price <= 100.50
// Note: This function only constructs the AST node and does not perform type validation.
func LE[L identType, R numericType | *Literal](l L, r R) *BinaryExpr {
	return Compare(l, r, token.LE)
}

// GT creates a greater-than comparison expression.
// Supports numeric comparisons like: age > 18, price > 100.50
// Note: This function only constructs the AST node and does not perform type validation.
func GT[L identType, R numericType | *Literal](l L, r R) *BinaryExpr {
	return Compare(l, r, token.GT)
}

// GE creates a greater-than-or-equal comparison expression.
// Supports numeric comparisons like: age >= 18, price >= 100.50
// Note: This function only constructs the AST node and does not perform type validation.
func GE[L identType, R numericType | *Literal](l L, r R) *BinaryExpr {
	return Compare(l, r, token.GE)
}

// Logic creates a logical binary expression between two computed expressions.
// This is a generic helper function for building logical operations like AND, OR.
// Note: This function only constructs the AST node and does not perform validation.
// Parameters:
//   - l: the left computed expression
//   - r: the right computed expression
//   - op: the logical operator kind
//
// Returns:
//   - a pointer to a new BinaryExpr representing the logical operation
func Logic[L ComputedExpr, R ComputedExpr](l L, r R, op token.Kind) *BinaryExpr {
	return &BinaryExpr{
		Left:  l,
		Op:    newKindToken(op),
		Right: r,
	}
}

// AND creates a logical AND expression between two computed expressions.
// Supports logical combinations like: (age > 18) AND (status == 'active')
// Note: This function only constructs the AST node and does not perform validation.
func AND[L ComputedExpr, R ComputedExpr](l L, r R) *BinaryExpr {
	return Logic(l, r, token.AND)
}

// OR creates a logical OR expression between two computed expressions.
// Supports logical combinations like: (status == 'active') OR (status == 'pending')
// Note: This function only constructs the AST node and does not perform validation.
func OR[L ComputedExpr, R ComputedExpr](l L, r R) *BinaryExpr {
	return Logic(l, r, token.OR)
}

// IN creates a membership test expression to check if an identifier's value exists in a list.
// Supports membership tests like: status IN ['active', 'pending', 'inactive'], id IN [1, 2, 3]
// Note: This function only constructs the AST node and does not perform validation.
func IN[L identType, R listLiteralType](l L, r R) *BinaryExpr {
	return &BinaryExpr{
		Left:  NewIdent(l),
		Op:    newKindToken(token.IN),
		Right: NewListLiteral(r),
	}
}

// LIKE creates a pattern matching expression for string comparison.
// Supports pattern matching like: name LIKE 'John%', email LIKE '%@gmail.com'
// Note: This function only constructs the AST node and does not perform validation.
func LIKE[L identType, R string | *Literal](l L, r R) *BinaryExpr {
	return &BinaryExpr{
		Left:  NewIdent(l),
		Op:    newKindToken(token.LIKE),
		Right: NewLiteral(r),
	}
}
