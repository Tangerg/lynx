package mariadb

import (
	"fmt"
	"strings"

	"github.com/Tangerg/lynx/core/vectorstore/filter/ast"
	"github.com/Tangerg/lynx/core/vectorstore/filter/token"
	"github.com/Tangerg/lynx/vectorstores/internal/filterhelp"
)

var _ ast.Visitor = (*Visitor)(nil)

// Visitor transforms AST filter expressions into a MariaDB WHERE
// fragment. Metadata is stored as JSON; the visitor reaches into it
// with JSON_VALUE plus a casting helper for numeric / boolean values
// so the comparison happens in the right SQL type.
//
// Output shape:
//
//	author == "Alice"          →  JSON_VALUE(metadata, '$.author') = ?
//	year >= 2020               →  CAST(JSON_VALUE(metadata, '$.year') AS DOUBLE) >= ?
//	published == true          →  JSON_VALUE(metadata, '$.published') = 'true'  (params: nope — bools render inline)
//	tag IN ("a", "b")          →  JSON_VALUE(metadata, '$.tag') IN (?, ?)
//	NOT (a == "x")             →  NOT (JSON_VALUE(metadata, '$.a') = ?)
//
// Bool literals render inline because MariaDB doesn't accept a true
// Go bool through the binary protocol for a JSON-comparison context.
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
		return fmt.Errorf("mariadb: cannot process nil expression")
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
		return fmt.Errorf("mariadb: unsupported root expression %T", node)
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
		return fmt.Errorf("mariadb: %w", err)
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
	jsonPath, err := buildJSONPath(expr.Left)
	if err != nil {
		return fmt.Errorf("mariadb: %w (at %s)", err, expr.Start().String())
	}
	value, err := filterhelp.ExtractValue(expr.Right)
	if err != nil {
		return fmt.Errorf("mariadb: %w (at %s)", err, expr.Start().String())
	}
	op, err := sqlOpFor(expr.Op.Kind)
	if err != nil {
		return err
	}

	v.appendJSONExtraction(jsonPath, value, expr.Op.Kind)
	v.sql.WriteByte(' ')
	v.sql.WriteString(op)
	v.sql.WriteByte(' ')
	v.appendValuePlaceholder(value)
	return nil
}

func (v *Visitor) visitInExpr(expr *ast.BinaryExpr) error {
	jsonPath, err := buildJSONPath(expr.Left)
	if err != nil {
		return fmt.Errorf("mariadb: %w (at %s)", err, expr.Start().String())
	}

	listLit, err := filterhelp.RequireListLiteral(expr)
	if err != nil {
		return fmt.Errorf("mariadb: %w", err)
	}

	values := make([]any, 0, len(listLit.Values))
	for _, lit := range listLit.Values {
		val, err := filterhelp.LiteralToValue(lit)
		if err != nil {
			return fmt.Errorf("mariadb: %w (at %s)", err, expr.Start().String())
		}
		values = append(values, val)
	}

	v.appendJSONExtraction(jsonPath, values[0], token.EQ)
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
	jsonPath, err := buildJSONPath(expr.Left)
	if err != nil {
		return fmt.Errorf("mariadb: %w (at %s)", err, expr.Start().String())
	}
	pattern, err := filterhelp.RequireStringPatternOnRight(expr)
	if err != nil {
		return fmt.Errorf("mariadb: %w", err)
	}

	v.appendJSONExtraction(jsonPath, "", token.EQ)
	v.sql.WriteString(" LIKE ")
	v.appendValuePlaceholder(pattern)
	return nil
}

// appendJSONExtraction emits the JSON_VALUE / CAST wrapper appropriate
// for the comparison's value type.
func (v *Visitor) appendJSONExtraction(jsonPath string, value any, op token.Kind) {
	switch value.(type) {
	case float64, int64, int:
		v.sql.WriteString("CAST(JSON_VALUE(")
		v.sql.WriteString(v.metadataColumn)
		v.sql.WriteString(", ")
		v.sql.WriteString(quoteSQLString(jsonPath))
		v.sql.WriteString(") AS DOUBLE)")
	default:
		if op.IsOrderingOperator() {
			// Ordering on non-numeric literals — force a numeric
			// cast so the comparison still has well-defined semantics.
			v.sql.WriteString("CAST(JSON_VALUE(")
			v.sql.WriteString(v.metadataColumn)
			v.sql.WriteString(", ")
			v.sql.WriteString(quoteSQLString(jsonPath))
			v.sql.WriteString(") AS DOUBLE)")
		} else {
			v.sql.WriteString("JSON_VALUE(")
			v.sql.WriteString(v.metadataColumn)
			v.sql.WriteString(", ")
			v.sql.WriteString(quoteSQLString(jsonPath))
			v.sql.WriteByte(')')
		}
	}
}

// appendValuePlaceholder binds the value and writes a `?`. Booleans
// render inline as 'true' / 'false' since JSON_VALUE returns strings.
func (v *Visitor) appendValuePlaceholder(value any) {
	if b, ok := value.(bool); ok {
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

func buildJSONPath(expr ast.Expr) (string, error) {
	keys, err := filterhelp.CollectKeyPath(expr)
	if err != nil {
		return "", err
	}
	if len(keys) == 0 {
		return "", fmt.Errorf("empty key path on left operand")
	}
	return "$." + strings.Join(keys, "."), nil
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
