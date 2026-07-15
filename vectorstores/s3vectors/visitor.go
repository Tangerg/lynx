package s3vectors

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/lynx/core/vectorstore/filter"
	"github.com/Tangerg/lynx/vectorstores/internal/filtercompile"
)

// Visitor transforms AST filter expressions into the JSON filter
// document S3 Vectors expects under the QueryVectors `Filter` field.
//
// The S3 Vectors filter language is Mongo-flavored:
//
//	author == "Alice"        →  {"author": {"$eq": "Alice"}}
//	year >= 2020             →  {"year":   {"$gte": 2020}}
//	tag IN ("a", "b")        →  {"tag":    {"$in": ["a", "b"]}}
//	NOT (author == "Alice")  →  {"$not":   {"author": {"$eq": "Alice"}}}
//	a == "x" AND b == "y"    →  {"$and": [{"a":{"$eq":"x"}}, {"b":{"$eq":"y"}}]}
var _ filter.Visitor = (*Visitor)(nil)

type Visitor struct {
	err    error
	result map[string]any
}

func NewVisitor() *Visitor { return &Visitor{} }

func (v *Visitor) Result() map[string]any {
	if v.err != nil {
		return nil
	}
	return v.result
}

func (v *Visitor) Visit(expr filter.Predicate) error {
	doc, err := v.translate(expr)
	v.err = err
	v.result = doc
	return v.err
}

func (v *Visitor) translate(expr filter.Expr) (map[string]any, error) {
	if expr == nil {
		return nil, errors.New("s3vectors: cannot process nil expression")
	}
	switch node := expr.(type) {
	case *filter.BinaryExpr:
		return v.translateBinary(node)
	case *filter.UnaryExpr:
		return v.translateUnary(node)
	default:
		return nil, fmt.Errorf("s3vectors: unsupported root expression %T", node)
	}
}

func (v *Visitor) translateBinary(expr *filter.BinaryExpr) (map[string]any, error) {
	switch {
	case expr.Op.IsLogicalOperator():
		return v.translateLogical(expr)
	case expr.Op.Is(filter.OpIn):
		return v.translateIn(expr)
	case expr.Op.IsEqualityOperator() || expr.Op.IsOrderingOperator():
		return v.translateComparison(expr)
	default:
		return nil, fmt.Errorf("s3vectors: unsupported binary operator '%s'", expr.Op.String())
	}
}

func (v *Visitor) translateUnary(expr *filter.UnaryExpr) (map[string]any, error) {
	if !expr.Op.Is(filter.OpNot) {
		return nil, fmt.Errorf("s3vectors: unsupported unary '%s'", expr.Op.String())
	}
	inner, err := v.translate(expr.Right)
	if err != nil {
		return nil, err
	}
	return map[string]any{"$not": inner}, nil
}

func (v *Visitor) translateLogical(expr *filter.BinaryExpr) (map[string]any, error) {
	left, err := v.translate(expr.Left)
	if err != nil {
		return nil, err
	}
	right, err := v.translate(expr.Right)
	if err != nil {
		return nil, err
	}
	op := "$and"
	if expr.Op.Is(filter.OpOr) {
		op = "$or"
	}
	return map[string]any{op: []any{left, right}}, nil
}

func (v *Visitor) translateComparison(expr *filter.BinaryExpr) (map[string]any, error) {
	key, err := keyName(expr.Left)
	if err != nil {
		return nil, err
	}
	value, err := filtercompile.ExtractValue(expr.Right)
	if err != nil {
		return nil, err
	}
	op, err := mongoOpFor(expr.Op)
	if err != nil {
		return nil, err
	}
	return map[string]any{key: map[string]any{op: value}}, nil
}

func (v *Visitor) translateIn(expr *filter.BinaryExpr) (map[string]any, error) {
	key, err := keyName(expr.Left)
	if err != nil {
		return nil, err
	}
	listLit, ok := expr.Right.(*filter.ListLiteral)
	if !ok {
		return nil, errors.New("s3vectors: 'IN' requires a list on the right")
	}
	if len(listLit.Values) == 0 {
		return nil, errors.New("s3vectors: 'IN' requires a non-empty list")
	}
	values := make([]any, 0, len(listLit.Values))
	for _, lit := range listLit.Values {
		val, err := filtercompile.LiteralToValue(lit)
		if err != nil {
			return nil, err
		}
		values = append(values, val)
	}
	return map[string]any{key: map[string]any{"$in": values}}, nil
}

func keyName(expr filter.Expr) (string, error) {
	switch node := expr.(type) {
	case *filter.Ident:
		return node.Value, nil
	case *filter.IndexExpr:
		keys, err := filtercompile.CollectKeyPath(node)
		if err != nil {
			return "", err
		}
		// S3 Vectors filters address flat metadata keys.
		return strings.Join(keys, "."), nil
	default:
		return "", fmt.Errorf("unsupported left operand %T", node)
	}
}

func mongoOpFor(kind filter.Operator) (string, error) {
	switch kind {
	case filter.OpEqual:
		return "$eq", nil
	case filter.OpNotEqual:
		return "$ne", nil
	case filter.OpLess:
		return "$lt", nil
	case filter.OpLessEqual:
		return "$lte", nil
	case filter.OpGreater:
		return "$gt", nil
	case filter.OpGreaterEqual:
		return "$gte", nil
	default:
		return "", fmt.Errorf("unexpected operator '%s'", kind.Name())
	}
}
