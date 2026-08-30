package chroma

import (
	"errors"
	"fmt"
	"strings"

	v2 "github.com/amikos-tech/chroma-go/pkg/api/v2"

	"github.com/Tangerg/scope/core/vectorstore/filter"
)

var _ filter.Visitor = (*Visitor)(nil)

// Visitor compiles Scope filter expressions into Chroma WhereClause values. A
// value is reusable: Visit replaces the previous result. Chroma exposes no
// standalone NOT or metadata LIKE operation, represents nested selectors as
// flat dotted keys, and accepts numbers only through int or float32 APIs;
// compilation rejects every unsupported or lossy mapping.
type Visitor struct {
	result v2.WhereClause
}

type chromaNumber struct {
	integer   int
	fraction  float32
	isInteger bool
}

func NewVisitor() *Visitor {
	return &Visitor{}
}

func (v *Visitor) Result() v2.WhereClause {
	return v.result
}

func (v *Visitor) Visit(predicate filter.Predicate) error {
	v.result = nil
	result, err := compilePredicate(predicate)
	if err != nil {
		return err
	}
	v.result = result
	return nil
}

func compilePredicate(predicate filter.Predicate) (v2.WhereClause, error) {
	switch expression := predicate.(type) {
	case *filter.BinaryExpr:
		return compileBinary(expression)
	case *filter.UnaryExpr:
		return nil, errors.New("chroma: NOT operator is not supported; rewrite using != or NIN")
	default:
		return nil, fmt.Errorf("chroma: unsupported predicate type %T", expression)
	}
}

func compileBinary(expression *filter.BinaryExpr) (v2.WhereClause, error) {
	switch operator := expression.Operator(); {
	case operator.IsLogicalOperator():
		return compileLogical(expression)
	case operator.IsEqualityOperator():
		return compileEquality(expression)
	case operator.IsOrderingOperator():
		return compileOrdering(expression)
	case operator.Is(filter.OpIn):
		return compileIn(expression)
	case operator.Is(filter.OpHas):
		return nil, fmt.Errorf("chroma: HAS is not supported because Chroma metadata values are scalar (at %s)", expression.Start())
	case operator.Is(filter.OpLike):
		return nil, fmt.Errorf("chroma: LIKE operator is not supported on metadata fields (at %s)", expression.Start())
	default:
		return nil, fmt.Errorf("chroma: unsupported binary operator %q at %s", operator, expression.Start())
	}
}

func compileLogical(expression *filter.BinaryExpr) (v2.WhereClause, error) {
	left, err := compileOperand(expression.Left())
	if err != nil {
		return nil, fmt.Errorf("chroma: process left operand of '%s' at %s: %w", expression.Operator(), expression.Start(), err)
	}
	right, err := compileOperand(expression.Right())
	if err != nil {
		return nil, fmt.Errorf("chroma: process right operand of '%s' at %s: %w", expression.Operator(), expression.Start(), err)
	}
	switch expression.Operator() {
	case filter.OpAnd:
		return v2.And(left, right), nil
	case filter.OpOr:
		return v2.Or(left, right), nil
	default:
		return nil, fmt.Errorf("chroma: unexpected logical operator '%s' at %s", expression.Operator(), expression.Start())
	}
}

func compileOperand(expression filter.Expr) (v2.WhereClause, error) {
	predicate, ok := expression.(filter.Predicate)
	if !ok {
		return nil, fmt.Errorf("chroma: unsupported expression type %T for clause building", expression)
	}
	return compilePredicate(predicate)
}

func compileEquality(expression *filter.BinaryExpr) (v2.WhereClause, error) {
	fieldKey, err := selectorKey(expression)
	if err != nil {
		return nil, fmt.Errorf("chroma: extract field key from left operand of '%s' at %s: %w", expression.Operator(), expression.Start(), err)
	}
	literal, err := expression.Literal()
	if err != nil {
		return nil, err
	}
	fieldValue, err := literalToValue(literal)
	if err != nil {
		return nil, fmt.Errorf("chroma: convert literal at %s: %w", literal.Start(), err)
	}
	clause, err := equalityClause(fieldKey, fieldValue, expression.Operator() == filter.OpEqual)
	if err != nil {
		return nil, fmt.Errorf("chroma: build equality clause for '%s' at %s: %w", expression.Operator(), expression.Start(), err)
	}
	return clause, nil
}

func equalityClause(fieldKey string, fieldValue any, equal bool) (v2.WhereClause, error) {
	switch value := fieldValue.(type) {
	case string:
		if equal {
			return v2.EqString(fieldKey, value), nil
		}
		return v2.NotEqString(fieldKey, value), nil
	case chromaNumber:
		return numericEqualityClause(fieldKey, value, equal), nil
	case bool:
		if equal {
			return v2.EqBool(fieldKey, value), nil
		}
		return v2.NotEqBool(fieldKey, value), nil
	default:
		return nil, fmt.Errorf("chroma: unsupported value type %T for equality condition", fieldValue)
	}
}

func numericEqualityClause(fieldKey string, value chromaNumber, equal bool) v2.WhereClause {
	if value.isInteger {
		if equal {
			return v2.EqInt(fieldKey, value.integer)
		}
		return v2.NotEqInt(fieldKey, value.integer)
	}
	if equal {
		return v2.EqFloat(fieldKey, value.fraction)
	}
	return v2.NotEqFloat(fieldKey, value.fraction)
}

func compileOrdering(expression *filter.BinaryExpr) (v2.WhereClause, error) {
	fieldKey, err := selectorKey(expression)
	if err != nil {
		return nil, fmt.Errorf("chroma: extract field key from left operand of '%s' at %s: %w", expression.Operator(), expression.Start(), err)
	}
	literal, err := expression.Literal()
	if err != nil {
		return nil, err
	}
	fieldValue, err := literalToValue(literal)
	if err != nil {
		return nil, fmt.Errorf("chroma: convert literal at %s: %w", literal.Start(), err)
	}
	numericValue, ok := fieldValue.(chromaNumber)
	if !ok {
		return nil, fmt.Errorf("chroma: cannot convert value to number for '%s' comparison at %s: expected number, got %T", expression.Operator(), expression.Start(), fieldValue)
	}
	return orderingClause(fieldKey, numericValue, expression.Operator(), expression.Start().String())
}

func orderingClause(fieldKey string, value chromaNumber, operator filter.Operator, position string) (v2.WhereClause, error) {
	switch operator {
	case filter.OpLess:
		if value.isInteger {
			return v2.LtInt(fieldKey, value.integer), nil
		}
		return v2.LtFloat(fieldKey, value.fraction), nil
	case filter.OpLessEqual:
		if value.isInteger {
			return v2.LteInt(fieldKey, value.integer), nil
		}
		return v2.LteFloat(fieldKey, value.fraction), nil
	case filter.OpGreater:
		if value.isInteger {
			return v2.GtInt(fieldKey, value.integer), nil
		}
		return v2.GtFloat(fieldKey, value.fraction), nil
	case filter.OpGreaterEqual:
		if value.isInteger {
			return v2.GteInt(fieldKey, value.integer), nil
		}
		return v2.GteFloat(fieldKey, value.fraction), nil
	default:
		return nil, fmt.Errorf("chroma: unexpected ordering operator '%s' at %s", operator, position)
	}
}

func compileIn(expression *filter.BinaryExpr) (v2.WhereClause, error) {
	fieldKey, err := selectorKey(expression)
	if err != nil {
		return nil, fmt.Errorf("chroma: extract field key from left operand of 'IN' at %s: %w", expression.Start(), err)
	}
	list, err := expression.List()
	if err != nil {
		return nil, fmt.Errorf("chroma: %w", err)
	}
	literals := list.Literals()
	switch {
	case literals[0].IsString():
		values, err := stringList(literals)
		if err != nil {
			return nil, err
		}
		return v2.InString(fieldKey, values...), nil
	case literals[0].IsNumber():
		return numericInClause(fieldKey, literals)
	case literals[0].IsBool():
		values, err := boolList(literals)
		if err != nil {
			return nil, err
		}
		return v2.InBool(fieldKey, values...), nil
	default:
		return nil, fmt.Errorf("chroma: unsupported value type %s in 'IN' list at %s", literals[0].Kind(), expression.Start())
	}
}

func stringList(literals []*filter.Literal) ([]string, error) {
	values := make([]string, 0, len(literals))
	for _, literal := range literals {
		value, err := literal.AsString()
		if err != nil {
			return nil, fmt.Errorf("chroma: IN string value: %w", err)
		}
		values = append(values, value)
	}
	return values, nil
}

func boolList(literals []*filter.Literal) ([]bool, error) {
	values := make([]bool, 0, len(literals))
	for _, literal := range literals {
		value, err := literal.AsBool()
		if err != nil {
			return nil, fmt.Errorf("chroma: IN bool value: %w", err)
		}
		values = append(values, value)
	}
	return values, nil
}

func numericInClause(fieldKey string, literals []*filter.Literal) (v2.WhereClause, error) {
	integers := make([]int, 0, len(literals))
	allIntegers := true
	for _, literal := range literals {
		integer, err := literal.IsInteger()
		if err != nil {
			return nil, fmt.Errorf("chroma: IN numeric value: %w", err)
		}
		if !integer {
			allIntegers = false
			break
		}
		value, err := literal.Int()
		if err != nil {
			return nil, fmt.Errorf("chroma: IN numeric value: %w", err)
		}
		integers = append(integers, value)
	}
	if allIntegers {
		return v2.InInt(fieldKey, integers...), nil
	}

	floats := make([]float32, 0, len(literals))
	for _, literal := range literals {
		value, err := literal.Float32()
		if err != nil {
			return nil, fmt.Errorf("chroma: IN numeric value: %w", err)
		}
		floats = append(floats, value)
	}
	return v2.InFloat(fieldKey, floats...), nil
}

func selectorKey(expression *filter.BinaryExpr) (string, error) {
	path, err := expression.Path()
	if err != nil {
		return "", err
	}
	return strings.Join(path, "."), nil
}

func literalToValue(literal *filter.Literal) (any, error) {
	if literal.IsString() {
		return literal.AsString()
	}
	if literal.IsNumber() {
		return numericLiteralValue(literal)
	}
	if literal.IsBool() {
		return literal.AsBool()
	}
	return nil, fmt.Errorf("chroma: unsupported literal type '%s'", literal.Kind())
}

func numericLiteralValue(literal *filter.Literal) (chromaNumber, error) {
	integer, err := literal.IsInteger()
	if err != nil {
		return chromaNumber{}, err
	}
	if integer {
		value, intErr := literal.Int()
		if intErr != nil {
			return chromaNumber{}, intErr
		}
		return chromaNumber{integer: value, isInteger: true}, nil
	}
	value, err := literal.Float32()
	if err != nil {
		return chromaNumber{}, err
	}
	return chromaNumber{fraction: value}, nil
}
