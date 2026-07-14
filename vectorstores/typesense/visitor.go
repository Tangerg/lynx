package typesense

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/Tangerg/lynx/core/vectorstore/filter"
	"github.com/Tangerg/lynx/vectorstores/internal/filterhelp"
)

// Visitor transforms AST filter expressions into Typesense `filter_by`
// syntax. The metadata field is a nested object on the collection
// schema (enabled via EnableNestedFields=true), so metadata keys are
// addressed under the configured `metadata.*` path.
//
// Output shape:
//
//	author == "Alice"         →  metadata.author:= Alice
//	year >= 2020              →  metadata.year:>= 2020
//	category IN ("a", "b")    →  metadata.category:= [a,b]
//	NOT (year >= 2020)        →  metadata.year:< 2020 (rewritten)
//	a == "x" AND b == "y"     →  (metadata.a:= x && metadata.b:= y)
//
// Typesense `filter_by` doesn't have a standalone NOT operator — the
// visitor rewrites `NOT (x op y)` into the operator's inverse.
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

func (v *Visitor) Visit(expr filter.Expr) error {
	v.err = v.visit(expr)
	return v.err
}

func (v *Visitor) visit(expr filter.Expr) error {
	if expr == nil {
		return errors.New("typesense: cannot process nil expression")
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
		return fmt.Errorf("typesense: unsupported root expression %T", node)
	}
}

func (v *Visitor) visitBinaryExpr(expr *filter.BinaryExpr) error {
	switch {
	case expr.Op.IsLogicalOperator():
		return v.visitLogicalExpr(expr)
	case expr.Op.Is(filter.OpIn):
		return v.visitInExpr(expr)
	case expr.Op.IsEqualityOperator() || expr.Op.IsOrderingOperator():
		return v.visitComparisonExpr(expr)
	default:
		return fmt.Errorf("typesense: unsupported binary operator '%s'", expr.Op.String())
	}
}

// visitUnaryExpr maps NOT (op) onto the operator's inverse because
// Typesense `filter_by` has no top-level NOT.
func (v *Visitor) visitUnaryExpr(expr *filter.UnaryExpr) error {
	if !expr.Op.Is(filter.OpNot) {
		return fmt.Errorf("typesense: unsupported unary '%s'", expr.Op.String())
	}
	bin, ok := expr.Right.(*filter.BinaryExpr)
	if !ok {
		return errors.New("typesense: NOT may only wrap a binary comparison")
	}
	inverted, err := invertBinary(bin)
	if err != nil {
		return err
	}
	return v.visit(inverted)
}

func invertBinary(expr *filter.BinaryExpr) (*filter.BinaryExpr, error) {
	clone := *expr
	switch expr.Op {
	case filter.OpEqual:
		clone.Op = filter.OpNotEqual
	case filter.OpNotEqual:
		clone.Op = filter.OpEqual
	case filter.OpLess:
		clone.Op = filter.OpGreaterEqual
	case filter.OpLessEqual:
		clone.Op = filter.OpGreater
	case filter.OpGreater:
		clone.Op = filter.OpLessEqual
	case filter.OpGreaterEqual:
		clone.Op = filter.OpLess
	default:
		return nil, fmt.Errorf("typesense: cannot invert operator '%s'", expr.Op.String())
	}
	return &clone, nil
}

func (v *Visitor) visitLogicalExpr(expr *filter.BinaryExpr) error {
	op := " && "
	if expr.Op.Is(filter.OpOr) {
		op = " || "
	}
	v.sql.WriteString("(")
	if err := v.visit(expr.Left); err != nil {
		return err
	}
	v.sql.WriteString(op)
	if err := v.visit(expr.Right); err != nil {
		return err
	}
	v.sql.WriteString(")")
	return nil
}

func (v *Visitor) visitComparisonExpr(expr *filter.BinaryExpr) error {
	field, err := v.fieldPath(expr.Left)
	if err != nil {
		return err
	}
	value, err := filterhelp.ExtractValue(expr.Right)
	if err != nil {
		return err
	}
	op, err := filterOpFor(expr.Op)
	if err != nil {
		return err
	}

	v.sql.WriteString(field)
	v.sql.WriteString(":")
	v.sql.WriteString(op)
	v.sql.WriteByte(' ')
	v.sql.WriteString(formatValue(value))
	return nil
}

func (v *Visitor) visitInExpr(expr *filter.BinaryExpr) error {
	field, err := v.fieldPath(expr.Left)
	if err != nil {
		return err
	}
	listLit, ok := expr.Right.(*filter.ListLiteral)
	if !ok {
		return errors.New("typesense: 'IN' requires a list on the right")
	}
	if len(listLit.Values) == 0 {
		return errors.New("typesense: 'IN' requires a non-empty list")
	}

	parts := make([]string, 0, len(listLit.Values))
	for _, lit := range listLit.Values {
		val, err := filterhelp.LiteralToValue(lit)
		if err != nil {
			return err
		}
		parts = append(parts, formatValue(val))
	}
	v.sql.WriteString(field)
	v.sql.WriteString(":= [")
	v.sql.WriteString(strings.Join(parts, ","))
	v.sql.WriteString("]")
	return nil
}

func (v *Visitor) fieldPath(expr filter.Expr) (string, error) {
	keys, err := filterhelp.CollectKeyPath(expr)
	if err != nil {
		return "", err
	}
	if len(keys) == 0 {
		return "", errors.New("empty key path")
	}
	joined := strings.Join(keys, ".")
	if v.metadataPrefix == "" {
		return joined, nil
	}
	return v.metadataPrefix + "." + joined, nil
}

func filterOpFor(kind filter.Operator) (string, error) {
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
		return "", fmt.Errorf("typesense: unexpected operator '%s'", kind.Name())
	}
}

func formatValue(v any) string {
	switch val := v.(type) {
	case string:
		if needsQuoting(val) {
			return "`" + strings.ReplaceAll(val, "`", "\\`") + "`"
		}
		return val
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

func needsQuoting(s string) bool {
	if s == "" {
		return true
	}
	for _, r := range s {
		switch r {
		case ' ', ',', '[', ']', '(', ')', '`', ':', '&', '|', '!', '<', '>', '=':
			return true
		}
	}
	return false
}
