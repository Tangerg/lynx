package vespa

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/Tangerg/scope/core/vectorstore/filter"
)

// Visitor transforms AST filter expressions into a Vespa YQL `where`
// clause. The metadata fields must be declared in the Vespa schema
// (sd file) — Vespa addresses them as flat top-level attributes.
//
// Output shape (when `metadataPrefix` is empty):
//
//	author == "Alice"        →  author contains "Alice"
//	year >= 2020             →  year >= 2020
//	tag IN ("a", "b")        →  tag in ("a", "b")
//	NOT (year >= 2020)       →  !(year >= 2020)
var _ filter.Visitor = (*Visitor)(nil)

type Visitor struct {
	err            error
	sql            strings.Builder
	metadataPrefix string
}

func NewVisitor(metadataPrefix string) *Visitor {
	return &Visitor{metadataPrefix: metadataPrefix}
}

func (v *Visitor) Result() string {
	if v.err != nil {
		return ""
	}
	return v.sql.String()
}

func (v *Visitor) Visit(expr filter.Predicate) error {
	v.err = nil
	v.sql.Reset()
	v.err = v.visit(expr)
	return v.err
}

func (v *Visitor) visit(expr filter.Expr) error {
	if expr == nil {
		return errors.New("vespa: cannot process nil expression")
	}
	if v.err != nil {
		return v.err
	}
	switch node := expr.(type) {
	case *filter.BinaryExpr:
		return v.visitBinaryExpr(node)
	case *filter.UnaryExpr:
		return v.visitUnaryExpr(node)
	default:
		return fmt.Errorf("vespa: unsupported root expression %T", node)
	}
}

func (v *Visitor) visitBinaryExpr(expr *filter.BinaryExpr) error {
	switch {
	case expr.Operator().IsLogicalOperator():
		return v.visitLogicalExpr(expr)
	case expr.Operator().Is(filter.OpIn):
		return v.visitInExpr(expr)
	case expr.Operator().Is(filter.OpHas):
		return v.visitHasExpr(expr)
	case expr.Operator().Is(filter.OpLike):
		return v.visitLikeExpr(expr)
	case expr.Operator().IsEqualityOperator() || expr.Operator().IsOrderingOperator():
		return v.visitComparisonExpr(expr)
	default:
		return fmt.Errorf("vespa: unsupported binary operator '%s'", expr.Operator().String())
	}
}

func (v *Visitor) visitHasExpr(expr *filter.BinaryExpr) error {
	field, err := v.fieldPath(expr)
	if err != nil {
		return err
	}
	value, err := expr.Value()
	if err != nil {
		return err
	}
	v.sql.WriteString(field)
	v.sql.WriteString(" contains ")
	v.sql.WriteString(yqlLiteral(value))
	return nil
}

func (v *Visitor) visitUnaryExpr(expr *filter.UnaryExpr) error {
	if !expr.Operator().Is(filter.OpNot) {
		return fmt.Errorf("vespa: unsupported unary '%s'", expr.Operator().String())
	}
	v.sql.WriteString("!(")
	if err := v.visit(expr.Right()); err != nil {
		return err
	}
	v.sql.WriteString(")")
	return nil
}

func (v *Visitor) visitLogicalExpr(expr *filter.BinaryExpr) error {
	op := " and "
	if expr.Operator().Is(filter.OpOr) {
		op = " or "
	}
	v.sql.WriteString("(")
	if err := v.visit(expr.Left()); err != nil {
		return err
	}
	v.sql.WriteString(op)
	if err := v.visit(expr.Right()); err != nil {
		return err
	}
	v.sql.WriteString(")")
	return nil
}

func (v *Visitor) visitComparisonExpr(expr *filter.BinaryExpr) error {
	field, err := v.fieldPath(expr)
	if err != nil {
		return err
	}
	value, err := expr.Value()
	if err != nil {
		return err
	}

	// String equality maps onto YQL `contains`; ordering / non-eq
	// numeric ops use the standard relational operators.
	if _, isString := value.(string); isString && expr.Operator().Is(filter.OpEqual) {
		v.sql.WriteString(field)
		v.sql.WriteString(" contains ")
		v.sql.WriteString(yqlLiteral(value))
		return nil
	}
	if _, isString := value.(string); isString && expr.Operator().Is(filter.OpNotEqual) {
		v.sql.WriteString("!(")
		v.sql.WriteString(field)
		v.sql.WriteString(" contains ")
		v.sql.WriteString(yqlLiteral(value))
		v.sql.WriteString(")")
		return nil
	}

	op, err := yqlOpFor(expr.Operator())
	if err != nil {
		return err
	}
	v.sql.WriteString(field)
	v.sql.WriteByte(' ')
	v.sql.WriteString(op)
	v.sql.WriteByte(' ')
	v.sql.WriteString(yqlLiteral(value))
	return nil
}

func (v *Visitor) visitInExpr(expr *filter.BinaryExpr) error {
	field, err := v.fieldPath(expr)
	if err != nil {
		return err
	}
	listLit, ok := expr.Right().(*filter.ListLiteral)
	if !ok {
		return errors.New("vespa: 'IN' requires a list on the right")
	}
	if listLit.Len() == 0 {
		return errors.New("vespa: 'IN' requires a non-empty list")
	}
	parts := make([]string, 0, listLit.Len())
	for _, lit := range listLit.Literals() {
		val, err := lit.Value()
		if err != nil {
			return err
		}
		parts = append(parts, yqlLiteral(val))
	}
	v.sql.WriteString(field)
	v.sql.WriteString(" in (")
	v.sql.WriteString(strings.Join(parts, ", "))
	v.sql.WriteByte(')')
	return nil
}

// visitLikeExpr maps SQL LIKE onto YQL `matches` (regex). `%` and
// `_` translate to `.*` / `.` respectively.
func (v *Visitor) visitLikeExpr(expr *filter.BinaryExpr) error {
	field, err := v.fieldPath(expr)
	if err != nil {
		return err
	}
	value, err := expr.Value()
	if err != nil {
		return err
	}
	pattern, ok := value.(string)
	if !ok {
		return fmt.Errorf("vespa: LIKE requires a string pattern, got %T", value)
	}
	var b strings.Builder
	for _, r := range pattern {
		switch r {
		case '%':
			b.WriteString(".*")
		case '_':
			b.WriteByte('.')
		case '.', '+', '*', '?', '(', ')', '[', ']', '{', '}', '|', '^', '$', '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}
	v.sql.WriteString(field)
	v.sql.WriteString(" matches ")
	v.sql.WriteString(yqlLiteral(b.String()))
	return nil
}

func (v *Visitor) fieldPath(expr *filter.BinaryExpr) (string, error) {
	keys, err := expr.Path()
	if err != nil {
		return "", err
	}
	if len(keys) == 0 {
		return "", errors.New("vespa: empty key path")
	}
	joined := strings.Join(keys, ".")
	if v.metadataPrefix == "" {
		return joined, nil
	}
	return v.metadataPrefix + "." + joined, nil
}

func yqlOpFor(kind filter.Operator) (string, error) {
	switch kind {
	case filter.OpEqual:
		return "=", nil
	case filter.OpNotEqual:
		return "!=", nil
	case filter.OpLess:
		return "<", nil
	case filter.OpLessEqual:
		return "<=", nil
	case filter.OpGreater:
		return ">", nil
	case filter.OpGreaterEqual:
		return ">=", nil
	default:
		return "", fmt.Errorf("vespa: unexpected operator '%s'", kind.Name())
	}
}

func yqlLiteral(v any) string {
	switch val := v.(type) {
	case string:
		return `"` + strings.ReplaceAll(val, `"`, `\"`) + `"`
	case bool:
		if val {
			return "true"
		}
		return "false"
	case int64:
		return strconv.FormatInt(val, 10)
	case float64:
		if float64(int64(val)) == val {
			return strconv.FormatInt(int64(val), 10)
		}
		return strconv.FormatFloat(val, 'f', -1, 64)
	default:
		return fmt.Sprint(val)
	}
}
