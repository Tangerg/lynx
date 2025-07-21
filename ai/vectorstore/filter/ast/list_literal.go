package ast

import (
	"github.com/Tangerg/lynx/ai/vectorstore/filter/token"
)

// ListLiteral represents a list literal node in the AST.
// It holds a collection of literal values enclosed in parentheses, such as (1, 2, 3) or ('a', 'b', 'c') or (true, false).
// List literals are atomic expressions that represent arrays or collections of constant values.
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

// listLiteralType defines the constraint for types that can be used to create list literals.
// Supported types include:
//   - Slices of all numeric types: integers (signed/unsigned), floating-point numbers
//   - Slice of strings: for text value collections
//   - Slice of booleans: for true/false value collections
//   - Slice of *Literal: for pre-existing literal collections
//   - *ListLiteral: for existing list literal nodes
type listLiteralType interface {
	[]int | []int8 | []int16 | []int32 | []int64 |
		[]uint | []uint8 | []uint16 | []uint32 | []uint64 |
		[]float32 | []float64 |
		[]string |
		[]bool |
		[]*Literal |
		*ListLiteral
}

// isListLiteralType performs runtime type checking to determine if a value can create a list literal.
// This function validates that the given value matches one of the supported list literal types.
// Parameters:
//   - v: the value to check
//
// Returns:
//   - true if the value can be used to create a list literal, false otherwise
func isListLiteralType(v any) bool {
	switch v.(type) {
	case []int, []int8, []int16, []int32, []int64,
		[]uint, []uint8, []uint16, []uint32, []uint64,
		[]float32, []float64:
		return true
	case []string:
		return true
	case []bool:
		return true
	case []*Literal:
		return true
	case *ListLiteral:
		return true
	default:
		return false
	}
}

// NewListLiteral creates a new list literal from the given value using Go generics.
// It automatically handles different slice types by converting their elements to *Literal nodes:
//   - Numeric slices are converted element by element using NewLiterals
//   - String slices become collections of STRING literals
//   - Boolean slices become collections of TRUE/FALSE literals
//   - []*Literal slices are used directly without conversion
//   - Existing *ListLiteral pointers are returned as-is (identity function)
//
// The created list literal uses synthetic parenthesis tokens with no position information.
// Parameters:
//   - value: the slice or existing list literal to create from (must satisfy listLiteralType constraint)
//
// Returns:
//   - a pointer to a new ListLiteral struct with appropriate literal values
func NewListLiteral[T listLiteralType](value T) *ListLiteral {
	listLiteral, ok := any(value).(*ListLiteral)
	if ok {
		return listLiteral
	}

	listLiteral = &ListLiteral{
		Lparen: newKindToken(token.LPAREN),
		Rparen: newKindToken(token.RPAREN),
	}

	switch typedValue := any(value).(type) {
	case []*Literal:
		listLiteral.Values = typedValue
	case []int:
		listLiteral.Values = NewLiterals(typedValue)
	case []int8:
		listLiteral.Values = NewLiterals(typedValue)
	case []int16:
		listLiteral.Values = NewLiterals(typedValue)
	case []int32:
		listLiteral.Values = NewLiterals(typedValue)
	case []int64:
		listLiteral.Values = NewLiterals(typedValue)
	case []uint:
		listLiteral.Values = NewLiterals(typedValue)
	case []uint8:
		listLiteral.Values = NewLiterals(typedValue)
	case []uint16:
		listLiteral.Values = NewLiterals(typedValue)
	case []uint32:
		listLiteral.Values = NewLiterals(typedValue)
	case []uint64:
		listLiteral.Values = NewLiterals(typedValue)
	case []float32:
		listLiteral.Values = NewLiterals(typedValue)
	case []float64:
		listLiteral.Values = NewLiterals(typedValue)
	case []string:
		listLiteral.Values = NewLiterals(typedValue)
	case []bool:
		listLiteral.Values = NewLiterals(typedValue)
	}

	return listLiteral
}
