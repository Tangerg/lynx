package weaviate

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/weaviate/weaviate-go-client/v5/weaviate/filters"

	"github.com/Tangerg/lynx/core/vectorstore/filter"
)

// Visitor compiles Lynx filter expressions into Weaviate where filters.
// A visitor can be reused; each call to Visit replaces the previous result.
type Visitor struct {
	result *filters.WhereBuilder
}

var _ filter.Visitor = (*Visitor)(nil)

// NewVisitor creates a Weaviate filter compiler.
func NewVisitor() *Visitor {
	return &Visitor{}
}

// Visit compiles the complete expression tree rooted at expr.
func (v *Visitor) Visit(expr filter.Predicate) error {
	v.result = nil
	result, err := compileFilter(expr)
	if err != nil {
		return err
	}
	v.result = result
	return nil
}

// Result returns the filter produced by the most recent successful Visit.
func (v *Visitor) Result() *filters.WhereBuilder {
	return v.result
}

// ToFilter compiles a Lynx filter expression into a Weaviate where filter.
func ToFilter(expr filter.Predicate) (*filters.WhereBuilder, error) {
	visitor := NewVisitor()
	if err := visitor.Visit(expr); err != nil {
		return nil, err
	}
	return visitor.Result(), nil
}

func compileFilter(expr filter.Expr) (*filters.WhereBuilder, error) {
	if expr == nil {
		return nil, errors.New("weaviate.filter: expression must not be nil")
	}

	switch node := expr.(type) {
	case *filter.BinaryExpr:
		if node == nil {
			return nil, errors.New("weaviate.filter: binary expression must not be nil")
		}
		return compileBinary(node)
	case *filter.UnaryExpr:
		if node == nil {
			return nil, errors.New("weaviate.filter: unary expression must not be nil")
		}
		return compileUnary(node)
	default:
		return nil, fmt.Errorf("weaviate.filter: expected predicate, got %T", expr)
	}
}

func compileBinary(expr *filter.BinaryExpr) (*filters.WhereBuilder, error) {
	switch {
	case expr.Op.IsNullOperator():
		return compileNullTest(expr)
	case expr.Op.IsLogicalOperator():
		return compileLogical(expr)
	case expr.Op.IsComparisonOperator():
		return compileComparison(expr)
	case expr.Op.Is(filter.OpIn):
		return compileIn(expr)
	case expr.Op.Is(filter.OpHas):
		return compileHas(expr)
	case expr.Op.Is(filter.OpLike):
		return compileLike(expr)
	default:
		return nil, fmt.Errorf("weaviate.filter: unsupported binary operator %q at %s",
			expr.Op.String(), expr.Start())
	}
}

func compileUnary(expr *filter.UnaryExpr) (*filters.WhereBuilder, error) {
	if !expr.Op.Is(filter.OpNot) {
		return nil, fmt.Errorf("weaviate.filter: unsupported unary operator %q at %s",
			expr.Op.String(), expr.Start())
	}
	operand, err := compileFilter(expr.Right)
	if err != nil {
		return nil, fmt.Errorf("weaviate.filter: NOT operand: %w", err)
	}
	return filters.Where().
		WithOperator(filters.Not).
		WithOperands([]*filters.WhereBuilder{operand}), nil
}

func compileLogical(expr *filter.BinaryExpr) (*filters.WhereBuilder, error) {
	left, err := compileFilter(expr.Left)
	if err != nil {
		return nil, fmt.Errorf("weaviate.filter: left operand of %s: %w", expr.Op, err)
	}
	right, err := compileFilter(expr.Right)
	if err != nil {
		return nil, fmt.Errorf("weaviate.filter: right operand of %s: %w", expr.Op, err)
	}

	var operator filters.WhereOperator
	switch expr.Op {
	case filter.OpAnd:
		operator = filters.And
	case filter.OpOr:
		operator = filters.Or
	default:
		return nil, fmt.Errorf("weaviate.filter: unsupported logical operator %q", expr.Op)
	}
	return filters.Where().
		WithOperator(operator).
		WithOperands([]*filters.WhereBuilder{left, right}), nil
}

func compileComparison(expr *filter.BinaryExpr) (*filters.WhereBuilder, error) {
	path, err := filter.CollectKeyPath(expr.Left)
	if err != nil {
		return nil, fmt.Errorf("weaviate.filter: left operand of %s: %w", expr.Op, err)
	}
	literal, ok := expr.Right.(*filter.Literal)
	if !ok || literal == nil {
		return nil, fmt.Errorf("weaviate.filter: right operand of %s must be a literal, got %T at %s",
			expr.Op, expr.Right, expr.Start())
	}

	operator, err := comparisonOperator(expr.Op)
	if err != nil {
		return nil, err
	}
	if !expr.Op.IsEqualityOperator() && !literal.IsNumber() {
		return nil, fmt.Errorf("weaviate.filter: %s requires a numeric right operand at %s",
			expr.Op, expr.Start())
	}
	return scalarFilter(path, operator, literal)
}

func comparisonOperator(operator filter.Operator) (filters.WhereOperator, error) {
	switch operator {
	case filter.OpEqual:
		return filters.Equal, nil
	case filter.OpNotEqual:
		return filters.NotEqual, nil
	case filter.OpLess:
		return filters.LessThan, nil
	case filter.OpLessEqual:
		return filters.LessThanEqual, nil
	case filter.OpGreater:
		return filters.GreaterThan, nil
	case filter.OpGreaterEqual:
		return filters.GreaterThanEqual, nil
	default:
		return "", fmt.Errorf("weaviate.filter: unsupported comparison operator %q", operator)
	}
}

func scalarFilter(
	path []string,
	operator filters.WhereOperator,
	literal *filter.Literal,
) (*filters.WhereBuilder, error) {
	builder := filters.Where().WithPath(path).WithOperator(operator)
	switch {
	case literal.IsString():
		value, err := literal.AsString()
		if err != nil {
			return nil, fmt.Errorf("weaviate.filter: string literal: %w", err)
		}
		return builder.WithValueText(value), nil
	case literal.IsBool():
		value, err := literal.AsBool()
		if err != nil {
			return nil, fmt.Errorf("weaviate.filter: boolean literal: %w", err)
		}
		return builder.WithValueBoolean(value), nil
	case literal.IsNumber():
		value, err := filter.LiteralToValue(literal)
		if err != nil {
			return nil, fmt.Errorf("weaviate.filter: number literal: %w", err)
		}
		switch number := value.(type) {
		case int64:
			return builder.WithValueInt(number), nil
		case uint64:
			if number > math.MaxInt64 {
				return nil, fmt.Errorf("weaviate.filter: integer %q exceeds Weaviate int64", literal.Value)
			}
			return builder.WithValueInt(int64(number)), nil
		case float64:
			return builder.WithValueNumber(number), nil
		default:
			return nil, fmt.Errorf("weaviate.filter: unsupported numeric value %T", value)
		}
	default:
		return nil, fmt.Errorf("weaviate.filter: unsupported literal kind %s", literal.Kind)
	}
}

func compileIn(expr *filter.BinaryExpr) (*filters.WhereBuilder, error) {
	path, err := filter.CollectKeyPath(expr.Left)
	if err != nil {
		return nil, fmt.Errorf("weaviate.filter: left operand of IN: %w", err)
	}
	list, err := filter.RequireListLiteral(expr)
	if err != nil {
		return nil, fmt.Errorf("weaviate.filter: %w", err)
	}
	if _, _, err := filter.ConvertListLiteral(list); err != nil {
		return nil, fmt.Errorf("weaviate.filter: IN values: %w", err)
	}

	operands := make([]*filters.WhereBuilder, 0, len(list.Values))
	for _, literal := range list.Values {
		operand, err := scalarFilter(path, filters.Equal, literal)
		if err != nil {
			return nil, fmt.Errorf("weaviate.filter: IN value: %w", err)
		}
		operands = append(operands, operand)
	}
	return filters.Where().
		WithOperator(filters.Or).
		WithOperands(operands), nil
}

func compileHas(expr *filter.BinaryExpr) (*filters.WhereBuilder, error) {
	path, err := filter.CollectKeyPath(expr.Left)
	if err != nil {
		return nil, fmt.Errorf("weaviate.filter: left operand of HAS: %w", err)
	}
	literal, ok := expr.Right.(*filter.Literal)
	if !ok || literal == nil {
		return nil, fmt.Errorf("weaviate.filter: right operand of HAS must be a literal, got %T at %s",
			expr.Right, expr.Start())
	}
	return scalarFilter(path, filters.ContainsAny, literal)
}

func compileLike(expr *filter.BinaryExpr) (*filters.WhereBuilder, error) {
	path, err := filter.CollectKeyPath(expr.Left)
	if err != nil {
		return nil, fmt.Errorf("weaviate.filter: left operand of LIKE: %w", err)
	}
	pattern, err := filter.RequireStringPatternOnRight(expr)
	if err != nil {
		return nil, fmt.Errorf("weaviate.filter: %w", err)
	}
	translated, err := weaviateLikePattern(pattern)
	if err != nil {
		return nil, err
	}
	return filters.Where().
		WithPath(path).
		WithOperator(filters.Like).
		WithValueText(translated), nil
}

// weaviateLikePattern translates SQL LIKE wildcards into Weaviate's wildcard
// syntax. Weaviate cannot escape literal '*' or '?', so accepting either
// would broaden the predicate and violate the source expression.
func weaviateLikePattern(pattern string) (string, error) {
	if strings.ContainsAny(pattern, "*?") {
		return "", errors.New("weaviate.filter: LIKE cannot represent literal '*' or '?' characters")
	}
	return strings.NewReplacer("%", "*", "_", "?").Replace(pattern), nil
}

func compileNullTest(expr *filter.BinaryExpr) (*filters.WhereBuilder, error) {
	path, err := filter.CollectKeyPath(expr.Left)
	if err != nil {
		return nil, fmt.Errorf("weaviate.filter: left operand of IS NULL: %w", err)
	}
	return filters.Where().
		WithPath(path).
		WithOperator(filters.IsNull).
		WithValueBoolean(true), nil
}
