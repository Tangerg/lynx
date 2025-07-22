// Package ast provides abstract syntax tree definitions for filter expressions.
package ast

import (
	"github.com/Tangerg/lynx/ai/vectorstore/filter/token"
)

// UnaryExpr represents a unary expression node in the AST.
// It consists of a unary operator applied to a single operand, such as:
// logical negation (Not condition), or other prefix operators.
// Unary expressions are computed expressions that apply an operation to one sub-expression.
type UnaryExpr struct {
	Op    token.Token  // The unary operator token (Not, etc.)
	Right ComputedExpr // The operand expression that the operator is applied to
}

func (u *UnaryExpr) expr()         {}
func (u *UnaryExpr) computedExpr() {}

func (u *UnaryExpr) Start() token.Position {
	return u.Op.Start
}

func (u *UnaryExpr) End() token.Position {
	return u.Right.End()
}

// Precedence returns the precedence level of this unary expression's operator.
// Higher values indicate higher precedence (evaluated first).
func (u *UnaryExpr) Precedence() int {
	return u.Op.Kind.Precedence()
}

// IsRightLower checks if the right operand has lower precedence than this expression.
// This is used to determine if the right operand needs parentheses when formatting.
// Returns true if the right operand is a binary or unary expression with lower precedence.
func (u *UnaryExpr) IsRightLower() bool {
	rightBinaryExpr, ok := u.Right.(*BinaryExpr)
	if ok {
		return rightBinaryExpr.Precedence() < u.Precedence()
	}

	rightUnaryExpr, ok := u.Right.(*UnaryExpr)
	if ok {
		return rightUnaryExpr.Precedence() < u.Precedence()
	}

	return false
}

// Not creates a logical negation unary expression.
// It applies the Not operator to negate the given computed expression.
// Supports logical negation like: Not (age > 18), Not (status == 'active')
// Note: This function only constructs the AST node and does not perform validation.
// Type checking and semantic validation occur in later processing stages.
// Parameters:
//   - r: the computed expression to be negated
//
// Returns:
//   - a pointer to a new UnaryExpr representing the logical negation
func Not[T ComputedExpr](r T) *UnaryExpr {
	return &UnaryExpr{
		Op:    newKindToken(token.NOT),
		Right: r,
	}
}
