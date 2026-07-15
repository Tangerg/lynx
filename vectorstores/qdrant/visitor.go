package qdrant

import (
	"errors"
	"fmt"

	"github.com/qdrant/go-client/qdrant"

	"github.com/Tangerg/lynx/core/vectorstore/filter"
	"github.com/Tangerg/lynx/vectorstores/internal/filterhelp"
)

// Visitor transforms AST filter expressions into Qdrant filter conditions.
// It traverses semantic filter expressions and converts them to the provider query shape
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
var _ filter.Visitor = (*Visitor)(nil)

type Visitor struct {
	err               error          // Last error encountered during conversion
	filter            *qdrant.Filter // The Qdrant filter being constructed
	currentFieldValue any            // Temporary storage for field values during extraction
	currentFieldKey   string         // Temporary storage for field keys during extraction
}

func NewVisitor() *Visitor {
	return &Visitor{
		filter: &qdrant.Filter{},
	}
}

// Filter returns the constructed Qdrant filter.
// Returns nil if an error occurred during conversion.
// Should only be called after Visit() completes.
func (v *Visitor) Filter() *qdrant.Filter {
	if v.err != nil {
		return nil
	}
	return v.filter
}

// Visit translates one semantic filter expression.
// It walks the whole tree rooted at expr and returns the first error
// encountered, or nil when the entire expression was accepted.
//
// This is the main entry point for AST traversal. The actual conversion logic
// is delegated to the visit method and its specialized handlers.
func (v *Visitor) Visit(expr filter.Predicate) error {
	v.err = v.visit(expr)
	return v.err
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
func (v *Visitor) visit(expr filter.Expr) error {
	if expr == nil {
		return errors.New("cannot process nil expression")
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
func (v *Visitor) visitBinaryExpr(expr *filter.BinaryExpr) error {
	if expr.Op.IsNullOperator() {
		return v.visitNullTestExpr(expr)
	}
	return filterhelp.DispatchBinaryErr(expr,
		v.visitLogicalExpr,
		v.visitComparisonExpr,
		v.visitInExpr,
		v.visitLikeExpr,
	)
}

// visitComparisonExpr splits equality vs ordering since qdrant emits
// distinct condition shapes for the two families.
func (v *Visitor) visitComparisonExpr(expr *filter.BinaryExpr) error {
	if expr.Op.IsEqualityOperator() {
		return v.visitEqualityExpr(expr)
	}
	return v.visitOrderingExpr(expr)
}

// visitNullTestExpr emits Qdrant's IS NULL condition (NewIsNull) on the
// field's payload key, added to filter.Must so "field is null" matches.
// The negated "field is not null" arrives as NOT(field IS NULL) and is
// rendered by visitNotExpr (MustNot wrap), so no separate handling is
// needed here.
func (v *Visitor) visitNullTestExpr(expr *filter.BinaryExpr) error {
	fieldKey, err := v.extractFieldKey(expr.Left)
	if err != nil {
		return fmt.Errorf("failed to extract field key from left operand of 'IS NULL' at %s: %w",
			expr.Start().String(), err)
	}

	v.filter.Must = append(v.filter.Must, qdrant.NewIsNull(fieldKey))
	return nil
}

// visitUnaryExpr handles unary expressions — only NOT today.
func (v *Visitor) visitUnaryExpr(expr *filter.UnaryExpr) error {
	return filterhelp.DispatchUnaryErr(expr, v.visitNotExpr)
}

// visitIdent extracts and stores the identifier name as the current field key.
// This method is typically called during field key extraction in binary expressions.
//
// Example: For expression "age > 18", this extracts "age" as the field key.
func (v *Visitor) visitIdent(ident *filter.Ident) error {
	v.currentFieldKey = ident.Value
	return nil
}

// visitLiteral converts an AST literal into its corresponding Go value and stores it.
// The conversion supports string, number, and boolean literals.
//
// This method is typically called during value extraction in binary expressions.
func (v *Visitor) visitLiteral(lit *filter.Literal) error {
	value, err := v.literalToValue(lit)
	if err != nil {
		return fmt.Errorf("failed to convert literal at %s: %w",
			lit.Start().String(), err)
	}
	v.currentFieldValue = value
	return nil
}

// visitListLiteral converts a list of literals into a Go slice and stores it.
// All elements in the list are converted using literalToValue.
//
// This method is used by the IN operator to extract the list of values
// for membership testing.
func (v *Visitor) visitListLiteral(list *filter.ListLiteral) error {
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

// visitIndexExpr processes indexed field access expressions and builds a dot-separated field path.
// This enables accessing nested fields using bracket notation.
//
// Example transformations:
//   - metadata["user"] -> "metadata.user"
//   - data["tags"][0] -> "data.tags.0"
//   - config["db"]["host"] -> "config.db.host"
func (v *Visitor) visitIndexExpr(expr *filter.IndexExpr) error {
	fieldKey, err := v.buildIndexedFieldKey(expr)
	if err != nil {
		return fmt.Errorf("failed to build field path at %s: %w",
			expr.Start().String(), err)
	}
	v.currentFieldKey = fieldKey
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
func (v *Visitor) visitLogicalExpr(expr *filter.BinaryExpr) error {
	leftCond, err := v.buildNestedCondition(expr.Left)
	if err != nil {
		return fmt.Errorf("failed to process left operand of '%s' at %s: %w",
			expr.Op.String(), expr.Start().String(), err)
	}

	rightCond, err := v.buildNestedCondition(expr.Right)
	if err != nil {
		return fmt.Errorf("failed to process right operand of '%s' at %s: %w",
			expr.Op.String(), expr.Start().String(), err)
	}

	switch expr.Op {
	case filter.OpAnd:
		v.filter.Must = append(v.filter.Must, leftCond, rightCond)
		return nil
	case filter.OpOr:
		v.filter.Should = append(v.filter.Should, leftCond, rightCond)
		return nil
	default:
		// Defensive programming: should never reach here
		return fmt.Errorf("unexpected logical operator '%s' at %s",
			expr.Op.String(), expr.Start().String())
	}
}

// visitNotExpr handles the NOT operator by wrapping the negated condition in filter.MustNot.
// The operand is converted using an isolated converter to maintain proper scoping.
//
// Example:
//   - "NOT (age > 18)" produces: MustNot[age>18]
//   - "NOT (status == 'active' OR role == 'admin')" produces: MustNot[Filter{Should[...]}]
func (v *Visitor) visitNotExpr(expr *filter.UnaryExpr) error {
	cond, err := v.buildNestedCondition(expr.Right)
	if err != nil {
		return fmt.Errorf("failed to process NOT operand at %s: %w",
			expr.Start().String(), err)
	}

	v.filter.MustNot = append(v.filter.MustNot, cond)
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
