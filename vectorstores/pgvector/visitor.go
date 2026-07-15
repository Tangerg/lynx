package pgvector

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/Tangerg/lynx/core/vectorstore/filter"
	"github.com/Tangerg/lynx/vectorstores/internal/filterhelp"
)

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
//   - indexed expression keeps the base identifier as the first path segment:
//     profile["author"] → metadata->'profile'->>'author'
//   - nested index — joined with -> for intermediate hops,
//     ->> only on the final step (since ->> casts to text):
//     profile["a"]["b"] → metadata->'profile'->'a'->>'b'
//
// Numeric / boolean values force a type cast on the JSON extraction so
// the comparison happens in the proper type, not lexicographic on text.
var _ filter.Visitor = (*Visitor)(nil)

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

func (v *Visitor) Visit(expr filter.Predicate) error {
	v.err = v.visit(expr)
	return v.err
}

func (v *Visitor) visit(expr filter.Expr) error {
	if expr == nil {
		return errors.New("pgvector: cannot process nil expression")
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
		return fmt.Errorf("pgvector: unsupported root expression type %T", node)
	}
}

func (v *Visitor) visitNotExpr(expr *filter.UnaryExpr) error {
	v.sql.WriteString("(NOT ")
	if err := v.visit(expr.Right); err != nil {
		return err
	}
	v.sql.WriteString(")")
	return nil
}

func (v *Visitor) visitLogicalExpr(expr *filter.BinaryExpr) error {
	op, err := filterhelp.LogicalOpString(expr.Op)
	if err != nil {
		return fmt.Errorf("pgvector: %w", err)
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

// visitComparisonExpr handles ==, !=, <, <=, >, >=. The JSON extraction
// expression on the left side is type-cast based on the value type:
// numbers → ::numeric, bools → ::boolean, strings → no cast.
func (v *Visitor) visitComparisonExpr(expr *filter.BinaryExpr) error {
	value, err := filterhelp.ExtractValue(expr.Right)
	if err != nil {
		return fmt.Errorf("pgvector: %w (at %s)", err, expr.Start().String())
	}

	jsonPath, err := buildJSONPath(expr.Left, v.metadataCol, comparisonCastFor(value, expr.Op))
	if err != nil {
		return fmt.Errorf("pgvector: %w (at %s)", err, expr.Start().String())
	}

	op, err := sqlOpFor(expr.Op)
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
func (v *Visitor) visitInExpr(expr *filter.BinaryExpr) error {
	listLit, err := filterhelp.RequireListLiteral(expr)
	if err != nil {
		return fmt.Errorf("pgvector: %w", err)
	}

	values, sample, err := filterhelp.ConvertListLiteral(listLit)
	if err != nil {
		return fmt.Errorf("pgvector: %w (at %s)", err, expr.Start().String())
	}

	jsonPath, err := buildJSONPath(expr.Left, v.metadataCol, comparisonCastFor(sample, filter.OpEqual))
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
func (v *Visitor) visitLikeExpr(expr *filter.BinaryExpr) error {
	pattern, err := filterhelp.RequireStringPatternOnRight(expr)
	if err != nil {
		return fmt.Errorf("pgvector: %w", err)
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

// visitNullTestExpr emits `(metadata->>'key' IS NULL)`. Postgres `->>`
// yields SQL NULL both when the key is absent and when the stored value
// is JSON null, matching the inmemory reference semantics. The negated
// `IS NOT NULL` arrives as NOT(… IS NULL) and is rendered by
// visitNotExpr, so no separate handling is needed here.
func (v *Visitor) visitNullTestExpr(expr *filter.BinaryExpr) error {
	jsonPath, err := buildJSONPath(expr.Left, v.metadataCol, castNone)
	if err != nil {
		return fmt.Errorf("pgvector: %w (at %s)", err, expr.Start().String())
	}
	v.sql.WriteString("(")
	v.sql.WriteString(jsonPath)
	v.sql.WriteString(" IS NULL)")
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

func comparisonCastFor(value any, op filter.Operator) jsonCast {
	switch value.(type) {
	case bool:
		return castBoolean
	case float64, int, int64, uint64:
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
func buildJSONPath(expr filter.Expr, metadataCol string, cast jsonCast) (string, error) {
	pathParts, err := filterhelp.CollectKeyPath(expr)
	if err != nil {
		return "", err
	}
	if len(pathParts) == 0 {
		return "", errors.New("empty key path on left operand")
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

func quoteSQLLiteral(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
