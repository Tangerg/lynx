package inmemory

import (
	"fmt"

	"github.com/Tangerg/lynx/core/vectorstore/filter/ast"
	"github.com/Tangerg/lynx/core/vectorstore/filter/token"
)

// matchesFilter returns whether metadata satisfies expr. Evaluation
// errors (type mismatch, unsupported node, etc.) are surfaced rather
// than swallowed — a malformed filter is a programmer bug.
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

// evalIndex resolves an `a["b"][0]`-style chain. Missing keys / OOB
// indices return nil (matching SQL NULL semantics); only structural
// type errors are reported.
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
	case token.IS:
		return evalNullTest(b, metadata)
	}
	return nil, fmt.Errorf("inmemory.evalBinary: unsupported binary operator %s", b.Op.Kind)
}

// evalNullTest evaluates `<field> IS NULL`: true when the field is
// absent or stored as nil. A missing key already evaluates to nil
// (lookupField / evalIndex return nil), so this collapses to a nil
// check. `IS NOT NULL` is the NOT wrapper around this, handled by
// evalUnary.
func evalNullTest(b *ast.BinaryExpr, metadata map[string]any) (any, error) {
	left, err := evalExpr(b.Left, metadata)
	if err != nil {
		return nil, err
	}
	return left == nil, nil
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

// likeMatch is SQL LIKE: % matches any run of characters, _ matches
// one. The pattern must match the whole input. Greedy backtracking is
// acceptable here because metadata strings are short.
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

// lookupField returns nil for absent fields — matching the SQL
// three-valued logic the ordering and equality paths assume.
func lookupField(metadata map[string]any, name string) any {
	if metadata == nil {
		return nil
	}
	return metadata[name]
}

// toFloat coerces numeric metadata into float64. Bool is rejected on
// purpose: `flag > 0` against a bool is almost always a programmer
// mistake.
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
	}
	// Numeric strings stay strings — `"12" < "9"` should fail loudly,
	// not silently coerce.
	return 0, false
}
