package couchbase

import (
	"fmt"
	"strings"

	"encoding/json"

	"github.com/Tangerg/lynx/core/vectorstore/filter/ast"
	"github.com/Tangerg/lynx/core/vectorstore/filter/token"
	"github.com/Tangerg/lynx/vectorstores/internal/filterhelp"
)

var _ ast.Visitor = (*Visitor)(nil)

// Visitor transforms AST filter expressions into a SQL++ (N1QL)
// predicate fragment usable in `WHERE` clauses of queries and
// DELETE statements.
//
// Output shape (metadata keys are addressed under metadata.*):
//
//	author == "Alice"          →  metadata.`author` = "Alice"
//	year >= 2020               →  metadata.`year` >= 2020
//	category IN ("a", "b")     →  metadata.`category` IN ["a", "b"]
//	NOT (year >= 2020)         →  NOT (metadata.`year` >= 2020)
//	a == "x" AND b == "y"      →  (metadata.`a` = "x" AND metadata.`b` = "y")
//
// Values are JSON-encoded — strings get double-quoted with embedded
// quotes escaped per JSON rules, which is also valid in SQL++ string
// literals.
type Visitor struct {
	err            error
	sql            strings.Builder
	metadataPrefix string // typically "metadata"
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

func (v *Visitor) Visit(expr ast.Expr) error {
	v.err = v.visit(expr)
	return v.err
}

func (v *Visitor) visit(expr ast.Expr) error {
	if expr == nil {
		return fmt.Errorf("couchbase: cannot process nil expression")
	}
	if v.err != nil {
		return v.err
	}

	switch node := expr.(type) {
	case *ast.BinaryExpr:
		if node.Op.Kind.IsNullOperator() {
			return v.visitNullTestExpr(node)
		}
		return v.visitBinaryExpr(node)
	case *ast.UnaryExpr:
		return v.visitUnaryExpr(node)
	default:
		return fmt.Errorf("couchbase: unsupported root expression %T", node)
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
		return fmt.Errorf("couchbase: unsupported binary operator '%s' at %s",
			expr.Op.Literal, expr.Start().String())
	}
}

func (v *Visitor) visitUnaryExpr(expr *ast.UnaryExpr) error {
	if !expr.Op.Kind.Is(token.NOT) {
		return fmt.Errorf("couchbase: unsupported unary operator '%s' at %s",
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
		return fmt.Errorf("couchbase: %w (at %s)", err, expr.Start().String())
	}
	value, err := filterhelp.ExtractValue(expr.Right)
	if err != nil {
		return fmt.Errorf("couchbase: %w (at %s)", err, expr.Start().String())
	}
	op, err := sqlOpFor(expr.Op.Kind)
	if err != nil {
		return err
	}

	v.sql.WriteString(field)
	v.sql.WriteByte(' ')
	v.sql.WriteString(op)
	v.sql.WriteByte(' ')
	v.sql.WriteString(jsonValue(value))
	return nil
}

func (v *Visitor) visitInExpr(expr *ast.BinaryExpr) error {
	field, err := v.fieldPath(expr.Left)
	if err != nil {
		return fmt.Errorf("couchbase: %w (at %s)", err, expr.Start().String())
	}

	listLit, ok := expr.Right.(*ast.ListLiteral)
	if !ok {
		return fmt.Errorf("couchbase: 'IN' requires a list on the right at %s, got %T",
			expr.Start().String(), expr.Right)
	}
	if len(listLit.Values) == 0 {
		return fmt.Errorf("couchbase: 'IN' requires a non-empty list at %s",
			expr.Start().String())
	}

	values := make([]any, 0, len(listLit.Values))
	for _, lit := range listLit.Values {
		val, err := filterhelp.LiteralToValue(lit)
		if err != nil {
			return fmt.Errorf("couchbase: %w (at %s)", err, expr.Start().String())
		}
		values = append(values, val)
	}

	v.sql.WriteString(field)
	v.sql.WriteString(" IN ")
	v.sql.WriteString(jsonValue(values))
	return nil
}

// visitLikeExpr emits SQL++ LIKE — SQL wildcards % / _ pass through
// untouched since LIKE uses the same syntax.
func (v *Visitor) visitLikeExpr(expr *ast.BinaryExpr) error {
	field, err := v.fieldPath(expr.Left)
	if err != nil {
		return fmt.Errorf("couchbase: %w (at %s)", err, expr.Start().String())
	}
	value, err := filterhelp.ExtractValue(expr.Right)
	if err != nil {
		return fmt.Errorf("couchbase: %w (at %s)", err, expr.Start().String())
	}
	pattern, ok := value.(string)
	if !ok {
		return fmt.Errorf("couchbase: LIKE requires a string pattern, got %T at %s",
			value, expr.Start().String())
	}

	v.sql.WriteString(field)
	v.sql.WriteString(" LIKE ")
	v.sql.WriteString(jsonValue(pattern))
	return nil
}

// visitNullTestExpr emits `(<path> IS NULL)`. In SQL++ a path that
// resolves to JSON null is IS NULL; an absent key resolves to MISSING,
// which IS NULL also matches in the FTS/N1QL evaluation used here,
// mirroring the inmemory reference semantics. The negated IS NOT NULL
// arrives as NOT(<path> IS NULL) and is rendered by visitUnaryExpr, so
// no separate handling is needed here. No bound parameter is required.
func (v *Visitor) visitNullTestExpr(expr *ast.BinaryExpr) error {
	field, err := v.fieldPath(expr.Left)
	if err != nil {
		return fmt.Errorf("couchbase: %w (at %s)", err, expr.Start().String())
	}
	v.sql.WriteString("(")
	v.sql.WriteString(field)
	v.sql.WriteString(" IS NULL)")
	return nil
}

// fieldPath builds the dotted SQL++ path for the left operand, with
// each segment backtick-quoted to allow special characters.
func (v *Visitor) fieldPath(expr ast.Expr) (string, error) {
	keys, err := filterhelp.CollectKeyPath(expr)
	if err != nil {
		return "", err
	}
	if len(keys) == 0 {
		return "", fmt.Errorf("empty key path on left operand")
	}
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, "`"+strings.ReplaceAll(k, "`", "``")+"`")
	}
	joined := strings.Join(parts, ".")
	if v.metadataPrefix == "" {
		return joined, nil
	}
	return v.metadataPrefix + "." + joined, nil
}

func sqlOpFor(kind token.Kind) (string, error) {
	switch kind {
	case token.EQ:
		return "=", nil
	case token.NE:
		return "!=", nil
	case token.LT:
		return "<", nil
	case token.LE:
		return "<=", nil
	case token.GT:
		return ">", nil
	case token.GE:
		return ">=", nil
	default:
		return "", fmt.Errorf("unexpected comparison operator '%s'", kind.Name())
	}
}

func jsonValue(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "null"
	}
	return string(b)
}
