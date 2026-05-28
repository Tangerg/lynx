package cassandra

import (
	"fmt"
	"strings"

	"github.com/Tangerg/lynx/core/vectorstore/filter/ast"
	"github.com/Tangerg/lynx/core/vectorstore/filter/token"
	"github.com/Tangerg/lynx/vectorstores/internal/filterhelp"
)

var _ ast.Visitor = (*Visitor)(nil)

// Visitor transforms AST filter expressions into a CQL WHERE
// fragment. Each metadata key must map to an actual indexed column on
// the underlying table — Cassandra has no JSON-path operator, so
// filters reference columns directly.
//
// Output shape:
//
//	author == "Alice"          →  "author" = ?
//	year >= 2020               →  "year" >= ?
//	tag IN ("a", "b")          →  "tag" IN ?
//
// IN values are bound as a single slice parameter so callers can pass
// `[]string{"a", "b"}` straight through.
type Visitor struct {
	err  error
	sql  strings.Builder
	args []any
}

func NewVisitor() *Visitor { return &Visitor{} }

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
		return fmt.Errorf("cassandra: cannot process nil expression")
	}
	if v.err != nil {
		return v.err
	}

	switch node := expr.(type) {
	case *ast.BinaryExpr:
		return v.visitBinaryExpr(node)
	case *ast.UnaryExpr:
		return fmt.Errorf("cassandra: NOT is not supported by CQL on metadata columns")
	default:
		return fmt.Errorf("cassandra: unsupported root expression %T", node)
	}
}

func (v *Visitor) visitBinaryExpr(expr *ast.BinaryExpr) error {
	switch {
	case expr.Op.Kind.IsLogicalOperator():
		if expr.Op.Kind.Is(token.OR) {
			// CQL doesn't support OR on regular columns; SAI indexes
			// can do it via composite predicates but it's a special
			// case best handled by the caller.
			return fmt.Errorf("cassandra: OR is not supported in CQL WHERE clauses")
		}
		return v.visitAnd(expr)
	case expr.Op.Kind.Is(token.IN):
		return v.visitInExpr(expr)
	case expr.Op.Kind.IsEqualityOperator() || expr.Op.Kind.IsOrderingOperator():
		return v.visitComparisonExpr(expr)
	default:
		return fmt.Errorf("cassandra: unsupported binary operator '%s' at %s",
			expr.Op.Literal, expr.Start().String())
	}
}

func (v *Visitor) visitAnd(expr *ast.BinaryExpr) error {
	if err := v.visit(expr.Left); err != nil {
		return err
	}
	v.sql.WriteString(" AND ")
	return v.visit(expr.Right)
}

func (v *Visitor) visitComparisonExpr(expr *ast.BinaryExpr) error {
	column, err := columnName(expr.Left)
	if err != nil {
		return fmt.Errorf("cassandra: %w (at %s)", err, expr.Start().String())
	}
	value, err := filterhelp.ExtractValue(expr.Right)
	if err != nil {
		return fmt.Errorf("cassandra: %w (at %s)", err, expr.Start().String())
	}
	op, err := cqlOpFor(expr.Op.Kind)
	if err != nil {
		return err
	}

	v.sql.WriteString(quoteIdentifier(column))
	v.sql.WriteByte(' ')
	v.sql.WriteString(op)
	v.sql.WriteString(" ?")
	v.args = append(v.args, value)
	return nil
}

func (v *Visitor) visitInExpr(expr *ast.BinaryExpr) error {
	column, err := columnName(expr.Left)
	if err != nil {
		return fmt.Errorf("cassandra: %w (at %s)", err, expr.Start().String())
	}

	listLit, ok := expr.Right.(*ast.ListLiteral)
	if !ok {
		return fmt.Errorf("cassandra: 'IN' requires a list on the right at %s, got %T",
			expr.Start().String(), expr.Right)
	}
	if len(listLit.Values) == 0 {
		return fmt.Errorf("cassandra: 'IN' requires a non-empty list at %s",
			expr.Start().String())
	}

	values, err := listToTypedSlice(listLit)
	if err != nil {
		return fmt.Errorf("cassandra: %w (at %s)", err, expr.Start().String())
	}

	v.sql.WriteString(quoteIdentifier(column))
	v.sql.WriteString(" IN ?")
	v.args = append(v.args, values)
	return nil
}

// columnName extracts the (single) column name from the left operand.
// Cassandra filters work on flat indexed columns — there's no JSON
// access — so an [ast.IndexExpr] is rejected.
func columnName(expr ast.Expr) (string, error) {
	switch node := expr.(type) {
	case *ast.Ident:
		return node.Value, nil
	case *ast.IndexExpr:
		return "", fmt.Errorf("indexed expressions are not supported — declare the metadata key as a column")
	default:
		return "", fmt.Errorf("unsupported left operand %T", node)
	}
}

// listToTypedSlice promotes the literal list to a Go slice typed by
// the first element. gocql binds typed slices to `IN ?` parameters.
func listToTypedSlice(list *ast.ListLiteral) (any, error) {
	first := list.Values[0]
	switch {
	case first.IsString():
		out := make([]string, 0, len(list.Values))
		for _, lit := range list.Values {
			s, err := lit.AsString()
			if err != nil {
				return nil, err
			}
			out = append(out, s)
		}
		return out, nil
	case first.IsNumber():
		// Use float64 to preserve fractional literals; gocql will
		// down-cast as needed via the column's reported type.
		out := make([]float64, 0, len(list.Values))
		allInt := true
		for _, lit := range list.Values {
			n, err := lit.AsNumber()
			if err != nil {
				return nil, err
			}
			out = append(out, n)
			if float64(int64(n)) != n {
				allInt = false
			}
		}
		if allInt {
			ints := make([]int64, 0, len(out))
			for _, n := range out {
				ints = append(ints, int64(n))
			}
			return ints, nil
		}
		return out, nil
	case first.IsBool():
		out := make([]bool, 0, len(list.Values))
		for _, lit := range list.Values {
			b, err := lit.AsBool()
			if err != nil {
				return nil, err
			}
			out = append(out, b)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unsupported list element kind %s", first.Token.Kind.Name())
	}
}

func cqlOpFor(kind token.Kind) (string, error) {
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
		return "", fmt.Errorf("unexpected operator '%s'", kind.Name())
	}
}

func quoteIdentifier(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}
