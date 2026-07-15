package qdrant

import (
	"fmt"
	"math"
	"strings"

	"github.com/qdrant/go-client/qdrant"

	"github.com/Tangerg/lynx/core/vectorstore/filter"
	"github.com/Tangerg/lynx/vectorstores/internal/filtercompile"
)

func (v *Visitor) visitEqualityExpr(expr *filter.BinaryExpr) error {
	fieldKey, err := v.extractFieldKey(expr.Left)
	if err != nil {
		return fmt.Errorf("failed to extract field key from left operand of '%s' at %s: %w",
			expr.Op.String(), expr.Start().String(), err)
	}

	fieldValue, err := v.extractFieldValue(expr.Right)
	if err != nil {
		return fmt.Errorf("failed to extract value from right operand of '%s' at %s: %w",
			expr.Op.String(), expr.Start().String(), err)
	}

	matchCond, err := v.buildMatchCondition(fieldKey, fieldValue)
	if err != nil {
		return fmt.Errorf("failed to create match condition for '%s' at %s: %w",
			expr.Op.String(), expr.Start().String(), err)
	}

	switch expr.Op {
	case filter.OpEqual:
		// == operator: field must equal value
		v.filter.Must = append(v.filter.Must, matchCond)
	case filter.OpNotEqual:
		// != operator: field must not equal value (negation)
		v.filter.MustNot = append(v.filter.MustNot, matchCond)
	default:
		// Defensive programming: should never reach here
		return fmt.Errorf("unexpected equality operator '%s' at %s",
			expr.Op.String(), expr.Start().String())
	}

	return nil
}

// buildMatchCondition creates an appropriate Qdrant match condition based on value type.
// The method automatically selects the correct Qdrant match function:
//   - string -> NewMatchKeyword (exact keyword match)
//   - int64 -> NewMatchInt
//   - bool -> NewMatchBool (boolean match)
//
// Returns an error if the value type is not supported for matching.
func (v *Visitor) buildMatchCondition(fieldKey string, fieldValue any) (*qdrant.Condition, error) {
	switch v := fieldValue.(type) {
	case string:
		// String: use keyword match (exact match)
		return qdrant.NewMatchKeyword(fieldKey, v), nil
	case int64:
		return qdrant.NewMatchInt(fieldKey, v), nil
	case uint64:
		if v > math.MaxInt64 {
			return nil, fmt.Errorf("integer %d exceeds Qdrant's int64 match range", v)
		}
		return qdrant.NewMatchInt(fieldKey, int64(v)), nil
	case float64:
		return nil, fmt.Errorf("Qdrant match requires an integer, got %v", v)
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
func (v *Visitor) visitOrderingExpr(expr *filter.BinaryExpr) error {
	fieldKey, err := v.extractFieldKey(expr.Left)
	if err != nil {
		return fmt.Errorf("failed to extract field key from left operand of '%s' at %s: %w",
			expr.Op.String(), expr.Start().String(), err)
	}

	literal, ok := expr.Right.(*filter.Literal)
	if !ok {
		return fmt.Errorf("right operand of '%s' at %s must be a number literal, got %T",
			expr.Op.String(), expr.Start().String(), expr.Right)
	}
	numericValue, err := filtercompile.NumberToFloat64(literal)
	if err != nil {
		return fmt.Errorf("cannot convert value for '%s' comparison at %s: %w",
			expr.Op.String(), expr.Start().String(), err)
	}

	switch expr.Op {
	case filter.OpLess:
		v.filter.Must = append(v.filter.Must, qdrant.NewRange(fieldKey, &qdrant.Range{
			Lt: &numericValue,
		}))
	case filter.OpLessEqual:
		v.filter.Must = append(v.filter.Must, qdrant.NewRange(fieldKey, &qdrant.Range{
			Lte: &numericValue,
		}))
	case filter.OpGreater:
		v.filter.Must = append(v.filter.Must, qdrant.NewRange(fieldKey, &qdrant.Range{
			Gt: &numericValue,
		}))
	case filter.OpGreaterEqual:
		v.filter.Must = append(v.filter.Must, qdrant.NewRange(fieldKey, &qdrant.Range{
			Gte: &numericValue,
		}))
	default:
		// Defensive programming: should never reach here
		return fmt.Errorf("unexpected ordering operator '%s' at %s",
			expr.Op.String(), expr.Start().String())
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
func (v *Visitor) visitInExpr(expr *filter.BinaryExpr) error {
	fieldKey, err := v.extractFieldKey(expr.Left)
	if err != nil {
		return fmt.Errorf("failed to extract field key from left operand of 'IN' at %s: %w",
			expr.Start().String(), err)
	}

	listLit, err := filtercompile.RequireListLiteral(expr)
	if err != nil {
		return fmt.Errorf("qdrant: %w", err)
	}

	// Determine list type and create appropriate condition based on first element
	switch {
	case listLit.Values[0].IsString():
		// String list: use MatchKeywords (OR semantics for multiple keywords)
		keywords := make([]string, 0, len(listLit.Values))
		for _, literal := range listLit.Values {
			value, err := literal.AsString()
			if err != nil {
				return err
			}
			keywords = append(keywords, value)
		}
		v.filter.Must = append(v.filter.Must, qdrant.NewMatchKeywords(fieldKey, keywords...))

	case listLit.Values[0].IsNumber():
		// Number list: use MatchInts (OR semantics for multiple integers)
		integers := make([]int64, 0, len(listLit.Values))
		for _, literal := range listLit.Values {
			value, err := filtercompile.NumberToInt64(literal)
			if err != nil {
				return fmt.Errorf("qdrant: IN numeric value: %w", err)
			}
			integers = append(integers, value)
		}
		v.filter.Must = append(v.filter.Must, qdrant.NewMatchInts(fieldKey, integers...))

	case listLit.Values[0].IsBool():
		// Boolean list: wrap Should conditions in a nested filter
		// This is necessary because:
		// 1. The SDK doesn't provide NewMatchBools
		// 2. Direct Should append would affect top-level filter semantics
		// 3. Nested filter isolates the OR logic for this specific condition
		boolConditions := make([]*qdrant.Condition, 0, len(listLit.Values))
		for _, literal := range listLit.Values {
			value, err := literal.AsBool()
			if err != nil {
				return err
			}
			boolConditions = append(boolConditions, qdrant.NewMatchBool(fieldKey, value))
		}
		v.filter.Must = append(v.filter.Must,
			qdrant.NewFilterAsCondition(&qdrant.Filter{
				Should: boolConditions,
			}))

	default:
		return fmt.Errorf("unsupported literal kind %s in 'IN' list at %s",
			listLit.Values[0].Kind, expr.Start().String())
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
func (v *Visitor) visitLikeExpr(expr *filter.BinaryExpr) error {
	fieldKey, err := v.extractFieldKey(expr.Left)
	if err != nil {
		return fmt.Errorf("failed to extract field key from left operand of 'LIKE' at %s: %w",
			expr.Start().String(), err)
	}

	lit, ok := expr.Right.(*filter.Literal)
	if !ok {
		return fmt.Errorf("'LIKE' operator requires a string literal on the right side at %s, got %T",
			expr.Start().String(), expr.Right)
	}

	if !lit.IsString() {
		return fmt.Errorf("'LIKE' operator requires a string pattern at %s, got %s",
			expr.Start().String(), lit.Kind)
	}

	if err = v.visitLiteral(lit); err != nil {
		return err
	}

	pattern, ok := v.currentFieldValue.(string)
	if !ok {
		return fmt.Errorf("failed to extract string pattern for 'LIKE' operator at %s",
			expr.Start().String())
	}

	// LIKE operation uses MatchText (full-text search)
	// Behavior depends on full-text index configuration in Qdrant
	v.filter.Must = append(v.filter.Must, qdrant.NewMatchText(fieldKey, pattern))
	return nil
}

// buildNestedCondition constructs a Qdrant condition from an AST expression
// using an isolated converter instance.
//
// This method is crucial for maintaining proper condition scoping in nested expressions.
// By creating a new converter for each nested expression, the approach ensures:
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
func (v *Visitor) buildNestedCondition(expr filter.Expr) (*qdrant.Condition, error) {
	switch node := expr.(type) {
	case *filter.BinaryExpr,
		*filter.UnaryExpr:
		// Isolated converter maintains proper condition scoping.
		nestedConv := NewVisitor()
		err := nestedConv.visit(node)
		if err != nil {
			return nil, err
		}
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
//   - *filter.Ident{Value: "age"} -> "age"
//   - metadata["user"] -> "metadata.user"
//   - data["tags"][0] -> "data.tags.0"
func (v *Visitor) extractFieldKey(expr filter.Expr) (string, error) {
	savedFieldKey := v.currentFieldKey
	v.currentFieldKey = ""

	err := v.visit(expr)

	// Restore state to prevent corruption in nested calls
	extractedKey := v.currentFieldKey
	v.currentFieldKey = savedFieldKey

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
//   - *filter.Literal{Value: "active"} -> "active"
//   - *filter.Literal{Value: 18.0} -> 18.0
//   - *filter.ListLiteral{Values: ["a", "b"]} -> []any{"a", "b"}
func (v *Visitor) extractFieldValue(expr filter.Expr) (any, error) {
	savedFieldValue := v.currentFieldValue
	v.currentFieldValue = nil

	err := v.visit(expr)

	// Restore state to prevent corruption in nested calls
	extractedValue := v.currentFieldValue
	v.currentFieldValue = savedFieldValue

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
func (v *Visitor) buildIndexedFieldKey(expr *filter.IndexExpr) (string, error) {
	var pathParts []string

	currentExpr := expr
	for {
		key, err := filtercompile.LiteralAsKey(currentExpr.Index)
		if err != nil {
			return "", err
		}
		pathParts = append([]string{key}, pathParts...)

		switch leftNode := currentExpr.Left.(type) {
		case *filter.IndexExpr:
			currentExpr = leftNode
		case *filter.Ident:
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
func (v *Visitor) literalToValue(lit *filter.Literal) (any, error) {
	if lit.IsString() {
		return lit.AsString()
	}

	if lit.IsNumber() {
		return filtercompile.LiteralToValue(lit)
	}

	if lit.IsBool() {
		return lit.AsBool()
	}

	return nil, fmt.Errorf("unsupported literal type '%s'", lit.Kind)
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
func ToFilter(expr filter.Predicate) (*qdrant.Filter, error) {
	conv := NewVisitor()
	if err := conv.Visit(expr); err != nil {
		return nil, err
	}
	return conv.Filter(), nil
}
