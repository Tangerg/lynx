package ast

import (
	"fmt"
	"strconv"

	"github.com/spf13/cast"

	"github.com/Tangerg/lynx/ai/vectorstore/filter/token"
)

// Literal represents a literal value node in the AST.
// It holds both the token information (including position and kind) and the string
// representation of the literal value. Literals are atomic expressions that represent
// constant values like strings, numbers, or boolean values.
type Literal struct {
	Token token.Token // The underlying token containing position and kind information
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

// IsString checks if this literal represents a string value
func (l *Literal) IsString() bool {
	return l.Token.Kind.Is(token.STRING)
}

// AsString returns the string value of this literal.
// Returns an error if the literal is not a string type.
func (l *Literal) AsString() (string, error) {
	if !l.IsString() {
		return "", fmt.Errorf("type mismatch: expected STRING literal, got %s with value '%s'",
			l.Token.Kind.Name(), l.Value)
	}
	return l.Value, nil
}

// IsNumber checks if this literal represents a numeric value
func (l *Literal) IsNumber() bool {
	return l.Token.Kind.Is(token.NUMBER)
}

// AsNumber parses and returns the numeric value of this literal as a float64.
// Returns an error if the literal is not a number type or cannot be parsed.
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

// IsBool checks if this literal represents a boolean value (true or false)
func (l *Literal) IsBool() bool {
	return l.Token.Kind.Is(token.TRUE) || l.Token.Kind.Is(token.FALSE)
}

// AsBool returns the boolean value of this literal.
// Returns an error if the literal is not a boolean type.
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

// numericType defines the constraint for all supported numeric types.
// This includes all standard Go integer, unsigned integer, and floating-point types.
type numericType interface {
	int | int8 | int16 | int32 | int64 |
		uint | uint8 | uint16 | uint32 | uint64 |
		float32 | float64
}

// isNumericType performs runtime type checking to determine if a value is numeric.
// This function complements the compile-time numericType constraint for runtime validation.
// Parameters:
//   - v: the value to check
//
// Returns:
//   - true if the value is any numeric type, false otherwise
func isNumericType(v any) bool {
	switch v.(type) {
	case int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64:
		return true
	default:
		return false
	}
}

// literalType defines the constraint for types that can be used to create literals.
// Supported types include:
//   - Numeric types: integers (signed/unsigned), floating-point numbers
//   - String type: for text literals
//   - Bool type: for true/false values
//   - *Literal: for existing literal nodes
type literalType interface {
	numericType |
		string |
		bool |
		*Literal
}

// isLiteralType performs runtime type checking to determine if a value can create a literal.
// This function validates that the given value matches one of the supported literal types.
// Parameters:
//   - v: the value to check
//
// Returns:
//   - true if the value can be used to create a literal, false otherwise
func isLiteralType(v any) bool {
	if isNumericType(v) {
		return true
	}
	switch v.(type) {
	case string:
		return true
	case bool:
		return true
	case *Literal:
		return true
	default:
		return false
	}
}

// NewLiteral creates a new literal from the given value using Go generics.
// It automatically determines the appropriate token kind based on the value type:
//   - Numeric types are converted to NUMBER tokens
//   - String values become STRING tokens
//   - Boolean values become TRUE or FALSE tokens
//   - Existing *Literal pointers are returned as-is (identity function)
//
// Parameters:
//   - value: the value to create a literal from (must satisfy literalType constraint)
//
// Returns:
//   - a pointer to a new Literal struct, or the existing *Literal if passed
func NewLiteral[T literalType](value T) *Literal {
	if isNumericType(value) {
		number := cast.ToString(value)
		return &Literal{
			Token: token.OfLiteral(token.NUMBER, number, token.NoPosition, token.NoPosition),
			Value: number,
		}
	}

	switch typedValue := any(value).(type) {
	case string:
		return &Literal{
			Token: token.OfLiteral(token.STRING, typedValue, token.NoPosition, token.NoPosition),
			Value: typedValue,
		}
	case bool:
		var kind = token.FALSE
		if typedValue {
			kind = token.TRUE
		}
		return &Literal{
			Token: newKindToken(kind),
			Value: kind.Literal(),
		}
	case *Literal:
		return typedValue
	default:
		return nil // This case should never occur due to generic constraints, included for compilation
	}
}

// NewLiterals creates a slice of literals from a slice of values.
// This is a convenience function that applies NewLiteral to each element in the input slice.
// Parameters:
//   - values: a slice of values that satisfy the literalType constraint
//
// Returns:
//   - a slice of *Literal pointers corresponding to the input values
func NewLiterals[T literalType](values []T) []*Literal {
	var literals []*Literal
	for _, value := range values {
		literals = append(literals, NewLiteral(value))
	}
	return literals
}

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
