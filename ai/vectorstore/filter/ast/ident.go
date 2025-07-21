package ast

import (
	"github.com/Tangerg/lynx/ai/vectorstore/filter/token"
)

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

// identType defines the constraint for types that can be used to create identifiers.
// It allows either a string value or an existing *Ident pointer.
type identType interface {
	string |
		*Ident
}

// isIdentType checks whether the given value can be used to create an identifier.
// It performs runtime type checking to determine if the value is either a string
// or an *Ident pointer.
// Parameters:
//   - v: the value to check
//
// Returns:
//   - true if the value is a string or *Ident, false otherwise
func isIdentType(v any) bool {
	switch v.(type) {
	case string:
		return true
	case *Ident:
		return true
	default:
		return false
	}
}

// NewIdent creates a new identifier from the given value using Go generics.
// It supports creating identifiers from either string values or existing *Ident pointers.
// When given a string, it creates a new Ident with a token but no position information.
// When given an *Ident, it returns the same pointer (identity function).
// Parameters:
//   - value: either a string to create a new identifier from, or an existing *Ident
//
// Returns:
//   - a pointer to an Ident struct, or nil if the type constraint is violated
//     (though nil should never be returned due to the generic constraint)
func NewIdent[T identType](value T) *Ident {
	switch typedValue := any(value).(type) {
	case string:
		return &Ident{
			Token: token.OfIdent(typedValue, token.NoPosition, token.NoPosition),
			Value: typedValue,
		}
	case *Ident:
		return typedValue
	default:
		return nil // This case should never occur due to generic constraints, included for compilation
	}
}
