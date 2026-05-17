package redis

import (
	"fmt"
	"strings"
	"strconv"

	"github.com/Tangerg/lynx/core/vectorstore/filter/ast"
	"github.com/Tangerg/lynx/vectorstores/internal/filterhelp"
	"github.com/Tangerg/lynx/core/vectorstore/filter/token"
)

var _ ast.Visitor = (*Visitor)(nil)

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

func (v *Visitor) Error() error { return v.err }

func (v *Visitor) Visit(expr ast.Expr) ast.Visitor {
	v.err = v.visit(expr)
	return nil
}

func (v *Visitor) visit(expr ast.Expr) error {
	if expr == nil {
		return fmt.Errorf("redis: cannot process nil expression")
	}
	if v.err != nil {
		return v.err
	}

	switch node := expr.(type) {
	case *ast.BinaryExpr:
		return v.visitBinaryExpr(node)
	case *ast.UnaryExpr:
		return v.visitUnaryExpr(node)
	default:
		return fmt.Errorf("redis: unsupported root expression %T", node)
	}
}

func (v *Visitor) visitBinaryExpr(expr *ast.BinaryExpr) error {
	switch {
	case expr.Op.Kind.IsLogicalOperator():
		return v.visitLogicalExpr(expr)
	case expr.Op.Kind.Is(token.IN):
		return v.visitInExpr(expr)
	case expr.Op.Kind.Is(token.LIKE):
		return v.visitTextFieldExpr(expr)
	case expr.Op.Kind.IsEqualityOperator() || expr.Op.Kind.IsOrderingOperator():
		return v.visitComparisonExpr(expr)
	default:
		return fmt.Errorf("redis: unsupported binary operator '%s' at %s",
			expr.Op.Literal, expr.Start().String())
	}
}

func (v *Visitor) visitUnaryExpr(expr *ast.UnaryExpr) error {
	if !expr.Op.Kind.Is(token.NOT) {
		return fmt.Errorf("redis: unsupported unary operator '%s' at %s",
			expr.Op.Literal, expr.Start().String())
	}
	v.sql.WriteString("-(")
	if err := v.visit(expr.Right); err != nil {
		return err
	}
	v.sql.WriteString(")")
	return nil
}

func (v *Visitor) visitLogicalExpr(expr *ast.BinaryExpr) error {
	sep := " "
	if expr.Op.Kind.Is(token.OR) {
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
func (v *Visitor) visitComparisonExpr(expr *ast.BinaryExpr) error {
	field, kind, err := v.resolveFieldKey(expr.Left)
	if err != nil {
		return fmt.Errorf("redis: %w (at %s)", err, expr.Start().String())
	}

	value, err := filterhelp.ExtractValue(expr.Right)
	if err != nil {
		return fmt.Errorf("redis: %w (at %s)", err, expr.Start().String())
	}

	op := expr.Op.Kind
	negate := op.Is(token.NE)
	if negate {
		op = token.EQ
	}

	if negate {
		v.sql.WriteString("-")
	}

	switch kind {
	case FieldTag:
		if !op.Is(token.EQ) {
			return fmt.Errorf("redis: TAG field '%s' only supports == / != / IN (got '%s')",
				field, expr.Op.Literal)
		}
		v.sql.WriteString("@")
		v.sql.WriteString(field)
		v.sql.WriteString(":{")
		v.sql.WriteString(escapeTagValue(fmt.Sprint(value)))
		v.sql.WriteString("}")

	case FieldNumeric:
		num, ok := toNumeric(value)
		if !ok {
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
		if !op.Is(token.EQ) {
			return fmt.Errorf("redis: TEXT field '%s' only supports == / != / LIKE (got '%s')",
				field, expr.Op.Literal)
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

func (v *Visitor) visitTextFieldExpr(expr *ast.BinaryExpr) error {
	field, kind, err := v.resolveFieldKey(expr.Left)
	if err != nil {
		return fmt.Errorf("redis: %w (at %s)", err, expr.Start().String())
	}
	if kind != FieldText {
		return fmt.Errorf("redis: LIKE only supports TEXT fields, got %d for '%s'",
			kind, field)
	}

	value, err := filterhelp.ExtractValue(expr.Right)
	if err != nil {
		return fmt.Errorf("redis: %w (at %s)", err, expr.Start().String())
	}
	s, ok := value.(string)
	if !ok {
		return fmt.Errorf("redis: LIKE requires a string pattern, got %T at %s",
			value, expr.Start().String())
	}

	v.sql.WriteString("@")
	v.sql.WriteString(field)
	v.sql.WriteString(":(")
	v.sql.WriteString(escapeTextValue(s))
	v.sql.WriteString(")")
	return nil
}

func (v *Visitor) visitInExpr(expr *ast.BinaryExpr) error {
	field, kind, err := v.resolveFieldKey(expr.Left)
	if err != nil {
		return fmt.Errorf("redis: %w (at %s)", err, expr.Start().String())
	}

	listLit, ok := expr.Right.(*ast.ListLiteral)
	if !ok {
		return fmt.Errorf("redis: 'IN' requires a list on the right at %s, got %T",
			expr.Start().String(), expr.Right)
	}
	if len(listLit.Values) == 0 {
		return fmt.Errorf("redis: 'IN' requires a non-empty list at %s",
			expr.Start().String())
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
func (v *Visitor) resolveFieldKey(expr ast.Expr) (string, MetadataFieldType, error) {
	var field string
	switch node := expr.(type) {
	case *ast.Ident:
		field = node.Value
	case *ast.IndexExpr:
		// metadata["author"] / metadata["a"]["b"]: collapse the path
		// with dots — RediSearch fields are flat so deep paths must
		// be declared as dotted field names.
		parts, err := flattenIndexExpr(node)
		if err != nil {
			return "", 0, err
		}
		field = strings.Join(parts, ".")
	default:
		return "", 0, fmt.Errorf("unsupported left operand %T", node)
	}

	kind, ok := v.fields[field]
	if !ok {
		return "", 0, fmt.Errorf("redis: filter references undeclared metadata field %q", field)
	}
	return field, kind, nil
}

// flattenIndexExpr collapses an [ast.IndexExpr] into a dotted path —
// e.g. metadata["a"]["b"] → ["a", "b"]. The base identifier
// ("metadata") is stripped so callers only see the inner key path.
func flattenIndexExpr(expr *ast.IndexExpr) ([]string, error) {
	var keys []string
	current := expr
	for {
		key, err := literalToString(current.Index)
		if err != nil {
			return nil, err
		}
		keys = append([]string{key}, keys...)

		switch inner := current.Left.(type) {
		case *ast.IndexExpr:
			current = inner
		case *ast.Ident:
			// Drop the base identifier — it's the field namespace
			// (often "metadata") rather than a key in the path.
			return keys, nil
		default:
			return nil, fmt.Errorf("unsupported index base %T", inner)
		}
	}
}

// numericRange builds the RediSearch [low high] bounds for an ordering
// operator. Inclusive bounds use the bare number; exclusive bounds use
// "(<n>".
func numericRange(op token.Kind, value float64) (string, string) {
	v := formatNumber(value)
	switch op {
	case token.EQ:
		return v, v
	case token.GT:
		return "(" + v, "+inf"
	case token.GE:
		return v, "+inf"
	case token.LT:
		return "-inf", "(" + v
	case token.LE:
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

func toNumeric(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	default:
		return 0, false
	}
}

func literalToString(lit *ast.Literal) (string, error) {
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
		return "", fmt.Errorf("unsupported literal kind %s", lit.Token.Kind.Name())
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
