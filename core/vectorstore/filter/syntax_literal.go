package filter

import (
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
)

// Number is any built-in numeric type or a user-defined type with the same
// underlying representation.
type Number interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 |
		~float32 | ~float64
}

// LiteralValue is an input accepted by [NewLiteral] and comparison
// constructors.
type LiteralValue interface {
	Number | string | bool | *Literal
}

func newLiteral(value any) (*Literal, error) {
	if value != nil {
		reflected := reflect.ValueOf(value)
		switch reflected.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return &Literal{Kind: LiteralNumber, Value: strconv.FormatInt(reflected.Int(), 10)}, nil
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			return &Literal{Kind: LiteralNumber, Value: strconv.FormatUint(reflected.Uint(), 10)}, nil
		case reflect.Float32, reflect.Float64:
			number := reflected.Float()
			if number == 0 {
				return &Literal{Kind: LiteralNumber, Value: "0"}, nil
			}
			return &Literal{
				Kind:  LiteralNumber,
				Value: strconv.FormatFloat(number, 'g', -1, reflected.Type().Bits()),
			}, nil
		}
	}

	switch typed := value.(type) {
	case string:
		return &Literal{Kind: LiteralString, Value: typed}, nil

	case bool:
		return &Literal{Kind: LiteralBool, Value: strconv.FormatBool(typed)}, nil

	case *Literal:
		return typed, nil

	default:
		return nil, fmt.Errorf("filter.newLiteral: unsupported literal type %T (%v)",
			value, value)
	}
}

func NewLiteral[T LiteralValue](value T) *Literal {
	lit, err := newLiteral(value)
	if err != nil {
		// Unreachable while the generic constraint is honored.
		panic(fmt.Errorf("filter.NewLiteral: %w", err))
	}
	return lit
}

func NewLiterals[T LiteralValue](values []T) []*Literal {
	literals := make([]*Literal, 0, len(values))
	for _, v := range values {
		literals = append(literals, NewLiteral(v))
	}
	return literals
}

func canonicalNumber(literal string) (string, error) {
	if !strings.ContainsAny(literal, ".eE") {
		if strings.HasPrefix(literal, "-") {
			number, err := strconv.ParseInt(literal, 10, 64)
			if err != nil {
				return "", fmt.Errorf("invalid integer %q", literal)
			}
			return strconv.FormatInt(number, 10), nil
		}
		number, err := strconv.ParseUint(literal, 10, 64)
		if err != nil {
			return "", fmt.Errorf("invalid integer %q", literal)
		}
		return strconv.FormatUint(number, 10), nil
	}

	number, err := strconv.ParseFloat(literal, 64)
	if err != nil || math.IsNaN(number) || math.IsInf(number, 0) {
		return "", fmt.Errorf("invalid number %q", literal)
	}
	if number == 0 {
		return "0", nil
	}
	return strconv.FormatFloat(number, 'g', -1, 64), nil
}

// ListValue is a homogeneous scalar slice, a pre-built literal slice, or an
// existing list node.
type ListValue interface {
	[]int | []int8 | []int16 | []int32 | []int64 |
		[]uint | []uint8 | []uint16 | []uint32 | []uint64 |
		[]float32 | []float64 | []string | []bool |
		[]*Literal | *ListLiteral
}

func newListLiteral(value any) (*ListLiteral, error) {
	if list, ok := value.(*ListLiteral); ok {
		return list, nil
	}

	result := &ListLiteral{}

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
	case []*Literal:
		result.Values = typed
	default:
		return nil, fmt.Errorf("filter.newListLiteral: unsupported list type %T (%v)",
			value, value)
	}

	return result, nil
}

// NewListLiteral builds a [*ListLiteral] from a slice of Go values or a
// pre-built node.
func NewListLiteral[T ListValue](value T) *ListLiteral {
	list, err := newListLiteral(value)
	if err != nil {
		// Unreachable while the generic constraint is honored.
		panic(fmt.Errorf("filter.NewListLiteral: %w", err))
	}
	return list
}
