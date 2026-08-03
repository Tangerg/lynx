package filter

import (
	"errors"
	"fmt"
)

// BinaryHandlers names the operator-specific branches of a filter compiler.
// A nil handler means that the backend does not support that operator.
type BinaryHandlers struct {
	Logical    func(*BinaryExpr) error
	Comparison func(*BinaryExpr) error
	In         func(*BinaryExpr) error
	Has        func(*BinaryExpr) error
	Like       func(*BinaryExpr) error
}

// DispatchBinary routes an expression to the handler for its operator family.
// Null tests are handled separately because backend support for them is not
// universal.
func DispatchBinary(expr *BinaryExpr, handlers BinaryHandlers) error {
	var handler func(*BinaryExpr) error
	switch {
	case expr.Op.IsLogicalOperator():
		handler = handlers.Logical
	case expr.Op.IsComparisonOperator():
		handler = handlers.Comparison
	case expr.Op.Is(OpIn):
		handler = handlers.In
	case expr.Op.Is(OpHas):
		handler = handlers.Has
	case expr.Op.Is(OpLike):
		handler = handlers.Like
	default:
		return fmt.Errorf("filter: unsupported binary operator %q at %s",
			expr.Op.String(), expr.Start().String())
	}
	if handler == nil {
		return fmt.Errorf("filter: binary operator %s is not supported at %s",
			expr.Op.Name(), expr.Start().String())
	}
	return handler(expr)
}

// DispatchUnary routes the only unary operator, NOT, to onNot.
func DispatchUnary(
	expr *UnaryExpr,
	onNot func(*UnaryExpr) error,
) error {
	if !expr.Op.Is(OpNot) {
		return fmt.Errorf("filter: unsupported unary operator %q at %s",
			expr.Op.String(), expr.Start().String())
	}
	return onNot(expr)
}

// LogicalOpString returns "AND" / "OR" for the matching token kind.
// Errors for any non-logical kind. Used by SQL / text-output backends
// that emit the operator verbatim into their query language.
func LogicalOpString(k Operator) (string, error) {
	switch k {
	case OpAnd:
		return "AND", nil
	case OpOr:
		return "OR", nil
	default:
		return "", fmt.Errorf("filter: expected logical operator, got %s", k.Name())
	}
}

// RequireListLiteral asserts the right operand of expr is a non-empty
// [*ListLiteral] — the contract every backend's IN handler needs.
// Centralizes the two error messages every vendor used to emit
// verbatim.
func RequireListLiteral(expr *BinaryExpr) (*ListLiteral, error) {
	list, ok := expr.Right.(*ListLiteral)
	if !ok {
		return nil, fmt.Errorf("filter: 'IN' requires a list on the right at %s, got %T",
			expr.Start().String(), expr.Right)
	}
	if len(list.Values) == 0 {
		return nil, fmt.Errorf("filter: 'IN' requires a non-empty list at %s",
			expr.Start().String())
	}
	return list, nil
}

// RequireStringPatternOnRight asserts the right side of expr resolves
// to a string literal and returns its value. Used by LIKE handlers.
// Wraps the [ExtractValue] + string-type-assert step every vendor's
// LIKE branch repeats.
func RequireStringPatternOnRight(expr *BinaryExpr) (string, error) {
	value, err := ExtractValue(expr.Right)
	if err != nil {
		return "", err
	}
	s, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("filter: 'LIKE' requires a string pattern, got %T at %s",
			value, expr.Start().String())
	}
	return s, nil
}

// ConvertListLiteral turns list into a typed Go slice keyed by the
// first element's kind:
//
//   - string literals → []string
//   - integer literals → []int64 or []uint64
//   - decimal literals → []float64
//   - boolean literals → []bool
//
// The element-type slice is returned as any (so the caller can hand it
// to a driver that auto-detects), plus a sample of the first element
// for branching on element type without re-inspecting the slice.
//
// Returns an error if the literals don't all share a kind or the kind
// is unsupported.
func ConvertListLiteral(list *ListLiteral) (slice any, sample any, err error) {
	if list == nil || len(list.Values) == 0 {
		return nil, nil, errors.New("filter: empty list literal")
	}
	first := list.Values[0]
	if first == nil {
		return nil, nil, errors.New("filter: list element 0 is nil")
	}
	switch {
	case first.IsString():
		out := make([]string, 0, len(list.Values))
		for _, lit := range list.Values {
			s, err := lit.AsString()
			if err != nil {
				return nil, nil, err
			}
			out = append(out, s)
		}
		return out, out[0], nil
	case first.IsNumber():
		return convertNumberList(list.Values)
	case first.IsBool():
		out := make([]bool, 0, len(list.Values))
		for _, lit := range list.Values {
			b, err := lit.AsBool()
			if err != nil {
				return nil, nil, err
			}
			out = append(out, b)
		}
		return out, out[0], nil
	default:
		return nil, nil, fmt.Errorf("filter: unsupported list element kind %s",
			first.Kind)
	}
}

func convertNumberList(literals []*Literal) (slice any, sample any, err error) {
	values := make([]any, 0, len(literals))
	hasFloat := false
	hasUint := false
	for _, literal := range literals {
		value, err := numberValue(literal)
		if err != nil {
			return nil, nil, err
		}
		values = append(values, value)
		switch value.(type) {
		case float64:
			hasFloat = true
		case uint64:
			hasUint = true
		}
	}

	if hasFloat {
		const maxExactFloatInteger = int64(1 << 53)
		out := make([]float64, 0, len(values))
		for _, value := range values {
			switch number := value.(type) {
			case int64:
				if number < -maxExactFloatInteger || number > maxExactFloatInteger {
					return nil, nil, fmt.Errorf("filter: integer %d loses precision in a decimal list", number)
				}
				out = append(out, float64(number))
			case uint64:
				if number > uint64(maxExactFloatInteger) {
					return nil, nil, fmt.Errorf("filter: integer %d loses precision in a decimal list", number)
				}
				out = append(out, float64(number))
			case float64:
				out = append(out, number)
			}
		}
		return out, out[0], nil
	}

	if hasUint {
		out := make([]uint64, 0, len(values))
		for _, value := range values {
			switch number := value.(type) {
			case int64:
				if number < 0 {
					return nil, nil, fmt.Errorf("filter: numeric list spans signed and unsigned integer ranges")
				}
				out = append(out, uint64(number))
			case uint64:
				out = append(out, number)
			}
		}
		return out, out[0], nil
	}

	out := make([]int64, 0, len(values))
	for _, value := range values {
		out = append(out, value.(int64))
	}
	return out, out[0], nil
}
