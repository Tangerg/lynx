package filterhelp

import (
	"errors"
	"fmt"
	"strconv"

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
		n, err := lit.AsNumber()
		if err != nil {
			return "", err
		}
		if WholeNumber(n) {
			return strconv.FormatInt(int64(n), 10), nil
		}
		return strconv.FormatFloat(n, 'f', -1, 64), nil
	default:
		return "", errors.New("filter: index must be a string or number literal")
	}
}

// LiteralToValue decodes a literal into a typed Go value:
// strings stay strings, integers come back as int64, fractional
// numbers as float64, booleans as bool.
func LiteralToValue(lit *filter.Literal) (any, error) {
	switch {
	case lit.IsString():
		return lit.AsString()
	case lit.IsNumber():
		n, err := lit.AsNumber()
		if err != nil {
			return nil, err
		}
		if WholeNumber(n) {
			return int64(n), nil
		}
		return n, nil
	case lit.IsBool():
		return lit.AsBool()
	default:
		return nil, fmt.Errorf("filter: unsupported literal kind %s", lit.Kind)
	}
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
//   - For *filter.IndexExpr chains (metadata["a"]["b"]["c"]) it returns
//     ["a", "b", "c"] — the base identifier ("metadata" in the
//     example) is dropped, since every backend stores metadata under
//     its own namespace.
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
			keys = append([]string{key}, keys...)
			switch inner := current.Left.(type) {
			case *filter.IndexExpr:
				current = inner
			case *filter.Ident:
				return keys, nil
			default:
				return nil, fmt.Errorf("filter: unsupported index base %T", inner)
			}
		}
	default:
		return nil, fmt.Errorf("filter: unsupported left operand %T", node)
	}
}

// WholeNumber reports whether f represents a whole-number value that
// round-trips through int64.
func WholeNumber(f float64) bool {
	return float64(int64(f)) == f
}
