package filter

import (
	"fmt"

	"github.com/spf13/cast"

	"github.com/Tangerg/lynx/ai/vectorstore/filter/ast"
	"github.com/Tangerg/lynx/ai/vectorstore/filter/token"
)

// newKindToken creates a Token with the specified kind and no position information.
func newKindToken(kind token.Kind) token.Token {
	return token.OfKind(kind, token.NoPosition, token.NoPosition)
}

// identType defines acceptable types for identifier construction.
// Supports string values for creating new identifiers or existing *ast.Ident for passthrough.
type identType interface {
	string | *ast.Ident
}

// newIdent creates an identifier AST node from various input types with validation.
// String inputs create new identifiers, *ast.Ident inputs pass through unchanged.
func newIdent(value any) (*ast.Ident, error) {
	switch typedValue := value.(type) {
	case string:
		return &ast.Ident{
			Token: token.OfIdent(typedValue, token.NoPosition, token.NoPosition),
			Value: typedValue,
		}, nil
	case *ast.Ident:
		return typedValue, nil
	default:
		return nil, fmt.Errorf("expected string or *ast.Ident, got %T: '%v'", value, value)
	}
}

// NewIdent creates a type-safe identifier with compile-time type validation.
// String inputs create new identifiers with NoPosition, *ast.Ident inputs pass through unchanged.
func NewIdent[T identType](value T) *ast.Ident {
	ident, err := newIdent(value)
	if err != nil {
		// Should never occur due to generic constraints
		panic(fmt.Sprintf("NewIdent: %v", err))
	}
	return ident
}

// numericType encompasses all Go numeric types for literal creation.
type numericType interface {
	int | int8 | int16 | int32 | int64 |
		uint | uint8 | uint16 | uint32 | uint64 |
		float32 | float64
}

// literalType defines acceptable types for literal construction.
// Includes all numeric types, strings, booleans, and existing *ast.Literal nodes.
type literalType interface {
	numericType | string | bool | *ast.Literal
}

// newLiteral creates a literal AST node from various input types with automatic token kind detection.
// Numeric types become NUMBER tokens, strings become STRING tokens, booleans become TRUE/FALSE tokens.
func newLiteral(value any) (*ast.Literal, error) {
	switch typedValue := value.(type) {
	case int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64:
		number := cast.ToString(value)
		return &ast.Literal{
			Token: token.OfLiteral(token.NUMBER, number, token.NoPosition, token.NoPosition),
			Value: number,
		}, nil

	case string:
		return &ast.Literal{
			Token: token.OfLiteral(token.STRING, typedValue, token.NoPosition, token.NoPosition),
			Value: typedValue,
		}, nil

	case bool:
		kind := token.FALSE
		if typedValue {
			kind = token.TRUE
		}
		return &ast.Literal{
			Token: newKindToken(kind),
			Value: kind.Literal(),
		}, nil

	case *ast.Literal:
		return typedValue, nil

	default:
		return nil, fmt.Errorf("expected literalType, got %T: '%v'", value, value)
	}
}

// NewLiteral creates a type-safe literal with automatic token kind detection.
// Converts numeric types to NUMBER tokens, strings to STRING tokens, booleans to TRUE/FALSE tokens.
// Existing *ast.Literal nodes pass through unchanged.
func NewLiteral[T literalType](value T) *ast.Literal {
	literal, err := newLiteral(value)
	if err != nil {
		// Should never occur due to generic constraints
		panic(fmt.Sprintf("NewLiteral: %v", err))
	}
	return literal
}

// NewLiterals converts a slice of values to corresponding literal AST nodes.
// Each element is processed through NewLiteral for consistent token creation.
func NewLiterals[T literalType](values []T) []*ast.Literal {
	literals := make([]*ast.Literal, 0, len(values))
	for _, value := range values {
		literals = append(literals, NewLiteral(value))
	}
	return literals
}

// listLiteralType defines acceptable types for list literal construction.
// Supports slices of all basic types and existing *ast.ListLiteral nodes.
type listLiteralType interface {
	[]int | []int8 | []int16 | []int32 | []int64 |
		[]uint | []uint8 | []uint16 | []uint32 | []uint64 |
		[]float32 | []float64 | []string | []bool |
		[]*ast.Literal | *ast.ListLiteral
}

// newListLiteral creates a list literal AST node from various slice types or existing nodes.
// Slice elements are automatically converted to *ast.Literal nodes with appropriate tokens.
func newListLiteral(value any) (*ast.ListLiteral, error) {
	if listLiteral, ok := value.(*ast.ListLiteral); ok {
		return listLiteral, nil
	}

	result := &ast.ListLiteral{
		Lparen: newKindToken(token.LPAREN),
		Rparen: newKindToken(token.RPAREN),
	}

	switch typedValue := value.(type) {
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
	case []*ast.Literal:
		result.Values = typedValue
	default:
		return nil, fmt.Errorf("expected listLiteralType, got %T: '%v'", value, value)
	}

	return result, nil
}

// NewListLiteral creates a type-safe list literal with automatic element conversion.
// Slice elements are converted to *ast.Literal nodes, existing *ast.ListLiteral passes through unchanged.
// Uses synthetic parenthesis tokens with NoPosition for proper AST structure.
func NewListLiteral[T listLiteralType](value T) *ast.ListLiteral {
	listLiteral, err := newListLiteral(value)
	if err != nil {
		// Should never occur due to generic constraints
		panic(fmt.Sprintf("NewListLiteral: %v", err))
	}
	return listLiteral
}

// compare builds a binary comparison expression between identifiers/index expressions and literals.
// Handles both identifier and index expression types on the left side with automatic type detection.
func compare[L identType | *ast.IndexExpr, R literalType](l L, r R, op token.Kind) *ast.BinaryExpr {
	expr := &ast.BinaryExpr{
		Op:    newKindToken(op),
		Right: NewLiteral(r),
	}
	switch typeL := any(l).(type) {
	case *ast.IndexExpr:
		expr.Left = typeL
	default:
		ident, _ := newIdent(l)
		expr.Left = ident
	}
	return expr
}

// Comparison operators create binary expressions for various comparison types.
// All operators support both identifier and index expression types on the left side.

// EQ creates equality comparison expressions.
// Examples: id == 1, name == "john", active == true, arr[0] == "value"
func EQ[L identType | *ast.IndexExpr, R literalType](l L, r R) *ast.BinaryExpr {
	return compare(l, r, token.EQ)
}

// NE creates inequality comparison expressions.
// Examples: id != 1, name != "john", active != false, arr[0] != "value"
func NE[L identType | *ast.IndexExpr, R literalType](l L, r R) *ast.BinaryExpr {
	return compare(l, r, token.NE)
}

// LT creates less-than comparison expressions for numeric values.
// Examples: age < 18, price < 100.50, scores[0] < 90
func LT[L identType | *ast.IndexExpr, R numericType | *ast.Literal](l L, r R) *ast.BinaryExpr {
	return compare(l, r, token.LT)
}

// LE creates less-than-or-equal comparison expressions for numeric values.
// Examples: age <= 18, price <= 100.50, scores[0] <= 90
func LE[L identType | *ast.IndexExpr, R numericType | *ast.Literal](l L, r R) *ast.BinaryExpr {
	return compare(l, r, token.LE)
}

// GT creates greater-than comparison expressions for numeric values.
// Examples: age > 18, price > 100.50, scores[0] > 90
func GT[L identType | *ast.IndexExpr, R numericType | *ast.Literal](l L, r R) *ast.BinaryExpr {
	return compare(l, r, token.GT)
}

// GE creates greater-than-or-equal comparison expressions for numeric values.
// Examples: age >= 18, price >= 100.50, scores[0] >= 90
func GE[L identType | *ast.IndexExpr, R numericType | *ast.Literal](l L, r R) *ast.BinaryExpr {
	return compare(l, r, token.GE)
}

// logic builds binary logical expressions between computed expressions.
// Used internally by And and Or operators to combine filter conditions.
func logic[L ast.ComputedExpr, R ast.ComputedExpr](l L, r R, op token.Kind) *ast.BinaryExpr {
	return &ast.BinaryExpr{
		Left:  l,
		Op:    newKindToken(op),
		Right: r,
	}
}

// And creates logical conjunction expressions.
// Examples: (age > 18) AND (status == "active"), (score >= 80) AND (active == true)
func And[L ast.ComputedExpr, R ast.ComputedExpr](l L, r R) *ast.BinaryExpr {
	return logic(l, r, token.AND)
}

// Or creates logical disjunction expressions.
// Examples: (status == "active") OR (status == "pending"), (role == "admin") OR (role == "owner")
func Or[L ast.ComputedExpr, R ast.ComputedExpr](l L, r R) *ast.BinaryExpr {
	return logic(l, r, token.OR)
}

// In creates membership test expressions for checking if a value exists in a list.
// Supports both identifier and index expression types on the left side.
// Examples: status IN ["active", "pending"], id IN [1, 2, 3], tags[0] IN ["important", "urgent"]
func In[L identType | *ast.IndexExpr, R listLiteralType](l L, r R) *ast.BinaryExpr {
	expr := &ast.BinaryExpr{
		Op:    newKindToken(token.IN),
		Right: NewListLiteral(r),
	}
	switch typeL := any(l).(type) {
	case *ast.IndexExpr:
		expr.Left = typeL
	default:
		ident, _ := newIdent(l)
		expr.Left = ident
	}
	return expr
}

// Like creates pattern matching expressions for string comparison with wildcards.
// Supports both identifier and index expression types on the left side.
// Examples: name LIKE "John%", email LIKE "%@gmail.com", addresses[0] LIKE "%Street"
func Like[L identType | *ast.IndexExpr, R string | *ast.Literal](l L, r R) *ast.BinaryExpr {
	expr := &ast.BinaryExpr{
		Op:    newKindToken(token.LIKE),
		Right: NewLiteral(r),
	}
	switch typeL := any(l).(type) {
	case *ast.IndexExpr:
		expr.Left = typeL
	default:
		ident, _ := newIdent(l)
		expr.Left = ident
	}
	return expr
}

// Not creates logical negation expressions for inverting filter conditions.
// Examples: NOT (age > 18), NOT (status == "active"), NOT (score IN [80, 90, 100])
func Not[T ast.ComputedExpr](r T) *ast.UnaryExpr {
	return &ast.UnaryExpr{
		Op:    newKindToken(token.NOT),
		Right: r,
	}
}

// Index creates array/map access expressions for accessing nested data structures.
// Supports chaining for multi-level access and both identifier and existing IndexExpr as left operand.
// Examples: arr[0], obj["key"], matrix[1][2], users[0]["name"]
func Index[L identType | *ast.IndexExpr, I numericType | string | *ast.Literal](left L, index I) *ast.IndexExpr {
	indexExpr := &ast.IndexExpr{
		LBrack: newKindToken(token.LBRACK),
		RBrack: newKindToken(token.RBRACK),
		Index:  NewLiteral(index),
	}

	switch typedL := any(left).(type) {
	case string:
		indexExpr.Left = NewIdent(typedL)
	case *ast.Ident:
		indexExpr.Left = typedL
	case *ast.IndexExpr:
		indexExpr.Left = typedL
	}

	return indexExpr
}
