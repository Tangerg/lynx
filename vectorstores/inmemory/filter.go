package inmemory

import (
	"fmt"

	"github.com/Tangerg/lynx/core/vectorstore/filter/ast"
	"github.com/Tangerg/lynx/core/vectorstore/filter/token"
)

// matchesFilter evaluates a parsed filter [ast.Expr] against the
// supplied metadata map. Returns true when the expression matches.
// An evaluation error (type mismatch, unknown identifier, etc.) is
// returned to the caller — in-memory stores treat malformed filters
// as a programmer error worth surfacing.
//
// The supported subset matches the filter mini-language fully:
// equality (== / !=), ordering (< / <= / > / >=), logical (AND / OR
// / NOT), membership (IN), and pattern match (LIKE with % / _
// wildcards). Nested index expressions (`meta["a"]["b"]`) resolve
// against nested map[string]any structures.
func matchesFilter(expr ast.Expr, metadata map[string]any) (bool, error) {
	v, err := evalExpr(expr, metadata)
	if err != nil {
		return false, err
	}
	b, ok := v.(bool)
	if !ok {
		return false, fmt.Errorf("inmemory.matchesFilter: filter expression did not yield bool, got %T", v)
	}
	return b, nil
}

// evalExpr is the recursive dispatcher; every node type maps to one
// case. The return type is `any` because leaf nodes can be
// string / float64 / bool / []any (lists). Callers that expect a
// specific shape do the type assertion at the point of use.
func evalExpr(expr ast.Expr, metadata map[string]any) (any, error) {
	switch e := expr.(type) {
	case *ast.Ident:
		return lookupField(metadata, e.Value), nil
	case *ast.Literal:
		return literalValue(e)
	case *ast.ListLiteral:
		return listValue(e)
	case *ast.IndexExpr:
		return evalIndex(e, metadata)
	case *ast.UnaryExpr:
		return evalUnary(e, metadata)
	case *ast.BinaryExpr:
		return evalBinary(e, metadata)
	}
	return nil, fmt.Errorf("inmemory.evalExpr: unsupported node %T", expr)
}

func literalValue(lit *ast.Literal) (any, error) {
	switch {
	case lit.IsString():
		return lit.AsString()
	case lit.IsNumber():
		return lit.AsNumber()
	case lit.IsBool():
		return lit.AsBool()
	}
	return nil, fmt.Errorf("inmemory.literalValue: literal %q has no decodable kind", lit.Value)
}

func listValue(list *ast.ListLiteral) (any, error) {
	out := make([]any, 0, len(list.Values))
	for _, item := range list.Values {
		v, err := literalValue(item)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

// evalIndex traverses an IndexExpr left-to-right against metadata.
// metadata["a"]["b"][0] is a chain of IndexExpr; we collect the keys
// and descend.
func evalIndex(idx *ast.IndexExpr, metadata map[string]any) (any, error) {
	keys, err := collectIndexKeys(idx)
	if err != nil {
		return nil, err
	}
	var cur any = metadata
	for _, key := range keys {
		switch typed := cur.(type) {
		case map[string]any:
			s, ok := key.(string)
			if !ok {
				return nil, fmt.Errorf("inmemory.evalIndex: map key must be string, got %T", key)
			}
			cur = typed[s]
		case []any:
			n, ok := key.(float64)
			if !ok {
				return nil, fmt.Errorf("inmemory.evalIndex: array index must be number, got %T", key)
			}
			i := int(n)
			if i < 0 || i >= len(typed) {
				return nil, nil
			}
			cur = typed[i]
		default:
			return nil, nil
		}
	}
	return cur, nil
}

// collectIndexKeys walks down an IndexExpr chain and returns the
// keys in outer-to-inner order. The bottom of the chain is an Ident
// (the base metadata field name) which becomes the first key.
func collectIndexKeys(idx *ast.IndexExpr) ([]any, error) {
	var chain []any
	cur := ast.Expr(idx)
	for {
		switch typed := cur.(type) {
		case *ast.IndexExpr:
			key, err := literalValue(typed.Index)
			if err != nil {
				return nil, err
			}
			chain = append([]any{key}, chain...)
			cur = typed.Left
		case *ast.Ident:
			chain = append([]any{typed.Value}, chain...)
			return chain, nil
		default:
			return nil, fmt.Errorf("inmemory.collectIndexKeys: unexpected base %T", cur)
		}
	}
}

func evalUnary(u *ast.UnaryExpr, metadata map[string]any) (any, error) {
	if u.Op.Kind != token.NOT {
		return nil, fmt.Errorf("inmemory.evalUnary: unsupported unary operator %s", u.Op.Kind)
	}
	v, err := evalExpr(u.Right, metadata)
	if err != nil {
		return nil, err
	}
	b, ok := v.(bool)
	if !ok {
		return nil, fmt.Errorf("inmemory.evalUnary: NOT operand must be bool, got %T", v)
	}
	return !b, nil
}

func evalBinary(b *ast.BinaryExpr, metadata map[string]any) (any, error) {
	switch b.Op.Kind {
	case token.AND, token.OR:
		return evalLogical(b, metadata)
	case token.EQ, token.NE:
		return evalEquality(b, metadata)
	case token.LT, token.LE, token.GT, token.GE:
		return evalOrdering(b, metadata)
	case token.IN:
		return evalIn(b, metadata)
	case token.LIKE:
		return evalLike(b, metadata)
	}
	return nil, fmt.Errorf("inmemory.evalBinary: unsupported binary operator %s", b.Op.Kind)
}

func evalLogical(b *ast.BinaryExpr, metadata map[string]any) (any, error) {
	left, err := evalExpr(b.Left, metadata)
	if err != nil {
		return nil, err
	}
	lb, ok := left.(bool)
	if !ok {
		return nil, fmt.Errorf("inmemory.evalLogical: %s left operand must be bool, got %T", b.Op.Kind, left)
	}
	// Short-circuit.
	if b.Op.Kind == token.AND && !lb {
		return false, nil
	}
	if b.Op.Kind == token.OR && lb {
		return true, nil
	}
	right, err := evalExpr(b.Right, metadata)
	if err != nil {
		return nil, err
	}
	rb, ok := right.(bool)
	if !ok {
		return nil, fmt.Errorf("inmemory.evalLogical: %s right operand must be bool, got %T", b.Op.Kind, right)
	}
	return rb, nil
}

func evalEquality(b *ast.BinaryExpr, metadata map[string]any) (any, error) {
	left, err := evalExpr(b.Left, metadata)
	if err != nil {
		return nil, err
	}
	right, err := evalExpr(b.Right, metadata)
	if err != nil {
		return nil, err
	}
	eq := equalValues(left, right)
	if b.Op.Kind == token.NE {
		return !eq, nil
	}
	return eq, nil
}

func equalValues(a, b any) bool {
	if a == nil || b == nil {
		return a == b
	}
	// Numbers: compare as float64.
	if af, ok := toFloat(a); ok {
		if bf, ok2 := toFloat(b); ok2 {
			return af == bf
		}
	}
	return a == b
}

func evalOrdering(b *ast.BinaryExpr, metadata map[string]any) (any, error) {
	left, err := evalExpr(b.Left, metadata)
	if err != nil {
		return nil, err
	}
	right, err := evalExpr(b.Right, metadata)
	if err != nil {
		return nil, err
	}
	lf, ok := toFloat(left)
	if !ok {
		return nil, fmt.Errorf("inmemory.evalOrdering: %s left operand must be numeric, got %T", b.Op.Kind, left)
	}
	rf, ok := toFloat(right)
	if !ok {
		return nil, fmt.Errorf("inmemory.evalOrdering: %s right operand must be numeric, got %T", b.Op.Kind, right)
	}
	switch b.Op.Kind {
	case token.LT:
		return lf < rf, nil
	case token.LE:
		return lf <= rf, nil
	case token.GT:
		return lf > rf, nil
	case token.GE:
		return lf >= rf, nil
	}
	return nil, fmt.Errorf("inmemory.evalOrdering: unreachable op %s", b.Op.Kind)
}

func evalIn(b *ast.BinaryExpr, metadata map[string]any) (any, error) {
	left, err := evalExpr(b.Left, metadata)
	if err != nil {
		return nil, err
	}
	right, err := evalExpr(b.Right, metadata)
	if err != nil {
		return nil, err
	}
	list, ok := right.([]any)
	if !ok {
		return nil, fmt.Errorf("inmemory.evalIn: right operand must be list, got %T", right)
	}
	for _, item := range list {
		if equalValues(left, item) {
			return true, nil
		}
	}
	return false, nil
}

func evalLike(b *ast.BinaryExpr, metadata map[string]any) (any, error) {
	left, err := evalExpr(b.Left, metadata)
	if err != nil {
		return nil, err
	}
	right, err := evalExpr(b.Right, metadata)
	if err != nil {
		return nil, err
	}
	s, ok := left.(string)
	if !ok {
		return nil, fmt.Errorf("inmemory.evalLike: LIKE left operand must be string, got %T", left)
	}
	pattern, ok := right.(string)
	if !ok {
		return nil, fmt.Errorf("inmemory.evalLike: LIKE right operand must be string, got %T", right)
	}
	return likeMatch(s, pattern), nil
}

// likeMatch implements SQL LIKE: % is "any sequence" and _ is "any
// single char". No anchoring — the pattern must match the whole
// input. Greedy / backtracking; fine for filter metadata strings
// (typically short).
func likeMatch(s, pattern string) bool {
	return likeMatchRunes([]rune(s), []rune(pattern))
}

func likeMatchRunes(s, p []rune) bool {
	si, pi := 0, 0
	starS, starP := -1, -1
	for si < len(s) {
		switch {
		case pi < len(p) && (p[pi] == '_' || p[pi] == s[si]):
			si++
			pi++
		case pi < len(p) && p[pi] == '%':
			starP = pi
			starS = si
			pi++
		case starP != -1:
			pi = starP + 1
			starS++
			si = starS
		default:
			return false
		}
	}
	for pi < len(p) && p[pi] == '%' {
		pi++
	}
	return pi == len(p)
}

// lookupField fetches an identifier value from the metadata map.
// Returns nil when absent — that matches the SQL three-valued logic
// callers already expect from missing fields.
func lookupField(metadata map[string]any, name string) any {
	if metadata == nil {
		return nil
	}
	return metadata[name]
}

// toFloat converts numeric metadata values into float64 for the
// ordering / equality paths. Bool is intentionally rejected — `b > 0`
// against a bool is almost always a bug.
func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int8:
		return float64(n), true
	case int16:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case uint:
		return float64(n), true
	case uint8:
		return float64(n), true
	case uint16:
		return float64(n), true
	case uint32:
		return float64(n), true
	case uint64:
		return float64(n), true
	case float32:
		return float64(n), true
	case float64:
		return n, true
	case string:
		// Numeric strings stay strings — explicit conversion avoids
		// surprises like "12" < "9" comparing wrong.
		_ = n
		return 0, false
	}
	return 0, false
}
