package clickhouse

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/lynx/core/vectorstore/filter"
	"github.com/Tangerg/lynx/vectorstores/internal/filterhelp"
)

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

func (v *Visitor) Visit(expr filter.Expr) error {
	v.err = v.visit(expr)
	return v.err
}

func (v *Visitor) visit(expr filter.Expr) error {
	if expr == nil {
		return errors.New("clickhouse: cannot process nil expression")
	}
	if v.err != nil {
		return v.err
	}
	switch node := expr.(type) {
	case *filter.BinaryExpr:
		if node.Op.IsNullOperator() {
			return v.visitNullTestExpr(node)
		}
		return v.visitBinaryExpr(node)
	case *filter.UnaryExpr:
		return v.visitUnaryExpr(node)
	default:
		return fmt.Errorf("clickhouse: unsupported root expression %T", node)
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
		return fmt.Errorf("clickhouse: unsupported binary operator '%s'", expr.Op.String())
	}
}

func (v *Visitor) visitUnaryExpr(expr *filter.UnaryExpr) error {
	if !expr.Op.Is(filter.OpNot) {
		return fmt.Errorf("clickhouse: unsupported unary '%s'", expr.Op.String())
	}
	v.sql.WriteString("NOT (")
	if err := v.visit(expr.Right); err != nil {
		return err
	}
	v.sql.WriteString(")")
	return nil
}

func (v *Visitor) visitLogicalExpr(expr *filter.BinaryExpr) error {
	op := " AND "
	if expr.Op.Is(filter.OpOr) {
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

func (v *Visitor) visitComparisonExpr(expr *filter.BinaryExpr) error {
	jsonPath, err := buildKeyPath(expr.Left)
	if err != nil {
		return fmt.Errorf("clickhouse: %w (at %s)", err, expr.Start().String())
	}
	value, err := filterhelp.ExtractValue(expr.Right)
	if err != nil {
		return fmt.Errorf("clickhouse: %w (at %s)", err, expr.Start().String())
	}
	op, err := sqlOpFor(expr.Op)
	if err != nil {
		return err
	}
	v.appendMapAccess(jsonPath, value, expr.Op)
	v.sql.WriteByte(' ')
	v.sql.WriteString(op)
	v.sql.WriteByte(' ')
	v.appendValuePlaceholder(value)
	return nil
}

func (v *Visitor) visitInExpr(expr *filter.BinaryExpr) error {
	jsonPath, err := buildKeyPath(expr.Left)
	if err != nil {
		return fmt.Errorf("clickhouse: %w (at %s)", err, expr.Start().String())
	}
	listLit, ok := expr.Right.(*filter.ListLiteral)
	if !ok {
		return errors.New("clickhouse: 'IN' requires a list on the right")
	}
	if len(listLit.Values) == 0 {
		return errors.New("clickhouse: 'IN' requires a non-empty list")
	}
	values := make([]any, 0, len(listLit.Values))
	for _, lit := range listLit.Values {
		val, err := filterhelp.LiteralToValue(lit)
		if err != nil {
			return err
		}
		values = append(values, val)
	}
	v.appendMapAccess(jsonPath, values[0], filter.OpEqual)
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
	v.appendMapAccess(jsonPath, "", filter.OpEqual)
	v.sql.WriteString(" LIKE ")
	v.appendValuePlaceholder(pattern)
	return nil
}

// visitNullTestExpr emits `NOT mapContains(metadata, 'key')` for an
// `IS NULL` test. ClickHouse stores metadata as a `Map(String, String)`,
// where a subscript on a missing key yields the type default (empty
// string) rather than SQL NULL — so `metadata['key'] IS NULL` can never
// match. Key absence is therefore the right model for "is null", which
// matches the inmemory reference semantics (a missing metadata key reads
// as null). The negated `IS NOT NULL` arrives as NOT(… IS NULL) and is
// rendered by visitUnaryExpr, so no separate handling is needed here.
func (v *Visitor) visitNullTestExpr(expr *filter.BinaryExpr) error {
	key, err := buildKeyPath(expr.Left)
	if err != nil {
		return fmt.Errorf("clickhouse: %w (at %s)", err, expr.Start().String())
	}
	v.sql.WriteString("(NOT mapContains(")
	v.sql.WriteString(v.metadataColumn)
	v.sql.WriteString(", ")
	v.sql.WriteString(quoteSQLString(key))
	v.sql.WriteString("))")
	return nil
}

// appendMapAccess writes `metadata['key']`, wrapping the access in
// `toFloat64OrZero(...)` when the comparison value implies numeric
// semantics.
func (v *Visitor) appendMapAccess(key string, value any, op filter.Operator) {
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

func buildKeyPath(expr filter.Expr) (string, error) {
	keys, err := filterhelp.CollectKeyPath(expr)
	if err != nil {
		return "", err
	}
	if len(keys) == 0 {
		return "", errors.New("clickhouse: empty key path")
	}
	return strings.Join(keys, "."), nil
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
		return "", fmt.Errorf("clickhouse: unexpected comparison operator '%s'", kind.Name())
	}
}

func quoteSQLString(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
