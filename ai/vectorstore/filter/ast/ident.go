package ast

import (
	"fmt"
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

// identType constrains the types that can be used to construct identifiers.
// Accepts either a string value for creating new identifiers or an existing
// *Ident pointer for passthrough operations.
type identType interface {
	string |
		*Ident
}

// newIdent is an internal constructor that creates an Ident from various input types.
// It handles type assertion and validation, returning appropriate errors for
// unsupported types.
func newIdent(value any) (*Ident, error) {
	switch typedValue := value.(type) {
	case string:
		return &Ident{
			Token: token.OfIdent(typedValue, token.NoPosition, token.NoPosition),
			Value: typedValue,
		}, nil
	case *Ident:
		return typedValue, nil
	default:
		return nil, fmt.Errorf("type mismatch: expected string or *Ident, got %T with value '%v'", value, value)
	}
}

// NewIdent creates a new identifier using Go generics with compile-time type safety.
// This function provides a type-safe interface for identifier creation, supporting
// both string-to-Ident conversion and Ident passthrough operations.
//
// For string inputs: Creates a new Ident with the provided value and generates
// a corresponding token (position information will be set to NoPosition).
//
// For *Ident inputs: Returns the same pointer unchanged (identity operation).
//
// Parameters:
//   - value: Either a string to create a new identifier from, or an existing *Ident pointer
//
// Returns:
//   - *Ident: A pointer to the identifier struct
//
// Note: This function will panic if called with invalid types, though the generic
// constraint should prevent such cases at compile time.
func NewIdent[T identType](value T) *Ident {
	ident, err := newIdent(value)
	if err != nil {
		// This should never occur due to generic type constraints
		panic(fmt.Sprintf("NewIdent: unexpected error: %v", err))
	}
	return ident
}
