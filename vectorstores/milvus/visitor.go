package milvus

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Tangerg/lynx/core/vectorstore/filter/ast"
	"github.com/Tangerg/lynx/core/vectorstore/filter/token"
	"github.com/Tangerg/lynx/vectorstores/internal/filterhelp"
)

var _ ast.Visitor = (*Visitor)(nil)

// Visitor transforms AST filter expressions into Milvus filter expression strings.
// It implements the ast.Visitor interface to traverse and convert expression trees
// into Milvus's native string expression format.
//
// The converter maintains internal state during traversal:
//   - result: The expression string being built
//   - currentFieldKey: Temporary storage for extracted field identifiers
//   - currentFieldValue: Temporary storage for encoded literal values
//   - err: The last error encountered during conversion
//
// Conversion strategy:
//   - Each visit method sets result to the expression string for the current node
//   - Nested expressions (logical operators, NOT) create isolated converters
//   - Field extraction methods preserve state during recursive calls
//
// Usage example:
//
//	expr := parseFilterExpression("age > 18 AND status == 'active'")
//	filter, err := ToFilter(expr)
//	if err != nil {
//	    log.Fatal(err)
//	}
type Visitor struct {
	err               error  // Last error encountered during conversion
	result            string // The Milvus expression string being built
	currentFieldKey   string // Temporary storage for field paths during extraction
	currentFieldValue string // Temporary storage for encoded values during extraction
}

func NewVisitor() *Visitor {
	return &Visitor{}
}

// Result returns the constructed Milvus filter expression string.
// Returns an empty string if an error occurred during conversion.
// Should only be called after Visit() completes.
func (v *Visitor) Result() string {
	if v.err != nil {
		return ""
	}
	return v.result
}

// Visit implements the ast.Visitor interface.
// It walks the whole tree rooted at expr and returns the first error
// encountered, or nil when the entire expression was accepted.
func (v *Visitor) Visit(expr ast.Expr) error {
	v.err = v.visit(expr)
	return v.err
}

// visit dispatches conversion to specialized methods based on expression type.
func (v *Visitor) visit(expr ast.Expr) error {
	if expr == nil {
		return fmt.Errorf("milvus: cannot process nil expression")
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
		return fmt.Errorf("milvus: unsupported expression type %T", node)
	}
}

// visitBinaryExpr routes via [filterhelp.DispatchBinaryErr]. The
// comparison wrapper splits equality vs ordering since milvus emits
// distinct expression shapes for the two families.
func (v *Visitor) visitBinaryExpr(expr *ast.BinaryExpr) error {
	return filterhelp.DispatchBinaryErr(expr,
		v.visitLogicalExpr,
		v.visitComparisonExpr,
		v.visitInExpr,
		v.visitLikeExpr,
	)
}

// visitComparisonExpr routes to equality or ordering based on the
// operator family.
func (v *Visitor) visitComparisonExpr(expr *ast.BinaryExpr) error {
	if expr.Op.Kind.IsEqualityOperator() {
		return v.visitEqualityExpr(expr)
	}
	return v.visitOrderingExpr(expr)
}

// visitUnaryExpr handles unary expressions — only NOT today.
func (v *Visitor) visitUnaryExpr(expr *ast.UnaryExpr) error {
	return filterhelp.DispatchUnaryErr(expr, v.visitNotExpr)
}

// visitIdent extracts and stores the identifier name as the current field key.
func (v *Visitor) visitIdent(ident *ast.Ident) error {
	v.currentFieldKey = ident.Value
	return nil
}

// visitLiteral converts an AST literal to its Milvus expression encoding and stores it.
//
// Encoding rules:
//   - Strings are wrapped in double quotes with internal double quotes escaped.
//   - Whole numbers are formatted as integers (no decimal point).
//   - Fractional numbers use %g notation.
//   - Booleans use Milvus syntax: True / False.
func (v *Visitor) visitLiteral(lit *ast.Literal) error {
	value, err := v.literalToString(lit)
	if err != nil {
		return err
	}
	v.currentFieldValue = value
	return nil
}

// visitListLiteral converts a list of literals to a Milvus list expression and stores it.
// Example output: ["active", "pending"] or [18, 21, 25]
func (v *Visitor) visitListLiteral(list *ast.ListLiteral) error {
	parts := make([]string, 0, len(list.Values))

	for i, lit := range list.Values {
		s, err := v.literalToString(lit)
		if err != nil {
			return fmt.Errorf("milvus: failed to convert list element at index %d: %w", i, err)
		}
		parts = append(parts, s)
	}

	v.currentFieldValue = "[" + strings.Join(parts, ", ") + "]"
	return nil
}

// visitIndexExpr processes indexed field access and builds a bracket-notation field path.
// Example transformations:
//   - metadata["user"] → metadata["user"]
//   - data["tags"][0] → data["tags"][0]
//   - config["db"]["host"] → config["db"]["host"]
func (v *Visitor) visitIndexExpr(expr *ast.IndexExpr) error {
	fieldKey, err := v.buildIndexedFieldKey(expr)
	if err != nil {
		return fmt.Errorf("milvus: failed to build field path at %s: %w",
			expr.Start().String(), err)
	}
	v.currentFieldKey = fieldKey
	return nil
}

// visitLogicalExpr handles logical operators (AND, OR).
// Each operand is converted using an isolated converter, then combined:
//   - AND: (left) and (right)
//   - OR:  (left) or (right)
func (v *Visitor) visitLogicalExpr(expr *ast.BinaryExpr) error {
	left, err := v.buildNestedExpr(expr.Left)
	if err != nil {
		return fmt.Errorf("milvus: failed to process left operand of '%s' at %s: %w",
			expr.Op.Literal, expr.Start().String(), err)
	}

	right, err := v.buildNestedExpr(expr.Right)
	if err != nil {
		return fmt.Errorf("milvus: failed to process right operand of '%s' at %s: %w",
			expr.Op.Literal, expr.Start().String(), err)
	}

	switch expr.Op.Kind {
	case token.AND:
		v.result = fmt.Sprintf("(%s) and (%s)", left, right)
	case token.OR:
		v.result = fmt.Sprintf("(%s) or (%s)", left, right)
	default:
		return fmt.Errorf("milvus: unexpected logical operator '%s' at %s",
			expr.Op.Literal, expr.Start().String())
	}

	return nil
}

// visitNotExpr handles the NOT operator.
// Example: NOT (age > 18) → not (age > 18)
func (v *Visitor) visitNotExpr(expr *ast.UnaryExpr) error {
	operand, err := v.buildNestedExpr(expr.Right)
	if err != nil {
		return fmt.Errorf("milvus: failed to process NOT operand at %s: %w",
			expr.Start().String(), err)
	}

	v.result = fmt.Sprintf("not (%s)", operand)
	return nil
}

// visitEqualityExpr handles equality operators (==, !=).
// Examples:
//   - status == "active"
//   - age != 18
func (v *Visitor) visitEqualityExpr(expr *ast.BinaryExpr) error {
	fieldKey, err := v.extractFieldKey(expr.Left)
	if err != nil {
		return fmt.Errorf("milvus: failed to extract field key from '%s' at %s: %w",
			expr.Op.Literal, expr.Start().String(), err)
	}

	fieldValue, err := v.extractFieldValue(expr.Right)
	if err != nil {
		return fmt.Errorf("milvus: failed to extract value from '%s' at %s: %w",
			expr.Op.Literal, expr.Start().String(), err)
	}

	switch expr.Op.Kind {
	case token.EQ:
		v.result = fmt.Sprintf("%s == %s", fieldKey, fieldValue)
	case token.NE:
		v.result = fmt.Sprintf("%s != %s", fieldKey, fieldValue)
	default:
		return fmt.Errorf("milvus: unexpected equality operator '%s' at %s",
			expr.Op.Literal, expr.Start().String())
	}

	return nil
}

// visitOrderingExpr handles ordering operators (<, <=, >, >=).
// Examples:
//   - age > 18
//   - price <= 99.99
func (v *Visitor) visitOrderingExpr(expr *ast.BinaryExpr) error {
	fieldKey, err := v.extractFieldKey(expr.Left)
	if err != nil {
		return fmt.Errorf("milvus: failed to extract field key from '%s' at %s: %w",
			expr.Op.Literal, expr.Start().String(), err)
	}

	fieldValue, err := v.extractFieldValue(expr.Right)
	if err != nil {
		return fmt.Errorf("milvus: failed to extract value from '%s' at %s: %w",
			expr.Op.Literal, expr.Start().String(), err)
	}

	switch expr.Op.Kind {
	case token.LT:
		v.result = fmt.Sprintf("%s < %s", fieldKey, fieldValue)
	case token.LE:
		v.result = fmt.Sprintf("%s <= %s", fieldKey, fieldValue)
	case token.GT:
		v.result = fmt.Sprintf("%s > %s", fieldKey, fieldValue)
	case token.GE:
		v.result = fmt.Sprintf("%s >= %s", fieldKey, fieldValue)
	default:
		return fmt.Errorf("milvus: unexpected ordering operator '%s' at %s",
			expr.Op.Literal, expr.Start().String())
	}

	return nil
}

// visitInExpr handles the IN operator for membership testing.
// The right operand must be a non-empty list literal.
// Example: status in ["active", "pending"]
func (v *Visitor) visitInExpr(expr *ast.BinaryExpr) error {
	fieldKey, err := v.extractFieldKey(expr.Left)
	if err != nil {
		return fmt.Errorf("milvus: failed to extract field key from 'IN' at %s: %w",
			expr.Start().String(), err)
	}

	listLit, err := filterhelp.RequireListLiteral(expr)
	if err != nil {
		return fmt.Errorf("milvus: %w", err)
	}

	if err = v.visitListLiteral(listLit); err != nil {
		return err
	}

	v.result = fmt.Sprintf("%s in %s", fieldKey, v.currentFieldValue)
	return nil
}

// visitLikeExpr handles the LIKE operator for pattern matching.
// The right operand must be a string literal.
// Example: name like "go%"
func (v *Visitor) visitLikeExpr(expr *ast.BinaryExpr) error {
	fieldKey, err := v.extractFieldKey(expr.Left)
	if err != nil {
		return fmt.Errorf("milvus: failed to extract field key from 'LIKE' at %s: %w",
			expr.Start().String(), err)
	}

	lit, ok := expr.Right.(*ast.Literal)
	if !ok {
		return fmt.Errorf("milvus: 'LIKE' operator requires a string literal on the right side at %s, got %T",
			expr.Start().String(), expr.Right)
	}
	if !lit.IsString() {
		return fmt.Errorf("milvus: 'LIKE' operator requires a string pattern at %s, got %s",
			expr.Start().String(), lit.Token.Kind.Name())
	}

	if err = v.visitLiteral(lit); err != nil {
		return err
	}

	v.result = fmt.Sprintf("%s like %s", fieldKey, v.currentFieldValue)
	return nil
}

// buildNestedExpr converts a sub-expression to a string using an isolated converter.
// This ensures that nested logical expressions maintain proper scoping.
func (v *Visitor) buildNestedExpr(expr ast.Expr) (string, error) {
	nested := NewVisitor()
	if err := nested.visit(expr); err != nil {
		return "", err
	}
	if nested.result != "" {
		return nested.result, nil
	}
	// Simple leaf expressions (ident, literal) set currentFieldKey/Value rather than result
	if nested.currentFieldKey != "" {
		return nested.currentFieldKey, nil
	}
	if nested.currentFieldValue != "" {
		return nested.currentFieldValue, nil
	}
	return "", fmt.Errorf("milvus: unsupported expression type %T for nested expression", expr)
}

// extractFieldKey extracts a field key (identifier or bracket path) from an expression.
// The converter's currentFieldKey state is preserved during extraction.
func (v *Visitor) extractFieldKey(expr ast.Expr) (string, error) {
	savedKey := v.currentFieldKey
	v.currentFieldKey = ""

	err := v.visit(expr)

	extracted := v.currentFieldKey
	v.currentFieldKey = savedKey

	if err != nil {
		return "", err
	}
	if extracted == "" {
		return "", fmt.Errorf("milvus: failed to extract field key from %T expression", expr)
	}

	return extracted, nil
}

// extractFieldValue extracts an encoded value (literal or list) from an expression.
// The converter's currentFieldValue state is preserved during extraction.
func (v *Visitor) extractFieldValue(expr ast.Expr) (string, error) {
	savedValue := v.currentFieldValue
	v.currentFieldValue = ""

	err := v.visit(expr)

	extracted := v.currentFieldValue
	v.currentFieldValue = savedValue

	if err != nil {
		return "", err
	}
	if extracted == "" {
		return "", fmt.Errorf("milvus: failed to extract value from %T expression", expr)
	}

	return extracted, nil
}

// buildIndexedFieldKey constructs a bracket-notation field path from an index expression.
// This method recursively processes nested index expressions to build the complete path.
//
// Transformation examples:
//   - user["name"]                → user["name"]
//   - metadata["tags"][0]         → metadata["tags"][0]
//   - config["db"]["host"]        → config["db"]["host"]
func (v *Visitor) buildIndexedFieldKey(expr *ast.IndexExpr) (string, error) {
	var parts []string

	current := expr
	for {
		if err := v.visitLiteral(current.Index); err != nil {
			return "", err
		}

		parts = append([]string{"[" + v.currentFieldValue + "]"}, parts...)

		switch left := current.Left.(type) {
		case *ast.IndexExpr:
			current = left
		case *ast.Ident:
			return left.Value + strings.Join(parts, ""), nil
		default:
			return "", fmt.Errorf("milvus: invalid left operand type %T in index expression, expected identifier or index",
				left)
		}
	}
}

// literalToString converts an AST literal to its Milvus expression string encoding.
func (v *Visitor) literalToString(lit *ast.Literal) (string, error) {
	if lit.IsString() {
		s, err := lit.AsString()
		if err != nil {
			return "", fmt.Errorf("milvus: failed to convert string literal at %s: %w",
				lit.Start().String(), err)
		}
		return fmt.Sprintf(`"%s"`, strings.ReplaceAll(s, `"`, `\"`)), nil
	}

	if lit.IsNumber() {
		n, err := lit.AsNumber()
		if err != nil {
			return "", fmt.Errorf("milvus: failed to convert number literal at %s: %w",
				lit.Start().String(), err)
		}
		if n == float64(int64(n)) {
			return strconv.FormatInt(int64(n), 10), nil
		}
		return strconv.FormatFloat(n, 'g', -1, 64), nil
	}

	if lit.IsBool() {
		b, err := lit.AsBool()
		if err != nil {
			return "", fmt.Errorf("milvus: failed to convert bool literal at %s: %w",
				lit.Start().String(), err)
		}
		if b {
			return "True", nil
		}
		return "False", nil
	}

	return "", fmt.Errorf("milvus: unsupported literal type '%s' at %s",
		lit.Token.Kind.Name(), lit.Start().String())
}

// ToFilter converts an AST filter expression into a Milvus filter expression string.
//
// This is the main entry point for converting filter expressions written in
// the Lynx filter DSL into Milvus's native string expression format.
//
// Supported operations:
//   - Logical:          AND, OR, NOT
//   - Equality:         ==, !=
//   - Ordering:         <, <=, >, >=
//   - Membership:       IN
//   - Pattern matching: LIKE
//
// Field access:
//   - Simple field:    age                   → age
//   - JSON key:        metadata["key"]       → metadata["key"]
//   - Nested JSON:     metadata["a"]["b"]    → metadata["a"]["b"]
//
// Value encoding:
//   - Strings: double-quoted with escaped internal double quotes
//   - Numbers: integer if whole, %g otherwise
//   - Booleans: True / False
//
// Example:
//
//	expr, _ := parser.Parse(`age > 18 AND status == "active"`)
//	filter, err := milvus.ToFilter(expr)
//	// filter: (age > 18) and (status == "active")
func ToFilter(expr ast.Expr) (string, error) {
	conv := NewVisitor()
	if err := conv.Visit(expr); err != nil {
		return "", err
	}
	return conv.Result(), nil
}
