package qdrant

import (
	"fmt"
	"strings"

	"github.com/qdrant/go-client/qdrant"
	"github.com/spf13/cast"

	"github.com/Tangerg/lynx/ai/vectorstore/filter/ast"
	"github.com/Tangerg/lynx/ai/vectorstore/filter/token"
	"github.com/Tangerg/lynx/pkg/ptr"
)

var _ ast.Visitor = (*Converter)(nil)

// Converter transforms AST filter expressions into Qdrant filter conditions.
// It implements the ast.Visitor interface to traverse and convert expression trees
// into Qdrant's native filter format.
//
// The converter maintains internal state during traversal:
//   - filter: The resulting Qdrant filter being built
//   - currentFieldValue: Temporary storage for extracted literal values
//   - currentFieldKey: Temporary storage for extracted field identifiers
//   - err: The last error encountered during conversion
//
// Conversion strategy:
//   - Each visit method directly appends conditions to the filter
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
	err               error          // Last error encountered during conversion
	filter            *qdrant.Filter // The Qdrant filter being constructed
	currentFieldValue any            // Temporary storage for field values during extraction
	currentFieldKey   string         // Temporary storage for field keys during extraction
}

// NewConverter creates a new converter instance with an empty filter.
// The returned converter is ready to process AST expressions.
func NewConverter() *Converter {
	return &Converter{
		filter: &qdrant.Filter{},
	}
}

// Filter returns the constructed Qdrant filter.
// Returns nil if an error occurred during conversion.
// Should only be called after Visit() completes.
func (c *Converter) Filter() *qdrant.Filter {
	if c.err != nil {
		return nil
	}
	return c.filter
}

// Error returns the last error encountered during conversion.
// Returns nil if the conversion was successful.
func (c *Converter) Error() error {
	return c.err
}

// Visit implements the ast.Visitor interface.
// It initiates the conversion process for the given expression and stores any error.
// Always returns nil to stop further traversal as conversion is done in a single pass.
//
// This is the main entry point for AST traversal. The actual conversion logic
// is delegated to the visit method and its specialized handlers.
func (c *Converter) Visit(expr ast.Expr) ast.Visitor {
	c.err = c.visit(expr)
	return nil
}

// visit dispatches conversion to specialized methods based on expression type.
// This is the main internal routing method that handles different AST node types.
//
// Supported node types:
//   - BinaryExpr: Binary operations (AND, OR, ==, !=, <, <=, >, >=, IN, LIKE)
//   - UnaryExpr: Unary operations (NOT)
//   - IndexExpr: Indexed field access (e.g., metadata["key"])
//   - Ident: Simple field identifiers
//   - Literal: Constant values
//   - ListLiteral: Array of constant values
func (c *Converter) visit(expr ast.Expr) error {
	if expr == nil {
		return fmt.Errorf("cannot process nil expression")
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
		return fmt.Errorf("unsupported expression type %T", node)
	}
}

// visitBinaryExpr routes binary expressions to appropriate handlers based on operator type.
//
// Binary operators are categorized as:
//   - Logical operators: AND, OR (handled by visitLogicalExpr)
//   - Equality operators: ==, != (handled by visitEqualityExpr)
//   - Ordering operators: <, <=, >, >= (handled by visitOrderingExpr)
//   - Membership operator: IN (handled by visitInExpr)
//   - Pattern matching operator: LIKE (handled by visitLikeExpr)
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
	return fmt.Errorf("unsupported binary operator '%s' at %s",
		expr.Op.Literal, expr.Start().String())
}

// visitUnaryExpr handles unary expressions.
// Currently only the NOT operator is supported for logical negation.
func (c *Converter) visitUnaryExpr(expr *ast.UnaryExpr) error {
	if !expr.Op.Kind.IsUnaryOperator() {
		return fmt.Errorf("'%s' is not a valid unary operator at %s",
			expr.Op.Literal, expr.Start().String())
	}

	switch expr.Op.Kind {
	case token.NOT:
		return c.visitNotExpr(expr)
	default:
		// Defensive programming: should never reach here due to IsUnaryOperator check
		return fmt.Errorf("unhandled unary operator '%s' at %s",
			expr.Op.Literal, expr.Start().String())
	}
}

// visitIdent extracts and stores the identifier name as the current field key.
// This method is typically called during field key extraction in binary expressions.
//
// Example: For expression "age > 18", this extracts "age" as the field key.
func (c *Converter) visitIdent(ident *ast.Ident) error {
	c.currentFieldKey = ident.Value
	return nil
}

// visitLiteral converts an AST literal into its corresponding Go value and stores it.
// The conversion supports string, number, and boolean literals.
//
// This method is typically called during value extraction in binary expressions.
func (c *Converter) visitLiteral(lit *ast.Literal) error {
	value, err := c.literalToValue(lit)
	if err != nil {
		return fmt.Errorf("failed to convert literal at %s: %w",
			lit.Start().String(), err)
	}
	c.currentFieldValue = value
	return nil
}

// visitListLiteral converts a list of literals into a Go slice and stores it.
// All elements in the list are converted using literalToValue.
//
// This method is used by the IN operator to extract the list of values
// for membership testing.
func (c *Converter) visitListLiteral(list *ast.ListLiteral) error {
	values := make([]any, 0, len(list.Values))
	for i, lit := range list.Values {
		value, err := c.literalToValue(lit)
		if err != nil {
			return fmt.Errorf("failed to convert list element at index %d: %w", i, err)
		}
		values = append(values, value)
	}
	c.currentFieldValue = values
	return nil
}

// visitIndexExpr processes indexed field access expressions and builds a dot-separated field path.
// This enables accessing nested fields using bracket notation.
//
// Example transformations:
//   - metadata["user"] -> "metadata.user"
//   - data["tags"][0] -> "data.tags.0"
//   - config["db"]["host"] -> "config.db.host"
func (c *Converter) visitIndexExpr(expr *ast.IndexExpr) error {
	fieldKey, err := c.buildIndexedFieldKey(expr)
	if err != nil {
		return fmt.Errorf("failed to build field path at %s: %w",
			expr.Start().String(), err)
	}
	c.currentFieldKey = fieldKey
	return nil
}

// visitLogicalExpr handles logical operators (AND, OR).
// Each operand is converted to a condition using an isolated converter,
// then both conditions are added to the appropriate filter clause:
//   - AND operator: Adds conditions to filter.Must (all conditions must match)
//   - OR operator: Adds conditions to filter.Should (at least one condition must match)
//
// Example:
//   - "age > 18 AND status == 'active'" produces: Must[age>18, status==active]
//   - "role == 'admin' OR role == 'owner'" produces: Should[role==admin, role==owner]
func (c *Converter) visitLogicalExpr(expr *ast.BinaryExpr) error {
	leftCond, err := c.buildNestedCondition(expr.Left)
	if err != nil {
		return fmt.Errorf("failed to process left operand of '%s' at %s: %w",
			expr.Op.Literal, expr.Start().String(), err)
	}

	rightCond, err := c.buildNestedCondition(expr.Right)
	if err != nil {
		return fmt.Errorf("failed to process right operand of '%s' at %s: %w",
			expr.Op.Literal, expr.Start().String(), err)
	}

	switch expr.Op.Kind {
	case token.AND:
		c.filter.Must = append(c.filter.Must, leftCond, rightCond)
		return nil
	case token.OR:
		c.filter.Should = append(c.filter.Should, leftCond, rightCond)
		return nil
	default:
		// Defensive programming: should never reach here
		return fmt.Errorf("unexpected logical operator '%s' at %s",
			expr.Op.Literal, expr.Start().String())
	}
}

// visitNotExpr handles the NOT operator by wrapping the negated condition in filter.MustNot.
// The operand is converted using an isolated converter to maintain proper scoping.
//
// Example:
//   - "NOT (age > 18)" produces: MustNot[age>18]
//   - "NOT (status == 'active' OR role == 'admin')" produces: MustNot[Filter{Should[...]}]
func (c *Converter) visitNotExpr(expr *ast.UnaryExpr) error {
	cond, err := c.buildNestedCondition(expr.Right)
	if err != nil {
		return fmt.Errorf("failed to process NOT operand at %s: %w",
			expr.Start().String(), err)
	}

	c.filter.MustNot = append(c.filter.MustNot, cond)
	return nil
}

// visitEqualityExpr handles equality operators (==, !=).
// It supports exact matching for different data types:
//   - Strings: Uses NewMatchKeyword for exact keyword matching
//   - Numbers: Uses NewMatchInt for integer matching
//   - Booleans: Uses NewMatchBool for boolean matching
//
// Operator semantics:
//   - == operator: Adds match condition to filter.Must (field must equal value)
//   - != operator: Adds match condition to filter.MustNot (field must not equal value)
//
// Examples:
//   - "status == 'active'" produces: Must[status==active]
//   - "age != 18" produces: MustNot[age==18]
func (c *Converter) visitEqualityExpr(expr *ast.BinaryExpr) error {
	fieldKey, err := c.extractFieldKey(expr.Left)
	if err != nil {
		return fmt.Errorf("failed to extract field key from left operand of '%s' at %s: %w",
			expr.Op.Literal, expr.Start().String(), err)
	}

	fieldValue, err := c.extractFieldValue(expr.Right)
	if err != nil {
		return fmt.Errorf("failed to extract value from right operand of '%s' at %s: %w",
			expr.Op.Literal, expr.Start().String(), err)
	}

	matchCond, err := c.buildMatchCondition(fieldKey, fieldValue)
	if err != nil {
		return fmt.Errorf("failed to create match condition for '%s' at %s: %w",
			expr.Op.Literal, expr.Start().String(), err)
	}

	switch expr.Op.Kind {
	case token.EQ:
		// == operator: field must equal value
		c.filter.Must = append(c.filter.Must, matchCond)
	case token.NE:
		// != operator: field must not equal value (negation)
		c.filter.MustNot = append(c.filter.MustNot, matchCond)
	default:
		// Defensive programming: should never reach here
		return fmt.Errorf("unexpected equality operator '%s' at %s",
			expr.Op.Literal, expr.Start().String())
	}

	return nil
}

// buildMatchCondition creates an appropriate Qdrant match condition based on value type.
// The method automatically selects the correct Qdrant match function:
//   - string -> NewMatchKeyword (exact keyword match)
//   - float64 -> NewMatchInt (integer match, value is cast to int64)
//   - bool -> NewMatchBool (boolean match)
//
// Returns an error if the value type is not supported for matching.
func (c *Converter) buildMatchCondition(fieldKey string, fieldValue any) (*qdrant.Condition, error) {
	switch v := fieldValue.(type) {
	case string:
		// String: use keyword match (exact match)
		return qdrant.NewMatchKeyword(fieldKey, v), nil
	case float64:
		// Number: use integer match (cast to int64)
		return qdrant.NewMatchInt(fieldKey, cast.ToInt64(v)), nil
	case bool:
		// Boolean: use bool match
		return qdrant.NewMatchBool(fieldKey, v), nil
	default:
		return nil, fmt.Errorf("unsupported value type %T for match condition", fieldValue)
	}
}

// visitOrderingExpr handles ordering/comparison operators (<, <=, >, >=).
// The right operand value is converted to float64 and used to create a range condition.
//
// Operator mappings:
//   - <  : Creates Range with Lt (less than)
//   - <= : Creates Range with Lte (less than or equal)
//   - >  : Creates Range with Gt (greater than)
//   - >= : Creates Range with Gte (greater than or equal)
//
// All range conditions are added to filter.Must.
//
// Examples:
//   - "age > 18" produces: Must[Range{Gt: 18}]
//   - "price <= 99.99" produces: Must[Range{Lte: 99.99}]
func (c *Converter) visitOrderingExpr(expr *ast.BinaryExpr) error {
	fieldKey, err := c.extractFieldKey(expr.Left)
	if err != nil {
		return fmt.Errorf("failed to extract field key from left operand of '%s' at %s: %w",
			expr.Op.Literal, expr.Start().String(), err)
	}

	fieldValue, err := c.extractFieldValue(expr.Right)
	if err != nil {
		return fmt.Errorf("failed to extract value from right operand of '%s' at %s: %w",
			expr.Op.Literal, expr.Start().String(), err)
	}

	// Convert value to float64 for range comparison
	numericValue, err := cast.ToFloat64E(fieldValue)
	if err != nil {
		return fmt.Errorf("cannot convert value to number for '%s' comparison at %s: expected number, got %T",
			expr.Op.Literal, expr.Start().String(), fieldValue)
	}

	switch expr.Op.Kind {
	case token.LT:
		// < operator: field value must be less than the specified value
		c.filter.Must = append(c.filter.Must, qdrant.NewRange(fieldKey, &qdrant.Range{
			Lt: ptr.Pointer(numericValue),
		}))
	case token.LE:
		// <= operator: field value must be less than or equal to the specified value
		c.filter.Must = append(c.filter.Must, qdrant.NewRange(fieldKey, &qdrant.Range{
			Lte: ptr.Pointer(numericValue),
		}))
	case token.GT:
		// > operator: field value must be greater than the specified value
		c.filter.Must = append(c.filter.Must, qdrant.NewRange(fieldKey, &qdrant.Range{
			Gt: ptr.Pointer(numericValue),
		}))
	case token.GE:
		// >= operator: field value must be greater than or equal to the specified value
		c.filter.Must = append(c.filter.Must, qdrant.NewRange(fieldKey, &qdrant.Range{
			Gte: ptr.Pointer(numericValue),
		}))
	default:
		// Defensive programming: should never reach here
		return fmt.Errorf("unexpected ordering operator '%s' at %s",
			expr.Op.Literal, expr.Start().String())
	}

	return nil
}

// visitInExpr handles the IN operator for membership testing.
// The right operand must be a non-empty list literal containing values of the same type.
//
// Creates appropriate Qdrant conditions based on list element type:
//   - String list: Uses NewMatchKeywords (matches if field equals any keyword in the list)
//   - Number list: Uses NewMatchInts (matches if field equals any integer in the list)
//   - Boolean list: Creates a nested Should filter with individual bool matches
//
// The boolean case requires special handling because the Qdrant SDK doesn't provide
// a NewMatchBools function. A nested filter with Should conditions is used to achieve
// OR semantics while maintaining proper isolation from other conditions.
//
// Examples:
//   - "status IN ['active', 'pending']" produces: Must[MatchKeywords(status, [active, pending])]
//   - "age IN [18, 21, 25]" produces: Must[MatchInts(age, [18, 21, 25])]
//   - "active IN [true, false]" produces: Must[Filter{Should[active==true, active==false]}]
func (c *Converter) visitInExpr(expr *ast.BinaryExpr) error {
	fieldKey, err := c.extractFieldKey(expr.Left)
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

	if err = c.visitListLiteral(listLit); err != nil {
		return err
	}

	values, ok := c.currentFieldValue.([]any)
	if !ok {
		return fmt.Errorf("failed to extract list values for 'IN' operator at %s",
			expr.Start().String())
	}

	// Safety check (should be caught earlier, but defensive programming)
	if len(values) == 0 {
		return fmt.Errorf("'IN' operator requires a non-empty list at %s",
			expr.Start().String())
	}

	// Determine list type and create appropriate condition based on first element
	switch values[0].(type) {
	case string:
		// String list: use MatchKeywords (OR semantics for multiple keywords)
		keywords := make([]string, 0, len(values))
		for _, val := range values {
			keywords = append(keywords, cast.ToString(val))
		}
		c.filter.Must = append(c.filter.Must, qdrant.NewMatchKeywords(fieldKey, keywords...))

	case float64:
		// Number list: use MatchInts (OR semantics for multiple integers)
		integers := make([]int64, 0, len(values))
		for _, val := range values {
			integers = append(integers, cast.ToInt64(val))
		}
		c.filter.Must = append(c.filter.Must, qdrant.NewMatchInts(fieldKey, integers...))

	case bool:
		// Boolean list: wrap Should conditions in a nested filter
		// This is necessary because:
		// 1. The SDK doesn't provide NewMatchBools
		// 2. Direct Should append would affect top-level filter semantics
		// 3. Nested filter isolates the OR logic for this specific condition
		boolConditions := make([]*qdrant.Condition, 0, len(values))
		for _, val := range values {
			boolConditions = append(boolConditions, qdrant.NewMatchBool(fieldKey, cast.ToBool(val)))
		}
		c.filter.Must = append(c.filter.Must,
			qdrant.NewFilterAsCondition(&qdrant.Filter{
				Should: boolConditions,
			}))

	default:
		return fmt.Errorf("unsupported value type %T in 'IN' list at %s",
			values[0], expr.Start().String())
	}

	return nil
}

// visitLikeExpr handles the LIKE operator for pattern matching.
// The right operand must be a string literal containing the search pattern.
//
// Uses Qdrant's NewMatchText function, which performs full-text search.
// The exact matching behavior depends on the full-text index configuration in Qdrant,
// typically supporting:
//   - Substring matching
//   - Tokenization
//   - Case insensitivity
//   - Other text search features configured in the collection
//
// Note: Ensure that the field has a full-text index configured in Qdrant
// for LIKE operations to work effectively.
//
// Example:
//   - "description LIKE 'python programming'" produces: Must[MatchText(description, "python programming")]
func (c *Converter) visitLikeExpr(expr *ast.BinaryExpr) error {
	fieldKey, err := c.extractFieldKey(expr.Left)
	if err != nil {
		return fmt.Errorf("failed to extract field key from left operand of 'LIKE' at %s: %w",
			expr.Start().String(), err)
	}

	lit, ok := expr.Right.(*ast.Literal)
	if !ok {
		return fmt.Errorf("'LIKE' operator requires a string literal on the right side at %s, got %T",
			expr.Start().String(), expr.Right)
	}

	if !lit.IsString() {
		return fmt.Errorf("'LIKE' operator requires a string pattern at %s, got %s",
			expr.Start().String(), lit.Token.Kind.Name())
	}

	if err = c.visitLiteral(lit); err != nil {
		return err
	}

	pattern, ok := c.currentFieldValue.(string)
	if !ok {
		return fmt.Errorf("failed to extract string pattern for 'LIKE' operator at %s",
			expr.Start().String())
	}

	// LIKE operation uses MatchText (full-text search)
	// Behavior depends on full-text index configuration in Qdrant
	c.filter.Must = append(c.filter.Must, qdrant.NewMatchText(fieldKey, pattern))
	return nil
}

// buildNestedCondition constructs a Qdrant condition from an AST expression
// using an isolated converter instance.
//
// This method is crucial for maintaining proper condition scoping in nested expressions.
// By creating a new converter for each nested expression, we ensure that:
//   - Logical operators (AND/OR) maintain separate Must/Should/MustNot lists
//   - Conditions don't leak between different parts of the expression tree
//   - Complex nested expressions are properly isolated
//
// The isolated converter processes the expression and its entire subtree,
// then the resulting filter is wrapped as a single condition.
//
// Examples:
//   - Simple: "age > 18" -> Filter{Must[age>18]}
//   - Logical: "age > 18 AND status == 'active'" -> Filter{Must[age>18, status==active]}
//   - Nested: "(age > 18 OR age < 10) AND status == 'active'" ->
//     Filter{Must[Filter{Should[age>18, age<10]}, status==active]}
func (c *Converter) buildNestedCondition(expr ast.Expr) (*qdrant.Condition, error) {
	switch node := expr.(type) {
	case *ast.BinaryExpr,
		*ast.UnaryExpr:
		// Create isolated converter to maintain proper condition scoping
		nestedConv := NewConverter()
		err := nestedConv.visit(node)
		if err != nil {
			return nil, err
		}
		// Wrap the nested filter as a single condition
		return qdrant.NewFilterAsCondition(nestedConv.filter), nil

	default:
		return nil, fmt.Errorf("unsupported expression type %T for condition building", node)
	}
}

// extractFieldKey extracts a field key (identifier or indexed path) from an expression.
// This method handles both simple identifiers and complex indexed expressions.
//
// The converter's state (currentFieldKey) is preserved during extraction to allow
// safe nested calls without state corruption.
//
// Supported expression types:
//   - Ident: Simple field name (e.g., "age")
//   - IndexExpr: Indexed field access (e.g., metadata["user"]["name"])
//
// Examples:
//   - *ast.Ident{Value: "age"} -> "age"
//   - metadata["user"] -> "metadata.user"
//   - data["tags"][0] -> "data.tags.0"
func (c *Converter) extractFieldKey(expr ast.Expr) (string, error) {
	savedFieldKey := c.currentFieldKey
	c.currentFieldKey = ""

	err := c.visit(expr)

	// Restore state to prevent corruption in nested calls
	extractedKey := c.currentFieldKey
	c.currentFieldKey = savedFieldKey

	if err != nil {
		return "", err
	}

	if extractedKey == "" {
		return "", fmt.Errorf("failed to extract field key from %T expression", expr)
	}

	return extractedKey, nil
}

// extractFieldValue extracts a value (literal or list) from an expression.
// This method handles both single literals and list literals.
//
// The converter's state (currentFieldValue) is preserved during extraction to allow
// safe nested calls without state corruption.
//
// Supported expression types:
//   - Literal: Single constant value (string, number, boolean)
//   - ListLiteral: Array of constant values
//
// Examples:
//   - *ast.Literal{Value: "active"} -> "active"
//   - *ast.Literal{Value: 18.0} -> 18.0
//   - *ast.ListLiteral{Values: ["a", "b"]} -> []any{"a", "b"}
func (c *Converter) extractFieldValue(expr ast.Expr) (any, error) {
	savedFieldValue := c.currentFieldValue
	c.currentFieldValue = nil

	err := c.visit(expr)

	// Restore state to prevent corruption in nested calls
	extractedValue := c.currentFieldValue
	c.currentFieldValue = savedFieldValue

	if err != nil {
		return nil, err
	}

	if extractedValue == nil {
		return nil, fmt.Errorf("failed to extract value from %T expression", expr)
	}

	return extractedValue, nil
}

// buildIndexedFieldKey constructs a dot-separated field path from an index expression.
// This method recursively processes nested index expressions to build the complete path.
//
// The conversion process:
//  1. Extracts index values from right to left
//  2. Supports both string and numeric indices
//  3. Continues until reaching the base identifier
//  4. Joins all parts with dots to form the final path
//
// Transformation examples:
//   - user["name"] -> "user.name"
//   - metadata["tags"][0] -> "metadata.tags.0"
//   - data["user"]["profile"]["age"] -> "data.user.profile.age"
//   - config["servers"][1]["host"] -> "config.servers.1.host"
//
// The method validates:
//   - Index values must be strings or numbers
//   - Left side must be either another IndexExpr or an Ident
//   - Base identifier must exist
func (c *Converter) buildIndexedFieldKey(expr *ast.IndexExpr) (string, error) {
	var pathParts []string

	currentExpr := expr
	for {
		// Extract the index value (the part in brackets)
		if err := c.visitLiteral(currentExpr.Index); err != nil {
			return "", err
		}

		indexVal := c.currentFieldValue
		switch v := indexVal.(type) {
		case string:
			// String index: prepend to path parts
			pathParts = append([]string{v}, pathParts...)
		case float64:
			// Numeric index: convert to string and prepend
			pathParts = append([]string{fmt.Sprintf("%d", int(v))}, pathParts...)
		default:
			return "", fmt.Errorf("invalid index type %T, expected string or number", indexVal)
		}

		// Process the left side of the index expression
		switch leftNode := currentExpr.Left.(type) {
		case *ast.IndexExpr:
			// Nested index expression: continue processing recursively
			currentExpr = leftNode
		case *ast.Ident:
			// Base identifier found: prepend and complete the path
			pathParts = append([]string{leftNode.Value}, pathParts...)
			return strings.Join(pathParts, "."), nil
		default:
			return "", fmt.Errorf("invalid left operand type %T in index expression, expected identifier or index", leftNode)
		}
	}
}

// literalToValue converts an AST literal node to its corresponding Go value.
// This method handles the three supported literal types in the filter DSL.
//
// Supported conversions:
//   - String literals -> string (with quote removal)
//   - Number literals -> float64 (supports integers and decimals)
//   - Boolean literals -> bool (true/false)
//
// Returns an error if the literal type is not supported or if conversion fails.
func (c *Converter) literalToValue(lit *ast.Literal) (any, error) {
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

// ToFilter converts an AST filter expression into a Qdrant filter.
//
// This is the main entry point for converting filter expressions written in
// the Lynx filter DSL into Qdrant's native filter format. The conversion process
// traverses the AST and maps filter operations to appropriate Qdrant conditions.
//
// Supported operations:
//   - Logical: AND, OR, NOT
//   - Equality: ==, !=
//   - Ordering: <, <=, >, >=
//   - Membership: IN
//   - Pattern matching: LIKE
//
// Conversion semantics:
//   - AND: All conditions must match (added to filter.Must)
//   - OR: At least one condition must match (added to filter.Should)
//   - NOT: Condition must not match (added to filter.MustNot)
//   - ==: Field must equal value (added to filter.Must)
//   - !=: Field must not equal value (added to filter.MustNot)
//   - <, <=, >, >=: Field must satisfy range condition (added to filter.Must)
//   - IN: Field must equal one of the values (added to filter.Must with OR semantics)
//   - LIKE: Field must match text pattern (added to filter.Must)
//
// Example usage:
//
//	// Parse a filter expression
//	expr, err := parser.Parse(`age > 18 AND status == "active"`)
//	if err != nil {
//	    return nil, err
//	}
//
//	// Convert to Qdrant filter
//	filter, err := qdrant.ToFilter(expr)
//	if err != nil {
//	    return nil, err
//	}
//
//	// Use in Qdrant search
//	results, err := client.Search(ctx, &qdrant.SearchPoints{
//	    CollectionName: "users",
//	    Filter:         filter,
//	    Vector:         queryVector,
//	    Limit:          10,
//	})
//
// Complex example:
//
//	// Expression: (age > 18 AND age < 65) AND (status == "active" OR role == "admin")
//	// Produces: Filter{
//	//   Must: [
//	//     Filter{Must: [age>18, age<65]},
//	//     Filter{Should: [status==active, role==admin]}
//	//   ]
//	// }
//
// Returns:
//   - *qdrant.Filter: The converted filter ready for use with Qdrant client
//   - error: Conversion error if the expression contains unsupported operations or syntax errors
func ToFilter(expr ast.Expr) (*qdrant.Filter, error) {
	conv := NewConverter()
	conv.Visit(expr)
	return conv.Filter(), conv.Error()
}
