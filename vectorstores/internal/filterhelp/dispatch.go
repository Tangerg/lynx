package filterhelp

import (
	"fmt"

	"github.com/Tangerg/lynx/core/vectorstore/filter/ast"
	"github.com/Tangerg/lynx/core/vectorstore/filter/token"
)

// DispatchBinary routes a [*ast.BinaryExpr] to one of four handlers
// based on the operator's category. The filter mini-language has
// exactly four families of binary operators:
//
//   - Logical:    AND, OR                             → onLogical
//   - Comparison: ==, !=, <, <=, >, >=                → onComparison
//   - Membership: IN                                  → onIn
//   - Pattern:    LIKE                                → onLike
//
// Operators outside these families produce a descriptive error. T is
// the visitor's emit type (string for SQL backends, *qdrant.Condition
// for qdrant, etc.).
//
// Example:
//
//	func (v *Visitor) visitBinaryExpr(e *ast.BinaryExpr) error {
//	    _, err := filterhelp.DispatchBinary[struct{}](
//	        e,
//	        func(e *ast.BinaryExpr) (struct{}, error) {
//	            return struct{}{}, v.visitLogicalExpr(e)
//	        },
//	        // ... etc
//	    )
//	    return err
//	}
func DispatchBinary[T any](
	expr *ast.BinaryExpr,
	onLogical func(*ast.BinaryExpr) (T, error),
	onComparison func(*ast.BinaryExpr) (T, error),
	onIn func(*ast.BinaryExpr) (T, error),
	onLike func(*ast.BinaryExpr) (T, error),
) (T, error) {
	var zero T
	switch {
	case expr.Op.Kind.IsLogicalOperator():
		return onLogical(expr)
	case expr.Op.Kind.IsComparisonOperator():
		return onComparison(expr)
	case expr.Op.Kind.Is(token.IN):
		return onIn(expr)
	case expr.Op.Kind.Is(token.LIKE):
		return onLike(expr)
	default:
		return zero, fmt.Errorf("filter: unsupported binary operator %q at %s",
			expr.Op.Literal, expr.Start().String())
	}
}

// DispatchUnary routes a [*ast.UnaryExpr] to onNot. The filter
// language only has one unary operator today (NOT); any other kind
// returns a descriptive error.
func DispatchUnary[T any](
	expr *ast.UnaryExpr,
	onNot func(*ast.UnaryExpr) (T, error),
) (T, error) {
	var zero T
	if !expr.Op.Kind.Is(token.NOT) {
		return zero, fmt.Errorf("filter: unsupported unary operator %q at %s",
			expr.Op.Literal, expr.Start().String())
	}
	return onNot(expr)
}

// DispatchBinaryErr is the error-only variant of [DispatchBinary] for
// visitors that emit into shared state (a SQL string builder, an
// SDK filter struct) and don't return per-node values. The dispatch
// rules are identical to DispatchBinary.
func DispatchBinaryErr(
	expr *ast.BinaryExpr,
	onLogical func(*ast.BinaryExpr) error,
	onComparison func(*ast.BinaryExpr) error,
	onIn func(*ast.BinaryExpr) error,
	onLike func(*ast.BinaryExpr) error,
) error {
	switch {
	case expr.Op.Kind.IsLogicalOperator():
		return onLogical(expr)
	case expr.Op.Kind.IsComparisonOperator():
		return onComparison(expr)
	case expr.Op.Kind.Is(token.IN):
		return onIn(expr)
	case expr.Op.Kind.Is(token.LIKE):
		return onLike(expr)
	default:
		return fmt.Errorf("filter: unsupported binary operator %q at %s",
			expr.Op.Literal, expr.Start().String())
	}
}

// DispatchUnaryErr is the error-only variant of [DispatchUnary] for
// stateful visitors.
func DispatchUnaryErr(
	expr *ast.UnaryExpr,
	onNot func(*ast.UnaryExpr) error,
) error {
	if !expr.Op.Kind.Is(token.NOT) {
		return fmt.Errorf("filter: unsupported unary operator %q at %s",
			expr.Op.Literal, expr.Start().String())
	}
	return onNot(expr)
}

// LogicalOpString returns "AND" / "OR" for the matching token kind.
// Errors for any non-logical kind. Used by SQL / text-output backends
// that emit the operator verbatim into their query language.
func LogicalOpString(k token.Kind) (string, error) {
	switch k {
	case token.AND:
		return "AND", nil
	case token.OR:
		return "OR", nil
	default:
		return "", fmt.Errorf("filter: expected logical operator, got %s", k.Name())
	}
}

// RequireListLiteral asserts the right operand of expr is a non-empty
// [*ast.ListLiteral] — the contract every backend's IN handler needs.
// Centralizes the two error messages every vendor used to emit
// verbatim.
func RequireListLiteral(expr *ast.BinaryExpr) (*ast.ListLiteral, error) {
	list, ok := expr.Right.(*ast.ListLiteral)
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
func RequireStringPatternOnRight(expr *ast.BinaryExpr) (string, error) {
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
//   - number literals → []float64
//   - boolean literals → []bool
//
// The element-type slice is returned as any (so the caller can hand it
// to a driver that auto-detects), plus a sample of the first element
// for branching on element type without re-inspecting the slice.
//
// Returns an error if the literals don't all share a kind or the kind
// is unsupported.
func ConvertListLiteral(list *ast.ListLiteral) (slice any, sample any, err error) {
	if list == nil || len(list.Values) == 0 {
		return nil, nil, fmt.Errorf("filter: empty list literal")
	}
	first := list.Values[0]
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
		out := make([]float64, 0, len(list.Values))
		for _, lit := range list.Values {
			n, err := lit.AsNumber()
			if err != nil {
				return nil, nil, err
			}
			out = append(out, n)
		}
		return out, out[0], nil
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
			first.Token.Kind.Name())
	}
}
