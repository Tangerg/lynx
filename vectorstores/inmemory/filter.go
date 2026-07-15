package inmemory

import (
	"fmt"
	"math"

	"github.com/Tangerg/lynx/core/vectorstore/filter"
	"github.com/Tangerg/lynx/vectorstores/internal/filterhelp"
)

type evaluator struct {
	metadata map[string]any
	match    bool
}

var _ filter.Visitor = (*evaluator)(nil)

func (e *evaluator) Visit(predicate filter.Predicate) error {
	value, err := evalExpr(predicate, e.metadata)
	if err != nil {
		return err
	}
	match, ok := value.(bool)
	if !ok {
		return fmt.Errorf("inmemory.evaluator: predicate yielded %T, want bool", value)
	}
	e.match = match
	return nil
}

// matchesFilter returns whether metadata satisfies expr. Evaluation
// errors (type mismatch, unsupported node, etc.) are surfaced rather
// than swallowed — a malformed filter is a programmer bug.
func matchesFilter(expr filter.Predicate, metadata map[string]any) (bool, error) {
	evaluator := evaluator{metadata: metadata}
	if err := evaluator.Visit(expr); err != nil {
		return false, err
	}
	return evaluator.match, nil
}

func evalExpr(expr filter.Expr, metadata map[string]any) (any, error) {
	switch e := expr.(type) {
	case *filter.Ident:
		return lookupField(metadata, e.Value), nil
	case *filter.Literal:
		return literalValue(e)
	case *filter.ListLiteral:
		return listValue(e)
	case *filter.IndexExpr:
		return evalIndex(e, metadata)
	case *filter.UnaryExpr:
		return evalUnary(e, metadata)
	case *filter.BinaryExpr:
		return evalBinary(e, metadata)
	}
	return nil, fmt.Errorf("inmemory.evalExpr: unsupported node %T", expr)
}

func literalValue(lit *filter.Literal) (any, error) {
	value, err := filterhelp.LiteralToValue(lit)
	if err != nil {
		return nil, fmt.Errorf("inmemory.literalValue: %w", err)
	}
	return value, nil
}

func listValue(list *filter.ListLiteral) (any, error) {
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
func evalIndex(idx *filter.IndexExpr, metadata map[string]any) (any, error) {
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
			index, ok := arrayIndex(key)
			if !ok {
				return nil, fmt.Errorf("inmemory.evalIndex: invalid array index %v (%T)", key, key)
			}
			if index >= uint64(len(typed)) {
				return nil, nil
			}
			cur = typed[int(index)]
		default:
			return nil, nil
		}
	}
	return cur, nil
}

func arrayIndex(value any) (uint64, bool) {
	switch number := value.(type) {
	case int64:
		if number < 0 {
			return 0, false
		}
		return uint64(number), true
	case uint64:
		return number, true
	case float64:
		if number < 0 || number >= math.Exp2(63) || math.Trunc(number) != number {
			return 0, false
		}
		return uint64(number), true
	default:
		return 0, false
	}
}

func collectIndexKeys(idx *filter.IndexExpr) ([]any, error) {
	var chain []any
	cur := filter.Expr(idx)
	for {
		switch typed := cur.(type) {
		case *filter.IndexExpr:
			key, err := literalValue(typed.Index)
			if err != nil {
				return nil, err
			}
			chain = append([]any{key}, chain...)
			cur = typed.Left
		case *filter.Ident:
			chain = append([]any{typed.Value}, chain...)
			return chain, nil
		default:
			return nil, fmt.Errorf("inmemory.collectIndexKeys: unexpected base %T", cur)
		}
	}
}

func evalUnary(u *filter.UnaryExpr, metadata map[string]any) (any, error) {
	if u.Op != filter.OpNot {
		return nil, fmt.Errorf("inmemory.evalUnary: unsupported unary operator %s", u.Op)
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

func evalBinary(b *filter.BinaryExpr, metadata map[string]any) (any, error) {
	switch b.Op {
	case filter.OpAnd, filter.OpOr:
		return evalLogical(b, metadata)
	case filter.OpEqual, filter.OpNotEqual:
		return evalEquality(b, metadata)
	case filter.OpLess, filter.OpLessEqual, filter.OpGreater, filter.OpGreaterEqual:
		return evalOrdering(b, metadata)
	case filter.OpIn:
		return evalIn(b, metadata)
	case filter.OpLike:
		return evalLike(b, metadata)
	case filter.OpIs:
		return evalNullTest(b, metadata)
	}
	return nil, fmt.Errorf("inmemory.evalBinary: unsupported binary operator %s", b.Op)
}

// evalNullTest evaluates `<field> IS NULL`: true when the field is
// absent or stored as nil. A missing key already evaluates to nil
// (lookupField / evalIndex return nil), so this collapses to a nil
// check. `IS NOT NULL` is the NOT wrapper around this, handled by
// evalUnary.
func evalNullTest(b *filter.BinaryExpr, metadata map[string]any) (any, error) {
	left, err := evalExpr(b.Left, metadata)
	if err != nil {
		return nil, err
	}
	return left == nil, nil
}

func evalLogical(b *filter.BinaryExpr, metadata map[string]any) (any, error) {
	left, err := evalExpr(b.Left, metadata)
	if err != nil {
		return nil, err
	}
	lb, ok := left.(bool)
	if !ok {
		return nil, fmt.Errorf("inmemory.evalLogical: %s left operand must be bool, got %T", b.Op, left)
	}
	// Short-circuit.
	if b.Op == filter.OpAnd && !lb {
		return false, nil
	}
	if b.Op == filter.OpOr && lb {
		return true, nil
	}
	right, err := evalExpr(b.Right, metadata)
	if err != nil {
		return nil, err
	}
	rb, ok := right.(bool)
	if !ok {
		return nil, fmt.Errorf("inmemory.evalLogical: %s right operand must be bool, got %T", b.Op, right)
	}
	return rb, nil
}

func evalEquality(b *filter.BinaryExpr, metadata map[string]any) (any, error) {
	left, err := evalExpr(b.Left, metadata)
	if err != nil {
		return nil, err
	}
	right, err := evalExpr(b.Right, metadata)
	if err != nil {
		return nil, err
	}
	eq := equalValues(left, right)
	if b.Op == filter.OpNotEqual {
		return !eq, nil
	}
	return eq, nil
}

func equalValues(a, b any) bool {
	if a == nil || b == nil {
		return a == b
	}
	if order, numeric, ordered := compareNumbers(a, b); numeric {
		return ordered && order == 0
	}
	return a == b
}

func evalOrdering(b *filter.BinaryExpr, metadata map[string]any) (any, error) {
	left, err := evalExpr(b.Left, metadata)
	if err != nil {
		return nil, err
	}
	right, err := evalExpr(b.Right, metadata)
	if err != nil {
		return nil, err
	}
	order, numeric, ordered := compareNumbers(left, right)
	if !numeric {
		return nil, fmt.Errorf("inmemory.evalOrdering: %s left operand must be numeric, got %T", b.Op, left)
	}
	if !ordered {
		return false, nil
	}
	switch b.Op {
	case filter.OpLess:
		return order < 0, nil
	case filter.OpLessEqual:
		return order <= 0, nil
	case filter.OpGreater:
		return order > 0, nil
	case filter.OpGreaterEqual:
		return order >= 0, nil
	}
	return nil, fmt.Errorf("inmemory.evalOrdering: unreachable op %s", b.Op)
}

type integerValue struct {
	signed bool
	i      int64
	u      uint64
}

func compareNumbers(left, right any) (order int, numeric, ordered bool) {
	leftInteger, leftIsInteger := asInteger(left)
	rightInteger, rightIsInteger := asInteger(right)
	if leftIsInteger && rightIsInteger {
		return compareIntegers(leftInteger, rightInteger), true, true
	}

	leftFloat, leftIsNumber := toFloat(left)
	rightFloat, rightIsNumber := toFloat(right)
	if !leftIsNumber || !rightIsNumber {
		return 0, false, false
	}
	if math.IsNaN(leftFloat) || math.IsNaN(rightFloat) {
		return 0, true, false
	}
	switch {
	case leftFloat < rightFloat:
		return -1, true, true
	case leftFloat > rightFloat:
		return 1, true, true
	default:
		return 0, true, true
	}
}

func asInteger(value any) (integerValue, bool) {
	switch number := value.(type) {
	case int:
		return integerValue{signed: true, i: int64(number)}, true
	case int8:
		return integerValue{signed: true, i: int64(number)}, true
	case int16:
		return integerValue{signed: true, i: int64(number)}, true
	case int32:
		return integerValue{signed: true, i: int64(number)}, true
	case int64:
		return integerValue{signed: true, i: number}, true
	case uint:
		return integerValue{u: uint64(number)}, true
	case uint8:
		return integerValue{u: uint64(number)}, true
	case uint16:
		return integerValue{u: uint64(number)}, true
	case uint32:
		return integerValue{u: uint64(number)}, true
	case uint64:
		return integerValue{u: number}, true
	default:
		return integerValue{}, false
	}
}

func compareIntegers(left, right integerValue) int {
	if left.signed && right.signed {
		return compareInt64(left.i, right.i)
	}
	if !left.signed && !right.signed {
		return compareUint64(left.u, right.u)
	}
	if left.signed {
		if left.i < 0 {
			return -1
		}
		return compareUint64(uint64(left.i), right.u)
	}
	if right.i < 0 {
		return 1
	}
	return compareUint64(left.u, uint64(right.i))
}

func compareInt64(left, right int64) int {
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}

func compareUint64(left, right uint64) int {
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}

func evalIn(b *filter.BinaryExpr, metadata map[string]any) (any, error) {
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

func evalLike(b *filter.BinaryExpr, metadata map[string]any) (any, error) {
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

// toFloat is the mixed integer/decimal fallback. Integer-only comparisons use
// compareIntegers so values above float64's exact range do not collapse.
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
