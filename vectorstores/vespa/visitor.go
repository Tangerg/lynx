package vespa

import (
	"fmt"
	"strings"
	"strconv"

	"github.com/Tangerg/lynx/core/vectorstore/filter/ast"
	"github.com/Tangerg/lynx/vectorstores/internal/filterhelp"
	"github.com/Tangerg/lynx/core/vectorstore/filter/token"
)

var _ ast.Visitor = (*Visitor)(nil)

// Visitor transforms AST filter expressions into a Vespa YQL `where`
// clause. The metadata fields must be declared in the Vespa schema
// (sd file) — Vespa addresses them as flat top-level attributes.
//
// Output shape (when `metadataPrefix` is empty):
//
//	author == "Alice"        →  author contains "Alice"
//	year >= 2020             →  year >= 2020
//	tag IN ("a", "b")        →  tag in ("a", "b")
//	NOT (year >= 2020)       →  !(year >= 2020)
type Visitor struct {
	err            error
	sql            strings.Builder
	metadataPrefix string
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
		return fmt.Errorf("vespa: cannot process nil expression")
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
		return fmt.Errorf("vespa: unsupported root expression %T", node)
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
		return fmt.Errorf("vespa: unsupported binary operator '%s'", expr.Op.Literal)
	}
}

func (v *Visitor) visitUnaryExpr(expr *ast.UnaryExpr) error {
	if !expr.Op.Kind.Is(token.NOT) {
		return fmt.Errorf("vespa: unsupported unary '%s'", expr.Op.Literal)
	}
	v.sql.WriteString("!(")
	if err := v.visit(expr.Right); err != nil {
		return err
	}
	v.sql.WriteString(")")
	return nil
}

func (v *Visitor) visitLogicalExpr(expr *ast.BinaryExpr) error {
	op := " and "
	if expr.Op.Kind.Is(token.OR) {
		op = " or "
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
		return err
	}
	value, err := filterhelp.ExtractValue(expr.Right)
	if err != nil {
		return err
	}

	// String equality maps onto YQL `contains`; ordering / non-eq
	// numeric ops use the standard relational operators.
	if _, isString := value.(string); isString && expr.Op.Kind.Is(token.EQ) {
		v.sql.WriteString(field)
		v.sql.WriteString(" contains ")
		v.sql.WriteString(yqlLiteral(value))
		return nil
	}
	if _, isString := value.(string); isString && expr.Op.Kind.Is(token.NE) {
		v.sql.WriteString("!(")
		v.sql.WriteString(field)
		v.sql.WriteString(" contains ")
		v.sql.WriteString(yqlLiteral(value))
		v.sql.WriteString(")")
		return nil
	}

	op, err := yqlOpFor(expr.Op.Kind)
	if err != nil {
		return err
	}
	v.sql.WriteString(field)
	v.sql.WriteByte(' ')
	v.sql.WriteString(op)
	v.sql.WriteByte(' ')
	v.sql.WriteString(yqlLiteral(value))
	return nil
}

func (v *Visitor) visitInExpr(expr *ast.BinaryExpr) error {
	field, err := v.fieldPath(expr.Left)
	if err != nil {
		return err
	}
	listLit, ok := expr.Right.(*ast.ListLiteral)
	if !ok {
		return fmt.Errorf("vespa: 'IN' requires a list on the right")
	}
	if len(listLit.Values) == 0 {
		return fmt.Errorf("vespa: 'IN' requires a non-empty list")
	}
	parts := make([]string, 0, len(listLit.Values))
	for _, lit := range listLit.Values {
		val, err := filterhelp.LiteralToValue(lit)
		if err != nil {
			return err
		}
		parts = append(parts, yqlLiteral(val))
	}
	v.sql.WriteString(field)
	v.sql.WriteString(" in (")
	v.sql.WriteString(strings.Join(parts, ", "))
	v.sql.WriteByte(')')
	return nil
}

// visitLikeExpr maps SQL LIKE onto YQL `matches` (regex). `%` and
// `_` translate to `.*` / `.` respectively.
func (v *Visitor) visitLikeExpr(expr *ast.BinaryExpr) error {
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
		return fmt.Errorf("vespa: LIKE requires a string pattern, got %T", value)
	}
	var b strings.Builder
	for _, r := range pattern {
		switch r {
		case '%':
			b.WriteString(".*")
		case '_':
			b.WriteByte('.')
		case '.', '+', '*', '?', '(', ')', '[', ']', '{', '}', '|', '^', '$', '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}
	v.sql.WriteString(field)
	v.sql.WriteString(" matches ")
	v.sql.WriteString(yqlLiteral(b.String()))
	return nil
}

func (v *Visitor) fieldPath(expr ast.Expr) (string, error) {
	keys, err := filterhelp.CollectKeyPath(expr)
	if err != nil {
		return "", err
	}
	if len(keys) == 0 {
		return "", fmt.Errorf("empty key path")
	}
	joined := strings.Join(keys, ".")
	if v.metadataPrefix == "" {
		return joined, nil
	}
	return v.metadataPrefix + "." + joined, nil
}


func yqlOpFor(kind token.Kind) (string, error) {
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
		return "", fmt.Errorf("vespa: unexpected operator '%s'", kind.Name())
	}
}

func yqlLiteral(v any) string {
	switch val := v.(type) {
	case string:
		return `"` + strings.ReplaceAll(val, `"`, `\"`) + `"`
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
