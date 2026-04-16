package milvus

import (
	"fmt"
	"strings"

	"github.com/Tangerg/lynx/core/vectorstore/filter/ast"
	"github.com/Tangerg/lynx/core/vectorstore/filter/token"
)

var _ ast.Visitor = (*Converter)(nil)

// Converter transforms AST filter expressions into Milvus filter expression strings.
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
type Converter struct {
	err               error  // Last error encountered during conversion
	result            string // The Milvus expression string being built
	currentFieldKey   string // Temporary storage for field paths during extraction
	currentFieldValue string // Temporary storage for encoded values during extraction
}

// NewConverter creates a new converter instance ready to process AST expressions.
func NewConverter() *Converter {
	return &Converter{}
}

// Result returns the constructed Milvus filter expression string.
// Returns an empty string if an error occurred during conversion.
// Should only be called after Visit() completes.
func (c *Converter) Result() string {
	if c.err != nil {
		return ""
	}
	return c.result
}

// Error returns the last error encountered during conversion.
// Returns nil if the conversion was successful.
func (c *Converter) Error() error {
	return c.err
}

// Visit implements the ast.Visitor interface.
// It initiates the conversion process for the given expression and stores any error.
// Always returns nil to stop further traversal as conversion is done in a single pass.
func (c *Converter) Visit(expr ast.Expr) ast.Visitor {
	c.err = c.visit(expr)
	return nil
}

// visit dispatches conversion to specialized methods based on expression type.
func (c *Converter) visit(expr ast.Expr) error {
	if expr == nil {
		return fmt.Errorf("milvus: cannot process nil expression")
	}
	if c.err != nil {
		return c.err
	}

	switch node := expr.(type) {
	case *ast.BinaryExpr:
		return c.visitBinaryExpr(node)
	case *ast.UnaryExpr:
		return c.visitUnaryExpr(node)
	case *ast.IndexExpr:
		return c.visitIndexExpr(node)
	case *ast.Ident:
		return c.visitIdent(node)
	case *ast.Literal:
		return c.visitLiteral(node)
	case *ast.ListLiteral:
		return c.visitListLiteral(node)
	default:
		return fmt.Errorf("milvus: unsupported expression type %T", node)
	}
}

// visitBinaryExpr routes binary expressions to appropriate handlers based on operator type.
func (c *Converter) visitBinaryExpr(expr *ast.BinaryExpr) error {
	if expr.Op.Kind.IsLogicalOperator() {
		return c.visitLogicalExpr(expr)
	}
	if expr.Op.Kind.IsEqualityOperator() {
		return c.visitEqualityExpr(expr)
	}
	if expr.Op.Kind.IsOrderingOperator() {
		return c.visitOrderingExpr(expr)
	}
	if expr.Op.Kind.Is(token.IN) {
		return c.visitInExpr(expr)
	}
	if expr.Op.Kind.Is(token.LIKE) {
		return c.visitLikeExpr(expr)
	}
	return fmt.Errorf("milvus: unsupported binary operator '%s' at %s",
		expr.Op.Literal, expr.Start().String())
}

// visitUnaryExpr handles unary expressions.
// Currently only the NOT operator is supported for logical negation.
func (c *Converter) visitUnaryExpr(expr *ast.UnaryExpr) error {
	if !expr.Op.Kind.IsUnaryOperator() {
		return fmt.Errorf("milvus: '%s' is not a valid unary operator at %s",
			expr.Op.Literal, expr.Start().String())
	}

	switch expr.Op.Kind {
	case token.NOT:
		return c.visitNotExpr(expr)
	default:
		return fmt.Errorf("milvus: unhandled unary operator '%s' at %s",
			expr.Op.Literal, expr.Start().String())
	}
}

// visitIdent extracts and stores the identifier name as the current field key.
func (c *Converter) visitIdent(ident *ast.Ident) error {
	c.currentFieldKey = ident.Value
	return nil
}

// visitLiteral converts an AST literal to its Milvus expression encoding and stores it.
//
// Encoding rules:
//   - Strings are wrapped in double quotes with internal double quotes escaped.
//   - Whole numbers are formatted as integers (no decimal point).
//   - Fractional numbers use %g notation.
//   - Booleans use Milvus syntax: True / False.
func (c *Converter) visitLiteral(lit *ast.Literal) error {
	value, err := c.literalToString(lit)
	if err != nil {
		return err
	}
	c.currentFieldValue = value
	return nil
}

// visitListLiteral converts a list of literals to a Milvus list expression and stores it.
// Example output: ["active", "pending"] or [18, 21, 25]
func (c *Converter) visitListLiteral(list *ast.ListLiteral) error {
	parts := make([]string, 0, len(list.Values))

	for i, lit := range list.Values {
		s, err := c.literalToString(lit)
		if err != nil {
			return fmt.Errorf("milvus: failed to convert list element at index %d: %w", i, err)
		}
		parts = append(parts, s)
	}

	c.currentFieldValue = "[" + strings.Join(parts, ", ") + "]"
	return nil
}

// visitIndexExpr processes indexed field access and builds a bracket-notation field path.
// Example transformations:
//   - metadata["user"] → metadata["user"]
//   - data["tags"][0] → data["tags"][0]
//   - config["db"]["host"] → config["db"]["host"]
func (c *Converter) visitIndexExpr(expr *ast.IndexExpr) error {
	fieldKey, err := c.buildIndexedFieldKey(expr)
	if err != nil {
		return fmt.Errorf("milvus: failed to build field path at %s: %w",
			expr.Start().String(), err)
	}
	c.currentFieldKey = fieldKey
	return nil
}

// visitLogicalExpr handles logical operators (AND, OR).
// Each operand is converted using an isolated converter, then combined:
//   - AND: (left) and (right)
//   - OR:  (left) or (right)
func (c *Converter) visitLogicalExpr(expr *ast.BinaryExpr) error {
	left, err := c.buildNestedExpr(expr.Left)
	if err != nil {
		return fmt.Errorf("milvus: failed to process left operand of '%s' at %s: %w",
			expr.Op.Literal, expr.Start().String(), err)
	}

	right, err := c.buildNestedExpr(expr.Right)
	if err != nil {
		return fmt.Errorf("milvus: failed to process right operand of '%s' at %s: %w",
			expr.Op.Literal, expr.Start().String(), err)
	}

	switch expr.Op.Kind {
	case token.AND:
		c.result = fmt.Sprintf("(%s) and (%s)", left, right)
	case token.OR:
		c.result = fmt.Sprintf("(%s) or (%s)", left, right)
	default:
		return fmt.Errorf("milvus: unexpected logical operator '%s' at %s",
			expr.Op.Literal, expr.Start().String())
	}

	return nil
}

// visitNotExpr handles the NOT operator.
// Example: NOT (age > 18) → not (age > 18)
func (c *Converter) visitNotExpr(expr *ast.UnaryExpr) error {
	operand, err := c.buildNestedExpr(expr.Right)
	if err != nil {
		return fmt.Errorf("milvus: failed to process NOT operand at %s: %w",
			expr.Start().String(), err)
	}

	c.result = fmt.Sprintf("not (%s)", operand)
	return nil
}

// visitEqualityExpr handles equality operators (==, !=).
// Examples:
//   - status == "active"
//   - age != 18
func (c *Converter) visitEqualityExpr(expr *ast.BinaryExpr) error {
	fieldKey, err := c.extractFieldKey(expr.Left)
	if err != nil {
		return fmt.Errorf("milvus: failed to extract field key from '%s' at %s: %w",
			expr.Op.Literal, expr.Start().String(), err)
	}

	fieldValue, err := c.extractFieldValue(expr.Right)
	if err != nil {
		return fmt.Errorf("milvus: failed to extract value from '%s' at %s: %w",
			expr.Op.Literal, expr.Start().String(), err)
	}

	switch expr.Op.Kind {
	case token.EQ:
		c.result = fmt.Sprintf("%s == %s", fieldKey, fieldValue)
	case token.NE:
		c.result = fmt.Sprintf("%s != %s", fieldKey, fieldValue)
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
func (c *Converter) visitOrderingExpr(expr *ast.BinaryExpr) error {
	fieldKey, err := c.extractFieldKey(expr.Left)
	if err != nil {
		return fmt.Errorf("milvus: failed to extract field key from '%s' at %s: %w",
			expr.Op.Literal, expr.Start().String(), err)
	}

	fieldValue, err := c.extractFieldValue(expr.Right)
	if err != nil {
		return fmt.Errorf("milvus: failed to extract value from '%s' at %s: %w",
			expr.Op.Literal, expr.Start().String(), err)
	}

	switch expr.Op.Kind {
	case token.LT:
		c.result = fmt.Sprintf("%s < %s", fieldKey, fieldValue)
	case token.LE:
		c.result = fmt.Sprintf("%s <= %s", fieldKey, fieldValue)
	case token.GT:
		c.result = fmt.Sprintf("%s > %s", fieldKey, fieldValue)
	case token.GE:
		c.result = fmt.Sprintf("%s >= %s", fieldKey, fieldValue)
	default:
		return fmt.Errorf("milvus: unexpected ordering operator '%s' at %s",
			expr.Op.Literal, expr.Start().String())
	}

	return nil
}

// visitInExpr handles the IN operator for membership testing.
// The right operand must be a non-empty list literal.
// Example: status in ["active", "pending"]
func (c *Converter) visitInExpr(expr *ast.BinaryExpr) error {
	fieldKey, err := c.extractFieldKey(expr.Left)
	if err != nil {
		return fmt.Errorf("milvus: failed to extract field key from 'IN' at %s: %w",
			expr.Start().String(), err)
	}

	listLit, ok := expr.Right.(*ast.ListLiteral)
	if !ok {
		return fmt.Errorf("milvus: 'IN' operator requires a list on the right side at %s, got %T",
			expr.Start().String(), expr.Right)
	}
	if len(listLit.Values) == 0 {
		return fmt.Errorf("milvus: 'IN' operator requires a non-empty list at %s",
			expr.Start().String())
	}

	if err = c.visitListLiteral(listLit); err != nil {
		return err
	}

	c.result = fmt.Sprintf("%s in %s", fieldKey, c.currentFieldValue)
	return nil
}

// visitLikeExpr handles the LIKE operator for pattern matching.
// The right operand must be a string literal.
// Example: name like "go%"
func (c *Converter) visitLikeExpr(expr *ast.BinaryExpr) error {
	fieldKey, err := c.extractFieldKey(expr.Left)
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

	if err = c.visitLiteral(lit); err != nil {
		return err
	}

	c.result = fmt.Sprintf("%s like %s", fieldKey, c.currentFieldValue)
	return nil
}

// buildNestedExpr converts a sub-expression to a string using an isolated converter.
// This ensures that nested logical expressions maintain proper scoping.
func (c *Converter) buildNestedExpr(expr ast.Expr) (string, error) {
	nested := NewConverter()
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
func (c *Converter) extractFieldKey(expr ast.Expr) (string, error) {
	savedKey := c.currentFieldKey
	c.currentFieldKey = ""

	err := c.visit(expr)

	extracted := c.currentFieldKey
	c.currentFieldKey = savedKey

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
func (c *Converter) extractFieldValue(expr ast.Expr) (string, error) {
	savedValue := c.currentFieldValue
	c.currentFieldValue = ""

	err := c.visit(expr)

	extracted := c.currentFieldValue
	c.currentFieldValue = savedValue

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
func (c *Converter) buildIndexedFieldKey(expr *ast.IndexExpr) (string, error) {
	var parts []string

	current := expr
	for {
		if err := c.visitLiteral(current.Index); err != nil {
			return "", err
		}

		parts = append([]string{"[" + c.currentFieldValue + "]"}, parts...)

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
func (c *Converter) literalToString(lit *ast.Literal) (string, error) {
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
			return fmt.Sprintf("%d", int64(n)), nil
		}
		return fmt.Sprintf("%g", n), nil
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
	conv := NewConverter()
	conv.Visit(expr)
	return conv.Result(), conv.Error()
}
