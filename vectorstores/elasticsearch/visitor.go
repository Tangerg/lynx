package elasticsearch

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Tangerg/lynx/core/vectorstore/filter/ast"
	"github.com/Tangerg/lynx/core/vectorstore/filter/token"
	"github.com/Tangerg/lynx/vectorstores/internal/filterhelp"
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
		return filterhelp.DispatchBinaryErr(node,
			v.visitLogicalExpr,
			v.visitComparisonExpr,
			v.visitInExpr,
			v.visitLikeExpr,
		)
	case *ast.UnaryExpr:
		return filterhelp.DispatchUnaryErr(node, v.visitNotExpr)
	default:
		return fmt.Errorf("elasticsearch: unsupported root expression %T", node)
	}
}

func (v *Visitor) visitNotExpr(expr *ast.UnaryExpr) error {
	v.sql.WriteString("NOT (")
	if err := v.visit(expr.Right); err != nil {
		return err
	}
	v.sql.WriteString(")")
	return nil
}

func (v *Visitor) visitLogicalExpr(expr *ast.BinaryExpr) error {
	op, err := filterhelp.LogicalOpString(expr.Op.Kind)
	if err != nil {
		return fmt.Errorf("elasticsearch: %w", err)
	}
	v.sql.WriteString("(")
	if err := v.visit(expr.Left); err != nil {
		return err
	}
	v.sql.WriteString(" ")
	v.sql.WriteString(op)
	v.sql.WriteString(" ")
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

	listLit, err := filterhelp.RequireListLiteral(expr)
	if err != nil {
		return fmt.Errorf("elasticsearch: %w", err)
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

	pattern, err := filterhelp.RequireStringPatternOnRight(expr)
	if err != nil {
		return fmt.Errorf("elasticsearch: %w", err)
	}

	v.sql.WriteString(field)
	v.sql.WriteString(":")
	v.sql.WriteString(luceneWildcards(pattern))
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

// luceneWildcards maps SQL LIKE wildcards (% / _) to Lucene
// wildcards (* / ?) and escapes any pre-existing wildcards in the
// source pattern so they round-trip as literals.
func luceneWildcards(pattern string) string {
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
	return b.String()
}
