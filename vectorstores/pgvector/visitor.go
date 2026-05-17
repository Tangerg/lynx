package pgvector

import (
	"fmt"
	"strings"
	"strconv"


	"github.com/Tangerg/lynx/core/vectorstore/filter/ast"
	"github.com/Tangerg/lynx/vectorstores/internal/filterhelp"
	"github.com/Tangerg/lynx/core/vectorstore/filter/token"
)

var _ ast.Visitor = (*Visitor)(nil)

// Visitor transforms AST filter expressions into a parameterized
// PostgreSQL WHERE-clause fragment plus the matching argument list.
//
// Output shape (using the default metadata column "metadata"):
//
//	author == "Alice"        →  (metadata->>'author' = $1)
//	year >= 2020             →  ((metadata->>'year')::numeric >= $1)
//	published == true        →  ((metadata->>'published')::boolean = $1)
//	tag IN ("rag","llm")     →  (metadata->>'tag' = ANY($1))
//	NOT (a == 1)             →  (NOT (metadata->>'a')::numeric = $1)
//
// Identifier paths:
//   - simple identifier — used as the top-level metadata key:
//     author → metadata->>'author'
//   - indexed expression strips the base identifier:
//     metadata["author"] → metadata->>'author'
//   - nested index — joined with -> for intermediate hops,
//     ->> only on the final step (since ->> casts to text):
//     metadata["a"]["b"] → metadata->'a'->>'b'
//
// Numeric / boolean values force a type cast on the JSON extraction so
// the comparison happens in the proper type, not lexicographic on text.
type Visitor struct {
	err         error
	sql         strings.Builder
	args        []any
	metadataCol string // SQL identifier — already validated by the caller
}


func NewVisitor(metadataCol string) *Visitor {
	if metadataCol == "" {
		metadataCol = "metadata"
	}
	return &Visitor{metadataCol: metadataCol}
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
		return fmt.Errorf("pgvector: cannot process nil expression")
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
		return fmt.Errorf("pgvector: unsupported root expression type %T", node)
	}
}

func (v *Visitor) visitBinaryExpr(expr *ast.BinaryExpr) error {
	switch {
	case expr.Op.Kind.IsLogicalOperator():
		return v.visitLogicalExpr(expr)
	case expr.Op.Kind.IsEqualityOperator():
		return v.visitComparisonExpr(expr)
	case expr.Op.Kind.IsOrderingOperator():
		return v.visitComparisonExpr(expr)
	case expr.Op.Kind.Is(token.IN):
		return v.visitInExpr(expr)
	case expr.Op.Kind.Is(token.LIKE):
		return v.visitLikeExpr(expr)
	default:
		return fmt.Errorf("pgvector: unsupported binary operator '%s' at %s",
			expr.Op.Literal, expr.Start().String())
	}
}

func (v *Visitor) visitUnaryExpr(expr *ast.UnaryExpr) error {
	if !expr.Op.Kind.Is(token.NOT) {
		return fmt.Errorf("pgvector: unsupported unary operator '%s' at %s",
			expr.Op.Literal, expr.Start().String())
	}

	v.sql.WriteString("(NOT ")
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

// visitComparisonExpr handles ==, !=, <, <=, >, >=. The JSON extraction
// expression on the left side is type-cast based on the value type:
// numbers → ::numeric, bools → ::boolean, strings → no cast.
func (v *Visitor) visitComparisonExpr(expr *ast.BinaryExpr) error {
	value, err := filterhelp.ExtractValue(expr.Right)
	if err != nil {
		return fmt.Errorf("pgvector: %w (at %s)", err, expr.Start().String())
	}

	jsonPath, err := buildJSONPath(expr.Left, v.metadataCol, comparisonCastFor(value, expr.Op.Kind))
	if err != nil {
		return fmt.Errorf("pgvector: %w (at %s)", err, expr.Start().String())
	}

	op, err := sqlOpFor(expr.Op.Kind)
	if err != nil {
		return err
	}

	v.args = append(v.args, value)
	v.sql.WriteString("(")
	v.sql.WriteString(jsonPath)
	v.sql.WriteString(" ")
	v.sql.WriteString(op)
	v.sql.WriteString(" $")
	v.sql.WriteString(strconv.Itoa(len(v.args)))
	v.sql.WriteString(")")
	return nil
}

// visitInExpr emits `key = ANY($N)` with a slice argument. Element type
// follows the literal type — pgx maps Go slices to a Postgres array of
// the matching type.
func (v *Visitor) visitInExpr(expr *ast.BinaryExpr) error {
	listLit, ok := expr.Right.(*ast.ListLiteral)
	if !ok {
		return fmt.Errorf("pgvector: 'IN' requires a list on the right at %s, got %T",
			expr.Start().String(), expr.Right)
	}
	if len(listLit.Values) == 0 {
		return fmt.Errorf("pgvector: 'IN' requires a non-empty list at %s",
			expr.Start().String())
	}

	values, sample, err := convertListLiteral(listLit)
	if err != nil {
		return fmt.Errorf("pgvector: %w (at %s)", err, expr.Start().String())
	}

	jsonPath, err := buildJSONPath(expr.Left, v.metadataCol, comparisonCastFor(sample, token.EQ))
	if err != nil {
		return fmt.Errorf("pgvector: %w (at %s)", err, expr.Start().String())
	}

	v.args = append(v.args, values)
	v.sql.WriteString("(")
	v.sql.WriteString(jsonPath)
	v.sql.WriteString(" = ANY($")
	v.sql.WriteString(strconv.Itoa(len(v.args)))
	v.sql.WriteString("))")
	return nil
}

// visitLikeExpr emits a SQL ILIKE so callers get the case-insensitive
// pattern-match that most filter DSLs assume. Right side must be a
// string literal.
func (v *Visitor) visitLikeExpr(expr *ast.BinaryExpr) error {
	value, err := filterhelp.ExtractValue(expr.Right)
	if err != nil {
		return fmt.Errorf("pgvector: %w (at %s)", err, expr.Start().String())
	}
	pattern, ok := value.(string)
	if !ok {
		return fmt.Errorf("pgvector: 'LIKE' requires a string pattern, got %T at %s",
			value, expr.Start().String())
	}

	jsonPath, err := buildJSONPath(expr.Left, v.metadataCol, castNone)
	if err != nil {
		return fmt.Errorf("pgvector: %w (at %s)", err, expr.Start().String())
	}

	v.args = append(v.args, pattern)
	v.sql.WriteString("(")
	v.sql.WriteString(jsonPath)
	v.sql.WriteString(" ILIKE $")
	v.sql.WriteString(strconv.Itoa(len(v.args)))
	v.sql.WriteString(")")
	return nil
}

// jsonCast names the Postgres type cast applied to the JSON
// extraction. castNone returns the raw text from ->>.
type jsonCast int

const (
	castNone jsonCast = iota
	castNumeric
	castBoolean
)

func comparisonCastFor(value any, op token.Kind) jsonCast {
	switch value.(type) {
	case bool:
		return castBoolean
	case float64, int, int64:
		return castNumeric
	default:
		// Ordering on non-numeric values still falls back to a
		// numeric cast — the user asked for an ordering comparison,
		// so coerce.
		if op.IsOrderingOperator() {
			return castNumeric
		}
		return castNone
	}
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
		return "", fmt.Errorf("pgvector: unexpected comparison operator '%s'", kind.Name())
	}
}

// buildJSONPath turns the left-side expression of a comparison into
// the metadata accessor.
//
//	ident            → metadata->>'ident'
//	metadata['k']    → metadata->>'k'
//	metadata['a']['b'] → metadata->'a'->>'b'
//
// For numeric / boolean comparisons the trailing ->> is wrapped in a
// type cast.
func buildJSONPath(expr ast.Expr, metadataCol string, cast jsonCast) (string, error) {
	pathParts, err := filterhelp.CollectKeyPath(expr)
	if err != nil {
		return "", err
	}
	if len(pathParts) == 0 {
		return "", fmt.Errorf("empty key path on left operand")
	}

	var b strings.Builder
	if cast != castNone {
		b.WriteString("(")
	}
	b.WriteString(metadataCol)

	for i, key := range pathParts {
		if i == len(pathParts)-1 {
			b.WriteString("->>")
		} else {
			b.WriteString("->")
		}
		b.WriteString(quoteSQLLiteral(key))
	}

	if cast != castNone {
		b.WriteString(")")
		switch cast {
		case castNumeric:
			b.WriteString("::numeric")
		case castBoolean:
			b.WriteString("::boolean")
		}
	}
	return b.String(), nil
}

// collectKeyPath walks the left operand of a comparison and returns
// the metadata sub-path it addresses, stripping a leading
// "metadataCol" identifier when present.

// convertListLiteral builds the typed slice passed to ANY($N) for IN
// expressions. The element type is decided by the first literal —
// pgx maps the slice straight to a Postgres array of that type.
func convertListLiteral(list *ast.ListLiteral) (any, any, error) {
	first := list.Values[0]
	switch {
	case first.IsString():
		out := make([]string, 0, len(list.Values))
		for _, lit := range list.Values {
			s, err := lit.AsString()
			if err != nil {
				return nil, nil, err
			}
			out = append(out, s)
		}
		return out, out[0], nil
	case first.IsNumber():
		out := make([]float64, 0, len(list.Values))
		for _, lit := range list.Values {
			n, err := lit.AsNumber()
			if err != nil {
				return nil, nil, err
			}
			out = append(out, n)
		}
		return out, out[0], nil
	case first.IsBool():
		out := make([]bool, 0, len(list.Values))
		for _, lit := range list.Values {
			b, err := lit.AsBool()
			if err != nil {
				return nil, nil, err
			}
			out = append(out, b)
		}
		return out, out[0], nil
	default:
		return nil, nil, fmt.Errorf("unsupported list element kind %s", first.Token.Kind.Name())
	}
}

func quoteSQLLiteral(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
