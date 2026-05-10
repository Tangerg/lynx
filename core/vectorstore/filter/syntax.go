package filter

import (
	"fmt"

	"github.com/Tangerg/lynx/core/vectorstore/filter/ast"
	"github.com/Tangerg/lynx/core/vectorstore/filter/token"
	"github.com/Tangerg/lynx/pkg/math"
)

// newKindToken builds a token of the given kind with no position
// information — used by the AST factories below, which produce nodes
// detached from any source text.
func newKindToken(kind token.Kind) token.Token {
	return token.OfKind(kind, token.NoPosition, token.NoPosition)
}

// identType is the input constraint for [NewIdent]: a raw string is
// turned into a fresh [ast.Ident]; an existing [*ast.Ident] passes
// through unchanged.
type identType interface {
	string | *ast.Ident
}

// newIdent is the runtime worker behind [NewIdent]. The generic wrapper
// guarantees the type switch is exhaustive — a default branch error
// only fires if a caller bypasses the constraint.
func newIdent(value any) (*ast.Ident, error) {
	switch typed := value.(type) {
	case string:
		return &ast.Ident{
			Token: token.OfIdent(typed, token.NoPosition, token.NoPosition),
			Value: typed,
		}, nil
	case *ast.Ident:
		return typed, nil
	default:
		return nil, fmt.Errorf("filter.newIdent: expected string or *ast.Ident, got %T (%v)",
			value, value)
	}
}

// NewIdent builds an [*ast.Ident] from either a string name or an
// existing identifier node. Position is always zero — these are
// hand-built nodes, not parsed ones.
func NewIdent[T identType](value T) *ast.Ident {
	ident, err := newIdent(value)
	if err != nil {
		// Unreachable while the generic constraint is honored.
		panic(fmt.Sprintf("filter.NewIdent: %v", err))
	}
	return ident
}

// literalType is the input constraint for [NewLiteral]: any numeric
// type, plus string, bool, or an existing [*ast.Literal].
type literalType interface {
	math.NumericType | string | bool | *ast.Literal
}

// newLiteral is the runtime worker behind [NewLiteral]. Numbers route
// through [token.OfNumericLiteral] for canonical formatting; bools
// become TRUE/FALSE keyword tokens; strings become STRING tokens.
func newLiteral(value any) (*ast.Literal, error) {
	switch typed := value.(type) {
	case int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64:
		// %v prints decimal form for ints and 'g'-format for floats —
		// OfNumericLiteral re-parses to float64 and re-formats anyway,
		// so any decimal/scientific representation works here.
		tk := token.OfNumericLiteral(fmt.Sprintf("%v", value), token.NoPosition, token.NoPosition)
		return &ast.Literal{Token: tk, Value: tk.Literal}, nil

	case string:
		return &ast.Literal{
			Token: token.OfLiteral(token.STRING, typed, token.NoPosition, token.NoPosition),
			Value: typed,
		}, nil

	case bool:
		kind := token.FALSE
		if typed {
			kind = token.TRUE
		}
		return &ast.Literal{
			Token: newKindToken(kind),
			Value: kind.Literal(),
		}, nil

	case *ast.Literal:
		return typed, nil

	default:
		return nil, fmt.Errorf("filter.newLiteral: unsupported literal type %T (%v)",
			value, value)
	}
}

// NewLiteral builds an [*ast.Literal] from a Go value. Numeric types
// produce NUMBER tokens; bools produce TRUE/FALSE; strings produce
// STRING. An existing [*ast.Literal] is returned unchanged.
func NewLiteral[T literalType](value T) *ast.Literal {
	lit, err := newLiteral(value)
	if err != nil {
		// Unreachable while the generic constraint is honored.
		panic(fmt.Sprintf("filter.NewLiteral: %v", err))
	}
	return lit
}

// NewLiterals maps a slice of Go values through [NewLiteral].
func NewLiterals[T literalType](values []T) []*ast.Literal {
	literals := make([]*ast.Literal, 0, len(values))
	for _, v := range values {
		literals = append(literals, NewLiteral(v))
	}
	return literals
}

// listLiteralType is the input constraint for [NewListLiteral]: any
// slice of a basic type, a pre-built slice of [*ast.Literal], or an
// existing [*ast.ListLiteral].
type listLiteralType interface {
	[]int | []int8 | []int16 | []int32 | []int64 |
		[]uint | []uint8 | []uint16 | []uint32 | []uint64 |
		[]float32 | []float64 | []string | []bool |
		[]*ast.Literal | *ast.ListLiteral
}

// newListLiteral is the runtime worker behind [NewListLiteral].
// Existing list nodes pass through; raw slices are mapped element-wise
// through [NewLiterals].
func newListLiteral(value any) (*ast.ListLiteral, error) {
	if list, ok := value.(*ast.ListLiteral); ok {
		return list, nil
	}

	result := &ast.ListLiteral{
		Lparen: newKindToken(token.LPAREN),
		Rparen: newKindToken(token.RPAREN),
	}

	switch typed := value.(type) {
	case []int:
		result.Values = NewLiterals(typed)
	case []int8:
		result.Values = NewLiterals(typed)
	case []int16:
		result.Values = NewLiterals(typed)
	case []int32:
		result.Values = NewLiterals(typed)
	case []int64:
		result.Values = NewLiterals(typed)
	case []uint:
		result.Values = NewLiterals(typed)
	case []uint8:
		result.Values = NewLiterals(typed)
	case []uint16:
		result.Values = NewLiterals(typed)
	case []uint32:
		result.Values = NewLiterals(typed)
	case []uint64:
		result.Values = NewLiterals(typed)
	case []float32:
		result.Values = NewLiterals(typed)
	case []float64:
		result.Values = NewLiterals(typed)
	case []string:
		result.Values = NewLiterals(typed)
	case []bool:
		result.Values = NewLiterals(typed)
	case []*ast.Literal:
		result.Values = typed
	default:
		return nil, fmt.Errorf("filter.newListLiteral: unsupported list type %T (%v)",
			value, value)
	}

	return result, nil
}

// NewListLiteral builds an [*ast.ListLiteral] from a slice of Go values
// or a pre-built node. Synthetic '(' / ')' tokens are attached so the
// node round-trips through [visitors.SQLLikeVisitor].
func NewListLiteral[T listLiteralType](value T) *ast.ListLiteral {
	list, err := newListLiteral(value)
	if err != nil {
		// Unreachable while the generic constraint is honored.
		panic(fmt.Sprintf("filter.NewListLiteral: %v", err))
	}
	return list
}

// identOrIndex resolves an `any` left operand to either an
// [*ast.IndexExpr] (passed through) or a freshly built [*ast.Ident].
// Used by both the `any`-typed [ExprBuilder] entry points and (via
// [leftOperand]) the generic helpers in this file.
func identOrIndex(l any) (ast.Expr, error) {
	if ix, ok := l.(*ast.IndexExpr); ok {
		return ix, nil
	}
	return newIdent(l)
}

// leftOperand is the generic shim around [identOrIndex] for [compare],
// [In], and [Like]. The constraint guarantees the input is always
// resolvable, so the error is unreachable.
func leftOperand[L identType | *ast.IndexExpr](l L) ast.Expr {
	expr, _ := identOrIndex(l)
	return expr
}

// compare is the shared body of [EQ] / [NE] / [LT] / [LE] / [GT] /
// [GE]. The left operand is either an identifier (string or
// [*ast.Ident]) or an [*ast.IndexExpr]; the right is any literal.
func compare[L identType | *ast.IndexExpr, R literalType](l L, r R, op token.Kind) *ast.BinaryExpr {
	return &ast.BinaryExpr{
		Left:  leftOperand(l),
		Op:    newKindToken(op),
		Right: NewLiteral(r),
	}
}

// EQ builds `l == r` — equality, any literal type. Examples:
// `id == 1`, `name == 'john'`, `arr[0] == 'value'`.
func EQ[L identType | *ast.IndexExpr, R literalType](l L, r R) *ast.BinaryExpr {
	return compare(l, r, token.EQ)
}

// NE builds `l != r` — inequality, any literal type.
func NE[L identType | *ast.IndexExpr, R literalType](l L, r R) *ast.BinaryExpr {
	return compare(l, r, token.NE)
}

// LT builds `l < r` — strict less-than. Right operand must be numeric.
func LT[L identType | *ast.IndexExpr, R math.NumericType | *ast.Literal](l L, r R) *ast.BinaryExpr {
	return compare(l, r, token.LT)
}

// LE builds `l <= r` — less-than-or-equal. Right operand must be
// numeric.
func LE[L identType | *ast.IndexExpr, R math.NumericType | *ast.Literal](l L, r R) *ast.BinaryExpr {
	return compare(l, r, token.LE)
}

// GT builds `l > r` — strict greater-than. Right operand must be
// numeric.
func GT[L identType | *ast.IndexExpr, R math.NumericType | *ast.Literal](l L, r R) *ast.BinaryExpr {
	return compare(l, r, token.GT)
}

// GE builds `l >= r` — greater-than-or-equal. Right operand must be
// numeric.
func GE[L identType | *ast.IndexExpr, R math.NumericType | *ast.Literal](l L, r R) *ast.BinaryExpr {
	return compare(l, r, token.GE)
}

// logic is the shared body of [And] and [Or].
func logic[L ast.ComputedExpr, R ast.ComputedExpr](l L, r R, op token.Kind) *ast.BinaryExpr {
	return &ast.BinaryExpr{
		Left:  l,
		Op:    newKindToken(op),
		Right: r,
	}
}

// And builds `l AND r`. Both operands must be computed expressions —
// raw literals or identifiers do not satisfy [ast.ComputedExpr].
func And[L ast.ComputedExpr, R ast.ComputedExpr](l L, r R) *ast.BinaryExpr {
	return logic(l, r, token.AND)
}

// Or builds `l OR r`. Both operands must be computed expressions.
func Or[L ast.ComputedExpr, R ast.ComputedExpr](l L, r R) *ast.BinaryExpr {
	return logic(l, r, token.OR)
}

// In builds `l IN (...)`. Right operand is converted via
// [NewListLiteral]. Examples: `status IN ('active','pending')`,
// `id IN (1,2,3)`.
func In[L identType | *ast.IndexExpr, R listLiteralType](l L, r R) *ast.BinaryExpr {
	return &ast.BinaryExpr{
		Left:  leftOperand(l),
		Op:    newKindToken(token.IN),
		Right: NewListLiteral(r),
	}
}

// Like builds `l LIKE r`. Right operand must be a string. Examples:
// `name LIKE 'John%'`, `email LIKE '%@gmail.com'`.
func Like[L identType | *ast.IndexExpr, R string | *ast.Literal](l L, r R) *ast.BinaryExpr {
	return &ast.BinaryExpr{
		Left:  leftOperand(l),
		Op:    newKindToken(token.LIKE),
		Right: NewLiteral(r),
	}
}

// Not builds `NOT r`. The operand must be a computed expression.
func Not[T ast.ComputedExpr](r T) *ast.UnaryExpr {
	return &ast.UnaryExpr{
		Op:    newKindToken(token.NOT),
		Right: r,
	}
}

// Index builds `left[index]`. Left can be a name, an existing
// identifier, or a previously built index expression — the latter
// supports nested access like `matrix[1][2]`. Index must be numeric or
// a string.
func Index[L identType | *ast.IndexExpr, I math.NumericType | string | *ast.Literal](left L, index I) *ast.IndexExpr {
	expr := &ast.IndexExpr{
		LBrack: newKindToken(token.LBRACK),
		RBrack: newKindToken(token.RBRACK),
		Index:  NewLiteral(index),
	}

	switch typedL := any(left).(type) {
	case string:
		expr.Left = NewIdent(typedL)
	case *ast.Ident:
		expr.Left = typedL
	case *ast.IndexExpr:
		expr.Left = typedL
	}
	return expr
}
