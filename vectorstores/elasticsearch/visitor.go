package elasticsearch

import (
	"fmt"
	"strings"
	"strconv"

	"github.com/Tangerg/lynx/core/vectorstore/filter/ast"
	"github.com/Tangerg/lynx/vectorstores/internal/filterhelp"
	"github.com/Tangerg/lynx/core/vectorstore/filter/token"
)

var _ ast.Visitor = (*Visitor)(nil)

// Visitor transforms AST filter expressions into Elasticsearch
// query-string syntax (Lucene). The output is meant to be plugged
// into a `query_string.query` clause inside the KNN filter.
//
// Output shape (metadata fields are prefixed with the configured
// metadata field path — default "metadata"):
//
//	author == "Alice"          →  metadata.author:"Alice"
//	year >= 2020               →  metadata.year:>=2020
//	year < 2025                →  metadata.year:<2025
//	category IN ("a", "b")     →  metadata.category:("a" OR "b")
//	NOT (author == "Alice")    →  NOT (metadata.author:"Alice")
//	a == "x" AND b == "y"      →  (metadata.a:"x" AND metadata.b:"y")
//
// Identifier paths:
//   - bare identifier      → <prefix>.<ident>
//   - metadata['k']        → <prefix>.k
//   - metadata['a']['b']   → <prefix>.a.b
type Visitor struct {
	err            error
	sql            strings.Builder
	metadataPrefix string // e.g. "metadata"
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

func (v *Visitor) Error() error { return v.err }

func (v *Visitor) Visit(expr ast.Expr) ast.Visitor {
	v.err = v.visit(expr)
	return nil
}

func (v *Visitor) visit(expr ast.Expr) error {
	if expr == nil {
		return fmt.Errorf("elasticsearch: cannot process nil expression")
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
		return fmt.Errorf("elasticsearch: unsupported root expression %T", node)
	}
}

func (v *Visitor) visitBinaryExpr(expr *ast.BinaryExpr) error {
	switch {
	case expr.Op.Kind.IsLogicalOperator():
		return v.visitLogicalExpr(expr)
	case expr.Op.Kind.Is(token.IN):
		return v.visitInExpr(expr)
	case expr.Op.Kind.Is(token.LIKE):
		return v.visitLikeExpr(expr)
	case expr.Op.Kind.IsEqualityOperator() || expr.Op.Kind.IsOrderingOperator():
		return v.visitComparisonExpr(expr)
	default:
		return fmt.Errorf("elasticsearch: unsupported binary operator '%s' at %s",
			expr.Op.Literal, expr.Start().String())
	}
}

func (v *Visitor) visitUnaryExpr(expr *ast.UnaryExpr) error {
	if !expr.Op.Kind.Is(token.NOT) {
		return fmt.Errorf("elasticsearch: unsupported unary operator '%s' at %s",
			expr.Op.Literal, expr.Start().String())
	}
	v.sql.WriteString("NOT (")
	if err := v.visit(expr.Right); err != nil {
		return err
	}
	v.sql.WriteString(")")
	return nil
}

func (v *Visitor) visitLogicalExpr(expr *ast.BinaryExpr) error {
	op := " AND "
	if expr.Op.Kind.Is(token.OR) {
		op = " OR "
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

func (v *Visitor) visitComparisonExpr(expr *ast.BinaryExpr) error {
	field, err := v.fieldPath(expr.Left)
	if err != nil {
		return fmt.Errorf("elasticsearch: %w (at %s)", err, expr.Start().String())
	}

	value, err := filterhelp.ExtractValue(expr.Right)
	if err != nil {
		return fmt.Errorf("elasticsearch: %w (at %s)", err, expr.Start().String())
	}

	switch expr.Op.Kind {
	case token.EQ:
		v.sql.WriteString(field)
		v.sql.WriteString(":")
		v.sql.WriteString(formatValue(value))
	case token.NE:
		v.sql.WriteString("NOT ")
		v.sql.WriteString(field)
		v.sql.WriteString(":")
		v.sql.WriteString(formatValue(value))
	case token.LT:
		v.sql.WriteString(field)
		v.sql.WriteString(":<")
		v.sql.WriteString(formatValue(value))
	case token.LE:
		v.sql.WriteString(field)
		v.sql.WriteString(":<=")
		v.sql.WriteString(formatValue(value))
	case token.GT:
		v.sql.WriteString(field)
		v.sql.WriteString(":>")
		v.sql.WriteString(formatValue(value))
	case token.GE:
		v.sql.WriteString(field)
		v.sql.WriteString(":>=")
		v.sql.WriteString(formatValue(value))
	default:
		return fmt.Errorf("elasticsearch: unexpected comparison operator '%s'", expr.Op.Literal)
	}
	return nil
}

func (v *Visitor) visitInExpr(expr *ast.BinaryExpr) error {
	field, err := v.fieldPath(expr.Left)
	if err != nil {
		return fmt.Errorf("elasticsearch: %w (at %s)", err, expr.Start().String())
	}

	listLit, ok := expr.Right.(*ast.ListLiteral)
	if !ok {
		return fmt.Errorf("elasticsearch: 'IN' requires a list on the right at %s, got %T",
			expr.Start().String(), expr.Right)
	}
	if len(listLit.Values) == 0 {
		return fmt.Errorf("elasticsearch: 'IN' requires a non-empty list at %s",
			expr.Start().String())
	}

	parts := make([]string, 0, len(listLit.Values))
	for _, lit := range listLit.Values {
		val, err := filterhelp.LiteralToValue(lit)
		if err != nil {
			return fmt.Errorf("elasticsearch: %w (at %s)", err, expr.Start().String())
		}
		parts = append(parts, formatValue(val))
	}

	v.sql.WriteString(field)
	v.sql.WriteString(":(")
	v.sql.WriteString(strings.Join(parts, " OR "))
	v.sql.WriteString(")")
	return nil
}

// visitLikeExpr maps LIKE onto Lucene wildcard syntax. Right operand
// must be a string pattern — % is translated to *, _ to ?.
func (v *Visitor) visitLikeExpr(expr *ast.BinaryExpr) error {
	field, err := v.fieldPath(expr.Left)
	if err != nil {
		return fmt.Errorf("elasticsearch: %w (at %s)", err, expr.Start().String())
	}

	value, err := filterhelp.ExtractValue(expr.Right)
	if err != nil {
		return fmt.Errorf("elasticsearch: %w (at %s)", err, expr.Start().String())
	}
	pattern, ok := value.(string)
	if !ok {
		return fmt.Errorf("elasticsearch: LIKE requires a string pattern, got %T at %s",
			value, expr.Start().String())
	}

	// SQL LIKE → Lucene wildcards. Escape any pre-existing wildcards
	// in the source pattern so they round-trip as literals.
	var b strings.Builder
	b.Grow(len(pattern))
	for _, r := range pattern {
		switch r {
		case '%':
			b.WriteByte('*')
		case '_':
			b.WriteByte('?')
		case '*', '?':
			b.WriteByte('\\')
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}
	v.sql.WriteString(field)
	v.sql.WriteString(":")
	v.sql.WriteString(b.String())
	return nil
}

// fieldPath assembles the dotted Elasticsearch field path for the
// metadata key on the left side of a comparison.
func (v *Visitor) fieldPath(expr ast.Expr) (string, error) {
	keys, err := filterhelp.CollectKeyPath(expr)
	if err != nil {
		return "", err
	}
	if len(keys) == 0 {
		return "", fmt.Errorf("empty key path on left operand")
	}
	if v.metadataPrefix == "" {
		return strings.Join(keys, "."), nil
	}
	return v.metadataPrefix + "." + strings.Join(keys, "."), nil
}


func formatValue(v any) string {
	switch val := v.(type) {
	case string:
		return `"` + escapeLuceneString(val) + `"`
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

func escapeLuceneString(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case '"', '\\':
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}
