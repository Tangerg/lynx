package chroma

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	v2 "github.com/amikos-tech/chroma-go/pkg/api/v2"
	"github.com/spf13/cast"

	"github.com/Tangerg/lynx/core/vectorstore/filter"
	"github.com/Tangerg/lynx/vectorstores/internal/filterhelp"
)

// Visitor transforms AST filter expressions into Chroma WhereClause conditions.
// It traverses semantic filter expressions and converts them to the provider query shape
// into Chroma's native filter format.
//
// Supported operations:
//   - Logical: AND, OR
//   - Equality: ==, !=
//   - Ordering: <, <=, >, >=
//   - Membership: IN
//
// Unsupported operations (return errors):
//   - NOT (Chroma has no standalone logical NOT)
//   - LIKE (Chroma does not support pattern matching on metadata fields)
//
// Field path conventions:
//   - Simple identifiers are used as-is: "author" → "author"
//   - Indexed expressions strip the base identifier: metadata["author"] → "author"
//   - Nested indexed expressions join inner keys with dots: metadata["a"]["b"] → "a.b"
//
// Numeric handling:
//   - Whole-number float64 literals are treated as int for Chroma's typed API
//   - Fractional values are cast to float32
type Visitor struct {
	err               error          // last error encountered during conversion
	result            v2.WhereClause // the Chroma filter clause being built
	currentFieldKey   string         // temporary storage for field key extraction
	currentFieldValue any            // temporary storage for field value extraction
}

func NewVisitor() *Visitor {
	return &Visitor{}
}

func (v *Visitor) Result() v2.WhereClause {
	if v.err != nil {
		return nil
	}
	return v.result
}

func (v *Visitor) Visit(expr filter.Expr) error {
	v.err = v.visit(expr)
	return v.err
}

// visit dispatches to the appropriate handler based on the expression type.
func (v *Visitor) visit(expr filter.Expr) error {
	if expr == nil {
		return errors.New("chroma: cannot process nil expression")
	}
	if v.err != nil {
		return v.err
	}

	switch node := expr.(type) {
	case *filter.BinaryExpr:
		return v.visitBinaryExpr(node)
	case *filter.UnaryExpr:
		return v.visitUnaryExpr(node)
	case *filter.IndexExpr:
		return v.visitIndexExpr(node)
	case *filter.Ident:
		return v.visitIdent(node)
	case *filter.Literal:
		return v.visitLiteral(node)
	case *filter.ListLiteral:
		return v.visitListLiteral(node)
	default:
		return fmt.Errorf("chroma: unsupported expression type %T", node)
	}
}

// visitBinaryExpr routes to the correct handler based on the operator category.
func (v *Visitor) visitBinaryExpr(expr *filter.BinaryExpr) error {
	return filterhelp.DispatchBinaryErr(expr,
		v.visitLogicalExpr,
		v.visitComparisonExpr,
		v.visitInExpr,
		v.visitLikeExpr,
	)
}

// visitComparisonExpr splits equality vs ordering since chroma emits
// distinct Where map shapes for the two families.
func (v *Visitor) visitComparisonExpr(expr *filter.BinaryExpr) error {
	if expr.Op.IsEqualityOperator() {
		return v.visitEqualityExpr(expr)
	}
	return v.visitOrderingExpr(expr)
}

// visitLikeExpr — Chroma metadata filters do not support LIKE.
func (v *Visitor) visitLikeExpr(expr *filter.BinaryExpr) error {
	return fmt.Errorf("chroma: LIKE operator is not supported on metadata fields (at %s)",
		expr.Start().String())
}

// visitUnaryExpr handles unary operators. Chroma does not expose a
// standalone logical NOT, so even the only valid unary kind (NOT)
// is rejected with a guidance message.
func (v *Visitor) visitUnaryExpr(expr *filter.UnaryExpr) error {
	return filterhelp.DispatchUnaryErr(expr, func(*filter.UnaryExpr) error {
		return errors.New("chroma: NOT operator is not supported; rewrite using != or NIN")
	})
}

// visitIdent stores the identifier name as the current field key.
func (v *Visitor) visitIdent(ident *filter.Ident) error {
	v.currentFieldKey = ident.Value
	return nil
}

// visitLiteral converts a literal node to a Go value and stores it.
func (v *Visitor) visitLiteral(lit *filter.Literal) error {
	value, err := v.literalToValue(lit)
	if err != nil {
		return fmt.Errorf("chroma: failed to convert literal at %s: %w", lit.Start().String(), err)
	}
	v.currentFieldValue = value
	return nil
}

// visitListLiteral converts a list of literals to a []any slice and stores it.
func (v *Visitor) visitListLiteral(list *filter.ListLiteral) error {
	values := make([]any, 0, len(list.Values))
	for i, lit := range list.Values {
		value, err := v.literalToValue(lit)
		if err != nil {
			return fmt.Errorf("chroma: failed to convert list element at index %d: %w", i, err)
		}
		values = append(values, value)
	}
	v.currentFieldValue = values
	return nil
}

// visitIndexExpr builds a field key from an indexed expression and stores it.
// metadata["author"]      → "author"
// metadata["a"]["b"]      → "a.b"
func (v *Visitor) visitIndexExpr(expr *filter.IndexExpr) error {
	fieldKey, err := v.buildIndexedFieldKey(expr)
	if err != nil {
		return fmt.Errorf("chroma: failed to build field path at %s: %w", expr.Start().String(), err)
	}
	v.currentFieldKey = fieldKey
	return nil
}

// visitLogicalExpr handles AND and OR operators.
// Each operand is processed in isolation and the results are combined with
// v2.And or v2.Or respectively.
func (v *Visitor) visitLogicalExpr(expr *filter.BinaryExpr) error {
	leftClause, err := v.buildNestedClause(expr.Left)
	if err != nil {
		return fmt.Errorf("chroma: failed to process left operand of '%s' at %s: %w",
			expr.Op.String(), expr.Start().String(), err)
	}

	rightClause, err := v.buildNestedClause(expr.Right)
	if err != nil {
		return fmt.Errorf("chroma: failed to process right operand of '%s' at %s: %w",
			expr.Op.String(), expr.Start().String(), err)
	}

	switch expr.Op {
	case filter.OpAnd:
		v.result = v2.And(leftClause, rightClause)
	case filter.OpOr:
		v.result = v2.Or(leftClause, rightClause)
	default:
		return fmt.Errorf("chroma: unexpected logical operator '%s' at %s",
			expr.Op.String(), expr.Start().String())
	}
	return nil
}

// visitEqualityExpr handles == and != operators.
// The appropriate Chroma Eq*/NotEq* function is chosen based on the value type.
func (v *Visitor) visitEqualityExpr(expr *filter.BinaryExpr) error {
	fieldKey, err := v.extractFieldKey(expr.Left)
	if err != nil {
		return fmt.Errorf("chroma: failed to extract field key from left operand of '%s' at %s: %w",
			expr.Op.String(), expr.Start().String(), err)
	}

	fieldValue, err := v.extractFieldValue(expr.Right)
	if err != nil {
		return fmt.Errorf("chroma: failed to extract value from right operand of '%s' at %s: %w",
			expr.Op.String(), expr.Start().String(), err)
	}

	clause, err := v.buildEqualityClause(fieldKey, fieldValue, expr.Op)
	if err != nil {
		return fmt.Errorf("chroma: failed to build equality clause for '%s' at %s: %w",
			expr.Op.String(), expr.Start().String(), err)
	}

	v.result = clause
	return nil
}

// buildEqualityClause creates an Eq or NotEq WhereClause for the given typed value.
// float64 values that are whole numbers are treated as int; fractional values as float32.
func (v *Visitor) buildEqualityClause(fieldKey string, fieldValue any, op filter.Operator) (v2.WhereClause, error) {
	isEq := op == filter.OpEqual

	switch val := fieldValue.(type) {
	case string:
		if isEq {
			return v2.EqString(fieldKey, val), nil
		}
		return v2.NotEqString(fieldKey, val), nil

	case float64:
		if intVal, isInt := toInt(val); isInt {
			if isEq {
				return v2.EqInt(fieldKey, intVal), nil
			}
			return v2.NotEqInt(fieldKey, intVal), nil
		}
		if isEq {
			return v2.EqFloat(fieldKey, float32(val)), nil
		}
		return v2.NotEqFloat(fieldKey, float32(val)), nil

	case bool:
		if isEq {
			return v2.EqBool(fieldKey, val), nil
		}
		return v2.NotEqBool(fieldKey, val), nil

	default:
		return nil, fmt.Errorf("chroma: unsupported value type %T for equality condition", fieldValue)
	}
}

// visitOrderingExpr handles <, <=, >, >= operators.
// Whole-number values use the int variants of the Chroma comparison functions;
// fractional values use the float32 variants.
func (v *Visitor) visitOrderingExpr(expr *filter.BinaryExpr) error {
	fieldKey, err := v.extractFieldKey(expr.Left)
	if err != nil {
		return fmt.Errorf("chroma: failed to extract field key from left operand of '%s' at %s: %w",
			expr.Op.String(), expr.Start().String(), err)
	}

	fieldValue, err := v.extractFieldValue(expr.Right)
	if err != nil {
		return fmt.Errorf("chroma: failed to extract value from right operand of '%s' at %s: %w",
			expr.Op.String(), expr.Start().String(), err)
	}

	numericValue, err := cast.ToFloat64E(fieldValue)
	if err != nil {
		return fmt.Errorf("chroma: cannot convert value to number for '%s' comparison at %s: expected number, got %T",
			expr.Op.String(), expr.Start().String(), fieldValue)
	}

	intVal, isInt := toInt(numericValue)
	f32Val := float32(numericValue)

	var clause v2.WhereClause
	switch expr.Op {
	case filter.OpLess:
		if isInt {
			clause = v2.LtInt(fieldKey, intVal)
		} else {
			clause = v2.LtFloat(fieldKey, f32Val)
		}
	case filter.OpLessEqual:
		if isInt {
			clause = v2.LteInt(fieldKey, intVal)
		} else {
			clause = v2.LteFloat(fieldKey, f32Val)
		}
	case filter.OpGreater:
		if isInt {
			clause = v2.GtInt(fieldKey, intVal)
		} else {
			clause = v2.GtFloat(fieldKey, f32Val)
		}
	case filter.OpGreaterEqual:
		if isInt {
			clause = v2.GteInt(fieldKey, intVal)
		} else {
			clause = v2.GteFloat(fieldKey, f32Val)
		}
	default:
		return fmt.Errorf("chroma: unexpected ordering operator '%s' at %s",
			expr.Op.String(), expr.Start().String())
	}

	v.result = clause
	return nil
}

// visitInExpr handles the IN operator for membership testing.
// The element type of the list determines which In* function is used:
//   - string list → v2.InString
//   - whole-number float64 list → v2.InInt
//   - fractional float64 list → v2.InFloat (float32 values)
//   - bool list → v2.InBool
func (v *Visitor) visitInExpr(expr *filter.BinaryExpr) error {
	fieldKey, err := v.extractFieldKey(expr.Left)
	if err != nil {
		return fmt.Errorf("chroma: failed to extract field key from left operand of 'IN' at %s: %w",
			expr.Start().String(), err)
	}

	listLit, err := filterhelp.RequireListLiteral(expr)
	if err != nil {
		return fmt.Errorf("chroma: %w", err)
	}

	if err = v.visitListLiteral(listLit); err != nil {
		return err
	}

	values, ok := v.currentFieldValue.([]any)
	if !ok || len(values) == 0 {
		return fmt.Errorf("chroma: failed to extract list values for 'IN' operator at %s",
			expr.Start().String())
	}

	switch values[0].(type) {
	case string:
		strs := make([]string, 0, len(values))
		for _, val := range values {
			strs = append(strs, cast.ToString(val))
		}
		v.result = v2.InString(fieldKey, strs...)

	case float64:
		// Decide int vs float32 by inspecting all values.
		allInt := true
		for _, val := range values {
			f := cast.ToFloat64(val)
			if _, ok := toInt(f); !ok {
				allInt = false
				break
			}
		}
		if allInt {
			ints := make([]int, 0, len(values))
			for _, val := range values {
				ints = append(ints, int(cast.ToFloat64(val)))
			}
			v.result = v2.InInt(fieldKey, ints...)
		} else {
			floats := make([]float32, 0, len(values))
			for _, val := range values {
				floats = append(floats, float32(cast.ToFloat64(val)))
			}
			v.result = v2.InFloat(fieldKey, floats...)
		}

	case bool:
		bools := make([]bool, 0, len(values))
		for _, val := range values {
			bools = append(bools, cast.ToBool(val))
		}
		v.result = v2.InBool(fieldKey, bools...)

	default:
		return fmt.Errorf("chroma: unsupported value type %T in 'IN' list at %s",
			values[0], expr.Start().String())
	}

	return nil
}

// buildNestedClause processes an expression with an isolated Visitor and returns
// the resulting WhereClause. This prevents condition state from leaking across
// different branches of a logical expression.
func (v *Visitor) buildNestedClause(expr filter.Expr) (v2.WhereClause, error) {
	switch node := expr.(type) {
	case *filter.BinaryExpr, *filter.UnaryExpr:
		nested := NewVisitor()
		if err := nested.visit(node); err != nil {
			return nil, err
		}
		return nested.result, nil
	default:
		return nil, fmt.Errorf("chroma: unsupported expression type %T for clause building", node)
	}
}

// extractFieldKey extracts and returns the field key from expr while preserving
// the caller's currentFieldKey state.
func (v *Visitor) extractFieldKey(expr filter.Expr) (string, error) {
	saved := v.currentFieldKey
	v.currentFieldKey = ""

	err := v.visit(expr)

	extracted := v.currentFieldKey
	v.currentFieldKey = saved

	if err != nil {
		return "", err
	}
	if extracted == "" {
		return "", fmt.Errorf("chroma: failed to extract field key from %T expression", expr)
	}
	return extracted, nil
}

// extractFieldValue extracts and returns the value from expr while preserving
// the caller's currentFieldValue state.
func (v *Visitor) extractFieldValue(expr filter.Expr) (any, error) {
	saved := v.currentFieldValue
	v.currentFieldValue = nil

	err := v.visit(expr)

	extracted := v.currentFieldValue
	v.currentFieldValue = saved

	if err != nil {
		return nil, err
	}
	if extracted == nil {
		return nil, fmt.Errorf("chroma: failed to extract value from %T expression", expr)
	}
	return extracted, nil
}

// buildIndexedFieldKey constructs a dot-separated field key from an IndexExpr,
// stripping the base identifier (e.g. "metadata") so that only the inner key
// path is returned. This matches Chroma's flat metadata key space.
//
// Examples:
//
//	metadata["author"]   → "author"
//	metadata["a"]["b"]   → "a.b"
func (v *Visitor) buildIndexedFieldKey(expr *filter.IndexExpr) (string, error) {
	var pathParts []string

	current := expr
	for {
		if err := v.visitLiteral(current.Index); err != nil {
			return "", err
		}

		switch val := v.currentFieldValue.(type) {
		case string:
			pathParts = append([]string{val}, pathParts...)
		case float64:
			pathParts = append([]string{strconv.Itoa(int(val))}, pathParts...)
		default:
			return "", fmt.Errorf("chroma: invalid index type %T, expected string or number", v.currentFieldValue)
		}

		switch left := current.Left.(type) {
		case *filter.IndexExpr:
			current = left
		case *filter.Ident:
			// Strip the base identifier (e.g. "metadata") — Chroma metadata
			// keys are flat, so only the inner path is needed.
			return strings.Join(pathParts, "."), nil
		default:
			return "", fmt.Errorf("chroma: invalid left operand type %T in index expression", left)
		}
	}
}

// literalToValue converts an AST literal to its Go equivalent.
func (v *Visitor) literalToValue(lit *filter.Literal) (any, error) {
	if lit.IsString() {
		return lit.AsString()
	}
	if lit.IsNumber() {
		return lit.AsNumber()
	}
	if lit.IsBool() {
		return lit.AsBool()
	}
	return nil, fmt.Errorf("chroma: unsupported literal type '%s'", lit.Kind)
}

// toInt returns (int(f), true) when f is a whole number that fits in int,
// or (0, false) when it has a fractional part.
func toInt(f float64) (int, bool) {
	i := int(f)
	if float64(i) == f {
		return i, true
	}
	return 0, false
}

// ToFilter converts an AST filter expression into a Chroma WhereClause.
//
// Returns (nil, nil) when expr is nil (no filter applied).
// The returned WhereClause satisfies the v2.WhereFilter interface required by
// v2.WithWhere.
func ToFilter(expr filter.Expr) (v2.WhereClause, error) {
	if expr == nil {
		return nil, nil
	}
	vis := NewVisitor()
	if err := vis.Visit(expr); err != nil {
		return nil, err
	}
	return vis.Result(), nil
}
