package qdrant

import (
	"errors"
	"fmt"

	"github.com/qdrant/go-client/qdrant"

	"github.com/Tangerg/scope/core/vectorstore/filter"
)

var _ filter.Visitor = (*Visitor)(nil)

// Visitor compiles Scope filter expressions into Qdrant conditions. A value is
// reusable: Visit replaces the previous result. Nested boolean operands compile
// in isolated visitors because Qdrant represents AND, OR, and NOT in separate
// condition lists; sharing temporary extraction state would change grouping.
// Unsupported or lossy provider mappings fail instead of approximating the
// source predicate.
type Visitor struct {
	err               error
	filter            *qdrant.Filter
	currentFieldValue any
	currentFieldKey   string
}

func NewVisitor() *Visitor {
	return &Visitor{
		filter: &qdrant.Filter{},
	}
}

// Result is nil until Visit succeeds and after any failed compilation.
func (v *Visitor) Result() *qdrant.Filter {
	if v.err != nil {
		return nil
	}
	return v.filter
}

// Visit replaces prior state and accepts only trees Qdrant can represent
// without changing their meaning.
func (v *Visitor) Visit(expr filter.Predicate) error {
	v.err = nil
	v.filter = &qdrant.Filter{}
	v.currentFieldValue = nil
	v.currentFieldKey = ""
	v.err = v.visit(expr)
	return v.err
}

func (v *Visitor) visit(expr filter.Expr) error {
	if expr == nil {
		return errors.New("cannot process nil expression")
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
		return fmt.Errorf("unsupported expression type %T", node)
	}
}

func (v *Visitor) visitBinaryExpr(expr *filter.BinaryExpr) error {
	if expr.Operator().IsNullOperator() {
		return v.visitNullTestExpr(expr)
	}
	return expr.Dispatch(filter.BinaryHandlers{
		Logical:    v.visitLogicalExpr,
		Comparison: v.visitComparisonExpr,
		In:         v.visitInExpr,
		Has:        v.visitHasExpr,
		Like:       v.visitLikeExpr,
	})
}

func (v *Visitor) visitComparisonExpr(expr *filter.BinaryExpr) error {
	if expr.Operator().IsEqualityOperator() {
		return v.visitEqualityExpr(expr)
	}
	return v.visitOrderingExpr(expr)
}

func (v *Visitor) visitNullTestExpr(expr *filter.BinaryExpr) error {
	fieldKey, err := v.extractFieldKey(expr.Left())
	if err != nil {
		return fmt.Errorf("extract field key from left operand of 'IS NULL' at %s: %w",
			expr.Start().String(), err)
	}

	v.filter.Must = append(v.filter.Must, qdrant.NewIsNull(fieldKey))
	return nil
}

func (v *Visitor) visitUnaryExpr(expr *filter.UnaryExpr) error {
	return expr.Dispatch(v.visitNotExpr)
}

func (v *Visitor) visitIdent(ident *filter.Ident) error {
	v.currentFieldKey = ident.Name()
	return nil
}

func (v *Visitor) visitLiteral(lit *filter.Literal) error {
	value, err := v.literalToValue(lit)
	if err != nil {
		return fmt.Errorf("convert literal at %s: %w",
			lit.Start().String(), err)
	}
	v.currentFieldValue = value
	return nil
}

func (v *Visitor) visitListLiteral(list *filter.ListLiteral) error {
	values := make([]any, 0, list.Len())
	for i, lit := range list.Literals() {
		value, err := v.literalToValue(lit)
		if err != nil {
			return fmt.Errorf("convert list element at index %d: %w", i, err)
		}
		values = append(values, value)
	}
	v.currentFieldValue = values
	return nil
}

func (v *Visitor) visitIndexExpr(expr *filter.IndexExpr) error {
	fieldKey, err := v.buildIndexedFieldKey(expr)
	if err != nil {
		return fmt.Errorf("build field path at %s: %w",
			expr.Start().String(), err)
	}
	v.currentFieldKey = fieldKey
	return nil
}

func (v *Visitor) visitLogicalExpr(expr *filter.BinaryExpr) error {
	leftCond, err := v.buildNestedCondition(expr.Left())
	if err != nil {
		return fmt.Errorf("process left operand of '%s' at %s: %w",
			expr.Operator().String(), expr.Start().String(), err)
	}

	rightCond, err := v.buildNestedCondition(expr.Right())
	if err != nil {
		return fmt.Errorf("process right operand of '%s' at %s: %w",
			expr.Operator().String(), expr.Start().String(), err)
	}

	switch expr.Operator() {
	case filter.OpAnd:
		v.filter.Must = append(v.filter.Must, leftCond, rightCond)
		return nil
	case filter.OpOr:
		v.filter.Should = append(v.filter.Should, leftCond, rightCond)
		return nil
	default:
		return fmt.Errorf("unexpected logical operator '%s' at %s",
			expr.Operator().String(), expr.Start().String())
	}
}

func (v *Visitor) visitNotExpr(expr *filter.UnaryExpr) error {
	cond, err := v.buildNestedCondition(expr.Right())
	if err != nil {
		return fmt.Errorf("process NOT operand at %s: %w",
			expr.Start().String(), err)
	}

	v.filter.MustNot = append(v.filter.MustNot, cond)
	return nil
}
