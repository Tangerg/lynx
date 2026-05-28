package clickhouse

import (
	"fmt"
	"strings"

	"github.com/Tangerg/lynx/core/vectorstore/filter/ast"
	"github.com/Tangerg/lynx/core/vectorstore/filter/token"
	"github.com/Tangerg/lynx/vectorstores/internal/filterhelp"
)

var _ ast.Visitor = (*Visitor)(nil)

// Visitor transforms AST filter expressions into a ClickHouse WHERE
// fragment. Metadata is stored as a `Map(String, String)` column;
// metadata keys are addressed with the map-subscript syntax
// (metadata['key']).
//
// Output shape:
//
//	author == "Alice"           →  metadata['author'] = ?
//	year >= 2020                →  toFloat64OrZero(metadata['year']) >= ?
//	tag IN ("a", "b")           →  metadata['tag'] IN (?, ?)
//	NOT (author == "Alice")     →  NOT (metadata['author'] = ?)
type Visitor struct {
	err            error
	sql            strings.Builder
	args           []any
	metadataColumn string
}

func NewVisitor(metadataColumn string) *Visitor {
	if metadataColumn == "" {
		metadataColumn = "metadata"
	}
	return &Visitor{metadataColumn: metadataColumn}
}

func (v *Visitor) Result() (string, []any) {
	if v.err != nil {
		return "", nil
	}
	return v.sql.String(), v.args
}

func (v *Visitor) Error() error { return v.err }

func (v *Visitor) Visit(expr ast.Expr) ast.Visitor {
	v.err = v.visit(expr)
	return nil
}

func (v *Visitor) visit(expr ast.Expr) error {
	if expr == nil {
		return fmt.Errorf("clickhouse: cannot process nil expression")
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
		return fmt.Errorf("clickhouse: unsupported root expression %T", node)
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
		return fmt.Errorf("clickhouse: unsupported binary operator '%s'", expr.Op.Literal)
	}
}

func (v *Visitor) visitUnaryExpr(expr *ast.UnaryExpr) error {
	if !expr.Op.Kind.Is(token.NOT) {
		return fmt.Errorf("clickhouse: unsupported unary '%s'", expr.Op.Literal)
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
	jsonPath, err := buildKeyPath(expr.Left)
	if err != nil {
		return fmt.Errorf("clickhouse: %w (at %s)", err, expr.Start().String())
	}
	value, err := filterhelp.ExtractValue(expr.Right)
	if err != nil {
		return fmt.Errorf("clickhouse: %w (at %s)", err, expr.Start().String())
	}
	op, err := sqlOpFor(expr.Op.Kind)
	if err != nil {
		return err
	}
	v.appendMapAccess(jsonPath, value, expr.Op.Kind)
	v.sql.WriteByte(' ')
	v.sql.WriteString(op)
	v.sql.WriteByte(' ')
	v.appendValuePlaceholder(value)
	return nil
}

func (v *Visitor) visitInExpr(expr *ast.BinaryExpr) error {
	jsonPath, err := buildKeyPath(expr.Left)
	if err != nil {
		return fmt.Errorf("clickhouse: %w (at %s)", err, expr.Start().String())
	}
	listLit, ok := expr.Right.(*ast.ListLiteral)
	if !ok {
		return fmt.Errorf("clickhouse: 'IN' requires a list on the right")
	}
	if len(listLit.Values) == 0 {
		return fmt.Errorf("clickhouse: 'IN' requires a non-empty list")
	}
	values := make([]any, 0, len(listLit.Values))
	for _, lit := range listLit.Values {
		val, err := filterhelp.LiteralToValue(lit)
		if err != nil {
			return err
		}
		values = append(values, val)
	}
	v.appendMapAccess(jsonPath, values[0], token.EQ)
	v.sql.WriteString(" IN (")
	for i, val := range values {
		if i > 0 {
			v.sql.WriteString(", ")
		}
		v.appendValuePlaceholder(val)
	}
	v.sql.WriteByte(')')
	return nil
}

func (v *Visitor) visitLikeExpr(expr *ast.BinaryExpr) error {
	jsonPath, err := buildKeyPath(expr.Left)
	if err != nil {
		return fmt.Errorf("clickhouse: %w (at %s)", err, expr.Start().String())
	}
	value, err := filterhelp.ExtractValue(expr.Right)
	if err != nil {
		return fmt.Errorf("clickhouse: %w (at %s)", err, expr.Start().String())
	}
	pattern, ok := value.(string)
	if !ok {
		return fmt.Errorf("clickhouse: LIKE requires a string pattern, got %T", value)
	}
	v.appendMapAccess(jsonPath, "", token.EQ)
	v.sql.WriteString(" LIKE ")
	v.appendValuePlaceholder(pattern)
	return nil
}

// appendMapAccess writes `metadata['key']`, wrapping the access in
// `toFloat64OrZero(...)` when the comparison value implies numeric
// semantics.
func (v *Visitor) appendMapAccess(key string, value any, op token.Kind) {
	switch value.(type) {
	case float64, int64, int:
		v.sql.WriteString("toFloat64OrZero(")
		v.sql.WriteString(v.metadataColumn)
		v.sql.WriteString("[")
		v.sql.WriteString(quoteSQLString(key))
		v.sql.WriteString("])")
	default:
		if op.IsOrderingOperator() {
			v.sql.WriteString("toFloat64OrZero(")
			v.sql.WriteString(v.metadataColumn)
			v.sql.WriteString("[")
			v.sql.WriteString(quoteSQLString(key))
			v.sql.WriteString("])")
		} else {
			v.sql.WriteString(v.metadataColumn)
			v.sql.WriteString("[")
			v.sql.WriteString(quoteSQLString(key))
			v.sql.WriteString("]")
		}
	}
}

func (v *Visitor) appendValuePlaceholder(value any) {
	if b, ok := value.(bool); ok {
		// ClickHouse Map(String, String) — booleans render as text.
		if b {
			v.sql.WriteString("'true'")
		} else {
			v.sql.WriteString("'false'")
		}
		return
	}
	v.args = append(v.args, value)
	v.sql.WriteByte('?')
}

func buildKeyPath(expr ast.Expr) (string, error) {
	keys, err := filterhelp.CollectKeyPath(expr)
	if err != nil {
		return "", err
	}
	if len(keys) == 0 {
		return "", fmt.Errorf("empty key path")
	}
	return strings.Join(keys, "."), nil
}

func sqlOpFor(kind token.Kind) (string, error) {
	switch kind {
	case token.EQ:
		return "=", nil
	case token.NE:
		return "<>", nil
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

func quoteSQLString(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
