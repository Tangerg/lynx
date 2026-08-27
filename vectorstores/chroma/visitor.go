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
	err               error
	result            v2.WhereClause
	currentFieldKey   string
	currentFieldValue any
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
	if v.err != nil {
		return nil
	}
	return v.result
}

func (v *Visitor) Visit(expr filter.Predicate) error {
	v.err = nil
	v.result = nil
	v.currentFieldKey = ""
	v.currentFieldValue = nil
	v.err = v.visit(expr)
	return v.err
}

func (v *Visitor) visit(expr filter.Expr) error {
	if expr == nil {
		return errors.New("chroma: cannot process nil expression")
	}
	if v.err != nil {
		return v.err
	}

	switch node := expr.(type) {
	case *filter.BinaryExpr:
		return v.visitBinaryExpr(node)
	case *filter.UnaryExpr:
		return v.visitUnaryExpr(node)
	case *filter.IndexExpr:
		return v.visitIndexExpr(node)
	case *filter.Ident:
		return v.visitIdent(node)
	case *filter.Literal:
		return v.visitLiteral(node)
	case *filter.ListLiteral:
		return v.visitListLiteral(node)
	default:
		return fmt.Errorf("chroma: unsupported expression type %T", node)
	}
}

func (v *Visitor) visitBinaryExpr(expr *filter.BinaryExpr) error {
	return expr.Dispatch(filter.BinaryHandlers{
		Logical:    v.visitLogicalExpr,
		Comparison: v.visitComparisonExpr,
		In:         v.visitInExpr,
		Has:        v.visitHasExpr,
		Like:       v.visitLikeExpr,
	})
}

func (v *Visitor) visitHasExpr(expr *filter.BinaryExpr) error {
	return fmt.Errorf("chroma: HAS is not supported because Chroma metadata values are scalar (at %s)",
		expr.Start().String())
}

func (v *Visitor) visitComparisonExpr(expr *filter.BinaryExpr) error {
	if expr.Operator().IsEqualityOperator() {
		return v.visitEqualityExpr(expr)
	}
	return v.visitOrderingExpr(expr)
}

func (v *Visitor) visitLikeExpr(expr *filter.BinaryExpr) error {
	return fmt.Errorf("chroma: LIKE operator is not supported on metadata fields (at %s)",
		expr.Start().String())
}

func (v *Visitor) visitUnaryExpr(expr *filter.UnaryExpr) error {
	return expr.Dispatch(func(*filter.UnaryExpr) error {
		return errors.New("chroma: NOT operator is not supported; rewrite using != or NIN")
	})
}

func (v *Visitor) visitIdent(ident *filter.Ident) error {
	v.currentFieldKey = ident.Name()
	return nil
}

func (v *Visitor) visitLiteral(lit *filter.Literal) error {
	value, err := v.literalToValue(lit)
	if err != nil {
		return fmt.Errorf("chroma: convert literal at %s: %w", lit.Start().String(), err)
	}
	v.currentFieldValue = value
	return nil
}

func (v *Visitor) visitListLiteral(list *filter.ListLiteral) error {
	values := make([]any, 0, list.Len())
	for i, lit := range list.Literals() {
		value, err := v.literalToValue(lit)
		if err != nil {
			return fmt.Errorf("chroma: convert list element at index %d: %w", i, err)
		}
		values = append(values, value)
	}
	v.currentFieldValue = values
	return nil
}

func (v *Visitor) visitIndexExpr(expr *filter.IndexExpr) error {
	fieldKey, err := v.buildIndexedFieldKey(expr)
	if err != nil {
		return fmt.Errorf("chroma: build field path at %s: %w", expr.Start().String(), err)
	}
	v.currentFieldKey = fieldKey
	return nil
}

func (v *Visitor) visitLogicalExpr(expr *filter.BinaryExpr) error {
	leftClause, err := v.buildNestedClause(expr.Left())
	if err != nil {
		return fmt.Errorf("chroma: process left operand of '%s' at %s: %w",
			expr.Operator().String(), expr.Start().String(), err)
	}

	rightClause, err := v.buildNestedClause(expr.Right())
	if err != nil {
		return fmt.Errorf("chroma: process right operand of '%s' at %s: %w",
			expr.Operator().String(), expr.Start().String(), err)
	}

	switch expr.Operator() {
	case filter.OpAnd:
		v.result = v2.And(leftClause, rightClause)
	case filter.OpOr:
		v.result = v2.Or(leftClause, rightClause)
	default:
		return fmt.Errorf("chroma: unexpected logical operator '%s' at %s",
			expr.Operator().String(), expr.Start().String())
	}
	return nil
}

func (v *Visitor) visitEqualityExpr(expr *filter.BinaryExpr) error {
	fieldKey, err := v.extractFieldKey(expr.Left())
	if err != nil {
		return fmt.Errorf("chroma: extract field key from left operand of '%s' at %s: %w",
			expr.Operator().String(), expr.Start().String(), err)
	}

	fieldValue, err := v.extractFieldValue(expr.Right())
	if err != nil {
		return fmt.Errorf("chroma: extract value from right operand of '%s' at %s: %w",
			expr.Operator().String(), expr.Start().String(), err)
	}

	clause, err := v.buildEqualityClause(fieldKey, fieldValue, expr.Operator())
	if err != nil {
		return fmt.Errorf("chroma: build equality clause for '%s' at %s: %w",
			expr.Operator().String(), expr.Start().String(), err)
	}

	v.result = clause
	return nil
}

func (v *Visitor) buildEqualityClause(fieldKey string, fieldValue any, op filter.Operator) (v2.WhereClause, error) {
	isEq := op == filter.OpEqual

	switch val := fieldValue.(type) {
	case string:
		if isEq {
			return v2.EqString(fieldKey, val), nil
		}
		return v2.NotEqString(fieldKey, val), nil

	case chromaNumber:
		if val.isInteger {
			if isEq {
				return v2.EqInt(fieldKey, val.integer), nil
			}
			return v2.NotEqInt(fieldKey, val.integer), nil
		}
		if isEq {
			return v2.EqFloat(fieldKey, val.fraction), nil
		}
		return v2.NotEqFloat(fieldKey, val.fraction), nil

	case bool:
		if isEq {
			return v2.EqBool(fieldKey, val), nil
		}
		return v2.NotEqBool(fieldKey, val), nil

	default:
		return nil, fmt.Errorf("chroma: unsupported value type %T for equality condition", fieldValue)
	}
}

func (v *Visitor) visitOrderingExpr(expr *filter.BinaryExpr) error {
	fieldKey, err := v.extractFieldKey(expr.Left())
	if err != nil {
		return fmt.Errorf("chroma: extract field key from left operand of '%s' at %s: %w",
			expr.Operator().String(), expr.Start().String(), err)
	}

	fieldValue, err := v.extractFieldValue(expr.Right())
	if err != nil {
		return fmt.Errorf("chroma: extract value from right operand of '%s' at %s: %w",
			expr.Operator().String(), expr.Start().String(), err)
	}

	numericValue, ok := fieldValue.(chromaNumber)
	if !ok {
		return fmt.Errorf("chroma: cannot convert value to number for '%s' comparison at %s: expected number, got %T",
			expr.Operator().String(), expr.Start().String(), fieldValue)
	}

	var clause v2.WhereClause
	switch expr.Operator() {
	case filter.OpLess:
		if numericValue.isInteger {
			clause = v2.LtInt(fieldKey, numericValue.integer)
		} else {
			clause = v2.LtFloat(fieldKey, numericValue.fraction)
		}
	case filter.OpLessEqual:
		if numericValue.isInteger {
			clause = v2.LteInt(fieldKey, numericValue.integer)
		} else {
			clause = v2.LteFloat(fieldKey, numericValue.fraction)
		}
	case filter.OpGreater:
		if numericValue.isInteger {
			clause = v2.GtInt(fieldKey, numericValue.integer)
		} else {
			clause = v2.GtFloat(fieldKey, numericValue.fraction)
		}
	case filter.OpGreaterEqual:
		if numericValue.isInteger {
			clause = v2.GteInt(fieldKey, numericValue.integer)
		} else {
			clause = v2.GteFloat(fieldKey, numericValue.fraction)
		}
	default:
		return fmt.Errorf("chroma: unexpected ordering operator '%s' at %s",
			expr.Operator().String(), expr.Start().String())
	}

	v.result = clause
	return nil
}

func (v *Visitor) visitInExpr(expr *filter.BinaryExpr) error {
	fieldKey, err := v.extractFieldKey(expr.Left())
	if err != nil {
		return fmt.Errorf("chroma: extract field key from left operand of 'IN' at %s: %w",
			expr.Start().String(), err)
	}

	listLit, err := expr.List()
	if err != nil {
		return fmt.Errorf("chroma: %w", err)
	}

	if err = v.visitListLiteral(listLit); err != nil {
		return err
	}

	values, ok := v.currentFieldValue.([]any)
	if !ok || len(values) == 0 {
		return fmt.Errorf("chroma: extract list values for 'IN' operator at %s",
			expr.Start().String())
	}

	switch values[0].(type) {
	case string:
		strs := make([]string, 0, len(values))
		for _, val := range values {
			value, ok := val.(string)
			if !ok {
				return fmt.Errorf("chroma: mixed value type %T in string list", val)
			}
			strs = append(strs, value)
		}
		v.result = v2.InString(fieldKey, strs...)

	case chromaNumber:
		// Keep an all-integral list on Chroma's int path. A mixed numeric
		// list uses float32 only when every element can be represented safely.
		allInt := true
		for _, val := range values {
			number, ok := val.(chromaNumber)
			if !ok {
				return fmt.Errorf("chroma: mixed value type %T in numeric list", val)
			}
			if !number.isInteger {
				allInt = false
				break
			}
		}
		if allInt {
			ints := make([]int, 0, len(values))
			for _, val := range values {
				ints = append(ints, val.(chromaNumber).integer)
			}
			v.result = v2.InInt(fieldKey, ints...)
		} else {
			floats := make([]float32, 0, listLit.Len())
			for _, literal := range listLit.Literals() {
				value, err := literal.Float32()
				if err != nil {
					return fmt.Errorf("chroma: IN numeric value: %w", err)
				}
				floats = append(floats, value)
			}
			v.result = v2.InFloat(fieldKey, floats...)
		}

	case bool:
		bools := make([]bool, 0, len(values))
		for _, val := range values {
			value, ok := val.(bool)
			if !ok {
				return fmt.Errorf("chroma: mixed value type %T in bool list", val)
			}
			bools = append(bools, value)
		}
		v.result = v2.InBool(fieldKey, bools...)

	default:
		return fmt.Errorf("chroma: unsupported value type %T in 'IN' list at %s",
			values[0], expr.Start().String())
	}

	return nil
}

func (v *Visitor) buildNestedClause(expr filter.Expr) (v2.WhereClause, error) {
	switch node := expr.(type) {
	case *filter.BinaryExpr, *filter.UnaryExpr:
		nested := NewVisitor()
		if err := nested.visit(node); err != nil {
			return nil, err
		}
		return nested.result, nil
	default:
		return nil, fmt.Errorf("chroma: unsupported expression type %T for clause building", node)
	}
}

func (v *Visitor) extractFieldKey(expr filter.Expr) (string, error) {
	saved := v.currentFieldKey
	v.currentFieldKey = ""

	err := v.visit(expr)

	extracted := v.currentFieldKey
	v.currentFieldKey = saved

	if err != nil {
		return "", err
	}
	if extracted == "" {
		return "", fmt.Errorf("chroma: extract field key from %T expression", expr)
	}
	return extracted, nil
}

func (v *Visitor) extractFieldValue(expr filter.Expr) (any, error) {
	saved := v.currentFieldValue
	v.currentFieldValue = nil

	err := v.visit(expr)

	extracted := v.currentFieldValue
	v.currentFieldValue = saved

	if err != nil {
		return nil, err
	}
	if extracted == nil {
		return nil, fmt.Errorf("chroma: extract value from %T expression", expr)
	}
	return extracted, nil
}

func (v *Visitor) buildIndexedFieldKey(expr *filter.IndexExpr) (string, error) {
	var pathParts []string

	current := expr
	for {
		key, err := current.Index().Key()
		if err != nil {
			return "", fmt.Errorf("chroma: %w", err)
		}
		pathParts = append([]string{key}, pathParts...)

		switch left := current.Left().(type) {
		case *filter.IndexExpr:
			current = left
		case *filter.Ident:
			pathParts = append([]string{left.Name()}, pathParts...)
			return strings.Join(pathParts, "."), nil
		default:
			return "", fmt.Errorf("chroma: invalid left operand type %T in index expression", left)
		}
	}
}

func (v *Visitor) literalToValue(lit *filter.Literal) (any, error) {
	if lit.IsString() {
		return lit.AsString()
	}
	if lit.IsNumber() {
		integer, err := lit.IsInteger()
		if err != nil {
			return nil, err
		}
		if integer {
			value, intErr := lit.Int()
			if intErr != nil {
				return nil, intErr
			}
			return chromaNumber{integer: value, isInteger: true}, nil
		}
		value, err := lit.Float32()
		if err != nil {
			return nil, err
		}
		return chromaNumber{fraction: value}, nil
	}
	if lit.IsBool() {
		return lit.AsBool()
	}
	return nil, fmt.Errorf("chroma: unsupported literal type '%s'", lit.Kind())
}
