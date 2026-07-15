package redis

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cast"

	"github.com/Tangerg/lynx/core/vectorstore/filter"
	"github.com/Tangerg/lynx/vectorstores/internal/filterhelp"
)

// Visitor transforms AST filter expressions into a RediSearch query
// fragment. The output is meant to be wrapped in parentheses and
// composed with a KNN vector tail by [Store].
//
// Output shape:
//
//	category == "tech"              →  @category:{tech}
//	category IN ("tech","ai")       →  @category:{tech|ai}
//	year >= 2020                    →  @year:[2020 +inf]
//	year == 2024                    →  @year:[2024 2024]
//	title LIKE "intro"              →  @title:(intro)
//	a == "x" AND b == "y"           →  (@a:{x} @b:{y})
//	a == "x" OR b == "y"            →  (@a:{x} | @b:{y})
//	NOT (a == "x")                  →  -(@a:{x})
//
// Field types come from [Store.fields] — keyed by metadata-field name —
// so the same operator dispatches to TAG / NUMERIC / TEXT syntax
// depending on the declared field kind.
var _ filter.Visitor = (*Visitor)(nil)

type Visitor struct {
	err    error
	sql    strings.Builder
	fields map[string]MetadataFieldType
}

func NewVisitor(fields map[string]MetadataFieldType) *Visitor {
	return &Visitor{fields: fields}
}

func (v *Visitor) Result() string {
	if v.err != nil {
		return ""
	}
	return v.sql.String()
}

func (v *Visitor) Visit(expr filter.Predicate) error {
	v.err = v.visit(expr)
	return v.err
}

func (v *Visitor) visit(expr filter.Expr) error {
	if expr == nil {
		return errors.New("redis: cannot process nil expression")
	}
	if v.err != nil {
		return v.err
	}

	switch node := expr.(type) {
	case *filter.BinaryExpr:
		return filterhelp.DispatchBinaryErr(node,
			v.visitLogicalExpr,
			v.visitComparisonExpr,
			v.visitInExpr,
			v.visitTextFieldExpr,
		)
	case *filter.UnaryExpr:
		return filterhelp.DispatchUnaryErr(node, v.visitNotExpr)
	default:
		return fmt.Errorf("redis: unsupported root expression %T", node)
	}
}

func (v *Visitor) visitNotExpr(expr *filter.UnaryExpr) error {
	v.sql.WriteString("-(")
	if err := v.visit(expr.Right); err != nil {
		return err
	}
	v.sql.WriteString(")")
	return nil
}

// visitLogicalExpr uses RediSearch's space separator for AND and the
// pipe (` | `) for OR — not the verbatim "AND"/"OR" strings other
// vendors emit. We don't call filterhelp.LogicalOpString here because
// of that mapping difference.
func (v *Visitor) visitLogicalExpr(expr *filter.BinaryExpr) error {
	sep := " "
	if expr.Op.Is(filter.OpOr) {
		sep = " | "
	}
	v.sql.WriteString("(")
	if err := v.visit(expr.Left); err != nil {
		return err
	}
	v.sql.WriteString(sep)
	if err := v.visit(expr.Right); err != nil {
		return err
	}
	v.sql.WriteString(")")
	return nil
}

// visitComparisonExpr dispatches based on the declared field type:
//   - TAG fields handle == and != (via NOT wrap)
//   - NUMERIC fields handle the full ordering set
//   - TEXT fields only support equality via the (value) syntax
func (v *Visitor) visitComparisonExpr(expr *filter.BinaryExpr) error {
	field, kind, err := v.resolveFieldKey(expr.Left)
	if err != nil {
		return fmt.Errorf("redis: %w (at %s)", err, expr.Start().String())
	}

	value, err := filterhelp.ExtractValue(expr.Right)
	if err != nil {
		return fmt.Errorf("redis: %w (at %s)", err, expr.Start().String())
	}

	op := expr.Op
	negate := op.Is(filter.OpNotEqual)
	if negate {
		op = filter.OpEqual
	}

	if negate {
		v.sql.WriteString("-")
	}

	switch kind {
	case FieldTag:
		if !op.Is(filter.OpEqual) {
			return fmt.Errorf("redis: TAG field '%s' only supports == / != / IN (got '%s')",
				field, expr.Op.String())
		}
		v.sql.WriteString("@")
		v.sql.WriteString(field)
		v.sql.WriteString(":{")
		v.sql.WriteString(escapeTagValue(fmt.Sprint(value)))
		v.sql.WriteString("}")

	case FieldNumeric:
		num, err := cast.ToFloat64E(value)
		if err != nil {
			return fmt.Errorf("redis: NUMERIC field '%s' requires a number value, got %T",
				field, value)
		}
		low, high := numericRange(op, num)
		v.sql.WriteString("@")
		v.sql.WriteString(field)
		v.sql.WriteString(":[")
		v.sql.WriteString(low)
		v.sql.WriteString(" ")
		v.sql.WriteString(high)
		v.sql.WriteString("]")

	case FieldText:
		if !op.Is(filter.OpEqual) {
			return fmt.Errorf("redis: TEXT field '%s' only supports == / != / LIKE (got '%s')",
				field, expr.Op.String())
		}
		v.sql.WriteString("@")
		v.sql.WriteString(field)
		v.sql.WriteString(":(")
		v.sql.WriteString(escapeTextValue(fmt.Sprint(value)))
		v.sql.WriteString(")")

	default:
		return fmt.Errorf("redis: unsupported field type %d for '%s'", kind, field)
	}
	return nil
}

func (v *Visitor) visitTextFieldExpr(expr *filter.BinaryExpr) error {
	field, kind, err := v.resolveFieldKey(expr.Left)
	if err != nil {
		return fmt.Errorf("redis: %w (at %s)", err, expr.Start().String())
	}
	if kind != FieldText {
		return fmt.Errorf("redis: LIKE only supports TEXT fields, got %d for '%s'",
			kind, field)
	}

	pattern, err := filterhelp.RequireStringPatternOnRight(expr)
	if err != nil {
		return fmt.Errorf("redis: %w", err)
	}

	v.sql.WriteString("@")
	v.sql.WriteString(field)
	v.sql.WriteString(":(")
	v.sql.WriteString(escapeTextValue(pattern))
	v.sql.WriteString(")")
	return nil
}

func (v *Visitor) visitInExpr(expr *filter.BinaryExpr) error {
	field, kind, err := v.resolveFieldKey(expr.Left)
	if err != nil {
		return fmt.Errorf("redis: %w (at %s)", err, expr.Start().String())
	}

	listLit, err := filterhelp.RequireListLiteral(expr)
	if err != nil {
		return fmt.Errorf("redis: %w", err)
	}

	switch kind {
	case FieldTag:
		parts := make([]string, 0, len(listLit.Values))
		for _, lit := range listLit.Values {
			val, err := literalToString(lit)
			if err != nil {
				return fmt.Errorf("redis: %w (at %s)", err, expr.Start().String())
			}
			parts = append(parts, escapeTagValue(val))
		}
		v.sql.WriteString("@")
		v.sql.WriteString(field)
		v.sql.WriteString(":{")
		v.sql.WriteString(strings.Join(parts, "|"))
		v.sql.WriteString("}")

	case FieldText:
		parts := make([]string, 0, len(listLit.Values))
		for _, lit := range listLit.Values {
			val, err := literalToString(lit)
			if err != nil {
				return fmt.Errorf("redis: %w (at %s)", err, expr.Start().String())
			}
			parts = append(parts, escapeTextValue(val))
		}
		v.sql.WriteString("@")
		v.sql.WriteString(field)
		v.sql.WriteString(":(")
		v.sql.WriteString(strings.Join(parts, "|"))
		v.sql.WriteString(")")

	default:
		return fmt.Errorf("redis: IN is not supported on field type %d for '%s'",
			kind, field)
	}
	return nil
}

// resolveFieldKey extracts the metadata field name from the left side
// of a comparison and looks up its declared kind. Returns the bare
// identifier (no @prefix) so the caller controls the surrounding
// syntax.
func (v *Visitor) resolveFieldKey(expr filter.Expr) (string, MetadataFieldType, error) {
	var field string
	switch node := expr.(type) {
	case *filter.Ident:
		field = node.Value
	case *filter.IndexExpr:
		// metadata["author"] / metadata["a"]["b"]: collapse the path
		// with dots — RediSearch fields are flat so deep paths must
		// be declared as dotted field names.
		parts, err := flattenIndexExpr(node)
		if err != nil {
			return "", 0, err
		}
		field = strings.Join(parts, ".")
	default:
		return "", 0, fmt.Errorf("redis: unsupported left operand %T", node)
	}

	kind, ok := v.fields[field]
	if !ok {
		return "", 0, fmt.Errorf("redis: filter references undeclared metadata field %q", field)
	}
	return field, kind, nil
}

// flattenIndexExpr collapses an [filter.IndexExpr] into a dotted path.
func flattenIndexExpr(expr *filter.IndexExpr) ([]string, error) {
	var keys []string
	current := expr
	for {
		key, err := filterhelp.LiteralAsKey(current.Index)
		if err != nil {
			return nil, err
		}
		keys = append([]string{key}, keys...)

		switch inner := current.Left.(type) {
		case *filter.IndexExpr:
			current = inner
		case *filter.Ident:
			keys = append([]string{inner.Value}, keys...)
			return keys, nil
		default:
			return nil, fmt.Errorf("redis: unsupported index base %T", inner)
		}
	}
}

// numericRange builds the RediSearch [low high] bounds for an ordering
// operator. Inclusive bounds use the bare number; exclusive bounds use
// "(<n>".
func numericRange(op filter.Operator, value float64) (string, string) {
	v := formatNumber(value)
	switch op {
	case filter.OpEqual:
		return v, v
	case filter.OpGreater:
		return "(" + v, "+inf"
	case filter.OpGreaterEqual:
		return v, "+inf"
	case filter.OpLess:
		return "-inf", "(" + v
	case filter.OpLessEqual:
		return "-inf", v
	default:
		// Unreachable — visitComparisonExpr filtered the operator.
		return "-inf", "+inf"
	}
}

func formatNumber(f float64) string {
	if float64(int64(f)) == f {
		return strconv.FormatInt(int64(f), 10)
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}

func literalToString(lit *filter.Literal) (string, error) {
	switch {
	case lit.IsString():
		return lit.AsString()
	case lit.IsNumber():
		n, err := lit.AsNumber()
		if err != nil {
			return "", err
		}
		return formatNumber(n), nil
	case lit.IsBool():
		b, err := lit.AsBool()
		if err != nil {
			return "", err
		}
		return strconv.FormatBool(b), nil
	default:
		return "", fmt.Errorf("redis: unsupported literal kind %s", lit.Kind)
	}
}

func escapeTagValue(value string) string {
	var b strings.Builder
	b.Grow(len(value))
	for _, r := range value {
		switch r {
		case '\\', '$', '|', '{', '}', '(', ')', '[', ']', '-', '\'', ',', ' ', '.':
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

func escapeTextValue(value string) string {
	var b strings.Builder
	b.Grow(len(value))
	for _, r := range value {
		switch r {
		case '\\', '$', '|', '{', '}', '(', ')', '[', ']', '"', '\'':
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}
