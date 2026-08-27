package milvus

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/Tangerg/scope/core/vectorstore/filter"
)

var _ filter.Visitor = (*Visitor)(nil)

// Visitor compiles Scope filter expressions into Milvus's string expression
// language. A value is reusable: Visit resets the previous result before
// compiling the complete tree. Nested boolean branches use isolated visitors
// so temporary field and literal state cannot leak between operands; numeric
// literals retain their canonical text instead of passing through float64.
type Visitor struct {
	err               error
	result            string
	currentFieldKey   string
	currentFieldValue string
}

func NewVisitor() *Visitor {
	return &Visitor{}
}

// Result is empty until Visit succeeds and after any failed compilation.
func (v *Visitor) Result() string {
	if v.err != nil {
		return ""
	}
	return v.result
}

// Visit replaces prior state and accepts only trees Milvus can represent
// without changing their meaning.
func (v *Visitor) Visit(expr filter.Predicate) error {
	v.err = nil
	v.result = ""
	v.currentFieldKey = ""
	v.currentFieldValue = ""
	v.err = v.visit(expr)
	return v.err
}

func (v *Visitor) visit(expr filter.Expr) error {
	if expr == nil {
		return errors.New("milvus: cannot process nil expression")
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
		return fmt.Errorf("milvus: unsupported expression type %T", node)
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
	fieldKey, err := v.extractFieldKey(expr.Left())
	if err != nil {
		return fmt.Errorf("milvus: extract collection field at %s: %w", expr.Start().String(), err)
	}
	fieldValue, err := v.extractFieldValue(expr.Right())
	if err != nil {
		return fmt.Errorf("milvus: extract collection member at %s: %w", expr.Start().String(), err)
	}
	v.result = fmt.Sprintf("ARRAY_CONTAINS(%s, %s)", fieldKey, fieldValue)
	return nil
}

func (v *Visitor) visitComparisonExpr(expr *filter.BinaryExpr) error {
	if expr.Operator().IsEqualityOperator() {
		return v.visitEqualityExpr(expr)
	}
	return v.visitOrderingExpr(expr)
}

func (v *Visitor) visitUnaryExpr(expr *filter.UnaryExpr) error {
	return expr.Dispatch(v.visitNotExpr)
}

func (v *Visitor) visitIdent(ident *filter.Ident) error {
	v.currentFieldKey = ident.Name()
	return nil
}

func (v *Visitor) visitLiteral(lit *filter.Literal) error {
	value, err := v.literalToString(lit)
	if err != nil {
		return err
	}
	v.currentFieldValue = value
	return nil
}

func (v *Visitor) visitListLiteral(list *filter.ListLiteral) error {
	parts := make([]string, 0, list.Len())

	for i, lit := range list.Literals() {
		s, err := v.literalToString(lit)
		if err != nil {
			return fmt.Errorf("milvus: convert list element at index %d: %w", i, err)
		}
		parts = append(parts, s)
	}

	v.currentFieldValue = "[" + strings.Join(parts, ", ") + "]"
	return nil
}

func (v *Visitor) visitIndexExpr(expr *filter.IndexExpr) error {
	fieldKey, err := v.buildIndexedFieldKey(expr)
	if err != nil {
		return fmt.Errorf("milvus: build field path at %s: %w",
			expr.Start().String(), err)
	}
	v.currentFieldKey = fieldKey
	return nil
}

func (v *Visitor) visitLogicalExpr(expr *filter.BinaryExpr) error {
	left, err := v.buildNestedExpr(expr.Left())
	if err != nil {
		return fmt.Errorf("milvus: process left operand of '%s' at %s: %w",
			expr.Operator().String(), expr.Start().String(), err)
	}

	right, err := v.buildNestedExpr(expr.Right())
	if err != nil {
		return fmt.Errorf("milvus: process right operand of '%s' at %s: %w",
			expr.Operator().String(), expr.Start().String(), err)
	}

	switch expr.Operator() {
	case filter.OpAnd:
		v.result = fmt.Sprintf("(%s) and (%s)", left, right)
	case filter.OpOr:
		v.result = fmt.Sprintf("(%s) or (%s)", left, right)
	default:
		return fmt.Errorf("milvus: unexpected logical operator '%s' at %s",
			expr.Operator().String(), expr.Start().String())
	}

	return nil
}

func (v *Visitor) visitNotExpr(expr *filter.UnaryExpr) error {
	operand, err := v.buildNestedExpr(expr.Right())
	if err != nil {
		return fmt.Errorf("milvus: process NOT operand at %s: %w",
			expr.Start().String(), err)
	}

	v.result = fmt.Sprintf("not (%s)", operand)
	return nil
}

func (v *Visitor) visitEqualityExpr(expr *filter.BinaryExpr) error {
	fieldKey, err := v.extractFieldKey(expr.Left())
	if err != nil {
		return fmt.Errorf("milvus: extract field key from '%s' at %s: %w",
			expr.Operator().String(), expr.Start().String(), err)
	}

	fieldValue, err := v.extractFieldValue(expr.Right())
	if err != nil {
		return fmt.Errorf("milvus: extract value from '%s' at %s: %w",
			expr.Operator().String(), expr.Start().String(), err)
	}

	switch expr.Operator() {
	case filter.OpEqual:
		v.result = fmt.Sprintf("%s == %s", fieldKey, fieldValue)
	case filter.OpNotEqual:
		v.result = fmt.Sprintf("%s != %s", fieldKey, fieldValue)
	default:
		return fmt.Errorf("milvus: unexpected equality operator '%s' at %s",
			expr.Operator().String(), expr.Start().String())
	}

	return nil
}

func (v *Visitor) visitOrderingExpr(expr *filter.BinaryExpr) error {
	fieldKey, err := v.extractFieldKey(expr.Left())
	if err != nil {
		return fmt.Errorf("milvus: extract field key from '%s' at %s: %w",
			expr.Operator().String(), expr.Start().String(), err)
	}

	fieldValue, err := v.extractFieldValue(expr.Right())
	if err != nil {
		return fmt.Errorf("milvus: extract value from '%s' at %s: %w",
			expr.Operator().String(), expr.Start().String(), err)
	}

	switch expr.Operator() {
	case filter.OpLess:
		v.result = fmt.Sprintf("%s < %s", fieldKey, fieldValue)
	case filter.OpLessEqual:
		v.result = fmt.Sprintf("%s <= %s", fieldKey, fieldValue)
	case filter.OpGreater:
		v.result = fmt.Sprintf("%s > %s", fieldKey, fieldValue)
	case filter.OpGreaterEqual:
		v.result = fmt.Sprintf("%s >= %s", fieldKey, fieldValue)
	default:
		return fmt.Errorf("milvus: unexpected ordering operator '%s' at %s",
			expr.Operator().String(), expr.Start().String())
	}

	return nil
}

func (v *Visitor) visitInExpr(expr *filter.BinaryExpr) error {
	fieldKey, err := v.extractFieldKey(expr.Left())
	if err != nil {
		return fmt.Errorf("milvus: extract field key from 'IN' at %s: %w",
			expr.Start().String(), err)
	}

	listLit, err := expr.List()
	if err != nil {
		return fmt.Errorf("milvus: %w", err)
	}

	if err = v.visitListLiteral(listLit); err != nil {
		return err
	}

	v.result = fmt.Sprintf("%s in %s", fieldKey, v.currentFieldValue)
	return nil
}

func (v *Visitor) visitLikeExpr(expr *filter.BinaryExpr) error {
	fieldKey, err := v.extractFieldKey(expr.Left())
	if err != nil {
		return fmt.Errorf("milvus: extract field key from 'LIKE' at %s: %w",
			expr.Start().String(), err)
	}

	lit, ok := expr.Right().(*filter.Literal)
	if !ok {
		return fmt.Errorf("milvus: 'LIKE' operator requires a string literal on the right side at %s, got %T",
			expr.Start().String(), expr.Right())
	}
	if !lit.IsString() {
		return fmt.Errorf("milvus: 'LIKE' operator requires a string pattern at %s, got %s",
			expr.Start().String(), lit.Kind())
	}

	if err = v.visitLiteral(lit); err != nil {
		return err
	}

	v.result = fmt.Sprintf("%s like %s", fieldKey, v.currentFieldValue)
	return nil
}

func (v *Visitor) buildNestedExpr(expr filter.Expr) (string, error) {
	nested := NewVisitor()
	if err := nested.visit(expr); err != nil {
		return "", err
	}
	if nested.result != "" {
		return nested.result, nil
	}
	if nested.currentFieldKey != "" {
		return nested.currentFieldKey, nil
	}
	if nested.currentFieldValue != "" {
		return nested.currentFieldValue, nil
	}
	return "", fmt.Errorf("milvus: unsupported expression type %T for nested expression", expr)
}

func (v *Visitor) extractFieldKey(expr filter.Expr) (string, error) {
	savedKey := v.currentFieldKey
	v.currentFieldKey = ""

	err := v.visit(expr)

	extracted := v.currentFieldKey
	v.currentFieldKey = savedKey

	if err != nil {
		return "", err
	}
	if extracted == "" {
		return "", fmt.Errorf("milvus: extract field key from %T expression", expr)
	}

	return extracted, nil
}

func (v *Visitor) extractFieldValue(expr filter.Expr) (string, error) {
	savedValue := v.currentFieldValue
	v.currentFieldValue = ""

	err := v.visit(expr)

	extracted := v.currentFieldValue
	v.currentFieldValue = savedValue

	if err != nil {
		return "", err
	}
	if extracted == "" {
		return "", fmt.Errorf("milvus: extract value from %T expression", expr)
	}

	return extracted, nil
}

func (v *Visitor) buildIndexedFieldKey(expr *filter.IndexExpr) (string, error) {
	var parts []string

	current := expr
	for {
		key, err := current.Index().Key()
		if err != nil {
			return "", fmt.Errorf("milvus: %w", err)
		}
		if current.Index().IsString() {
			key = strconv.Quote(key)
		}
		parts = append([]string{"[" + key + "]"}, parts...)

		switch left := current.Left().(type) {
		case *filter.IndexExpr:
			current = left
		case *filter.Ident:
			return left.Name() + strings.Join(parts, ""), nil
		default:
			return "", fmt.Errorf("milvus: invalid left operand type %T in index expression, expected identifier or index",
				left)
		}
	}
}

func (v *Visitor) literalToString(lit *filter.Literal) (string, error) {
	if lit.IsString() {
		s, err := lit.AsString()
		if err != nil {
			return "", fmt.Errorf("milvus: convert string literal at %s: %w",
				lit.Start().String(), err)
		}
		return fmt.Sprintf(`"%s"`, strings.ReplaceAll(s, `"`, `\"`)), nil
	}

	if lit.IsNumber() {
		n, err := lit.NumberText()
		if err != nil {
			return "", fmt.Errorf("milvus: convert number literal at %s: %w",
				lit.Start().String(), err)
		}
		return n, nil
	}

	if lit.IsBool() {
		b, err := lit.AsBool()
		if err != nil {
			return "", fmt.Errorf("milvus: convert bool literal at %s: %w",
				lit.Start().String(), err)
		}
		if b {
			return "True", nil
		}
		return "False", nil
	}

	return "", fmt.Errorf("milvus: unsupported literal type '%s' at %s",
		lit.Kind(), lit.Start().String())
}
