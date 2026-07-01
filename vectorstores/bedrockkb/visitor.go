package bedrockkb

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/service/bedrockagentruntime/document"
	"github.com/aws/aws-sdk-go-v2/service/bedrockagentruntime/types"

	"github.com/Tangerg/lynx/core/vectorstore/filter/ast"
	"github.com/Tangerg/lynx/core/vectorstore/filter/token"
)

// BuildRetrievalFilter transforms an AST filter expression into a
// Bedrock RetrievalFilter. Bedrock Knowledge Bases address metadata
// keys directly by name; nested paths are not supported, so the left
// operand must reduce to a bare identifier (or a single-level
// indexed expression).
func BuildRetrievalFilter(expr ast.Expr) (types.RetrievalFilter, error) {
	if expr == nil {
		return nil, nil
	}
	return convertExpr(expr)
}

func convertExpr(expr ast.Expr) (types.RetrievalFilter, error) {
	switch node := expr.(type) {
	case *ast.BinaryExpr:
		return convertBinary(node)
	case *ast.UnaryExpr:
		return convertUnary(node)
	default:
		return nil, fmt.Errorf("bedrockkb: unsupported root expression %T", node)
	}
}

func convertBinary(expr *ast.BinaryExpr) (types.RetrievalFilter, error) {
	switch {
	case expr.Op.Kind.Is(token.AND), expr.Op.Kind.Is(token.OR):
		return convertLogical(expr)
	case expr.Op.Kind.Is(token.IN):
		return convertIn(expr)
	case expr.Op.Kind.Is(token.LIKE):
		return convertLike(expr)
	case expr.Op.Kind.IsEqualityOperator() || expr.Op.Kind.IsOrderingOperator():
		return convertComparison(expr)
	default:
		return nil, fmt.Errorf("bedrockkb: unsupported binary operator '%s'", expr.Op.Literal)
	}
}

// convertUnary handles NOT by rewriting the negated child into its
// inverse, since Bedrock has no top-level NOT filter member.
func convertUnary(expr *ast.UnaryExpr) (types.RetrievalFilter, error) {
	if !expr.Op.Kind.Is(token.NOT) {
		return nil, fmt.Errorf("bedrockkb: unsupported unary '%s'", expr.Op.Literal)
	}
	bin, ok := expr.Right.(*ast.BinaryExpr)
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
func invertBinary(expr *ast.BinaryExpr) (*ast.BinaryExpr, error) {
	clone := *expr
	switch expr.Op.Kind {
	case token.EQ:
		clone.Op.Kind = token.NE
	case token.NE:
		clone.Op.Kind = token.EQ
	case token.LT:
		clone.Op.Kind = token.GE
	case token.LE:
		clone.Op.Kind = token.GT
	case token.GT:
		clone.Op.Kind = token.LE
	case token.GE:
		clone.Op.Kind = token.LT
	default:
		return nil, fmt.Errorf("bedrockkb: cannot invert operator '%s'", expr.Op.Literal)
	}
	return &clone, nil
}

func convertLogical(expr *ast.BinaryExpr) (types.RetrievalFilter, error) {
	left, err := convertExpr(expr.Left)
	if err != nil {
		return nil, err
	}
	right, err := convertExpr(expr.Right)
	if err != nil {
		return nil, err
	}
	if expr.Op.Kind.Is(token.OR) {
		return &types.RetrievalFilterMemberOrAll{Value: []types.RetrievalFilter{left, right}}, nil
	}
	return &types.RetrievalFilterMemberAndAll{Value: []types.RetrievalFilter{left, right}}, nil
}

func convertComparison(expr *ast.BinaryExpr) (types.RetrievalFilter, error) {
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
	switch expr.Op.Kind {
	case token.EQ:
		return &types.RetrievalFilterMemberEquals{Value: attr}, nil
	case token.NE:
		return &types.RetrievalFilterMemberNotEquals{Value: attr}, nil
	case token.LT:
		return &types.RetrievalFilterMemberLessThan{Value: attr}, nil
	case token.LE:
		return &types.RetrievalFilterMemberLessThanOrEquals{Value: attr}, nil
	case token.GT:
		return &types.RetrievalFilterMemberGreaterThan{Value: attr}, nil
	case token.GE:
		return &types.RetrievalFilterMemberGreaterThanOrEquals{Value: attr}, nil
	default:
		return nil, fmt.Errorf("bedrockkb: unexpected comparison operator '%s'", expr.Op.Literal)
	}
}

func convertIn(expr *ast.BinaryExpr) (types.RetrievalFilter, error) {
	key, err := keyName(expr.Left)
	if err != nil {
		return nil, err
	}
	listLit, ok := expr.Right.(*ast.ListLiteral)
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
func convertLike(expr *ast.BinaryExpr) (types.RetrievalFilter, error) {
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

func keyName(expr ast.Expr) (string, error) {
	switch node := expr.(type) {
	case *ast.Ident:
		return node.Value, nil
	case *ast.IndexExpr:
		// Single-level metadata["author"] only — Bedrock doesn't do
		// nested attribute paths in filters.
		idx := node.Index
		if idx == nil {
			return "", errors.New("missing index literal")
		}
		switch {
		case idx.IsString():
			return idx.AsString()
		case idx.IsNumber():
			n, err := idx.AsNumber()
			if err != nil {
				return "", err
			}
			if float64(int64(n)) == n {
				return strconv.FormatInt(int64(n), 10), nil
			}
			return strconv.FormatFloat(n, 'f', -1, 64), nil
		default:
			return "", errors.New("index must be a string or number literal")
		}
	default:
		return "", fmt.Errorf("unsupported left operand %T", node)
	}
}

func extractLiteralValue(expr ast.Expr) (any, error) {
	lit, ok := expr.(*ast.Literal)
	if !ok {
		return nil, fmt.Errorf("expected literal, got %T", expr)
	}
	return literalToValue(lit)
}

func literalToValue(lit *ast.Literal) (any, error) {
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
		return nil, fmt.Errorf("unsupported literal kind %s", lit.Token.Kind.Name())
	}
}
