package pinecone

import (
	"fmt"
	"strings"

	"google.golang.org/protobuf/types/known/structpb"

	"github.com/Tangerg/lynx/core/vectorstore/filter/ast"
	"github.com/Tangerg/lynx/core/vectorstore/filter/token"
)

var _ ast.Visitor = (*Visitor)(nil)

// Visitor transforms AST filter expressions into Pinecone metadata filter conditions.
// It implements the ast.Visitor interface to traverse and convert expression trees
// into Pinecone's native metadata filter format (structpb.Struct).
//
// The converter maintains internal state during traversal:
//   - result: The filter condition map being built
//   - currentFieldKey: Temporary storage for extracted field identifiers
//   - currentFieldValue: Temporary storage for extracted literal values
//   - err: The last error encountered during conversion
//
// Conversion strategy:
//   - Logical operators (AND, OR) produce {"$and":[...]} / {"$or":[...]}
//   - Equality operators produce {"field": {"$eq": value}} / {"field": {"$ne": value}}
//   - Ordering operators produce {"field": {"$gt": value}}, etc.
//   - IN operator produces {"field": {"$in": [...]}}
//   - NOT operator produces {"$nor": [condition]}
//   - LIKE is not supported by Pinecone metadata filters
//
// Usage example:
//
//	expr := parseFilterExpression("age > 18 AND status == 'active'")
//	filter, err := ToFilter(expr)
//	if err != nil {
//	    log.Fatal(err)
//	}
type Visitor struct {
	err               error                  // Last error encountered during conversion
	result            map[string]interface{} // The Pinecone filter condition being constructed
	currentFieldKey   string                 // Temporary storage for field paths during extraction
	currentFieldValue interface{}            // Temporary storage for field values during extraction
}

// NewVisitor creates a new visitor instance ready to process AST expressions.
func NewVisitor() *Visitor {
	return &Visitor{}
}

// Filter converts the accumulated result into a Pinecone MetadataFilter (*structpb.Struct).
// Returns nil if an error occurred or if no result was produced.
// Should only be called after Visit() completes.
func (c *Visitor) Filter() (*structpb.Struct, error) {
	if c.err != nil {
		return nil, c.err
	}
	if c.result == nil {
		return nil, nil
	}
	return structpb.NewStruct(c.result)
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
		return fmt.Errorf("pinecone: cannot process nil expression")
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
		return fmt.Errorf("pinecone: unsupported expression type %T", node)
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
		return fmt.Errorf("pinecone: LIKE operator is not supported in Pinecone metadata filters at %s",
			expr.Start().String())
	}
	return fmt.Errorf("pinecone: unsupported binary operator '%s' at %s",
		expr.Op.Literal, expr.Start().String())
}

// visitUnaryExpr handles unary expressions.
// Only the NOT operator is supported.
func (c *Visitor) visitUnaryExpr(expr *ast.UnaryExpr) error {
	if !expr.Op.Kind.IsUnaryOperator() {
		return fmt.Errorf("pinecone: '%s' is not a valid unary operator at %s",
			expr.Op.Literal, expr.Start().String())
	}

	switch expr.Op.Kind {
	case token.NOT:
		return c.visitNotExpr(expr)
	default:
		return fmt.Errorf("pinecone: unhandled unary operator '%s' at %s",
			expr.Op.Literal, expr.Start().String())
	}
}

// visitIdent extracts and stores the identifier name as the current field key.
func (c *Visitor) visitIdent(ident *ast.Ident) error {
	c.currentFieldKey = ident.Value
	return nil
}

// visitLiteral converts an AST literal to its Go value and stores it as the current field value.
func (c *Visitor) visitLiteral(lit *ast.Literal) error {
	value, err := c.literalToValue(lit)
	if err != nil {
		return err
	}
	c.currentFieldValue = value
	return nil
}

// visitListLiteral converts a list of literals into a Go slice and stores it.
func (c *Visitor) visitListLiteral(list *ast.ListLiteral) error {
	values := make([]interface{}, 0, len(list.Values))

	for i, lit := range list.Values {
		value, err := c.literalToValue(lit)
		if err != nil {
			return fmt.Errorf("pinecone: failed to convert list element at index %d: %w", i, err)
		}
		values = append(values, value)
	}

	c.currentFieldValue = values
	return nil
}

// visitIndexExpr processes indexed field access and builds a dot-separated field path.
// Example transformations:
//   - metadata["user"]       → "metadata.user"
//   - data["tags"][0]        → "data.tags.0"
//   - config["db"]["host"]   → "config.db.host"
func (c *Visitor) visitIndexExpr(expr *ast.IndexExpr) error {
	fieldKey, err := c.buildIndexedFieldKey(expr)
	if err != nil {
		return fmt.Errorf("pinecone: failed to build field path at %s: %w",
			expr.Start().String(), err)
	}
	c.currentFieldKey = fieldKey
	return nil
}

// visitLogicalExpr handles logical operators (AND, OR).
// Produces {"$and": [left, right]} or {"$or": [left, right]}.
func (c *Visitor) visitLogicalExpr(expr *ast.BinaryExpr) error {
	left, err := c.buildNestedExpr(expr.Left)
	if err != nil {
		return fmt.Errorf("pinecone: failed to process left operand of '%s' at %s: %w",
			expr.Op.Literal, expr.Start().String(), err)
	}

	right, err := c.buildNestedExpr(expr.Right)
	if err != nil {
		return fmt.Errorf("pinecone: failed to process right operand of '%s' at %s: %w",
			expr.Op.Literal, expr.Start().String(), err)
	}

	switch expr.Op.Kind {
	case token.AND:
		c.result = map[string]interface{}{"$and": []interface{}{left, right}}
	case token.OR:
		c.result = map[string]interface{}{"$or": []interface{}{left, right}}
	default:
		return fmt.Errorf("pinecone: unexpected logical operator '%s' at %s",
			expr.Op.Literal, expr.Start().String())
	}

	return nil
}

// visitNotExpr handles the NOT operator.
// Pinecone has no direct $not logical operator, so $nor is used as the equivalent:
// {"$nor": [condition]} means "not condition".
func (c *Visitor) visitNotExpr(expr *ast.UnaryExpr) error {
	cond, err := c.buildNestedExpr(expr.Right)
	if err != nil {
		return fmt.Errorf("pinecone: failed to process NOT operand at %s: %w",
			expr.Start().String(), err)
	}

	c.result = map[string]interface{}{"$nor": []interface{}{cond}}
	return nil
}

// visitEqualityExpr handles equality operators (==, !=).
// Examples:
//   - status == "active"  → {"status": {"$eq": "active"}}
//   - age != 18           → {"age": {"$ne": 18}}
func (c *Visitor) visitEqualityExpr(expr *ast.BinaryExpr) error {
	fieldKey, err := c.extractFieldKey(expr.Left)
	if err != nil {
		return fmt.Errorf("pinecone: failed to extract field key from '%s' at %s: %w",
			expr.Op.Literal, expr.Start().String(), err)
	}

	fieldValue, err := c.extractFieldValue(expr.Right)
	if err != nil {
		return fmt.Errorf("pinecone: failed to extract value from '%s' at %s: %w",
			expr.Op.Literal, expr.Start().String(), err)
	}

	switch expr.Op.Kind {
	case token.EQ:
		c.result = map[string]interface{}{fieldKey: map[string]interface{}{"$eq": fieldValue}}
	case token.NE:
		c.result = map[string]interface{}{fieldKey: map[string]interface{}{"$ne": fieldValue}}
	default:
		return fmt.Errorf("pinecone: unexpected equality operator '%s' at %s",
			expr.Op.Literal, expr.Start().String())
	}

	return nil
}

// visitOrderingExpr handles ordering operators (<, <=, >, >=).
// Examples:
//   - age > 18     → {"age": {"$gt": 18}}
//   - price <= 99  → {"price": {"$lte": 99}}
func (c *Visitor) visitOrderingExpr(expr *ast.BinaryExpr) error {
	fieldKey, err := c.extractFieldKey(expr.Left)
	if err != nil {
		return fmt.Errorf("pinecone: failed to extract field key from '%s' at %s: %w",
			expr.Op.Literal, expr.Start().String(), err)
	}

	fieldValue, err := c.extractFieldValue(expr.Right)
	if err != nil {
		return fmt.Errorf("pinecone: failed to extract value from '%s' at %s: %w",
			expr.Op.Literal, expr.Start().String(), err)
	}

	switch expr.Op.Kind {
	case token.LT:
		c.result = map[string]interface{}{fieldKey: map[string]interface{}{"$lt": fieldValue}}
	case token.LE:
		c.result = map[string]interface{}{fieldKey: map[string]interface{}{"$lte": fieldValue}}
	case token.GT:
		c.result = map[string]interface{}{fieldKey: map[string]interface{}{"$gt": fieldValue}}
	case token.GE:
		c.result = map[string]interface{}{fieldKey: map[string]interface{}{"$gte": fieldValue}}
	default:
		return fmt.Errorf("pinecone: unexpected ordering operator '%s' at %s",
			expr.Op.Literal, expr.Start().String())
	}

	return nil
}

// visitInExpr handles the IN operator for membership testing.
// The right operand must be a non-empty list literal.
// Example: status IN ["active", "pending"] → {"status": {"$in": ["active", "pending"]}}
func (c *Visitor) visitInExpr(expr *ast.BinaryExpr) error {
	fieldKey, err := c.extractFieldKey(expr.Left)
	if err != nil {
		return fmt.Errorf("pinecone: failed to extract field key from 'IN' at %s: %w",
			expr.Start().String(), err)
	}

	listLit, ok := expr.Right.(*ast.ListLiteral)
	if !ok {
		return fmt.Errorf("pinecone: 'IN' operator requires a list on the right side at %s, got %T",
			expr.Start().String(), expr.Right)
	}
	if len(listLit.Values) == 0 {
		return fmt.Errorf("pinecone: 'IN' operator requires a non-empty list at %s",
			expr.Start().String())
	}

	if err = c.visitListLiteral(listLit); err != nil {
		return err
	}

	c.result = map[string]interface{}{fieldKey: map[string]interface{}{"$in": c.currentFieldValue}}
	return nil
}

// buildNestedExpr converts a sub-expression to a filter map using an isolated visitor instance.
// This ensures that nested logical expressions maintain proper scoping.
func (c *Visitor) buildNestedExpr(expr ast.Expr) (map[string]interface{}, error) {
	nested := NewVisitor()
	if err := nested.visit(expr); err != nil {
		return nil, err
	}
	if nested.result != nil {
		return nested.result, nil
	}
	return nil, fmt.Errorf("pinecone: unsupported expression type %T for nested expression", expr)
}

// extractFieldKey extracts a field key (identifier or dot-separated path) from an expression.
// The visitor's currentFieldKey state is preserved during extraction.
func (c *Visitor) extractFieldKey(expr ast.Expr) (string, error) {
	savedKey := c.currentFieldKey
	c.currentFieldKey = ""

	err := c.visit(expr)

	extracted := c.currentFieldKey
	c.currentFieldKey = savedKey

	if err != nil {
		return "", err
	}
	if extracted == "" {
		return "", fmt.Errorf("pinecone: failed to extract field key from %T expression", expr)
	}

	return extracted, nil
}

// extractFieldValue extracts a value (literal or list) from an expression.
// The visitor's currentFieldValue state is preserved during extraction.
func (c *Visitor) extractFieldValue(expr ast.Expr) (interface{}, error) {
	savedValue := c.currentFieldValue
	c.currentFieldValue = nil

	err := c.visit(expr)

	extracted := c.currentFieldValue
	c.currentFieldValue = savedValue

	if err != nil {
		return nil, err
	}
	if extracted == nil {
		return nil, fmt.Errorf("pinecone: failed to extract value from %T expression", expr)
	}

	return extracted, nil
}

// buildIndexedFieldKey constructs a dot-separated field path from an index expression.
// Transformation examples:
//   - user["name"]                → "user.name"
//   - metadata["tags"][0]         → "metadata.tags.0"
//   - config["db"]["host"]        → "config.db.host"
func (c *Visitor) buildIndexedFieldKey(expr *ast.IndexExpr) (string, error) {
	var parts []string

	current := expr
	for {
		if err := c.visitLiteral(current.Index); err != nil {
			return "", err
		}

		idxValue := c.currentFieldValue
		switch v := idxValue.(type) {
		case string:
			parts = append([]string{v}, parts...)
		case float64:
			parts = append([]string{fmt.Sprintf("%d", int(v))}, parts...)
		default:
			return "", fmt.Errorf("pinecone: invalid index type %T, expected string or number", idxValue)
		}

		switch left := current.Left.(type) {
		case *ast.IndexExpr:
			current = left
		case *ast.Ident:
			parts = append([]string{left.Value}, parts...)
			return strings.Join(parts, "."), nil
		default:
			return "", fmt.Errorf("pinecone: invalid left operand type %T in index expression, expected identifier or index",
				left)
		}
	}
}

// literalToValue converts an AST literal node to its corresponding Go value.
// Supported conversions: string → string, number → float64, boolean → bool.
func (c *Visitor) literalToValue(lit *ast.Literal) (interface{}, error) {
	if lit.IsString() {
		return lit.AsString()
	}
	if lit.IsNumber() {
		return lit.AsNumber()
	}
	if lit.IsBool() {
		return lit.AsBool()
	}
	return nil, fmt.Errorf("pinecone: unsupported literal type '%s'", lit.Token.Kind.Name())
}

// ToFilter converts an AST filter expression into a Pinecone MetadataFilter (*structpb.Struct).
//
// This is the main entry point for converting filter expressions written in
// the Lynx filter DSL into Pinecone's native metadata filter format.
//
// Supported operations:
//   - Logical:    AND, OR, NOT
//   - Equality:   ==, !=
//   - Ordering:   <, <=, >, >=
//   - Membership: IN
//
// Note: The LIKE operator is not supported by Pinecone metadata filters.
//
// Conversion semantics:
//   - AND: {"$and": [left, right]}
//   - OR:  {"$or":  [left, right]}
//   - NOT: {"$nor": [condition]}  (Pinecone has no direct $not logical operator)
//   - ==:  {"field": {"$eq": value}}
//   - !=:  {"field": {"$ne": value}}
//   - <:   {"field": {"$lt": value}}
//   - <=:  {"field": {"$lte": value}}
//   - >:   {"field": {"$gt": value}}
//   - >=:  {"field": {"$gte": value}}
//   - IN:  {"field": {"$in": [values...]}}
//
// Field access:
//   - Simple field:  age                   → "age"
//   - Indexed key:   metadata["key"]       → "metadata.key"
//   - Nested key:    metadata["a"]["b"]    → "metadata.a.b"
//
// Example usage:
//
//	expr, _ := parser.Parse(`age > 18 AND status == "active"`)
//	filter, err := pinecone.ToFilter(expr)
//	// filter encodes: {"$and": [{"age": {"$gt": 18}}, {"status": {"$eq": "active"}}]}
func ToFilter(expr ast.Expr) (*structpb.Struct, error) {
	conv := NewVisitor()
	conv.Visit(expr)
	return conv.Filter()
}
