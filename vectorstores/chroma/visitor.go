package chroma

import (
	"fmt"
	"strings"

	v2 "github.com/amikos-tech/chroma-go/pkg/api/v2"
	"github.com/spf13/cast"

	"github.com/Tangerg/lynx/core/vectorstore/filter/ast"
	"github.com/Tangerg/lynx/core/vectorstore/filter/token"
)

var _ ast.Visitor = (*Visitor)(nil)

// Visitor transforms AST filter expressions into Chroma WhereClause conditions.
// It implements the ast.Visitor interface to traverse and convert expression trees
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
	err               error        // last error encountered during conversion
	result            v2.WhereClause // the Chroma filter clause being built
	currentFieldKey   string       // temporary storage for field key extraction
	currentFieldValue any          // temporary storage for field value extraction
}

// NewVisitor creates a new Visitor ready to process AST expressions.
func NewVisitor() *Visitor {
	return &Visitor{}
}

// Result returns the constructed WhereClause.
// Returns nil if an error occurred during conversion.
// Should only be called after Visit() completes.
func (v *Visitor) Result() v2.WhereClause {
	if v.err != nil {
		return nil
	}
	return v.result
}

// Error returns the last error encountered during conversion.
func (v *Visitor) Error() error {
	return v.err
}

// Visit implements ast.Visitor. It processes the expression in a single pass
// and always returns nil to halt further automatic traversal.
func (v *Visitor) Visit(expr ast.Expr) ast.Visitor {
	v.err = v.visit(expr)
	return nil
}

// visit dispatches to the appropriate handler based on the expression type.
func (v *Visitor) visit(expr ast.Expr) error {
	if expr == nil {
		return fmt.Errorf("cannot process nil expression")
	}
	if v.err != nil {
		return v.err
	}

	switch node := expr.(type) {
	case *ast.BinaryExpr:
		return v.visitBinaryExpr(node)
	case *ast.UnaryExpr:
		return v.visitUnaryExpr(node)
	case *ast.IndexExpr:
		return v.visitIndexExpr(node)
	case *ast.Ident:
		return v.visitIdent(node)
	case *ast.Literal:
		return v.visitLiteral(node)
	case *ast.ListLiteral:
		return v.visitListLiteral(node)
	default:
		return fmt.Errorf("unsupported expression type %T", node)
	}
}

// visitBinaryExpr routes to the correct handler based on the operator category.
func (v *Visitor) visitBinaryExpr(expr *ast.BinaryExpr) error {
	switch {
	case expr.Op.Kind.IsLogicalOperator():
		return v.visitLogicalExpr(expr)
	case expr.Op.Kind.IsEqualityOperator():
		return v.visitEqualityExpr(expr)
	case expr.Op.Kind.IsOrderingOperator():
		return v.visitOrderingExpr(expr)
	case expr.Op.Kind.Is(token.IN):
		return v.visitInExpr(expr)
	case expr.Op.Kind.Is(token.LIKE):
		return fmt.Errorf("chroma: LIKE operator is not supported on metadata fields")
	default:
		return fmt.Errorf("unsupported binary operator '%s' at %s",
			expr.Op.Literal, expr.Start().String())
	}
}

// visitUnaryExpr handles unary operators. Only NOT is defined by the AST;
// Chroma does not expose a standalone logical NOT, so it always returns an error.
func (v *Visitor) visitUnaryExpr(expr *ast.UnaryExpr) error {
	if !expr.Op.Kind.IsUnaryOperator() {
		return fmt.Errorf("'%s' is not a valid unary operator at %s",
			expr.Op.Literal, expr.Start().String())
	}
	return fmt.Errorf("chroma: NOT operator is not supported; rewrite using != or NIN")
}

// visitIdent stores the identifier name as the current field key.
func (v *Visitor) visitIdent(ident *ast.Ident) error {
	v.currentFieldKey = ident.Value
	return nil
}

// visitLiteral converts a literal node to a Go value and stores it.
func (v *Visitor) visitLiteral(lit *ast.Literal) error {
	value, err := v.literalToValue(lit)
	if err != nil {
		return fmt.Errorf("failed to convert literal at %s: %w", lit.Start().String(), err)
	}
	v.currentFieldValue = value
	return nil
}

// visitListLiteral converts a list of literals to a []any slice and stores it.
func (v *Visitor) visitListLiteral(list *ast.ListLiteral) error {
	values := make([]any, 0, len(list.Values))
	for i, lit := range list.Values {
		value, err := v.literalToValue(lit)
		if err != nil {
			return fmt.Errorf("failed to convert list element at index %d: %w", i, err)
		}
		values = append(values, value)
	}
	v.currentFieldValue = values
	return nil
}

// visitIndexExpr builds a field key from an indexed expression and stores it.
// metadata["author"]      → "author"
// metadata["a"]["b"]      → "a.b"
func (v *Visitor) visitIndexExpr(expr *ast.IndexExpr) error {
	fieldKey, err := v.buildIndexedFieldKey(expr)
	if err != nil {
		return fmt.Errorf("failed to build field path at %s: %w", expr.Start().String(), err)
	}
	v.currentFieldKey = fieldKey
	return nil
}

// visitLogicalExpr handles AND and OR operators.
// Each operand is processed in isolation and the results are combined with
// v2.And or v2.Or respectively.
func (v *Visitor) visitLogicalExpr(expr *ast.BinaryExpr) error {
	leftClause, err := v.buildNestedClause(expr.Left)
	if err != nil {
		return fmt.Errorf("failed to process left operand of '%s' at %s: %w",
			expr.Op.Literal, expr.Start().String(), err)
	}

	rightClause, err := v.buildNestedClause(expr.Right)
	if err != nil {
		return fmt.Errorf("failed to process right operand of '%s' at %s: %w",
			expr.Op.Literal, expr.Start().String(), err)
	}

	switch expr.Op.Kind {
	case token.AND:
		v.result = v2.And(leftClause, rightClause)
	case token.OR:
		v.result = v2.Or(leftClause, rightClause)
	default:
		return fmt.Errorf("unexpected logical operator '%s' at %s",
			expr.Op.Literal, expr.Start().String())
	}
	return nil
}

// visitEqualityExpr handles == and != operators.
// The appropriate Chroma Eq*/NotEq* function is chosen based on the value type.
func (v *Visitor) visitEqualityExpr(expr *ast.BinaryExpr) error {
	fieldKey, err := v.extractFieldKey(expr.Left)
	if err != nil {
		return fmt.Errorf("failed to extract field key from left operand of '%s' at %s: %w",
			expr.Op.Literal, expr.Start().String(), err)
	}

	fieldValue, err := v.extractFieldValue(expr.Right)
	if err != nil {
		return fmt.Errorf("failed to extract value from right operand of '%s' at %s: %w",
			expr.Op.Literal, expr.Start().String(), err)
	}

	clause, err := v.buildEqualityClause(fieldKey, fieldValue, expr.Op.Kind)
	if err != nil {
		return fmt.Errorf("failed to build equality clause for '%s' at %s: %w",
			expr.Op.Literal, expr.Start().String(), err)
	}

	v.result = clause
	return nil
}

// buildEqualityClause creates an Eq or NotEq WhereClause for the given typed value.
// float64 values that are whole numbers are treated as int; fractional values as float32.
func (v *Visitor) buildEqualityClause(fieldKey string, fieldValue any, op token.Kind) (v2.WhereClause, error) {
	isEq := op == token.EQ

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
		return nil, fmt.Errorf("unsupported value type %T for equality condition", fieldValue)
	}
}

// visitOrderingExpr handles <, <=, >, >= operators.
// Whole-number values use the int variants of the Chroma comparison functions;
// fractional values use the float32 variants.
func (v *Visitor) visitOrderingExpr(expr *ast.BinaryExpr) error {
	fieldKey, err := v.extractFieldKey(expr.Left)
	if err != nil {
		return fmt.Errorf("failed to extract field key from left operand of '%s' at %s: %w",
			expr.Op.Literal, expr.Start().String(), err)
	}

	fieldValue, err := v.extractFieldValue(expr.Right)
	if err != nil {
		return fmt.Errorf("failed to extract value from right operand of '%s' at %s: %w",
			expr.Op.Literal, expr.Start().String(), err)
	}

	numericValue, err := cast.ToFloat64E(fieldValue)
	if err != nil {
		return fmt.Errorf("cannot convert value to number for '%s' comparison at %s: expected number, got %T",
			expr.Op.Literal, expr.Start().String(), fieldValue)
	}

	intVal, isInt := toInt(numericValue)
	f32Val := float32(numericValue)

	var clause v2.WhereClause
	switch expr.Op.Kind {
	case token.LT:
		if isInt {
			clause = v2.LtInt(fieldKey, intVal)
		} else {
			clause = v2.LtFloat(fieldKey, f32Val)
		}
	case token.LE:
		if isInt {
			clause = v2.LteInt(fieldKey, intVal)
		} else {
			clause = v2.LteFloat(fieldKey, f32Val)
		}
	case token.GT:
		if isInt {
			clause = v2.GtInt(fieldKey, intVal)
		} else {
			clause = v2.GtFloat(fieldKey, f32Val)
		}
	case token.GE:
		if isInt {
			clause = v2.GteInt(fieldKey, intVal)
		} else {
			clause = v2.GteFloat(fieldKey, f32Val)
		}
	default:
		return fmt.Errorf("unexpected ordering operator '%s' at %s",
			expr.Op.Literal, expr.Start().String())
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
func (v *Visitor) visitInExpr(expr *ast.BinaryExpr) error {
	fieldKey, err := v.extractFieldKey(expr.Left)
	if err != nil {
		return fmt.Errorf("failed to extract field key from left operand of 'IN' at %s: %w",
			expr.Start().String(), err)
	}

	listLit, ok := expr.Right.(*ast.ListLiteral)
	if !ok {
		return fmt.Errorf("'IN' operator requires a list on the right side at %s, got %T",
			expr.Start().String(), expr.Right)
	}
	if len(listLit.Values) == 0 {
		return fmt.Errorf("'IN' operator requires a non-empty list at %s",
			expr.Start().String())
	}

	if err = v.visitListLiteral(listLit); err != nil {
		return err
	}

	values, ok := v.currentFieldValue.([]any)
	if !ok || len(values) == 0 {
		return fmt.Errorf("failed to extract list values for 'IN' operator at %s",
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
		return fmt.Errorf("unsupported value type %T in 'IN' list at %s",
			values[0], expr.Start().String())
	}

	return nil
}

// buildNestedClause processes an expression with an isolated Visitor and returns
// the resulting WhereClause. This prevents condition state from leaking across
// different branches of a logical expression.
func (v *Visitor) buildNestedClause(expr ast.Expr) (v2.WhereClause, error) {
	switch node := expr.(type) {
	case *ast.BinaryExpr, *ast.UnaryExpr:
		nested := NewVisitor()
		if err := nested.visit(node); err != nil {
			return nil, err
		}
		return nested.result, nil
	default:
		return nil, fmt.Errorf("unsupported expression type %T for clause building", node)
	}
}

// extractFieldKey extracts and returns the field key from expr while preserving
// the caller's currentFieldKey state.
func (v *Visitor) extractFieldKey(expr ast.Expr) (string, error) {
	saved := v.currentFieldKey
	v.currentFieldKey = ""

	err := v.visit(expr)

	extracted := v.currentFieldKey
	v.currentFieldKey = saved

	if err != nil {
		return "", err
	}
	if extracted == "" {
		return "", fmt.Errorf("failed to extract field key from %T expression", expr)
	}
	return extracted, nil
}

// extractFieldValue extracts and returns the value from expr while preserving
// the caller's currentFieldValue state.
func (v *Visitor) extractFieldValue(expr ast.Expr) (any, error) {
	saved := v.currentFieldValue
	v.currentFieldValue = nil

	err := v.visit(expr)

	extracted := v.currentFieldValue
	v.currentFieldValue = saved

	if err != nil {
		return nil, err
	}
	if extracted == nil {
		return nil, fmt.Errorf("failed to extract value from %T expression", expr)
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
func (v *Visitor) buildIndexedFieldKey(expr *ast.IndexExpr) (string, error) {
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
			pathParts = append([]string{fmt.Sprintf("%d", int(val))}, pathParts...)
		default:
			return "", fmt.Errorf("invalid index type %T, expected string or number", v.currentFieldValue)
		}

		switch left := current.Left.(type) {
		case *ast.IndexExpr:
			current = left
		case *ast.Ident:
			// Strip the base identifier (e.g. "metadata") — Chroma metadata
			// keys are flat, so only the inner path is needed.
			return strings.Join(pathParts, "."), nil
		default:
			return "", fmt.Errorf("invalid left operand type %T in index expression", left)
		}
	}
}

// literalToValue converts an AST literal to its Go equivalent.
func (v *Visitor) literalToValue(lit *ast.Literal) (any, error) {
	if lit.IsString() {
		return lit.AsString()
	}
	if lit.IsNumber() {
		return lit.AsNumber()
	}
	if lit.IsBool() {
		return lit.AsBool()
	}
	return nil, fmt.Errorf("unsupported literal type '%s'", lit.Token.Kind.Name())
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
func ToFilter(expr ast.Expr) (v2.WhereClause, error) {
	if expr == nil {
		return nil, nil
	}
	vis := NewVisitor()
	vis.Visit(expr)
	return vis.Result(), vis.Error()
}
