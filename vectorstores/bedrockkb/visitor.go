package bedrockkb

import (
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/bedrockagentruntime/document"
	"github.com/aws/aws-sdk-go-v2/service/bedrockagentruntime/types"

	"github.com/Tangerg/lynx/core/vectorstore/filter"
)

// BuildRetrievalFilter transforms an AST filter expression into a
// Bedrock RetrievalFilter. Bedrock Knowledge Bases address metadata
// keys directly by name; nested paths are not supported, so the left
// operand must be a bare identifier.
func BuildRetrievalFilter(expr filter.Predicate) (types.RetrievalFilter, error) {
	if expr == nil {
		return nil, nil
	}
	return convertExpr(expr)
}

func convertExpr(expr filter.Expr) (types.RetrievalFilter, error) {
	switch node := expr.(type) {
	case *filter.BinaryExpr:
		return convertBinary(node)
	case *filter.UnaryExpr:
		return convertUnary(node)
	default:
		return nil, fmt.Errorf("bedrockkb: unsupported root expression %T", node)
	}
}

func convertBinary(expr *filter.BinaryExpr) (types.RetrievalFilter, error) {
	switch {
	case expr.Op.Is(filter.OpAnd), expr.Op.Is(filter.OpOr):
		return convertLogical(expr)
	case expr.Op.Is(filter.OpIn):
		return convertIn(expr)
	case expr.Op.Is(filter.OpLike):
		return convertLike(expr)
	case expr.Op.IsEqualityOperator() || expr.Op.IsOrderingOperator():
		return convertComparison(expr)
	default:
		return nil, fmt.Errorf("bedrockkb: unsupported binary operator '%s'", expr.Op.String())
	}
}

// convertUnary handles NOT by rewriting the negated child into its
// inverse, since Bedrock has no top-level NOT filter member.
func convertUnary(expr *filter.UnaryExpr) (types.RetrievalFilter, error) {
	if !expr.Op.Is(filter.OpNot) {
		return nil, fmt.Errorf("bedrockkb: unsupported unary '%s'", expr.Op.String())
	}
	bin, ok := expr.Right.(*filter.BinaryExpr)
	if !ok {
		return nil, errors.New("bedrockkb: NOT may only wrap a binary comparison")
	}
	inverted, err := invertBinary(bin)
	if err != nil {
		return nil, err
	}
	return convertExpr(inverted)
}

// invertBinary returns the boolean inverse of a single comparison —
// EQ↔NE, LT↔GE, LE↔GT, IN↔NIN.
func invertBinary(expr *filter.BinaryExpr) (*filter.BinaryExpr, error) {
	clone := *expr
	switch expr.Op {
	case filter.OpEqual:
		clone.Op = filter.OpNotEqual
	case filter.OpNotEqual:
		clone.Op = filter.OpEqual
	case filter.OpLess:
		clone.Op = filter.OpGreaterEqual
	case filter.OpLessEqual:
		clone.Op = filter.OpGreater
	case filter.OpGreater:
		clone.Op = filter.OpLessEqual
	case filter.OpGreaterEqual:
		clone.Op = filter.OpLess
	default:
		return nil, fmt.Errorf("bedrockkb: cannot invert operator '%s'", expr.Op.String())
	}
	return &clone, nil
}

func convertLogical(expr *filter.BinaryExpr) (types.RetrievalFilter, error) {
	left, err := convertExpr(expr.Left)
	if err != nil {
		return nil, err
	}
	right, err := convertExpr(expr.Right)
	if err != nil {
		return nil, err
	}
	if expr.Op.Is(filter.OpOr) {
		return &types.RetrievalFilterMemberOrAll{Value: []types.RetrievalFilter{left, right}}, nil
	}
	return &types.RetrievalFilterMemberAndAll{Value: []types.RetrievalFilter{left, right}}, nil
}

func convertComparison(expr *filter.BinaryExpr) (types.RetrievalFilter, error) {
	key, err := keyName(expr.Left)
	if err != nil {
		return nil, err
	}
	value, err := extractLiteralValue(expr.Right)
	if err != nil {
		return nil, err
	}
	attr := types.FilterAttribute{
		Key:   &key,
		Value: document.NewLazyDocument(value),
	}
	switch expr.Op {
	case filter.OpEqual:
		return &types.RetrievalFilterMemberEquals{Value: attr}, nil
	case filter.OpNotEqual:
		return &types.RetrievalFilterMemberNotEquals{Value: attr}, nil
	case filter.OpLess:
		return &types.RetrievalFilterMemberLessThan{Value: attr}, nil
	case filter.OpLessEqual:
		return &types.RetrievalFilterMemberLessThanOrEquals{Value: attr}, nil
	case filter.OpGreater:
		return &types.RetrievalFilterMemberGreaterThan{Value: attr}, nil
	case filter.OpGreaterEqual:
		return &types.RetrievalFilterMemberGreaterThanOrEquals{Value: attr}, nil
	default:
		return nil, fmt.Errorf("bedrockkb: unexpected comparison operator '%s'", expr.Op.String())
	}
}

func convertIn(expr *filter.BinaryExpr) (types.RetrievalFilter, error) {
	key, err := keyName(expr.Left)
	if err != nil {
		return nil, err
	}
	listLit, ok := expr.Right.(*filter.ListLiteral)
	if !ok {
		return nil, errors.New("bedrockkb: 'IN' requires a list on the right")
	}
	if len(listLit.Values) == 0 {
		return nil, errors.New("bedrockkb: 'IN' requires a non-empty list")
	}
	values := make([]any, 0, len(listLit.Values))
	for _, lit := range listLit.Values {
		val, err := literalToValue(lit)
		if err != nil {
			return nil, err
		}
		values = append(values, val)
	}
	return &types.RetrievalFilterMemberIn{
		Value: types.FilterAttribute{
			Key:   &key,
			Value: document.NewLazyDocument(values),
		},
	}, nil
}

// convertLike maps LIKE onto Bedrock's StringContains / StartsWith
// depending on the pattern shape.
func convertLike(expr *filter.BinaryExpr) (types.RetrievalFilter, error) {
	key, err := keyName(expr.Left)
	if err != nil {
		return nil, err
	}
	value, err := extractLiteralValue(expr.Right)
	if err != nil {
		return nil, err
	}
	pattern, ok := value.(string)
	if !ok {
		return nil, fmt.Errorf("bedrockkb: LIKE requires a string pattern, got %T", value)
	}

	// Trim wildcards and pick the closest Bedrock filter operator.
	hasLead := len(pattern) > 0 && pattern[0] == '%'
	hasTrail := len(pattern) > 0 && pattern[len(pattern)-1] == '%'
	core := pattern
	if hasLead {
		core = core[1:]
	}
	if hasTrail && len(core) > 0 {
		core = core[:len(core)-1]
	}

	attr := types.FilterAttribute{
		Key:   &key,
		Value: document.NewLazyDocument(core),
	}
	switch {
	case !hasLead && hasTrail:
		// "foo%" → StartsWith
		return &types.RetrievalFilterMemberStartsWith{Value: attr}, nil
	default:
		// "%foo", "%foo%", or "foo" → StringContains
		return &types.RetrievalFilterMemberStringContains{Value: attr}, nil
	}
}

func keyName(expr filter.Expr) (string, error) {
	switch node := expr.(type) {
	case *filter.Ident:
		return node.Value, nil
	case *filter.IndexExpr:
		return "", errors.New("bedrockkb: nested metadata paths are not supported")
	default:
		return "", fmt.Errorf("unsupported left operand %T", node)
	}
}

func extractLiteralValue(expr filter.Expr) (any, error) {
	lit, ok := expr.(*filter.Literal)
	if !ok {
		return nil, fmt.Errorf("expected literal, got %T", expr)
	}
	return literalToValue(lit)
}

func literalToValue(lit *filter.Literal) (any, error) {
	switch {
	case lit.IsString():
		return lit.AsString()
	case lit.IsNumber():
		n, err := lit.AsNumber()
		if err != nil {
			return nil, err
		}
		if float64(int64(n)) == n {
			return int64(n), nil
		}
		return n, nil
	case lit.IsBool():
		return lit.AsBool()
	default:
		return nil, fmt.Errorf("unsupported literal kind %s", lit.Kind)
	}
}
