package azurecosmos

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/lynx/core/vectorstore/filter"
	"github.com/Tangerg/lynx/vectorstores/internal/filterhelp"
)

// Visitor transforms AST filter expressions into a Cosmos DB SQL
// predicate fragment. Metadata keys live under c.metadata.* by default
// (the document alias used in Search / DeleteWhere is `c`).
//
// Output shape:
//
//	author == "Alice"        →  c.metadata.author = @p1
//	year >= 2020             →  c.metadata.year >= @p1
//	category IN ("a", "b")   →  c.metadata.category IN (@p1, @p2)
//	NOT (a == "x")           →  NOT (c.metadata.a = @p1)
//	a == "x" AND b == "y"    →  (c.metadata.a = @p1 AND c.metadata.b = @p2)
type Visitor struct {
	err            error
	sql            strings.Builder
	params         []NamedParam
	alias          string
	metadataPrefix string
}

// NamedParam pairs a `@N`-style placeholder with its value. Cosmos
// SDK uses named parameters via QueryParameters.
type NamedParam struct {
	Name  string
	Value any
}

func NewVisitor(alias, metadataPrefix string) *Visitor {
	if alias == "" {
		alias = "c"
	}
	return &Visitor{alias: alias, metadataPrefix: metadataPrefix}
}

func (v *Visitor) Result() (string, []NamedParam) {
	if v.err != nil {
		return "", nil
	}
	return v.sql.String(), v.params
}

func (v *Visitor) Visit(expr filter.Expr) error {
	v.err = v.visit(expr)
	return v.err
}

func (v *Visitor) visit(expr filter.Expr) error {
	if expr == nil {
		return errors.New("azurecosmos: cannot process nil expression")
	}
	if v.err != nil {
		return v.err
	}
	switch node := expr.(type) {
	case *filter.BinaryExpr:
		return v.visitBinaryExpr(node)
	case *filter.UnaryExpr:
		return v.visitUnaryExpr(node)
	default:
		return fmt.Errorf("azurecosmos: unsupported root expression %T", node)
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
		return fmt.Errorf("azurecosmos: unsupported binary operator '%s'", expr.Op.String())
	}
}

func (v *Visitor) visitUnaryExpr(expr *filter.UnaryExpr) error {
	if !expr.Op.Is(filter.OpNot) {
		return fmt.Errorf("azurecosmos: unsupported unary '%s'", expr.Op.String())
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
	field, err := v.fieldPath(expr.Left)
	if err != nil {
		return err
	}
	value, err := filterhelp.ExtractValue(expr.Right)
	if err != nil {
		return err
	}
	op, err := sqlOpFor(expr.Op)
	if err != nil {
		return err
	}
	param := v.bindParam(value)
	v.sql.WriteString(field)
	v.sql.WriteByte(' ')
	v.sql.WriteString(op)
	v.sql.WriteByte(' ')
	v.sql.WriteString(param)
	return nil
}

func (v *Visitor) visitInExpr(expr *filter.BinaryExpr) error {
	field, err := v.fieldPath(expr.Left)
	if err != nil {
		return err
	}
	listLit, ok := expr.Right.(*filter.ListLiteral)
	if !ok {
		return errors.New("azurecosmos: 'IN' requires a list on the right")
	}
	if len(listLit.Values) == 0 {
		return errors.New("azurecosmos: 'IN' requires a non-empty list")
	}

	v.sql.WriteString(field)
	v.sql.WriteString(" IN (")
	for i, lit := range listLit.Values {
		val, err := filterhelp.LiteralToValue(lit)
		if err != nil {
			return err
		}
		if i > 0 {
			v.sql.WriteString(", ")
		}
		v.sql.WriteString(v.bindParam(val))
	}
	v.sql.WriteByte(')')
	return nil
}

// visitLikeExpr maps LIKE onto Cosmos SQL's CONTAINS function — SQL
// wildcards `%` translate to the open-ended CONTAINS semantics. For
// fuller pattern support callers should preprocess the value.
func (v *Visitor) visitLikeExpr(expr *filter.BinaryExpr) error {
	field, err := v.fieldPath(expr.Left)
	if err != nil {
		return err
	}
	value, err := filterhelp.ExtractValue(expr.Right)
	if err != nil {
		return err
	}
	pattern, ok := value.(string)
	if !ok {
		return fmt.Errorf("azurecosmos: LIKE requires a string pattern, got %T", value)
	}
	// Strip leading/trailing % for a substring match.
	pattern = strings.TrimPrefix(pattern, "%")
	pattern = strings.TrimSuffix(pattern, "%")
	param := v.bindParam(pattern)
	v.sql.WriteString("CONTAINS(")
	v.sql.WriteString(field)
	v.sql.WriteString(", ")
	v.sql.WriteString(param)
	v.sql.WriteString(")")
	return nil
}

func (v *Visitor) fieldPath(expr filter.Expr) (string, error) {
	keys, err := filterhelp.CollectKeyPath(expr)
	if err != nil {
		return "", err
	}
	if len(keys) == 0 {
		return "", errors.New("empty key path")
	}
	parts := []string{v.alias}
	if v.metadataPrefix != "" {
		parts = append(parts, v.metadataPrefix)
	}
	parts = append(parts, keys...)
	return strings.Join(parts, "."), nil
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
		return "", fmt.Errorf("azurecosmos: unexpected operator '%s'", kind.Name())
	}
}

func (v *Visitor) bindParam(value any) string {
	name := fmt.Sprintf("@p%d", len(v.params)+1)
	v.params = append(v.params, NamedParam{Name: name, Value: value})
	return name
}
