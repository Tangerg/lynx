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

// literalType constrains the types that can be used to construct literals.
// Supported types include:
//   - Numeric types: integers (signed/unsigned) and floating-point numbers
//   - String type: for text literals
//   - Bool type: for true/false values
//   - *Literal: for existing literal nodes (passthrough)
type literalType interface {
	numericType |
		string |
		bool |
		*Literal
}

// newLiteral is an internal constructor that creates a Literal from various input types.
// It handles type assertion, validation, and appropriate token generation based on
// the input value type.
func newLiteral(value any) (*Literal, error) {
	switch typedValue := value.(type) {
	case int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64:
		number := cast.ToString(value)
		return &Literal{
			Token: token.OfLiteral(token.NUMBER, number, token.NoPosition, token.NoPosition),
			Value: number,
		}, nil

	case string:
		return &Literal{
			Token: token.OfLiteral(token.STRING, typedValue, token.NoPosition, token.NoPosition),
			Value: typedValue,
		}, nil

	case bool:
		var kind = token.FALSE
		if typedValue {
			kind = token.TRUE
		}
		return &Literal{
			Token: newKindToken(kind),
			Value: kind.Literal(),
		}, nil

	case *Literal:
		return typedValue, nil

	default:
		return nil, fmt.Errorf("type mismatch: expected literalType, got %T with value '%v'", value, value)
	}
}

// NewLiteral creates a new literal using Go generics with compile-time type safety.
// It automatically determines the appropriate token kind based on the input type:
//   - Numeric types are converted to NUMBER tokens
//   - String values become STRING tokens
//   - Boolean values become TRUE or FALSE tokens
//   - Existing *Literal pointers are returned unchanged (identity operation)
//
// All created literals use NoPosition for token positioning.
//
// Parameters:
//   - value: the value to create a literal from (must satisfy literalType constraint)
//
// Returns:
//   - *Literal: A pointer to the literal struct
func NewLiteral[T literalType](value T) *Literal {
	newliteral, err := newLiteral(value)
	if err != nil {
		// This should never occur due to generic type constraints
		panic(fmt.Sprintf("NewLiteral: unexpected error: %v", err))
	}
	return newliteral
}

// NewLiterals creates a slice of literals from a slice of values.
// This is a convenience function that applies NewLiteral to each element in the input slice.
//
// Parameters:
//   - values: a slice of values that satisfy the literalType constraint
//
// Returns:
//   - []*Literal: A slice of literal pointers corresponding to the input values
func NewLiterals[T literalType](values []T) []*Literal {
	literals := make([]*Literal, 0, len(values))
	for _, value := range values {
		literals = append(literals, NewLiteral(value))
	}
	return literals
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

// listLiteralType constrains the types that can be used to construct list literals.
// Supported types include:
//   - Slices of all numeric types: integers (signed/unsigned) and floating-point numbers
//   - Slice of strings: for text value collections
//   - Slice of booleans: for true/false value collections
//   - Slice of *Literal: for pre-existing literal collections
//   - *ListLiteral: for existing list literal nodes (passthrough)
type listLiteralType interface {
	[]int | []int8 | []int16 | []int32 | []int64 |
		[]uint | []uint8 | []uint16 | []uint32 | []uint64 |
		[]float32 | []float64 |
		[]string |
		[]bool |
		[]*Literal |
		*ListLiteral
}

// newListLiteral is an internal constructor that creates a ListLiteral from various input types.
// It handles type assertion, validation, and conversion of slice elements to *Literal nodes.
// The function generates synthetic parenthesis tokens with NoPosition information.
func newListLiteral(value any) (*ListLiteral, error) {
	if listLiteral, ok := value.(*ListLiteral); ok {
		return listLiteral, nil
	}

	result := &ListLiteral{
		Lparen: newKindToken(token.LPAREN),
		Rparen: newKindToken(token.RPAREN),
	}

	switch typedValue := value.(type) {
	case []*Literal:
		result.Values = typedValue
	case []int:
		result.Values = NewLiterals(typedValue)
	case []int8:
		result.Values = NewLiterals(typedValue)
	case []int16:
		result.Values = NewLiterals(typedValue)
	case []int32:
		result.Values = NewLiterals(typedValue)
	case []int64:
		result.Values = NewLiterals(typedValue)
	case []uint:
		result.Values = NewLiterals(typedValue)
	case []uint8:
		result.Values = NewLiterals(typedValue)
	case []uint16:
		result.Values = NewLiterals(typedValue)
	case []uint32:
		result.Values = NewLiterals(typedValue)
	case []uint64:
		result.Values = NewLiterals(typedValue)
	case []float32:
		result.Values = NewLiterals(typedValue)
	case []float64:
		result.Values = NewLiterals(typedValue)
	case []string:
		result.Values = NewLiterals(typedValue)
	case []bool:
		result.Values = NewLiterals(typedValue)
	default:
		return nil, fmt.Errorf("type mismatch: expected listLiteralType, got %T with value '%v'", value, value)
	}

	return result, nil
}

// NewListLiteral creates a new list literal using Go generics with compile-time type safety.
// It automatically handles different slice types by converting their elements to *Literal nodes:
//   - Numeric slices: elements are converted using NewLiterals
//   - String slices: become collections of STRING literals
//   - Boolean slices: become collections of TRUE/FALSE literals
//   - []*Literal slices: are used directly without conversion
//   - Existing *ListLiteral pointers: are returned unchanged (identity operation)
//
// The created list literal uses synthetic parenthesis tokens with NoPosition information.
//
// Parameters:
//   - value: the slice or existing list literal (must satisfy listLiteralType constraint)
//
// Returns:
//   - *ListLiteral: A pointer to the list literal struct with appropriate values
func NewListLiteral[T listLiteralType](value T) *ListLiteral {
	listLiteral, err := newListLiteral(value)
	if err != nil {
		// This should never occur due to generic type constraints
		panic(fmt.Sprintf("NewListLiteral: unexpected error: %v", err))
	}
	return listLiteral
}
