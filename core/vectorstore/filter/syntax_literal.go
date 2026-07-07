package filter

import (
	"fmt"

	"github.com/Tangerg/lynx/core/vectorstore/filter/ast"
	"github.com/Tangerg/lynx/core/vectorstore/filter/token"
	"github.com/Tangerg/lynx/pkg/math"
)

// literalType is the input constraint for [NewLiteral]: any numeric
// type, plus string, bool, or an existing [*ast.Literal].
type literalType interface {
	math.NumericType | string | bool | *ast.Literal
}

func newLiteral(value any) (*ast.Literal, error) {
	switch typed := value.(type) {
	case int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64:
		// %v prints decimal form for ints and 'g'-format for floats —
		// OfNumericLiteral re-parses to float64 and re-formats anyway,
		// so any decimal/scientific representation works here.
		tk := token.OfNumericLiteral(fmt.Sprintf("%v", value), token.NoPosition, token.NoPosition)
		return &ast.Literal{Token: tk, Value: tk.Literal}, nil

	case string:
		return &ast.Literal{
			Token: token.OfLiteral(token.STRING, typed, token.NoPosition, token.NoPosition),
			Value: typed,
		}, nil

	case bool:
		kind := token.FALSE
		if typed {
			kind = token.TRUE
		}
		return &ast.Literal{
			Token: newKindToken(kind),
			Value: kind.Literal(),
		}, nil

	case *ast.Literal:
		return typed, nil

	default:
		return nil, fmt.Errorf("filter.newLiteral: unsupported literal type %T (%v)",
			value, value)
	}
}

func NewLiteral[T literalType](value T) *ast.Literal {
	lit, err := newLiteral(value)
	if err != nil {
		// Unreachable while the generic constraint is honored.
		panic(fmt.Errorf("filter.NewLiteral: %w", err))
	}
	return lit
}

func NewLiterals[T literalType](values []T) []*ast.Literal {
	literals := make([]*ast.Literal, 0, len(values))
	for _, v := range values {
		literals = append(literals, NewLiteral(v))
	}
	return literals
}

// listLiteralType is the input constraint for [NewListLiteral]: any
// slice of a basic type, a pre-built slice of [*ast.Literal], or an
// existing [*ast.ListLiteral].
type listLiteralType interface {
	[]int | []int8 | []int16 | []int32 | []int64 |
		[]uint | []uint8 | []uint16 | []uint32 | []uint64 |
		[]float32 | []float64 | []string | []bool |
		[]*ast.Literal | *ast.ListLiteral
}

func newListLiteral(value any) (*ast.ListLiteral, error) {
	if list, ok := value.(*ast.ListLiteral); ok {
		return list, nil
	}

	result := &ast.ListLiteral{
		Lparen: newKindToken(token.LPAREN),
		Rparen: newKindToken(token.RPAREN),
	}

	switch typed := value.(type) {
	case []int:
		result.Values = NewLiterals(typed)
	case []int8:
		result.Values = NewLiterals(typed)
	case []int16:
		result.Values = NewLiterals(typed)
	case []int32:
		result.Values = NewLiterals(typed)
	case []int64:
		result.Values = NewLiterals(typed)
	case []uint:
		result.Values = NewLiterals(typed)
	case []uint8:
		result.Values = NewLiterals(typed)
	case []uint16:
		result.Values = NewLiterals(typed)
	case []uint32:
		result.Values = NewLiterals(typed)
	case []uint64:
		result.Values = NewLiterals(typed)
	case []float32:
		result.Values = NewLiterals(typed)
	case []float64:
		result.Values = NewLiterals(typed)
	case []string:
		result.Values = NewLiterals(typed)
	case []bool:
		result.Values = NewLiterals(typed)
	case []*ast.Literal:
		result.Values = typed
	default:
		return nil, fmt.Errorf("filter.newListLiteral: unsupported list type %T (%v)",
			value, value)
	}

	return result, nil
}

// NewListLiteral builds an [*ast.ListLiteral] from a slice of Go values
// or a pre-built node. Synthetic '(' / ')' tokens are attached so the
// node round-trips through [visitors.SQLLikeVisitor].
func NewListLiteral[T listLiteralType](value T) *ast.ListLiteral {
	list, err := newListLiteral(value)
	if err != nil {
		// Unreachable while the generic constraint is honored.
		panic(fmt.Errorf("filter.NewListLiteral: %w", err))
	}
	return list
}
