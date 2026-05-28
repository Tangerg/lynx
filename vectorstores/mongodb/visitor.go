package mongodb

import (
	"fmt"
	"strings"

	"github.com/Tangerg/lynx/core/vectorstore/filter/ast"
	"github.com/Tangerg/lynx/core/vectorstore/filter/token"
	"github.com/Tangerg/lynx/vectorstores/internal/filterhelp"
)

var _ ast.Visitor = (*Visitor)(nil)

// Visitor transforms AST filter expressions into the MongoDB query
// document shape used by Atlas Vector Search's `filter` clause.
//
// Output shape (MongoDB query language, keyed on metadata.* paths):
//
//	author == "Alice"             →  {"metadata.author": {"$eq": "Alice"}}
//	year >= 2020                  →  {"metadata.year":   {"$gte": 2020}}
//	category IN ("a", "b")        →  {"metadata.category": {"$in": ["a","b"]}}
//	NOT (year >= 2020)            →  {"$nor": [{"metadata.year": {"$gte": 2020}}]}
//	a == "x" AND b == "y"         →  {"$and": [{"metadata.a": {"$eq":"x"}},
//	                                            {"metadata.b": {"$eq":"y"}}]}
//	a == "x" OR b == "y"          →  {"$or":  […]}
//
// Identifier paths:
//   - bare identifier      → <metadataPrefix>.<ident>
//   - metadata['k']        → <metadataPrefix>.k
//   - metadata['a']['b']   → <metadataPrefix>.a.b
type Visitor struct {
	err            error
	result         map[string]any
	metadataPrefix string // typically "metadata"
}

func NewVisitor(metadataPrefix string) *Visitor {
	return &Visitor{metadataPrefix: metadataPrefix}
}

func (v *Visitor) Result() map[string]any {
	if v.err != nil {
		return nil
	}
	return v.result
}

func (v *Visitor) Error() error { return v.err }

func (v *Visitor) Visit(expr ast.Expr) ast.Visitor {
	doc, err := v.translate(expr)
	v.err = err
	v.result = doc
	return nil
}

// translate is the recursive worker — it builds and returns the
// MongoDB sub-document for one expression, leaving the receiver state
// untouched. This avoids the stateful "currentField" shuffle the other
// visitors need.
func (v *Visitor) translate(expr ast.Expr) (map[string]any, error) {
	if expr == nil {
		return nil, fmt.Errorf("mongodb: cannot process nil expression")
	}

	switch node := expr.(type) {
	case *ast.BinaryExpr:
		return v.translateBinary(node)
	case *ast.UnaryExpr:
		return v.translateUnary(node)
	default:
		return nil, fmt.Errorf("mongodb: unsupported root expression %T at %s",
			node, expr.Start().String())
	}
}

func (v *Visitor) translateBinary(expr *ast.BinaryExpr) (map[string]any, error) {
	switch {
	case expr.Op.Kind.IsLogicalOperator():
		return v.translateLogical(expr)
	case expr.Op.Kind.Is(token.IN):
		return v.translateIn(expr, "$in")
	case expr.Op.Kind.Is(token.LIKE):
		return v.translateLike(expr)
	case expr.Op.Kind.IsEqualityOperator() || expr.Op.Kind.IsOrderingOperator():
		return v.translateComparison(expr)
	default:
		return nil, fmt.Errorf("mongodb: unsupported binary operator '%s' at %s",
			expr.Op.Literal, expr.Start().String())
	}
}

func (v *Visitor) translateUnary(expr *ast.UnaryExpr) (map[string]any, error) {
	if !expr.Op.Kind.Is(token.NOT) {
		return nil, fmt.Errorf("mongodb: unsupported unary operator '%s' at %s",
			expr.Op.Literal, expr.Start().String())
	}
	inner, err := v.translate(expr.Right)
	if err != nil {
		return nil, err
	}
	// MongoDB has no top-level $not — $nor over a single-element array
	// is the idiomatic equivalent for "match documents that do NOT
	// satisfy this sub-expression".
	return map[string]any{"$nor": []any{inner}}, nil
}

func (v *Visitor) translateLogical(expr *ast.BinaryExpr) (map[string]any, error) {
	left, err := v.translate(expr.Left)
	if err != nil {
		return nil, err
	}
	right, err := v.translate(expr.Right)
	if err != nil {
		return nil, err
	}
	op := "$and"
	if expr.Op.Kind.Is(token.OR) {
		op = "$or"
	}
	return map[string]any{op: []any{left, right}}, nil
}

func (v *Visitor) translateComparison(expr *ast.BinaryExpr) (map[string]any, error) {
	field, err := v.fieldPath(expr.Left)
	if err != nil {
		return nil, fmt.Errorf("mongodb: %w (at %s)", err, expr.Start().String())
	}
	value, err := filterhelp.ExtractValue(expr.Right)
	if err != nil {
		return nil, fmt.Errorf("mongodb: %w (at %s)", err, expr.Start().String())
	}
	op, err := mongoOpFor(expr.Op.Kind)
	if err != nil {
		return nil, err
	}
	return map[string]any{field: map[string]any{op: value}}, nil
}

func (v *Visitor) translateIn(expr *ast.BinaryExpr, op string) (map[string]any, error) {
	field, err := v.fieldPath(expr.Left)
	if err != nil {
		return nil, fmt.Errorf("mongodb: %w (at %s)", err, expr.Start().String())
	}

	listLit, ok := expr.Right.(*ast.ListLiteral)
	if !ok {
		return nil, fmt.Errorf("mongodb: 'IN' requires a list on the right at %s, got %T",
			expr.Start().String(), expr.Right)
	}
	if len(listLit.Values) == 0 {
		return nil, fmt.Errorf("mongodb: 'IN' requires a non-empty list at %s",
			expr.Start().String())
	}

	values := make([]any, 0, len(listLit.Values))
	for _, lit := range listLit.Values {
		val, err := filterhelp.LiteralToValue(lit)
		if err != nil {
			return nil, fmt.Errorf("mongodb: %w (at %s)", err, expr.Start().String())
		}
		values = append(values, val)
	}

	return map[string]any{field: map[string]any{op: values}}, nil
}

// translateLike maps LIKE onto MongoDB $regex with SQL wildcards
// (% → .*, _ → .) and anchors the pattern. The match is
// case-insensitive ($options "i") for parity with most SQL engines'
// default behavior on LIKE.
func (v *Visitor) translateLike(expr *ast.BinaryExpr) (map[string]any, error) {
	field, err := v.fieldPath(expr.Left)
	if err != nil {
		return nil, fmt.Errorf("mongodb: %w (at %s)", err, expr.Start().String())
	}

	value, err := filterhelp.ExtractValue(expr.Right)
	if err != nil {
		return nil, fmt.Errorf("mongodb: %w (at %s)", err, expr.Start().String())
	}
	pattern, ok := value.(string)
	if !ok {
		return nil, fmt.Errorf("mongodb: LIKE requires a string pattern, got %T at %s",
			value, expr.Start().String())
	}

	var b strings.Builder
	b.Grow(len(pattern) + 2)
	b.WriteByte('^')
	for _, r := range pattern {
		switch r {
		case '%':
			b.WriteString(".*")
		case '_':
			b.WriteByte('.')
		// Escape regex metacharacters so the source pattern remains literal.
		case '.', '+', '*', '?', '(', ')', '[', ']', '{', '}', '|', '^', '$', '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('$')

	return map[string]any{
		field: map[string]any{
			"$regex":   b.String(),
			"$options": "i",
		},
	}, nil
}

// fieldPath assembles the dotted field path used by MongoDB.
func (v *Visitor) fieldPath(expr ast.Expr) (string, error) {
	keys, err := filterhelp.CollectKeyPath(expr)
	if err != nil {
		return "", err
	}
	if len(keys) == 0 {
		return "", fmt.Errorf("empty key path on left operand")
	}
	if v.metadataPrefix == "" {
		return strings.Join(keys, "."), nil
	}
	return v.metadataPrefix + "." + strings.Join(keys, "."), nil
}

func mongoOpFor(kind token.Kind) (string, error) {
	switch kind {
	case token.EQ:
		return "$eq", nil
	case token.NE:
		return "$ne", nil
	case token.LT:
		return "$lt", nil
	case token.LE:
		return "$lte", nil
	case token.GT:
		return "$gt", nil
	case token.GE:
		return "$gte", nil
	default:
		return "", fmt.Errorf("unexpected operator '%s'", kind.Name())
	}
}
