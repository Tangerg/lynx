package weaviate

import (
	"fmt"

	"github.com/spf13/cast"
	"github.com/weaviate/weaviate-go-client/v5/weaviate/filters"

	"github.com/Tangerg/lynx/core/vectorstore/filter/ast"
	"github.com/Tangerg/lynx/core/vectorstore/filter/token"
)

var _ ast.Visitor = (*Visitor)(nil)

// Visitor transforms AST filter expressions into Weaviate WhereBuilder conditions.
// It implements the ast.Visitor interface to traverse and convert expression trees
// into Weaviate's native filter format.
//
// The visitor maintains internal state during traversal:
//   - result: The WhereBuilder being constructed for the current node
//   - currentFieldPath: Temporary storage for extracted field path segments
//   - currentFieldValue: Temporary storage for extracted literal values
//   - err: The last error encountered during conversion
//
// Conversion strategy:
//   - Compound expressions (BinaryExpr, UnaryExpr) set result to a new WhereBuilder
//   - Leaf expressions (Ident, IndexExpr) set currentFieldPath
//   - Literal/ListLiteral expressions set currentFieldValue
//   - Nested expressions use isolated visitors to maintain proper scoping
//
// Field path mapping:
//   - Simple identifier "age"           → path: ["age"]
//   - Indexed access metadata["key"]    → path: ["metadata", "key"]
//   - Deeply nested metadata["a"]["b"]  → path: ["metadata", "a", "b"]
//
// Usage example:
//
//	expr := parseFilterExpression("age > 18 AND status == 'active'")
//	filter, err := ToFilter(expr)
//	if err != nil {
//	    log.Fatal(err)
//	}
type Visitor struct {
	err               error                 // Last error encountered during conversion
	result            *filters.WhereBuilder // The WhereBuilder being constructed
	currentFieldPath  []string              // Temporary storage for field path segments
	currentFieldValue any                   // Temporary storage for field values
}

// NewVisitor creates a new visitor instance ready to process AST expressions.
func NewVisitor() *Visitor {
	return &Visitor{}
}

// Result returns the constructed WhereBuilder.
// Returns nil if an error occurred during conversion.
// Should only be called after Visit() completes.
func (c *Visitor) Result() *filters.WhereBuilder {
	if c.err != nil {
		return nil
	}
	return c.result
}

// Error returns the last error encountered during conversion.
// Returns nil if the conversion was successful.
func (c *Visitor) Error() error {
	return c.err
}

// Visit implements the ast.Visitor interface.
// It initiates the conversion process for the given expression and stores any error.
// Always returns nil to stop further traversal as conversion is done in a single pass.
func (c *Visitor) Visit(expr ast.Expr) ast.Visitor {
	c.err = c.visit(expr)
	return nil
}

// visit dispatches conversion to specialized methods based on expression type.
func (c *Visitor) visit(expr ast.Expr) error {
	if expr == nil {
		return fmt.Errorf("weaviate: cannot process nil expression")
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
		return fmt.Errorf("weaviate: unsupported expression type %T", node)
	}
}

// visitBinaryExpr routes binary expressions to appropriate handlers based on operator type.
func (c *Visitor) visitBinaryExpr(expr *ast.BinaryExpr) error {
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
	return fmt.Errorf("weaviate: unsupported binary operator '%s' at %s",
		expr.Op.Literal, expr.Start().String())
}

// visitUnaryExpr handles unary expressions.
// Currently only the NOT operator is supported for logical negation.
func (c *Visitor) visitUnaryExpr(expr *ast.UnaryExpr) error {
	if !expr.Op.Kind.IsUnaryOperator() {
		return fmt.Errorf("weaviate: '%s' is not a valid unary operator at %s",
			expr.Op.Literal, expr.Start().String())
	}

	switch expr.Op.Kind {
	case token.NOT:
		return c.visitNotExpr(expr)
	default:
		return fmt.Errorf("weaviate: unhandled unary operator '%s' at %s",
			expr.Op.Literal, expr.Start().String())
	}
}

// visitIdent extracts and stores the identifier name as a single-element field path.
//
// Example: Ident("age") → currentFieldPath = ["age"]
func (c *Visitor) visitIdent(ident *ast.Ident) error {
	c.currentFieldPath = []string{ident.Value}
	return nil
}

// visitLiteral converts an AST literal into its corresponding Go value and stores it.
func (c *Visitor) visitLiteral(lit *ast.Literal) error {
	value, err := c.literalToValue(lit)
	if err != nil {
		return fmt.Errorf("weaviate: failed to convert literal at %s: %w", lit.Start().String(), err)
	}
	c.currentFieldValue = value
	return nil
}

// visitListLiteral converts a list of literals into a Go slice and stores it.
func (c *Visitor) visitListLiteral(list *ast.ListLiteral) error {
	values := make([]any, 0, len(list.Values))
	for i, lit := range list.Values {
		value, err := c.literalToValue(lit)
		if err != nil {
			return fmt.Errorf("weaviate: failed to convert list element at index %d: %w", i, err)
		}
		values = append(values, value)
	}
	c.currentFieldValue = values
	return nil
}

// visitIndexExpr processes indexed field access and builds a slice-based field path.
// Weaviate's WhereBuilder uses []string paths for nested property access.
//
// Example transformations:
//   - metadata["user"]          → ["metadata", "user"]
//   - data["tags"][0]           → ["data", "tags", "0"]
//   - config["db"]["host"]      → ["config", "db", "host"]
func (c *Visitor) visitIndexExpr(expr *ast.IndexExpr) error {
	path, err := c.buildIndexedFieldPath(expr)
	if err != nil {
		return fmt.Errorf("weaviate: failed to build field path at %s: %w",
			expr.Start().String(), err)
	}
	c.currentFieldPath = path
	return nil
}

// visitLogicalExpr handles logical operators (AND, OR).
// Each operand is converted using an isolated visitor, then combined:
//   - AND: WithOperator(filters.And).WithOperands([left, right])
//   - OR:  WithOperator(filters.Or).WithOperands([left, right])
func (c *Visitor) visitLogicalExpr(expr *ast.BinaryExpr) error {
	left, err := c.buildNestedFilter(expr.Left)
	if err != nil {
		return fmt.Errorf("weaviate: failed to process left operand of '%s' at %s: %w",
			expr.Op.Literal, expr.Start().String(), err)
	}

	right, err := c.buildNestedFilter(expr.Right)
	if err != nil {
		return fmt.Errorf("weaviate: failed to process right operand of '%s' at %s: %w",
			expr.Op.Literal, expr.Start().String(), err)
	}

	switch expr.Op.Kind {
	case token.AND:
		c.result = filters.Where().
			WithOperator(filters.And).
			WithOperands([]*filters.WhereBuilder{left, right})
	case token.OR:
		c.result = filters.Where().
			WithOperator(filters.Or).
			WithOperands([]*filters.WhereBuilder{left, right})
	default:
		return fmt.Errorf("weaviate: unexpected logical operator '%s' at %s",
			expr.Op.Literal, expr.Start().String())
	}

	return nil
}

// visitNotExpr handles the NOT operator by wrapping the negated condition.
//
// Example:
//   - NOT (age > 18) → WithOperator(filters.Not).WithOperands([age>18])
func (c *Visitor) visitNotExpr(expr *ast.UnaryExpr) error {
	operand, err := c.buildNestedFilter(expr.Right)
	if err != nil {
		return fmt.Errorf("weaviate: failed to process NOT operand at %s: %w",
			expr.Start().String(), err)
	}

	c.result = filters.Where().
		WithOperator(filters.Not).
		WithOperands([]*filters.WhereBuilder{operand})

	return nil
}

// visitEqualityExpr handles equality operators (==, !=).
// Supports string, number, and boolean values:
//   - strings  → WithValueText
//   - numbers  → WithValueNumber
//   - booleans → WithValueBoolean
//
// Examples:
//   - status == "active"  → path:["status"], Equal, text:"active"
//   - age != 18           → path:["age"], NotEqual, number:18
func (c *Visitor) visitEqualityExpr(expr *ast.BinaryExpr) error {
	fieldPath, err := c.extractFieldPath(expr.Left)
	if err != nil {
		return fmt.Errorf("weaviate: failed to extract field path from '%s' at %s: %w",
			expr.Op.Literal, expr.Start().String(), err)
	}

	fieldValue, err := c.extractFieldValue(expr.Right)
	if err != nil {
		return fmt.Errorf("weaviate: failed to extract value from '%s' at %s: %w",
			expr.Op.Literal, expr.Start().String(), err)
	}

	var op filters.WhereOperator
	switch expr.Op.Kind {
	case token.EQ:
		op = filters.Equal
	case token.NE:
		op = filters.NotEqual
	default:
		return fmt.Errorf("weaviate: unexpected equality operator '%s' at %s",
			expr.Op.Literal, expr.Start().String())
	}

	builder, err := c.buildValueFilter(fieldPath, op, fieldValue)
	if err != nil {
		return fmt.Errorf("weaviate: failed to build filter for '%s' at %s: %w",
			expr.Op.Literal, expr.Start().String(), err)
	}

	c.result = builder
	return nil
}

// visitOrderingExpr handles ordering/comparison operators (<, <=, >, >=).
// The right operand is converted to float64 for the number filter:
//
//   - <  → filters.LessThan
//   - <= → filters.LessThanEqual
//   - >  → filters.GreaterThan
//   - >= → filters.GreaterThanEqual
//
// Examples:
//   - age > 18     → path:["age"], GreaterThan, number:18
//   - price <= 9.9 → path:["price"], LessThanEqual, number:9.9
func (c *Visitor) visitOrderingExpr(expr *ast.BinaryExpr) error {
	fieldPath, err := c.extractFieldPath(expr.Left)
	if err != nil {
		return fmt.Errorf("weaviate: failed to extract field path from '%s' at %s: %w",
			expr.Op.Literal, expr.Start().String(), err)
	}

	fieldValue, err := c.extractFieldValue(expr.Right)
	if err != nil {
		return fmt.Errorf("weaviate: failed to extract value from '%s' at %s: %w",
			expr.Op.Literal, expr.Start().String(), err)
	}

	numericValue, err := cast.ToFloat64E(fieldValue)
	if err != nil {
		return fmt.Errorf("weaviate: cannot convert value to number for '%s' at %s: expected number, got %T",
			expr.Op.Literal, expr.Start().String(), fieldValue)
	}

	var op filters.WhereOperator
	switch expr.Op.Kind {
	case token.LT:
		op = filters.LessThan
	case token.LE:
		op = filters.LessThanEqual
	case token.GT:
		op = filters.GreaterThan
	case token.GE:
		op = filters.GreaterThanEqual
	default:
		return fmt.Errorf("weaviate: unexpected ordering operator '%s' at %s",
			expr.Op.Literal, expr.Start().String())
	}

	c.result = filters.Where().
		WithPath(fieldPath).
		WithOperator(op).
		WithValueNumber(numericValue)

	return nil
}

// visitInExpr handles the IN operator for membership testing.
// The right operand must be a non-empty list literal.
// Uses ContainsAny operator with the appropriate value type method:
//   - string list  → WithValueText
//   - number list  → WithValueNumber
//   - boolean list → WithValueBoolean
//
// Examples:
//   - status IN ["active", "pending"] → path:["status"], ContainsAny, text:["active","pending"]
//   - score IN [1, 2, 3]              → path:["score"], ContainsAny, number:[1,2,3]
func (c *Visitor) visitInExpr(expr *ast.BinaryExpr) error {
	fieldPath, err := c.extractFieldPath(expr.Left)
	if err != nil {
		return fmt.Errorf("weaviate: failed to extract field path from 'IN' at %s: %w",
			expr.Start().String(), err)
	}

	listLit, ok := expr.Right.(*ast.ListLiteral)
	if !ok {
		return fmt.Errorf("weaviate: 'IN' operator requires a list on the right side at %s, got %T",
			expr.Start().String(), expr.Right)
	}
	if len(listLit.Values) == 0 {
		return fmt.Errorf("weaviate: 'IN' operator requires a non-empty list at %s",
			expr.Start().String())
	}

	if err = c.visitListLiteral(listLit); err != nil {
		return err
	}

	values, ok := c.currentFieldValue.([]any)
	if !ok || len(values) == 0 {
		return fmt.Errorf("weaviate: failed to extract list values for 'IN' at %s",
			expr.Start().String())
	}

	switch values[0].(type) {
	case string:
		strs := make([]string, len(values))
		for i, v := range values {
			strs[i] = cast.ToString(v)
		}
		c.result = filters.Where().
			WithPath(fieldPath).
			WithOperator(filters.ContainsAny).
			WithValueText(strs...)

	case float64:
		nums := make([]float64, len(values))
		for i, v := range values {
			nums[i] = cast.ToFloat64(v)
		}
		c.result = filters.Where().
			WithPath(fieldPath).
			WithOperator(filters.ContainsAny).
			WithValueNumber(nums...)

	case bool:
		bools := make([]bool, len(values))
		for i, v := range values {
			bools[i] = cast.ToBool(v)
		}
		c.result = filters.Where().
			WithPath(fieldPath).
			WithOperator(filters.ContainsAny).
			WithValueBoolean(bools...)

	default:
		return fmt.Errorf("weaviate: unsupported value type %T in 'IN' list at %s",
			values[0], expr.Start().String())
	}

	return nil
}

// visitLikeExpr handles the LIKE operator for pattern matching.
// The right operand must be a string literal.
// Uses Weaviate's Like operator which supports wildcards (* and ?).
//
// Example:
//   - name LIKE "Jo*" → path:["name"], Like, text:"Jo*"
func (c *Visitor) visitLikeExpr(expr *ast.BinaryExpr) error {
	fieldPath, err := c.extractFieldPath(expr.Left)
	if err != nil {
		return fmt.Errorf("weaviate: failed to extract field path from 'LIKE' at %s: %w",
			expr.Start().String(), err)
	}

	lit, ok := expr.Right.(*ast.Literal)
	if !ok {
		return fmt.Errorf("weaviate: 'LIKE' operator requires a string literal on the right side at %s, got %T",
			expr.Start().String(), expr.Right)
	}
	if !lit.IsString() {
		return fmt.Errorf("weaviate: 'LIKE' operator requires a string pattern at %s, got %s",
			expr.Start().String(), lit.Token.Kind.Name())
	}

	if err = c.visitLiteral(lit); err != nil {
		return err
	}

	pattern, ok := c.currentFieldValue.(string)
	if !ok {
		return fmt.Errorf("weaviate: failed to extract string pattern for 'LIKE' at %s",
			expr.Start().String())
	}

	c.result = filters.Where().
		WithPath(fieldPath).
		WithOperator(filters.Like).
		WithValueText(pattern)

	return nil
}

// buildNestedFilter converts a sub-expression to a WhereBuilder using an isolated visitor.
// This ensures that nested logical expressions maintain proper scoping.
func (c *Visitor) buildNestedFilter(expr ast.Expr) (*filters.WhereBuilder, error) {
	nested := NewVisitor()
	if err := nested.visit(expr); err != nil {
		return nil, err
	}
	if nested.result != nil {
		return nested.result, nil
	}
	return nil, fmt.Errorf("weaviate: unsupported expression type %T for nested filter", expr)
}

// extractFieldPath extracts a field path from an expression.
// The visitor's currentFieldPath state is preserved during extraction.
//
// Supported expression types:
//   - Ident: Simple field name → ["age"]
//   - IndexExpr: Nested field access → ["metadata", "key"]
func (c *Visitor) extractFieldPath(expr ast.Expr) ([]string, error) {
	savedPath := c.currentFieldPath
	c.currentFieldPath = nil

	err := c.visit(expr)

	extractedPath := c.currentFieldPath
	c.currentFieldPath = savedPath

	if err != nil {
		return nil, err
	}
	if len(extractedPath) == 0 {
		return nil, fmt.Errorf("weaviate: failed to extract field path from %T expression", expr)
	}

	return extractedPath, nil
}

// extractFieldValue extracts a value (literal or list) from an expression.
// The visitor's currentFieldValue state is preserved during extraction.
func (c *Visitor) extractFieldValue(expr ast.Expr) (any, error) {
	savedValue := c.currentFieldValue
	c.currentFieldValue = nil

	err := c.visit(expr)

	extractedValue := c.currentFieldValue
	c.currentFieldValue = savedValue

	if err != nil {
		return nil, err
	}
	if extractedValue == nil {
		return nil, fmt.Errorf("weaviate: failed to extract value from %T expression", expr)
	}

	return extractedValue, nil
}

// buildIndexedFieldPath constructs a slice-based field path from an index expression.
// This method recursively processes nested index expressions to build the complete path.
//
// Transformation examples:
//   - user["name"]              → ["user", "name"]
//   - metadata["tags"][0]       → ["metadata", "tags", "0"]
//   - config["db"]["host"]      → ["config", "db", "host"]
func (c *Visitor) buildIndexedFieldPath(expr *ast.IndexExpr) ([]string, error) {
	var parts []string

	current := expr
	for {
		if err := c.visitLiteral(current.Index); err != nil {
			return nil, err
		}

		indexVal := c.currentFieldValue
		var segment string
		switch v := indexVal.(type) {
		case string:
			segment = v
		case float64:
			segment = fmt.Sprintf("%d", int(v))
		default:
			return nil, fmt.Errorf("weaviate: invalid index type %T, expected string or number", indexVal)
		}
		parts = append([]string{segment}, parts...)

		switch left := current.Left.(type) {
		case *ast.IndexExpr:
			current = left
		case *ast.Ident:
			parts = append([]string{left.Value}, parts...)
			return parts, nil
		default:
			return nil, fmt.Errorf("weaviate: invalid left operand type %T in index expression, expected identifier or index",
				left)
		}
	}
}

// buildValueFilter creates a WhereBuilder for equality/inequality conditions.
// Selects the appropriate value method based on the Go type of the value:
//   - string  → WithValueText
//   - float64 → WithValueNumber
//   - bool    → WithValueBoolean
func (c *Visitor) buildValueFilter(path []string, op filters.WhereOperator, value any) (*filters.WhereBuilder, error) {
	switch v := value.(type) {
	case string:
		return filters.Where().WithPath(path).WithOperator(op).WithValueText(v), nil
	case float64:
		return filters.Where().WithPath(path).WithOperator(op).WithValueNumber(v), nil
	case bool:
		return filters.Where().WithPath(path).WithOperator(op).WithValueBoolean(v), nil
	default:
		return nil, fmt.Errorf("weaviate: unsupported value type %T for filter", value)
	}
}

// literalToValue converts an AST literal node to its corresponding Go value.
//
// Supported conversions:
//   - String literals  → string (with quote removal)
//   - Number literals  → float64
//   - Boolean literals → bool
func (c *Visitor) literalToValue(lit *ast.Literal) (any, error) {
	if lit.IsString() {
		return lit.AsString()
	}
	if lit.IsNumber() {
		return lit.AsNumber()
	}
	if lit.IsBool() {
		return lit.AsBool()
	}
	return nil, fmt.Errorf("weaviate: unsupported literal type '%s'", lit.Token.Kind.Name())
}

// ToFilter converts an AST filter expression into a Weaviate WhereBuilder.
//
// This is the main entry point for converting filter expressions written in
// the Lynx filter DSL into Weaviate's native filter format.
//
// Supported operations:
//   - Logical:          AND, OR, NOT
//   - Equality:         ==, !=
//   - Ordering:         <, <=, >, >=
//   - Membership:       IN  (mapped to ContainsAny)
//   - Pattern matching: LIKE
//
// Field access:
//   - Simple field:     age               → path: ["age"]
//   - Nested JSON:      metadata["key"]   → path: ["metadata", "key"]
//   - Deep nesting:     a["b"]["c"]       → path: ["a", "b", "c"]
//
// Value types:
//   - Strings  → WithValueText
//   - Numbers  → WithValueNumber
//   - Booleans → WithValueBoolean
//
// Example:
//
//	expr, _ := parser.Parse(`age > 18 AND status == "active"`)
//	whereFilter, err := weaviate.ToFilter(expr)
//	// Used with: getBuilder.WithWhere(whereFilter)
func ToFilter(expr ast.Expr) (*filters.WhereBuilder, error) {
	conv := NewVisitor()
	conv.Visit(expr)
	return conv.Result(), conv.Error()
}
