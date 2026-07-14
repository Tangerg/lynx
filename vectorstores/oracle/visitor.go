package oracle

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/Tangerg/lynx/core/vectorstore/filter"
	"github.com/Tangerg/lynx/vectorstores/internal/filterhelp"
)

// Visitor transforms AST filter expressions into an Oracle WHERE
// fragment that reaches into the JSON `metadata` column. Numeric and
// boolean comparisons use Oracle's typed json_value extractors so the
// comparison happens in the correct SQL type.
//
// Output shape:
//
//	author == "Alice"          →  json_value(metadata, '$.author') = :1
//	year >= 2020               →  json_value(metadata, '$.year' RETURNING NUMBER) >= :1
//	published == true          →  json_value(metadata, '$.published') = 'true'
//	tag IN ("a", "b")          →  json_value(metadata, '$.tag') IN (:1, :2)
//	NOT (a == "x")             →  NOT (json_value(metadata, '$.a') = :1)
type Visitor struct {
	err            error
	sql            strings.Builder
	args           []any
	paramCount     int
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

func (v *Visitor) Visit(expr filter.Expr) error {
	v.err = v.visit(expr)
	return v.err
}

func (v *Visitor) visit(expr filter.Expr) error {
	if expr == nil {
		return errors.New("oracle: cannot process nil expression")
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
		return fmt.Errorf("oracle: unsupported root expression %T", node)
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
		return fmt.Errorf("oracle: %w", err)
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
	jsonPath, err := buildJSONPath(expr.Left)
	if err != nil {
		return fmt.Errorf("oracle: %w (at %s)", err, expr.Start().String())
	}
	value, err := filterhelp.ExtractValue(expr.Right)
	if err != nil {
		return fmt.Errorf("oracle: %w (at %s)", err, expr.Start().String())
	}
	op, err := sqlOpFor(expr.Op)
	if err != nil {
		return err
	}

	v.appendJSONExtraction(jsonPath, value, expr.Op)
	v.sql.WriteByte(' ')
	v.sql.WriteString(op)
	v.sql.WriteByte(' ')
	v.appendValuePlaceholder(value)
	return nil
}

func (v *Visitor) visitInExpr(expr *filter.BinaryExpr) error {
	jsonPath, err := buildJSONPath(expr.Left)
	if err != nil {
		return fmt.Errorf("oracle: %w (at %s)", err, expr.Start().String())
	}

	listLit, err := filterhelp.RequireListLiteral(expr)
	if err != nil {
		return fmt.Errorf("oracle: %w", err)
	}

	values := make([]any, 0, len(listLit.Values))
	for _, lit := range listLit.Values {
		val, err := filterhelp.LiteralToValue(lit)
		if err != nil {
			return fmt.Errorf("oracle: %w (at %s)", err, expr.Start().String())
		}
		values = append(values, val)
	}

	v.appendJSONExtraction(jsonPath, values[0], filter.OpEqual)
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

func (v *Visitor) visitLikeExpr(expr *filter.BinaryExpr) error {
	jsonPath, err := buildJSONPath(expr.Left)
	if err != nil {
		return fmt.Errorf("oracle: %w (at %s)", err, expr.Start().String())
	}
	pattern, err := filterhelp.RequireStringPatternOnRight(expr)
	if err != nil {
		return fmt.Errorf("oracle: %w", err)
	}

	v.appendJSONExtraction(jsonPath, "", filter.OpEqual)
	v.sql.WriteString(" LIKE ")
	v.appendValuePlaceholder(pattern)
	return nil
}

// visitNullTestExpr emits `(json_value(metadata, '$.key') IS NULL)`.
// Oracle's json_value yields SQL NULL both when the path is absent and
// when the stored value is JSON null, matching the inmemory reference
// semantics. The negated `IS NOT NULL` arrives as NOT(… IS NULL) and is
// rendered by visitNotExpr, so no separate handling is needed here.
func (v *Visitor) visitNullTestExpr(expr *filter.BinaryExpr) error {
	jsonPath, err := buildJSONPath(expr.Left)
	if err != nil {
		return fmt.Errorf("oracle: %w (at %s)", err, expr.Start().String())
	}
	v.sql.WriteString("(json_value(")
	v.sql.WriteString(v.metadataColumn)
	v.sql.WriteString(", ")
	v.sql.WriteString(quoteSQLString(jsonPath))
	v.sql.WriteString(") IS NULL)")
	return nil
}

// appendJSONExtraction emits the appropriate json_value() expression
// for the value's type. Numeric / boolean comparisons use Oracle's
// typed RETURNING clause so the type round-trips correctly.
func (v *Visitor) appendJSONExtraction(jsonPath string, value any, op filter.Operator) {
	switch value.(type) {
	case float64, int64, int:
		v.sql.WriteString("json_value(")
		v.sql.WriteString(v.metadataColumn)
		v.sql.WriteString(", ")
		v.sql.WriteString(quoteSQLString(jsonPath))
		v.sql.WriteString(" RETURNING NUMBER)")
	default:
		if op.IsOrderingOperator() {
			v.sql.WriteString("json_value(")
			v.sql.WriteString(v.metadataColumn)
			v.sql.WriteString(", ")
			v.sql.WriteString(quoteSQLString(jsonPath))
			v.sql.WriteString(" RETURNING NUMBER)")
		} else {
			v.sql.WriteString("json_value(")
			v.sql.WriteString(v.metadataColumn)
			v.sql.WriteString(", ")
			v.sql.WriteString(quoteSQLString(jsonPath))
			v.sql.WriteByte(')')
		}
	}
}

// appendValuePlaceholder binds the value via :N and writes the
// placeholder.
func (v *Visitor) appendValuePlaceholder(value any) {
	if b, ok := value.(bool); ok {
		if b {
			v.sql.WriteString("'true'")
		} else {
			v.sql.WriteString("'false'")
		}
		return
	}
	v.paramCount++
	v.args = append(v.args, value)
	v.sql.WriteByte(':')
	v.sql.WriteString(strconv.Itoa(v.paramCount))
}

func buildJSONPath(expr filter.Expr) (string, error) {
	keys, err := filterhelp.CollectKeyPath(expr)
	if err != nil {
		return "", err
	}
	if len(keys) == 0 {
		return "", errors.New("empty key path on left operand")
	}
	return "$." + strings.Join(keys, "."), nil
}

func sqlOpFor(kind filter.Operator) (string, error) {
	switch kind {
	case filter.OpEqual:
		return "=", nil
	case filter.OpNotEqual:
		return "<>", nil
	case filter.OpLess:
		return "<", nil
	case filter.OpLessEqual:
		return "<=", nil
	case filter.OpGreater:
		return ">", nil
	case filter.OpGreaterEqual:
		return ">=", nil
	default:
		return "", fmt.Errorf("unexpected comparison operator '%s'", kind.Name())
	}
}

func quoteSQLString(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
