// Package ast provides abstract syntax tree definitions for filter expressions.
// It defines the core interfaces and utilities for representing and manipulating
// filter expression trees in the vector store filtering system.
package ast

import (
	"github.com/Tangerg/lynx/ai/vectorstore/filter/token"
)

// Expr represents the base interface for all expression nodes in the AST.
// All expression types must implement this interface to be part of the syntax tree.
type Expr interface {
	// expr is a marker method to ensure only intended types implement this interface
	expr()
	// Start returns the starting position of this expression in the source
	Start() token.Position
	// End returns the ending position of this expression in the source
	End() token.Position
}

// AtomicExpr represents atomic (leaf) expressions in the AST.
// These are the basic building blocks that cannot be further decomposed,
// such as literals, identifiers, or simple values.
type AtomicExpr interface {
	Expr
	// atomicExpr is a marker method to distinguish atomic expressions
	atomicExpr()
}

// ComputedExpr represents computed (composite) expressions in the AST.
// These are expressions that involve operations or computations,
// such as unary operations, binary operations, or complex expressions.
type ComputedExpr interface {
	Expr
	// computedExpr is a marker method to distinguish computed expressions
	computedExpr()
}

// newKindToken creates a new token with the specified kind but no position information.
// This is a utility function for creating tokens when position tracking is not needed.
// Parameters:
//   - kind: the token kind to create
//
// Returns:
//   - a new token with the given kind and no position information
func newKindToken(kind token.Kind) token.Token {
	return token.OfKind(kind, token.NoPosition, token.NoPosition)
}
