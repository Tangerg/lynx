package azureaisearch

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/Tangerg/lynx/core/vectorstore/filter"
	"github.com/Tangerg/lynx/vectorstores/internal/filterhelp"
)

// Visitor transforms AST filter expressions into Azure AI Search OData
// `$filter` syntax. Metadata is treated as flat top-level fields on
// the indexed document — Azure AI Search doesn't support nested
// metadata in $filter expressions, so each filterable key must exist
// as its own top-level field on the index schema.
//
// Output shape:
//
//	author == "Alice"          →  author eq 'Alice'
//	year >= 2020               →  year ge 2020
//	category IN ("a", "b")     →  search.in(category, 'a,b', ',')
//	NOT (year >= 2020)         →  not (year ge 2020)
type Visitor struct {
	err error
	sql strings.Builder
}

func NewVisitor() *Visitor { return &Visitor{} }

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
		return errors.New("azureaisearch: cannot process nil expression")
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
		return fmt.Errorf("azureaisearch: unsupported root expression %T", node)
	}
}

func (v *Visitor) visitBinaryExpr(expr *filter.BinaryExpr) error {
	switch {
	case expr.Op.IsLogicalOperator():
		return v.visitLogicalExpr(expr)
	case expr.Op.Is(filter.OpIn):
		return v.visitInExpr(expr)
	case expr.Op.Is(filter.OpLike):
		return v.visitLikeExpr(expr)
	case expr.Op.IsEqualityOperator() || expr.Op.IsOrderingOperator():
		return v.visitComparisonExpr(expr)
	default:
		return fmt.Errorf("azureaisearch: unsupported binary operator '%s'", expr.Op.String())
	}
}

func (v *Visitor) visitUnaryExpr(expr *filter.UnaryExpr) error {
	if !expr.Op.Is(filter.OpNot) {
		return fmt.Errorf("azureaisearch: unsupported unary '%s'", expr.Op.String())
	}
	v.sql.WriteString("not (")
	if err := v.visit(expr.Right); err != nil {
		return err
	}
	v.sql.WriteString(")")
	return nil
}

func (v *Visitor) visitLogicalExpr(expr *filter.BinaryExpr) error {
	op := " and "
	if expr.Op.Is(filter.OpOr) {
		op = " or "
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
	field, err := fieldName(expr.Left)
	if err != nil {
		return err
	}
	value, err := filterhelp.ExtractValue(expr.Right)
	if err != nil {
		return err
	}
	op, err := odataOpFor(expr.Op)
	if err != nil {
		return err
	}
	v.sql.WriteString(field)
	v.sql.WriteByte(' ')
	v.sql.WriteString(op)
	v.sql.WriteByte(' ')
	v.sql.WriteString(odataLiteral(value))
	return nil
}

func (v *Visitor) visitInExpr(expr *filter.BinaryExpr) error {
	field, err := fieldName(expr.Left)
	if err != nil {
		return err
	}
	listLit, ok := expr.Right.(*filter.ListLiteral)
	if !ok {
		return errors.New("azureaisearch: 'IN' requires a list on the right")
	}
	if len(listLit.Values) == 0 {
		return errors.New("azureaisearch: 'IN' requires a non-empty list")
	}

	parts := make([]string, 0, len(listLit.Values))
	for _, lit := range listLit.Values {
		val, err := filterhelp.LiteralToValue(lit)
		if err != nil {
			return err
		}
		s := fmt.Sprint(val)
		// search.in's third argument is the separator — pick something
		// that's unlikely to appear in tag values.
		parts = append(parts, strings.ReplaceAll(s, "|", `\|`))
	}
	v.sql.WriteString("search.in(")
	v.sql.WriteString(field)
	v.sql.WriteString(", '")
	v.sql.WriteString(strings.ReplaceAll(strings.Join(parts, "|"), "'", "''"))
	v.sql.WriteString("', '|')")
	return nil
}

// visitLikeExpr maps LIKE onto Azure AI Search's wildcard syntax via
// search.ismatch. The full Lucene wildcard syntax `*` / `?` is what
// AI Search expects; SQL's `%` / `_` are forwarded accordingly.
func (v *Visitor) visitLikeExpr(expr *filter.BinaryExpr) error {
	field, err := fieldName(expr.Left)
	if err != nil {
		return err
	}
	value, err := filterhelp.ExtractValue(expr.Right)
	if err != nil {
		return err
	}
	pattern, ok := value.(string)
	if !ok {
		return fmt.Errorf("azureaisearch: LIKE requires a string pattern, got %T", value)
	}
	var b strings.Builder
	b.Grow(len(pattern))
	for _, r := range pattern {
		switch r {
		case '%':
			b.WriteByte('*')
		case '_':
			b.WriteByte('?')
		case '\'':
			b.WriteString("''")
		default:
			b.WriteRune(r)
		}
	}
	v.sql.WriteString("search.ismatch('")
	v.sql.WriteString(b.String())
	v.sql.WriteString("', '")
	v.sql.WriteString(field)
	v.sql.WriteString("')")
	return nil
}

// fieldName extracts the (flat) field identifier — Azure AI Search
// doesn't support nested-property paths in $filter, so the left
// operand must reduce to a single bare identifier.
func fieldName(expr filter.Expr) (string, error) {
	switch node := expr.(type) {
	case *filter.Ident:
		return node.Value, nil
	case *filter.IndexExpr:
		// metadata["author"] → "author" — drop the wrapper.
		keys, err := filterhelp.CollectKeyPath(node)
		if err != nil {
			return "", err
		}
		if len(keys) != 1 {
			return "", fmt.Errorf("azureaisearch: nested paths are not supported; got %s",
				strings.Join(keys, "."))
		}
		return keys[0], nil
	default:
		return "", fmt.Errorf("unsupported left operand %T", node)
	}
}

func odataOpFor(kind filter.Operator) (string, error) {
	switch kind {
	case filter.OpEqual:
		return "eq", nil
	case filter.OpNotEqual:
		return "ne", nil
	case filter.OpLess:
		return "lt", nil
	case filter.OpLessEqual:
		return "le", nil
	case filter.OpGreater:
		return "gt", nil
	case filter.OpGreaterEqual:
		return "ge", nil
	default:
		return "", fmt.Errorf("azureaisearch: unexpected operator '%s'", kind.Name())
	}
}

func odataLiteral(v any) string {
	switch val := v.(type) {
	case string:
		return "'" + strings.ReplaceAll(val, "'", "''") + "'"
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
