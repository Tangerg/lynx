package neo4j

import (
	"fmt"
	"strings"

	"github.com/Tangerg/lynx/core/vectorstore/filter/ast"
	"github.com/Tangerg/lynx/core/vectorstore/filter/token"
	"github.com/Tangerg/lynx/vectorstores/internal/filterhelp"
)

var _ ast.Visitor = (*Visitor)(nil)

// Visitor transforms AST filter expressions into a Cypher predicate
// string plus the matching parameter map. The output is intended to
// follow a `WHERE` clause in the search / delete statement.
//
// Output shape (using the default node alias "node" and metadata
// property prefix "metadata."):
//
//	author == "Alice"          →  node.`metadata.author` = $p1
//	year >= 2020               →  node.`metadata.year`   >= $p1
//	category IN ("a", "b")     →  node.`metadata.category` IN $p1
//	NOT (a == "x")             →  NOT (node.`metadata.a` = $p1)
//	a == "x" AND b == "y"      →  (node.`metadata.a` = $p1 AND node.`metadata.b` = $p2)
//
// Property paths follow Spring AI's convention: metadata keys are
// stored as flat node properties named `metadata.<key>` and addressed
// with backtick-quoted Cypher identifiers.
type Visitor struct {
	err            error
	sql            strings.Builder
	params         map[string]any
	nodeAlias      string
	metadataPrefix string
}

func NewVisitor(nodeAlias, metadataPrefix string) *Visitor {
	if nodeAlias == "" {
		nodeAlias = "node"
	}
	return &Visitor{
		nodeAlias:      nodeAlias,
		metadataPrefix: metadataPrefix,
		params:         make(map[string]any),
	}
}

func (v *Visitor) Result() (string, map[string]any) {
	if v.err != nil {
		return "", nil
	}
	return v.sql.String(), v.params
}

func (v *Visitor) Error() error { return v.err }

func (v *Visitor) Visit(expr ast.Expr) ast.Visitor {
	v.err = v.visit(expr)
	return nil
}

func (v *Visitor) visit(expr ast.Expr) error {
	if expr == nil {
		return fmt.Errorf("neo4j: cannot process nil expression")
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
		return fmt.Errorf("neo4j: unsupported root expression %T", node)
	}
}

func (v *Visitor) visitBinaryExpr(expr *ast.BinaryExpr) error {
	switch {
	case expr.Op.Kind.IsNullOperator():
		return v.visitNullTestExpr(expr)
	case expr.Op.Kind.IsLogicalOperator():
		return v.visitLogicalExpr(expr)
	case expr.Op.Kind.Is(token.IN):
		return v.visitInExpr(expr)
	case expr.Op.Kind.Is(token.LIKE):
		return v.visitLikeExpr(expr)
	case expr.Op.Kind.IsEqualityOperator() || expr.Op.Kind.IsOrderingOperator():
		return v.visitComparisonExpr(expr)
	default:
		return fmt.Errorf("neo4j: unsupported binary operator '%s' at %s",
			expr.Op.Literal, expr.Start().String())
	}
}

func (v *Visitor) visitUnaryExpr(expr *ast.UnaryExpr) error {
	if !expr.Op.Kind.Is(token.NOT) {
		return fmt.Errorf("neo4j: unsupported unary operator '%s' at %s",
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
	prop, err := v.propertyAccess(expr.Left)
	if err != nil {
		return fmt.Errorf("neo4j: %w (at %s)", err, expr.Start().String())
	}
	value, err := filterhelp.ExtractValue(expr.Right)
	if err != nil {
		return fmt.Errorf("neo4j: %w (at %s)", err, expr.Start().String())
	}
	op, err := cypherOpFor(expr.Op.Kind)
	if err != nil {
		return err
	}

	param := v.bindParam(value)
	v.sql.WriteString(prop)
	v.sql.WriteByte(' ')
	v.sql.WriteString(op)
	v.sql.WriteByte(' ')
	v.sql.WriteString(param)
	return nil
}

func (v *Visitor) visitInExpr(expr *ast.BinaryExpr) error {
	prop, err := v.propertyAccess(expr.Left)
	if err != nil {
		return fmt.Errorf("neo4j: %w (at %s)", err, expr.Start().String())
	}

	listLit, ok := expr.Right.(*ast.ListLiteral)
	if !ok {
		return fmt.Errorf("neo4j: 'IN' requires a list on the right at %s, got %T",
			expr.Start().String(), expr.Right)
	}
	if len(listLit.Values) == 0 {
		return fmt.Errorf("neo4j: 'IN' requires a non-empty list at %s",
			expr.Start().String())
	}

	values := make([]any, 0, len(listLit.Values))
	for _, lit := range listLit.Values {
		val, err := filterhelp.LiteralToValue(lit)
		if err != nil {
			return fmt.Errorf("neo4j: %w (at %s)", err, expr.Start().String())
		}
		values = append(values, val)
	}

	param := v.bindParam(values)
	v.sql.WriteString(prop)
	v.sql.WriteString(" IN ")
	v.sql.WriteString(param)
	return nil
}

// visitLikeExpr maps LIKE onto Cypher's regex operator =~. SQL
// wildcards translate to regex equivalents and the match is anchored.
func (v *Visitor) visitLikeExpr(expr *ast.BinaryExpr) error {
	prop, err := v.propertyAccess(expr.Left)
	if err != nil {
		return fmt.Errorf("neo4j: %w (at %s)", err, expr.Start().String())
	}
	value, err := filterhelp.ExtractValue(expr.Right)
	if err != nil {
		return fmt.Errorf("neo4j: %w (at %s)", err, expr.Start().String())
	}
	pattern, ok := value.(string)
	if !ok {
		return fmt.Errorf("neo4j: LIKE requires a string pattern, got %T at %s",
			value, expr.Start().String())
	}

	var b strings.Builder
	b.Grow(len(pattern) + 2)
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

	param := v.bindParam(b.String())
	v.sql.WriteString(prop)
	v.sql.WriteString(" =~ ")
	v.sql.WriteString(param)
	return nil
}

// visitNullTestExpr emits `(node.`+"`metadata.key`"+` IS NULL)`. Cypher's
// IS NULL is true both when the property is absent and when it is
// explicitly null, matching the inmemory reference semantics. The
// negated `IS NOT NULL` arrives as NOT(… IS NULL) and is rendered by
// visitUnaryExpr as `NOT (… IS NULL)`, which Cypher treats as
// equivalent, so no separate handling is needed here. No bound
// parameter — `IS NULL` is inline in Cypher.
func (v *Visitor) visitNullTestExpr(expr *ast.BinaryExpr) error {
	prop, err := v.propertyAccess(expr.Left)
	if err != nil {
		return fmt.Errorf("neo4j: %w (at %s)", err, expr.Start().String())
	}
	v.sql.WriteString("(")
	v.sql.WriteString(prop)
	v.sql.WriteString(" IS NULL)")
	return nil
}

// propertyAccess assembles the Cypher property accessor for the left
// side of a comparison, e.g. “node.`metadata.foo` “.
func (v *Visitor) propertyAccess(expr ast.Expr) (string, error) {
	keys, err := filterhelp.CollectKeyPath(expr)
	if err != nil {
		return "", err
	}
	if len(keys) == 0 {
		return "", fmt.Errorf("empty key path on left operand")
	}
	propName := strings.Join(keys, ".")
	if v.metadataPrefix != "" {
		propName = v.metadataPrefix + "." + propName
	}
	// Backtick-quote so dotted property names survive the Cypher
	// parser. A literal backtick inside the identifier is doubled
	// per Cypher's escaping rules.
	escaped := strings.ReplaceAll(propName, "`", "``")
	return v.nodeAlias + ".`" + escaped + "`", nil
}

func cypherOpFor(kind token.Kind) (string, error) {
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
		return "", fmt.Errorf("neo4j: unexpected comparison operator '%s'", kind.Name())
	}
}

func (v *Visitor) bindParam(value any) string {
	name := fmt.Sprintf("p%d", len(v.params)+1)
	v.params[name] = value
	return "$" + name
}
