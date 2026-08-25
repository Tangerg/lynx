package filter

import (
	"errors"
	"fmt"
	"slices"
)

// ListLiteral is an immutable homogeneous list used by IN expressions.
type ListLiteral struct {
	values []*Literal
	start  Position
	end    Position
}

func (*ListLiteral) expr() {}

func (l *ListLiteral) Len() int {
	if l == nil {
		return 0
	}
	return len(l.values)
}

func (l *ListLiteral) Literals() []*Literal {
	if l == nil {
		return nil
	}
	return slices.Clone(l.values)
}

func (l *ListLiteral) First() (*Literal, error) {
	if l == nil || len(l.values) == 0 {
		return nil, errors.New("filter.ListLiteral.First: list is empty")
	}
	return l.values[0], nil
}

func (l *ListLiteral) Start() Position {
	if l == nil {
		return Position{}
	}
	return l.start
}

func (l *ListLiteral) End() Position {
	if l == nil {
		return Position{}
	}
	return l.end
}

func (l *ListLiteral) Equal(other Expr) bool {
	o, ok := other.(*ListLiteral)
	if !ok || l == nil || o == nil || len(l.values) != len(o.values) {
		return false
	}
	for i := range l.values {
		if !equalExpr(l.values[i], o.values[i]) {
			return false
		}
	}
	return true
}

// Values decodes the immutable list into a homogeneous Go slice:
//
//   - string literals → []string
//   - integer literals → []int64 or []uint64
//   - decimal literals → []float64
//   - boolean literals → []bool
func (l *ListLiteral) Values() (any, error) {
	if l == nil || len(l.values) == 0 {
		return nil, errors.New("filter.ListLiteral.Values: list is empty")
	}
	first := l.values[0]
	if first == nil {
		return nil, errors.New("filter.ListLiteral.Values: element 0 is nil")
	}
	switch {
	case first.IsString():
		out := make([]string, 0, len(l.values))
		for _, literal := range l.values {
			value, err := literal.AsString()
			if err != nil {
				return nil, err
			}
			out = append(out, value)
		}
		return out, nil
	case first.IsNumber():
		return l.numberValues()
	case first.IsBool():
		out := make([]bool, 0, len(l.values))
		for _, literal := range l.values {
			value, err := literal.AsBool()
			if err != nil {
				return nil, err
			}
			out = append(out, value)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("filter.ListLiteral.Values: unsupported element kind %s", first.Kind())
	}
}

func (l *ListLiteral) numberValues() (any, error) {
	values := make([]any, 0, len(l.values))
	hasFloat := false
	hasUint := false
	for _, literal := range l.values {
		value, err := literal.Value()
		if err != nil {
			return nil, err
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
					return nil, fmt.Errorf("filter: integer %d loses precision in a decimal list", number)
				}
				out = append(out, float64(number))
			case uint64:
				if number > uint64(maxExactFloatInteger) {
					return nil, fmt.Errorf("filter: integer %d loses precision in a decimal list", number)
				}
				out = append(out, float64(number))
			case float64:
				out = append(out, number)
			}
		}
		return out, nil
	}

	if hasUint {
		out := make([]uint64, 0, len(values))
		for _, value := range values {
			switch number := value.(type) {
			case int64:
				if number < 0 {
					return nil, errors.New("filter: numeric list spans signed and unsigned integer ranges")
				}
				out = append(out, uint64(number))
			case uint64:
				out = append(out, number)
			}
		}
		return out, nil
	}

	out := make([]int64, 0, len(values))
	for _, value := range values {
		out = append(out, value.(int64))
	}
	return out, nil
}
