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
//   - currentCondition: The condition being constructed for the current expression
//   - currentFieldValue: Temporary storage for extracted literal values
//   - currentFieldKey: Temporary storage for extracted field identifiers
//
// Usage:
//
//	expr := parseFilterExpression("age > 18 AND status == 'active'")
//	filter, err := ConvertAstExprToFilter(expr)
//	if err != nil {
//	    log.Fatal(err)
//	}
type Converter struct {
	err               error             // Last error encountered during conversion
	filter            *qdrant.Filter    // The Qdrant filter being constructed
	currentCondition  *qdrant.Condition // Current condition being built
	currentFieldValue any               // Temporary storage for field values
	currentFieldKey   string            // Temporary storage for field keys
}

// NewConverter creates a new converter instance with an empty filter.
func NewConverter() *Converter {
	return &Converter{
		filter: &qdrant.Filter{},
	}
}

// Filter returns the constructed Qdrant filter.
// Should only be called after Visit() completes successfully.
func (c *Converter) Filter() *qdrant.Filter {
	return c.filter
}

// Error returns the last error encountered during conversion.
// Returns nil if conversion was successful.
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
// This is the main internal routing method that handles different AST node types.
func (c *Converter) visit(expr ast.Expr) error {
	if expr == nil {
		return fmt.Errorf("cannot process nil expression")
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
// Binary operators include:
//   - Logical: AND, OR
//   - Equality: ==, !=
//   - Ordering: <, <=, >, >=
//   - Membership: IN
//   - Pattern matching: LIKE
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

// visitUnaryExpr handles unary expressions, currently only supporting NOT operator.
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
// Example: For expression "age > 18", this extracts "age".
func (c *Converter) visitIdent(ident *ast.Ident) error {
	c.currentFieldKey = ident.Value
	return nil
}

// visitLiteral converts an AST literal into a Go value and stores it.
// Supports string, number, and boolean literals.
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
// Example: metadata["user"]["age"] becomes "metadata.user.age"
func (c *Converter) visitIndexExpr(expr *ast.IndexExpr) error {
	fieldPath, err := c.buildIndexedFieldKey(expr)
	if err != nil {
		return fmt.Errorf("failed to build field path at %s: %w",
			expr.Start().String(), err)
	}
	c.currentFieldKey = fieldPath
	return nil
}

// visitLogicalExpr handles logical operators (AND, OR).
// Converts both operands to conditions and adds them to the appropriate filter clause:
//   - AND operators add conditions to filter.Must
//   - OR operators add conditions to filter.Should
func (c *Converter) visitLogicalExpr(expr *ast.BinaryExpr) error {
	leftCond, err := c.buildCondition(expr.Left)
	if err != nil {
		return fmt.Errorf("failed to process left operand of '%s' at %s: %w",
			expr.Op.Literal, expr.Start().String(), err)
	}

	rightCond, err := c.buildCondition(expr.Right)
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

// visitNotExpr handles NOT operator by wrapping the negated condition in filter.MustNot.
// Example: NOT (age > 18) becomes a MustNot condition.
func (c *Converter) visitNotExpr(expr *ast.UnaryExpr) error {
	cond, err := c.buildCondition(expr.Right)
	if err != nil {
		return fmt.Errorf("failed to process NOT operand at %s: %w",
			expr.Start().String(), err)
	}

	c.filter.MustNot = append(c.filter.MustNot, cond)
	return nil
}

// visitEqualityExpr handles equality operators (==, !=).
// Supports exact matching for different data types:
//   - Strings: Uses NewMatchKeyword for exact keyword matching
//   - Numbers: Uses NewMatchInt for integer matching
//   - Booleans: Uses NewMatchBool for boolean matching
//
// For != operator, wraps the match condition in MustNot.
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

	switch expr.Op.Kind {
	case token.EQ:
		// == operator: exact match
		matchCond, err := c.buildMatchCondition(fieldKey, fieldValue)
		if err != nil {
			return fmt.Errorf("failed to create match condition for '==' at %s: %w",
				expr.Start().String(), err)
		}
		c.currentCondition = matchCond

	case token.NE:
		// != operator: negated match
		matchCond, err := c.buildMatchCondition(fieldKey, fieldValue)
		if err != nil {
			return fmt.Errorf("failed to create match condition for '!=' at %s: %w",
				expr.Start().String(), err)
		}
		c.currentCondition = qdrant.NewFilterAsCondition(&qdrant.Filter{
			MustNot: []*qdrant.Condition{matchCond},
		})

	default:
		// Defensive programming: should never reach here
		return fmt.Errorf("unexpected equality operator '%s' at %s",
			expr.Op.Literal, expr.Start().String())
	}

	return nil
}

// buildMatchCondition creates an appropriate Qdrant match condition based on value type.
func (c *Converter) buildMatchCondition(fieldKey string, fieldValue any) (*qdrant.Condition, error) {
	switch v := fieldValue.(type) {
	case string:
		// String: use keyword match (exact match)
		return qdrant.NewMatchKeyword(fieldKey, v), nil
	case float64:
		// Number: use integer match
		return qdrant.NewMatchInt(fieldKey, cast.ToInt64(v)), nil
	case bool:
		// Boolean: use bool match
		return qdrant.NewMatchBool(fieldKey, v), nil
	default:
		return nil, fmt.Errorf("unsupported value type %T for match condition", fieldValue)
	}
}

// visitOrderingExpr handles ordering/comparison operators (<, <=, >, >=).
// Converts the right operand value to float64 and creates a range condition:
//   - < : Creates Range with Lt (less than)
//   - <= : Creates Range with Lte (less than or equal)
//   - > : Creates Range with Gt (greater than)
//   - >= : Creates Range with Gte (greater than or equal)
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
		// < operator: less than
		c.currentCondition = qdrant.NewRange(fieldKey, &qdrant.Range{
			Lt: ptr.Pointer(numericValue),
		})
	case token.LE:
		// <= operator: less than or equal
		c.currentCondition = qdrant.NewRange(fieldKey, &qdrant.Range{
			Lte: ptr.Pointer(numericValue),
		})
	case token.GT:
		// > operator: greater than
		c.currentCondition = qdrant.NewRange(fieldKey, &qdrant.Range{
			Gt: ptr.Pointer(numericValue),
		})
	case token.GE:
		// >= operator: greater than or equal
		c.currentCondition = qdrant.NewRange(fieldKey, &qdrant.Range{
			Gte: ptr.Pointer(numericValue),
		})
	default:
		// Defensive programming: should never reach here
		return fmt.Errorf("unexpected ordering operator '%s' at %s",
			expr.Op.Literal, expr.Start().String())
	}

	return nil
}

// visitInExpr handles IN operator for membership testing.
// The right operand must be a list literal containing values of the same type.
// Creates appropriate Qdrant conditions based on list element type:
//   - String list: Uses NewMatchKeywords for multiple keyword matching
//   - Number list: Uses NewMatchInts for multiple integer matching
//   - Boolean list: Creates Should filter with multiple bool matches (SDK limitation)
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

	// Safety check (should be caught earlier)
	if len(values) == 0 {
		return fmt.Errorf("'IN' operator requires a non-empty list at %s",
			expr.Start().String())
	}

	// Determine list type and create appropriate condition based on first element
	switch values[0].(type) {
	case string:
		// String list: use MatchKeywords (OR match for multiple keywords)
		keywords := make([]string, 0, len(values))
		for _, val := range values {
			keywords = append(keywords, cast.ToString(val))
		}
		c.currentCondition = qdrant.NewMatchKeywords(fieldKey, keywords...)

	case float64:
		// Number list: use MatchInts (OR match for multiple integers)
		integers := make([]int64, 0, len(values))
		for _, val := range values {
			integers = append(integers, cast.ToInt64(val))
		}
		c.currentCondition = qdrant.NewMatchInts(fieldKey, integers...)

	case bool:
		// Boolean list: manually build Should condition
		// SDK doesn't provide NewMatchBools, so we create individual conditions
		boolConditions := make([]*qdrant.Condition, 0, len(values))
		for _, val := range values {
			cond := qdrant.NewMatchBool(fieldKey, cast.ToBool(val))
			boolConditions = append(boolConditions, cond)
		}
		c.currentCondition = qdrant.NewFilterAsCondition(&qdrant.Filter{
			Should: boolConditions,
		})

	default:
		return fmt.Errorf("unsupported value type %T in 'IN' list at %s",
			values[0], expr.Start().String())
	}

	return nil
}

// visitLikeExpr handles LIKE operator for pattern matching.
// The right operand must be a string literal containing the search pattern.
// Uses Qdrant's NewMatchText for full-text search functionality.
//
// Note: The exact matching behavior depends on the full-text index configuration in Qdrant.
// Typically supports substring matching, tokenization, and other text search features.
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

	// LIKE operation uses MatchText (full-text search/fuzzy match)
	// Behavior depends on full-text index configuration in Qdrant
	c.currentCondition = qdrant.NewMatchText(fieldKey, pattern)
	return nil
}

// buildCondition constructs a Qdrant condition from an AST expression.
// This method handles the recursive conversion of nested expressions:
//   - For logical expressions (AND/OR), creates a nested converter to maintain proper scope
//   - For other expressions, processes them directly and returns the resulting condition
//
// The method preserves converter state by saving and restoring current values,
// allowing for safe nested condition building.
func (c *Converter) buildCondition(expr ast.Expr) (*qdrant.Condition, error) {
	// Save current state to restore later
	savedCondition := c.currentCondition
	savedFieldValue := c.currentFieldValue
	savedFieldKey := c.currentFieldKey

	// Reset state for this operation
	c.currentCondition = nil
	c.currentFieldValue = nil
	c.currentFieldKey = ""

	// Restore state when done
	defer func() {
		c.currentCondition = savedCondition
		c.currentFieldValue = savedFieldValue
		c.currentFieldKey = savedFieldKey
	}()

	var err error
	switch node := expr.(type) {
	case *ast.BinaryExpr:
		// Handle nested logical expressions with a new converter instance
		// This ensures proper scoping of Must/Should/MustNot clauses
		if node.Op.Kind.IsLogicalOperator() {
			nestedConv := NewConverter()
			err = nestedConv.visit(node)
			if err != nil {
				return nil, err
			}
			return qdrant.NewFilterAsCondition(nestedConv.filter), nil
		}
		err = c.visitBinaryExpr(node)

	case *ast.UnaryExpr:
		err = c.visitUnaryExpr(node)

	default:
		return nil, fmt.Errorf("unsupported expression type %T for condition building", node)
	}

	if err != nil {
		return nil, err
	}

	return c.currentCondition, nil
}

// extractFieldKey extracts a field key (identifier or indexed path) from an expression.
// Temporarily stores the result in currentFieldKey and returns it.
// The converter's state is preserved before and after extraction.
func (c *Converter) extractFieldKey(expr ast.Expr) (string, error) {
	savedFieldKey := c.currentFieldKey
	c.currentFieldKey = ""

	err := c.visit(expr)
	if err != nil {
		c.currentFieldKey = savedFieldKey
		return "", err
	}

	extractedKey := c.currentFieldKey
	c.currentFieldKey = savedFieldKey

	if extractedKey == "" {
		return "", fmt.Errorf("failed to extract field key from %T expression", expr)
	}

	return extractedKey, nil
}

// extractFieldValue extracts a value (literal or list) from an expression.
// Temporarily stores the result in currentFieldValue and returns it.
// The converter's state is preserved before and after extraction.
func (c *Converter) extractFieldValue(expr ast.Expr) (any, error) {
	savedFieldValue := c.currentFieldValue
	c.currentFieldValue = nil

	err := c.visit(expr)
	if err != nil {
		c.currentFieldValue = savedFieldValue
		return nil, err
	}

	extractedValue := c.currentFieldValue
	c.currentFieldValue = savedFieldValue

	if extractedValue == nil {
		return nil, fmt.Errorf("failed to extract value from %T expression", expr)
	}

	return extractedValue, nil
}

// buildIndexedFieldKey constructs a dot-separated field path from an index expression.
// Recursively processes nested index expressions to build the complete path.
//
// Example transformations:
//   - user["name"] -> "user.name"
//   - metadata["tags"][0] -> "metadata.tags.0"
//   - data["user"]["profile"]["age"] -> "data.user.profile.age"
//
// The method supports both string and numeric indices.
func (c *Converter) buildIndexedFieldKey(expr *ast.IndexExpr) (string, error) {
	var pathParts []string

	currentExpr := expr
	for {
		// Extract the index value
		if err := c.visitLiteral(currentExpr.Index); err != nil {
			return "", err
		}

		indexVal := c.currentFieldValue
		switch v := indexVal.(type) {
		case string:
			pathParts = append([]string{v}, pathParts...)
		case float64:
			pathParts = append([]string{fmt.Sprintf("%d", int(v))}, pathParts...)
		default:
			return "", fmt.Errorf("invalid index type %T, expected string or number", indexVal)
		}

		// Process the left side of the index expression
		switch leftNode := currentExpr.Left.(type) {
		case *ast.IndexExpr:
			// Nested index expression, continue processing
			currentExpr = leftNode
		case *ast.Ident:
			// Base identifier found, prepend and complete
			pathParts = append([]string{leftNode.Value}, pathParts...)
			return strings.Join(pathParts, "."), nil
		default:
			return "", fmt.Errorf("invalid left operand type %T in index expression, expected identifier or index", leftNode)
		}
	}
}

// literalToValue converts an AST literal node to its corresponding Go value.
// Supports three literal types:
//   - String literals -> string
//   - Number literals -> float64
//   - Boolean literals -> bool
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
// Returns:
//   - *qdrant.Filter: The converted filter ready for use with Qdrant client
//   - error: Conversion error if the expression contains unsupported operations
func ToFilter(expr ast.Expr) (*qdrant.Filter, error) {
	conv := NewConverter()
	conv.Visit(expr)
	return conv.Filter(), conv.Error()
}
