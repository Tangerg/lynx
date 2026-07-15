package filtercompile

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"slices"
	"strconv"
	"strings"

	"github.com/Tangerg/lynx/core/vectorstore/filter"
)

// NumberText returns the exact canonical text stored by a number literal.
// Text-based provider DSLs should prefer it over converting through float64.
func NumberText(lit *filter.Literal) (string, error) {
	if _, err := numberRat(lit); err != nil {
		return "", err
	}
	return lit.Value, nil
}

// NumberIsInteger reports whether a number literal has an integral value.
func NumberIsInteger(lit *filter.Literal) (bool, error) {
	number, err := numberRat(lit)
	if err != nil {
		return false, err
	}
	return number.IsInt(), nil
}

// NumberToInt64 converts an integral number literal without rounding.
func NumberToInt64(lit *filter.Literal) (int64, error) {
	number, err := numberRat(lit)
	if err != nil {
		return 0, err
	}
	if !number.IsInt() {
		return 0, fmt.Errorf("filter: number %q is not an integer", lit.Value)
	}
	integer := number.Num()
	if !integer.IsInt64() {
		return 0, fmt.Errorf("filter: integer %q exceeds int64", lit.Value)
	}
	return integer.Int64(), nil
}

// NumberToInt converts an integral number literal without rounding and rejects
// values outside the platform int range.
func NumberToInt(lit *filter.Literal) (int, error) {
	value, err := NumberToInt64(lit)
	if err != nil {
		return 0, err
	}
	converted := int(value)
	if int64(converted) != value {
		return 0, fmt.Errorf("filter: integer %q exceeds int", lit.Value)
	}
	return converted, nil
}

// NumberToFloat64 converts a number for provider APIs that only accept a
// double. Fractional values follow the provider's float semantics; integral
// values are rejected when the conversion would change their exact value.
func NumberToFloat64(lit *filter.Literal) (float64, error) {
	number, err := numberRat(lit)
	if err != nil {
		return 0, err
	}
	numberValue, err := lit.AsNumber()
	if err != nil {
		return 0, err
	}
	value, err := numberValue.Float64()
	if err != nil {
		return 0, fmt.Errorf("filter: number %q is not a float64: %w", lit.Value, err)
	}
	if number.IsInt() && new(big.Rat).SetFloat64(value).Cmp(number) != 0 {
		return 0, fmt.Errorf("filter: integer %q loses precision as float64", lit.Value)
	}
	return value, nil
}

// NumberToFloat32 converts a number for provider APIs that only accept a
// float. It rejects overflow and integral values that would be rounded.
func NumberToFloat32(lit *filter.Literal) (float32, error) {
	number, err := numberRat(lit)
	if err != nil {
		return 0, err
	}
	numberValue, err := lit.AsNumber()
	if err != nil {
		return 0, err
	}
	value, err := numberValue.Float64()
	if err != nil {
		return 0, fmt.Errorf("filter: number %q is not a float64: %w", lit.Value, err)
	}
	converted := float32(value)
	if math.IsInf(float64(converted), 0) {
		return 0, fmt.Errorf("filter: number %q exceeds float32", lit.Value)
	}
	if number.IsInt() && new(big.Rat).SetFloat64(float64(converted)).Cmp(number) != 0 {
		return 0, fmt.Errorf("filter: integer %q loses precision as float32", lit.Value)
	}
	return converted, nil
}

func numberRat(lit *filter.Literal) (*big.Rat, error) {
	if lit == nil || !lit.IsNumber() {
		return nil, fmt.Errorf("filter: expected number literal, got %v", lit)
	}
	number, ok := new(big.Rat).SetString(lit.Value)
	if !ok {
		return nil, fmt.Errorf("filter: invalid number literal %q", lit.Value)
	}
	return number, nil
}

// LiteralAsKey turns a literal used as an index (e.g. metadata["k"]
// or metadata[3]) into its bare string form. Booleans aren't valid
// keys.
func LiteralAsKey(lit *filter.Literal) (string, error) {
	switch {
	case lit.IsString():
		return lit.AsString()
	case lit.IsNumber():
		value, err := numberValue(lit)
		if err != nil {
			return "", err
		}
		switch number := value.(type) {
		case int64:
			if number < 0 {
				return "", errors.New("filter: numeric index must be non-negative")
			}
			return strconv.FormatInt(number, 10), nil
		case uint64:
			if number > math.MaxInt64 {
				return "", errors.New("filter: numeric index exceeds int64")
			}
			return strconv.FormatUint(number, 10), nil
		case float64:
			if number < 0 || number >= math.Exp2(63) || math.Trunc(number) != number {
				return "", errors.New("filter: numeric index must be a non-negative integer")
			}
			return strconv.FormatUint(uint64(number), 10), nil
		default:
			return "", fmt.Errorf("filter: unsupported numeric index type %T", value)
		}
	default:
		return "", errors.New("filter: index must be a string or number literal")
	}
}

// LiteralToValue decodes a literal into a typed Go value:
// strings stay strings, integers come back as int64 (or uint64 above
// math.MaxInt64), fractional numbers as float64, and booleans as bool.
func LiteralToValue(lit *filter.Literal) (any, error) {
	if lit == nil {
		return nil, errors.New("filter: literal is nil")
	}
	switch {
	case lit.IsString():
		return lit.AsString()
	case lit.IsNumber():
		return numberValue(lit)
	case lit.IsBool():
		return lit.AsBool()
	default:
		return nil, fmt.Errorf("filter: unsupported literal kind %s", lit.Kind)
	}
}

func numberValue(lit *filter.Literal) (any, error) {
	if lit == nil || !lit.IsNumber() {
		return nil, fmt.Errorf("filter: expected number literal, got %v", lit)
	}
	value := lit.Value
	if strings.ContainsAny(value, ".eE") {
		number, err := strconv.ParseFloat(value, 64)
		if err != nil || math.IsNaN(number) || math.IsInf(number, 0) {
			return nil, fmt.Errorf("filter: invalid number literal %q", value)
		}
		return number, nil
	}
	if strings.HasPrefix(value, "-") {
		number, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("filter: invalid integer literal %q: %w", value, err)
		}
		return number, nil
	}
	number, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("filter: invalid integer literal %q: %w", value, err)
	}
	if number <= math.MaxInt64 {
		return int64(number), nil
	}
	return number, nil
}

// ExtractValue asserts expr is an [filter.Literal] then delegates to
// [LiteralToValue]. Used by the comparison branch of every visitor.
func ExtractValue(expr filter.Expr) (any, error) {
	lit, ok := expr.(*filter.Literal)
	if !ok {
		return nil, fmt.Errorf("filter: expected literal, got %T", expr)
	}
	return LiteralToValue(lit)
}

// CollectKeyPath walks the left operand of a comparison to recover
// the metadata key path it addresses.
//
//   - For a bare *filter.Ident the path is just [ident].
//   - For *filter.IndexExpr chains (profile["name"]["first"]) it returns
//     ["profile", "name", "first"].
//
// Callers can extend this by joining the slice with "." (for nested
// dotted paths) or by treating the first element as a flat key.
func CollectKeyPath(expr filter.Expr) ([]string, error) {
	switch node := expr.(type) {
	case *filter.Ident:
		return []string{node.Value}, nil
	case *filter.IndexExpr:
		var keys []string
		current := node
		for {
			key, err := LiteralAsKey(current.Index)
			if err != nil {
				return nil, err
			}
			keys = append(keys, key)
			switch inner := current.Left.(type) {
			case *filter.IndexExpr:
				current = inner
			case *filter.Ident:
				keys = append(keys, inner.Value)
				slices.Reverse(keys)
				return keys, nil
			default:
				return nil, fmt.Errorf("filter: unsupported index base %T", inner)
			}
		}
	default:
		return nil, fmt.Errorf("filter: unsupported left operand %T", node)
	}
}
