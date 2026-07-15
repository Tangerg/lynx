package elasticsearch

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/Tangerg/lynx/core/vectorstore/filter"
	"github.com/Tangerg/lynx/vectorstores/internal/filterhelp"
)

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
var _ filter.Visitor = (*Visitor)(nil)

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

func (v *Visitor) Visit(expr filter.Predicate) error {
	v.err = v.visit(expr)
	return v.err
}

func (v *Visitor) visit(expr filter.Expr) error {
	if expr == nil {
		return errors.New("elasticsearch: cannot process nil expression")
	}
	if v.err != nil {
		return v.err
	}

	switch node := expr.(type) {
	case *filter.BinaryExpr:
		if node.Op.IsNullOperator() {
			return v.visitNullTestExpr(node)
		}
		return filterhelp.DispatchBinaryErr(node,
			v.visitLogicalExpr,
			v.visitComparisonExpr,
			v.visitInExpr,
			v.visitLikeExpr,
		)
	case *filter.UnaryExpr:
		return filterhelp.DispatchUnaryErr(node, v.visitNotExpr)
	default:
		return fmt.Errorf("elasticsearch: unsupported root expression %T", node)
	}
}

func (v *Visitor) visitNotExpr(expr *filter.UnaryExpr) error {
	v.sql.WriteString("NOT (")
	if err := v.visit(expr.Right); err != nil {
		return err
	}
	v.sql.WriteString(")")
	return nil
}

func (v *Visitor) visitLogicalExpr(expr *filter.BinaryExpr) error {
	op, err := filterhelp.LogicalOpString(expr.Op)
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

func (v *Visitor) visitComparisonExpr(expr *filter.BinaryExpr) error {
	field, err := v.fieldPath(expr.Left)
	if err != nil {
		return fmt.Errorf("elasticsearch: %w (at %s)", err, expr.Start().String())
	}

	value, err := filterhelp.ExtractValue(expr.Right)
	if err != nil {
		return fmt.Errorf("elasticsearch: %w (at %s)", err, expr.Start().String())
	}

	switch expr.Op {
	case filter.OpEqual:
		v.sql.WriteString(field)
		v.sql.WriteString(":")
		v.sql.WriteString(formatValue(value))
	case filter.OpNotEqual:
		v.sql.WriteString("NOT ")
		v.sql.WriteString(field)
		v.sql.WriteString(":")
		v.sql.WriteString(formatValue(value))
	case filter.OpLess:
		v.sql.WriteString(field)
		v.sql.WriteString(":<")
		v.sql.WriteString(formatValue(value))
	case filter.OpLessEqual:
		v.sql.WriteString(field)
		v.sql.WriteString(":<=")
		v.sql.WriteString(formatValue(value))
	case filter.OpGreater:
		v.sql.WriteString(field)
		v.sql.WriteString(":>")
		v.sql.WriteString(formatValue(value))
	case filter.OpGreaterEqual:
		v.sql.WriteString(field)
		v.sql.WriteString(":>=")
		v.sql.WriteString(formatValue(value))
	default:
		return fmt.Errorf("elasticsearch: unexpected comparison operator '%s'", expr.Op.String())
	}
	return nil
}

func (v *Visitor) visitInExpr(expr *filter.BinaryExpr) error {
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

// visitNullTestExpr emits a "field is null" test as `NOT _exists_:<path>`.
// In Lucene query-string syntax `_exists_:<path>` matches documents where
// the field is present, so its negation matches absent (null) fields —
// matching the inmemory reference semantics where a missing or JSON-null
// metadata key is treated as null.
//
// The negated `IS NOT NULL` arrives as NOT(field IS NULL) and is rendered
// by visitNotExpr, which wraps this clause in another `NOT (...)`. The
// resulting `NOT (NOT _exists_:<path>)` is a double negation equivalent to
// `_exists_:<path>` — the existence check — so no separate handling is
// needed here.
func (v *Visitor) visitNullTestExpr(expr *filter.BinaryExpr) error {
	field, err := v.fieldPath(expr.Left)
	if err != nil {
		return fmt.Errorf("elasticsearch: %w (at %s)", err, expr.Start().String())
	}
	v.sql.WriteString("NOT _exists_:")
	v.sql.WriteString(field)
	return nil
}

// visitLikeExpr maps LIKE onto Lucene wildcard syntax. Right operand
// must be a string pattern — % is translated to *, _ to ?.
func (v *Visitor) visitLikeExpr(expr *filter.BinaryExpr) error {
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
func (v *Visitor) fieldPath(expr filter.Expr) (string, error) {
	keys, err := filterhelp.CollectKeyPath(expr)
	if err != nil {
		return "", err
	}
	if len(keys) == 0 {
		return "", errors.New("empty key path on left operand")
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
