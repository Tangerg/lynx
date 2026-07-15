package filterhelp

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/Tangerg/lynx/core/vectorstore/filter"
)

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
