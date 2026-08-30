package qdrant

import (
	"fmt"
	"math"
	"strings"

	"github.com/qdrant/go-client/qdrant"

	"github.com/Tangerg/scope/core/vectorstore/filter"
)

func (v *visitor) visitEqualityExpr(expr *filter.BinaryExpr) error {
	fieldKey, err := v.extractFieldKey(expr.Left())
	if err != nil {
		return fmt.Errorf("extract field key from left operand of '%s' at %s: %w",
			expr.Operator().String(), expr.Start().String(), err)
	}

	fieldValue, err := v.extractFieldValue(expr.Right())
	if err != nil {
		return fmt.Errorf("extract value from right operand of '%s' at %s: %w",
			expr.Operator().String(), expr.Start().String(), err)
	}

	matchCond, err := v.buildMatchCondition(fieldKey, fieldValue)
	if err != nil {
		return fmt.Errorf("create match condition for '%s' at %s: %w",
			expr.Operator().String(), expr.Start().String(), err)
	}

	switch expr.Operator() {
	case filter.OpEqual:
		v.filter.Must = append(v.filter.Must, matchCond)
	case filter.OpNotEqual:
		v.filter.MustNot = append(v.filter.MustNot, matchCond)
	default:
		return fmt.Errorf("unexpected equality operator '%s' at %s",
			expr.Operator().String(), expr.Start().String())
	}

	return nil
}

func (v *visitor) visitHasExpr(expr *filter.BinaryExpr) error {
	fieldKey, err := v.extractFieldKey(expr.Left())
	if err != nil {
		return fmt.Errorf("extract collection field at %s: %w", expr.Start().String(), err)
	}
	fieldValue, err := v.extractFieldValue(expr.Right())
	if err != nil {
		return fmt.Errorf("extract collection member at %s: %w", expr.Start().String(), err)
	}
	matchCondition, err := v.buildMatchCondition(fieldKey, fieldValue)
	if err != nil {
		return fmt.Errorf("create collection membership condition at %s: %w", expr.Start().String(), err)
	}
	v.filter.Must = append(v.filter.Must, matchCondition)
	return nil
}

func (v *visitor) buildMatchCondition(fieldKey string, fieldValue any) (*qdrant.Condition, error) {
	switch v := fieldValue.(type) {
	case string:
		return qdrant.NewMatchKeyword(fieldKey, v), nil
	case int64:
		return qdrant.NewMatchInt(fieldKey, v), nil
	case uint64:
		if v > math.MaxInt64 {
			return nil, fmt.Errorf("integer %d exceeds Qdrant's int64 match range", v)
		}
		return qdrant.NewMatchInt(fieldKey, int64(v)), nil
	case float64:
		return nil, fmt.Errorf("qdrant match requires an integer, got %v", v)
	case bool:
		return qdrant.NewMatchBool(fieldKey, v), nil
	default:
		return nil, fmt.Errorf("unsupported value type %T for match condition", fieldValue)
	}
}

func (v *visitor) visitOrderingExpr(expr *filter.BinaryExpr) error {
	fieldKey, err := v.extractFieldKey(expr.Left())
	if err != nil {
		return fmt.Errorf("extract field key from left operand of '%s' at %s: %w",
			expr.Operator().String(), expr.Start().String(), err)
	}

	literal, ok := expr.Right().(*filter.Literal)
	if !ok {
		return fmt.Errorf("right operand of '%s' at %s must be a number literal, got %T",
			expr.Operator().String(), expr.Start().String(), expr.Right())
	}
	numericValue, err := literal.Float64()
	if err != nil {
		return fmt.Errorf("cannot convert value for '%s' comparison at %s: %w",
			expr.Operator().String(), expr.Start().String(), err)
	}

	switch expr.Operator() {
	case filter.OpLess:
		v.filter.Must = append(v.filter.Must, qdrant.NewRange(fieldKey, &qdrant.Range{
			Lt: &numericValue,
		}))
	case filter.OpLessEqual:
		v.filter.Must = append(v.filter.Must, qdrant.NewRange(fieldKey, &qdrant.Range{
			Lte: &numericValue,
		}))
	case filter.OpGreater:
		v.filter.Must = append(v.filter.Must, qdrant.NewRange(fieldKey, &qdrant.Range{
			Gt: &numericValue,
		}))
	case filter.OpGreaterEqual:
		v.filter.Must = append(v.filter.Must, qdrant.NewRange(fieldKey, &qdrant.Range{
			Gte: &numericValue,
		}))
	default:
		return fmt.Errorf("unexpected ordering operator '%s' at %s",
			expr.Operator().String(), expr.Start().String())
	}

	return nil
}

func (v *visitor) visitInExpr(expr *filter.BinaryExpr) error {
	fieldKey, err := v.extractFieldKey(expr.Left())
	if err != nil {
		return fmt.Errorf("extract field key from left operand of 'IN' at %s: %w",
			expr.Start().String(), err)
	}

	listLit, err := expr.List()
	if err != nil {
		return fmt.Errorf("qdrant: %w", err)
	}
	first, err := listLit.First()
	if err != nil {
		return fmt.Errorf("qdrant: IN values: %w", err)
	}

	switch {
	case first.IsString():
		keywords := make([]string, 0, listLit.Len())
		for _, literal := range listLit.Literals() {
			value, err := literal.AsString()
			if err != nil {
				return err
			}
			keywords = append(keywords, value)
		}
		v.filter.Must = append(v.filter.Must, qdrant.NewMatchKeywords(fieldKey, keywords...))

	case first.IsNumber():
		integers := make([]int64, 0, listLit.Len())
		for _, literal := range listLit.Literals() {
			value, err := literal.Int64()
			if err != nil {
				return fmt.Errorf("qdrant: IN numeric value: %w", err)
			}
			integers = append(integers, value)
		}
		v.filter.Must = append(v.filter.Must, qdrant.NewMatchInts(fieldKey, integers...))

	case first.IsBool():
		// Qdrant has no boolean-list matcher; nesting Should keeps this IN
		// expression's OR local instead of widening the enclosing filter.
		boolConditions := make([]*qdrant.Condition, 0, listLit.Len())
		for _, literal := range listLit.Literals() {
			value, err := literal.AsBool()
			if err != nil {
				return err
			}
			boolConditions = append(boolConditions, qdrant.NewMatchBool(fieldKey, value))
		}
		v.filter.Must = append(v.filter.Must,
			qdrant.NewFilterAsCondition(&qdrant.Filter{
				Should: boolConditions,
			}))

	default:
		return fmt.Errorf("unsupported literal kind %s in 'IN' list at %s",
			first.Kind(), expr.Start().String())
	}

	return nil
}

func (v *visitor) visitLikeExpr(expr *filter.BinaryExpr) error {
	fieldKey, err := v.extractFieldKey(expr.Left())
	if err != nil {
		return fmt.Errorf("qdrant: extract field key from left operand of LIKE at %s: %w",
			expr.Start().String(), err)
	}

	lit, ok := expr.Right().(*filter.Literal)
	if !ok {
		return fmt.Errorf("qdrant: LIKE requires a string literal on the right side at %s, got %T",
			expr.Start().String(), expr.Right())
	}

	if !lit.IsString() {
		return fmt.Errorf("qdrant: LIKE requires a string pattern at %s, got %s",
			expr.Start().String(), lit.Kind())
	}
	pattern, err := lit.AsString()
	if err != nil {
		return fmt.Errorf("qdrant: read LIKE pattern at %s: %w", expr.Start().String(), err)
	}
	if strings.ContainsAny(pattern, "%_") {
		return fmt.Errorf("qdrant: LIKE pattern %q cannot be represented exactly by Qdrant filters", pattern)
	}

	v.filter.Must = append(v.filter.Must, qdrant.NewMatchKeyword(fieldKey, pattern))
	return nil
}

func (v *visitor) buildNestedCondition(expr filter.Expr) (*qdrant.Condition, error) {
	switch node := expr.(type) {
	case *filter.BinaryExpr,
		*filter.UnaryExpr:
		nestedConv := newVisitor()
		err := nestedConv.visit(node)
		if err != nil {
			return nil, err
		}
		return qdrant.NewFilterAsCondition(nestedConv.filter), nil

	default:
		return nil, fmt.Errorf("unsupported expression type %T for condition building", node)
	}
}

func (v *visitor) extractFieldKey(expr filter.Expr) (string, error) {
	savedFieldKey := v.currentFieldKey
	v.currentFieldKey = ""

	err := v.visit(expr)

	extractedKey := v.currentFieldKey
	v.currentFieldKey = savedFieldKey

	if err != nil {
		return "", err
	}

	if extractedKey == "" {
		return "", fmt.Errorf("extract field key from %T expression", expr)
	}

	return extractedKey, nil
}

func (v *visitor) extractFieldValue(expr filter.Expr) (any, error) {
	savedFieldValue := v.currentFieldValue
	v.currentFieldValue = nil

	err := v.visit(expr)

	extractedValue := v.currentFieldValue
	v.currentFieldValue = savedFieldValue

	if err != nil {
		return nil, err
	}

	if extractedValue == nil {
		return nil, fmt.Errorf("extract value from %T expression", expr)
	}

	return extractedValue, nil
}

func (v *visitor) buildIndexedFieldKey(expr *filter.IndexExpr) (string, error) {
	var pathParts []string

	currentExpr := expr
	for {
		key, err := currentExpr.Index().Key()
		if err != nil {
			return "", err
		}
		pathParts = append([]string{key}, pathParts...)

		switch leftNode := currentExpr.Left().(type) {
		case *filter.IndexExpr:
			currentExpr = leftNode
		case *filter.Ident:
			pathParts = append([]string{leftNode.Name()}, pathParts...)
			return strings.Join(pathParts, "."), nil
		default:
			return "", fmt.Errorf("invalid left operand type %T in index expression, expected identifier or index", leftNode)
		}
	}
}

func (v *visitor) literalToValue(lit *filter.Literal) (any, error) {
	if lit.IsString() {
		return lit.AsString()
	}

	if lit.IsNumber() {
		return lit.Value()
	}

	if lit.IsBool() {
		return lit.AsBool()
	}

	return nil, fmt.Errorf("unsupported literal type '%s'", lit.Kind())
}
