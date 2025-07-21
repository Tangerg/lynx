package ast

import (
	"github.com/Tangerg/lynx/ai/vectorstore/filter/token"
)

// BrackExpr represents a bracket expression node in the AST.
// It represents array/map access operations like arr[0], obj['key'], or nested access like arr[0][1].
// Bracket expressions are computed expressions that involve indexing or key-based access operations.
type BrackExpr struct {
	LBrack  token.Token // The left bracket token '['
	RBrack  token.Token // The right bracket token ']'
	Left    Expr        // The expression being accessed (identifier or another bracket expression)
	Literal *Literal    // The index or key literal used for access
}

func (b *BrackExpr) expr()         {}
func (b *BrackExpr) computedExpr() {}

func (b *BrackExpr) Start() token.Position {
	return b.Left.Start()
}

func (b *BrackExpr) End() token.Position {
	return b.RBrack.End
}

// Brack creates a new bracket expression from a left expression and an index/key literal.
// It supports creating bracket expressions from either:
//   - *Ident: for simple variable access like "arr[0]" or "obj['key']"
//   - *BrackExpr: for nested access like "arr[0][1]" or "obj['key']['subkey']"
//
// The function creates synthetic bracket tokens with no position information.
// Parameters:
//   - left: the expression being accessed (must be either *Ident or *BrackExpr)
//   - literal: the index or key literal used for the access operation
//
// Returns:
//   - a pointer to a new BrackExpr representing the bracket access operation
func Brack[T *Ident | *BrackExpr](left T, literal *Literal) *BrackExpr {
	brackExpr := &BrackExpr{
		LBrack:  newKindToken(token.LBRACK),
		RBrack:  newKindToken(token.RBRACK),
		Literal: literal,
	}

	switch typedL := any(left).(type) {
	case *Ident:
		brackExpr.Left = typedL
	case *BrackExpr:
		brackExpr.Left = typedL
	}

	return brackExpr
}
