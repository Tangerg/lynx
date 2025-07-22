// Package ast provides abstract syntax tree definitions for filter expressions.
package ast

import (
	"github.com/Tangerg/lynx/ai/vectorstore/filter/token"
)

// ParenExpr represents a parenthesized expression node in the AST.
// It wraps another computed expression in parentheses to control logical operator precedence
// or improve readability, such as condition1 And (condition2 Or condition3).
// Parenthesized expressions are computed expressions that contain other computed expressions.
type ParenExpr struct {
	Lparen token.Token  // The left parenthesis token '('
	Rparen token.Token  // The right parenthesis token ')'
	Inner  ComputedExpr // The computed expression enclosed within the parentheses
}

func (p *ParenExpr) expr()         {}
func (p *ParenExpr) computedExpr() {}

func (p *ParenExpr) Start() token.Position {
	return p.Lparen.Start
}

func (p *ParenExpr) End() token.Position {
	return p.Rparen.End
}

// Paren creates a new parenthesized expression wrapping the given computed expression.
// This function is used to explicitly group logical expressions and control operator precedence
// in filter conditions, such as grouping And operations within Or operations.
// The function creates synthetic parenthesis tokens with no position information.
// Parameters:
//   - inner: the computed expression to be enclosed in parentheses
//
// Returns:
//   - a pointer to a new ParenExpr that wraps the inner expression
func Paren[T ComputedExpr](inner T) *ParenExpr {
	return &ParenExpr{
		Lparen: newKindToken(token.LPAREN),
		Rparen: newKindToken(token.RPAREN),
		Inner:  inner,
	}
}
