package pinecone

import (
	"fmt"
	"strconv"
	"strings"

	"google.golang.org/protobuf/types/known/structpb"

	"github.com/Tangerg/lynx/core/vectorstore/filter/ast"
	"github.com/Tangerg/lynx/core/vectorstore/filter/token"
	"github.com/Tangerg/lynx/vectorstores/internal/filterhelp"
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
	err               error          // Last error encountered during conversion
	result            map[string]any // The Pinecone filter condition being constructed
	currentFieldKey   string         // Temporary storage for field paths during extraction
	currentFieldValue any            // Temporary storage for field values during extraction
}

func NewVisitor() *Visitor {
	return &Visitor{}
}

// Filter converts the accumulated result into a Pinecone MetadataFilter (*structpb.Struct).
// Returns nil if an error occurred or if no result was produced.
// Should only be called after Visit() completes.
func (v *Visitor) Filter() (*structpb.Struct, error) {
	if v.err != nil {
		return nil, v.err
	}
	if v.result == nil {
		return nil, nil
	}
	return structpb.NewStruct(v.result)
}

// Error returns the last error encountered during conversion.
// Returns nil if the conversion was successful.
func (v *Visitor) Error() error {
	return v.err
}

// Visit implements the ast.Visitor interface.
// It initiates the conversion process for the given expression and stores any error.
// Always returns nil to stop further traversal as conversion is done in a single pass.
func (v *Visitor) Visit(expr ast.Expr) ast.Visitor {
	v.err = v.visit(expr)
	return nil
}

// visit dispatches conversion to specialized methods based on expression type.
func (v *Visitor) visit(expr ast.Expr) error {
	if expr == nil {
		return fmt.Errorf("pinecone: cannot process nil expression")
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
		return fmt.Errorf("pinecone: unsupported expression type %T", node)
	}
}

// visitBinaryExpr routes binary expressions to the appropriate
// handler via [filterhelp.DispatchBinaryErr]. visitComparisonExpr
// internally splits equality vs ordering since pinecone emits
// different filter shapes for the two families.
func (v *Visitor) visitBinaryExpr(expr *ast.BinaryExpr) error {
	return filterhelp.DispatchBinaryErr(expr,
		v.visitLogicalExpr,
		v.visitComparisonExpr,
		v.visitInExpr,
		v.visitLikeExpr,
	)
}

// visitComparisonExpr routes to equality or ordering based on the
// operator family. Pinecone uses distinct filter shapes for each.
func (v *Visitor) visitComparisonExpr(expr *ast.BinaryExpr) error {
	if expr.Op.Kind.IsEqualityOperator() {
		return v.visitEqualityExpr(expr)
	}
	return v.visitOrderingExpr(expr)
}

// visitLikeExpr — Pinecone metadata filters do not support LIKE.
func (v *Visitor) visitLikeExpr(expr *ast.BinaryExpr) error {
	return fmt.Errorf("pinecone: LIKE operator is not supported in Pinecone metadata filters at %s",
		expr.Start().String())
}

// visitUnaryExpr handles unary expressions.
// Only the NOT operator is supported.
func (v *Visitor) visitUnaryExpr(expr *ast.UnaryExpr) error {
	return filterhelp.DispatchUnaryErr(expr, v.visitNotExpr)
}

// visitIdent extracts and stores the identifier name as the current field key.
func (v *Visitor) visitIdent(ident *ast.Ident) error {
	v.currentFieldKey = ident.Value
	return nil
}

// visitLiteral converts an AST literal to its Go value and stores it as the current field value.
func (v *Visitor) visitLiteral(lit *ast.Literal) error {
	value, err := v.literalToValue(lit)
	if err != nil {
		return err
	}
	v.currentFieldValue = value
	return nil
}

// visitListLiteral converts a list of literals into a Go slice and stores it.
func (v *Visitor) visitListLiteral(list *ast.ListLiteral) error {
	values := make([]any, 0, len(list.Values))

	for i, lit := range list.Values {
		value, err := v.literalToValue(lit)
		if err != nil {
			return fmt.Errorf("pinecone: failed to convert list element at index %d: %w", i, err)
		}
		values = append(values, value)
	}

	v.currentFieldValue = values
	return nil
}

// visitIndexExpr processes indexed field access and builds a dot-separated field path.
// Example transformations:
//   - metadata["user"]       → "metadata.user"
//   - data["tags"][0]        → "data.tags.0"
//   - config["db"]["host"]   → "config.db.host"
func (v *Visitor) visitIndexExpr(expr *ast.IndexExpr) error {
	fieldKey, err := v.buildIndexedFieldKey(expr)
	if err != nil {
		return fmt.Errorf("pinecone: failed to build field path at %s: %w",
			expr.Start().String(), err)
	}
	v.currentFieldKey = fieldKey
	return nil
}

// visitLogicalExpr handles logical operators (AND, OR).
// Produces {"$and": [left, right]} or {"$or": [left, right]}.
func (v *Visitor) visitLogicalExpr(expr *ast.BinaryExpr) error {
	left, err := v.buildNestedExpr(expr.Left)
	if err != nil {
		return fmt.Errorf("pinecone: failed to process left operand of '%s' at %s: %w",
			expr.Op.Literal, expr.Start().String(), err)
	}

	right, err := v.buildNestedExpr(expr.Right)
	if err != nil {
		return fmt.Errorf("pinecone: failed to process right operand of '%s' at %s: %w",
			expr.Op.Literal, expr.Start().String(), err)
	}

	switch expr.Op.Kind {
	case token.AND:
		v.result = map[string]any{"$and": []any{left, right}}
	case token.OR:
		v.result = map[string]any{"$or": []any{left, right}}
	default:
		return fmt.Errorf("pinecone: unexpected logical operator '%s' at %s",
			expr.Op.Literal, expr.Start().String())
	}

	return nil
}

// visitNotExpr handles the NOT operator.
// Pinecone has no direct $not logical operator, so $nor is used as the equivalent:
// {"$nor": [condition]} means "not condition".
func (v *Visitor) visitNotExpr(expr *ast.UnaryExpr) error {
	cond, err := v.buildNestedExpr(expr.Right)
	if err != nil {
		return fmt.Errorf("pinecone: failed to process NOT operand at %s: %w",
			expr.Start().String(), err)
	}

	v.result = map[string]any{"$nor": []any{cond}}
	return nil
}

// visitEqualityExpr handles equality operators (==, !=).
// Examples:
//   - status == "active"  → {"status": {"$eq": "active"}}
//   - age != 18           → {"age": {"$ne": 18}}
func (v *Visitor) visitEqualityExpr(expr *ast.BinaryExpr) error {
	fieldKey, err := v.extractFieldKey(expr.Left)
	if err != nil {
		return fmt.Errorf("pinecone: failed to extract field key from '%s' at %s: %w",
			expr.Op.Literal, expr.Start().String(), err)
	}

	fieldValue, err := v.extractFieldValue(expr.Right)
	if err != nil {
		return fmt.Errorf("pinecone: failed to extract value from '%s' at %s: %w",
			expr.Op.Literal, expr.Start().String(), err)
	}

	switch expr.Op.Kind {
	case token.EQ:
		v.result = map[string]any{fieldKey: map[string]any{"$eq": fieldValue}}
	case token.NE:
		v.result = map[string]any{fieldKey: map[string]any{"$ne": fieldValue}}
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
func (v *Visitor) visitOrderingExpr(expr *ast.BinaryExpr) error {
	fieldKey, err := v.extractFieldKey(expr.Left)
	if err != nil {
		return fmt.Errorf("pinecone: failed to extract field key from '%s' at %s: %w",
			expr.Op.Literal, expr.Start().String(), err)
	}

	fieldValue, err := v.extractFieldValue(expr.Right)
	if err != nil {
		return fmt.Errorf("pinecone: failed to extract value from '%s' at %s: %w",
			expr.Op.Literal, expr.Start().String(), err)
	}

	switch expr.Op.Kind {
	case token.LT:
		v.result = map[string]any{fieldKey: map[string]any{"$lt": fieldValue}}
	case token.LE:
		v.result = map[string]any{fieldKey: map[string]any{"$lte": fieldValue}}
	case token.GT:
		v.result = map[string]any{fieldKey: map[string]any{"$gt": fieldValue}}
	case token.GE:
		v.result = map[string]any{fieldKey: map[string]any{"$gte": fieldValue}}
	default:
		return fmt.Errorf("pinecone: unexpected ordering operator '%s' at %s",
			expr.Op.Literal, expr.Start().String())
	}

	return nil
}

// visitInExpr handles the IN operator for membership testing.
// The right operand must be a non-empty list literal.
// Example: status IN ["active", "pending"] → {"status": {"$in": ["active", "pending"]}}
func (v *Visitor) visitInExpr(expr *ast.BinaryExpr) error {
	fieldKey, err := v.extractFieldKey(expr.Left)
	if err != nil {
		return fmt.Errorf("pinecone: failed to extract field key from 'IN' at %s: %w",
			expr.Start().String(), err)
	}

	listLit, err := filterhelp.RequireListLiteral(expr)
	if err != nil {
		return fmt.Errorf("pinecone: %w", err)
	}

	if err = v.visitListLiteral(listLit); err != nil {
		return err
	}

	v.result = map[string]any{fieldKey: map[string]any{"$in": v.currentFieldValue}}
	return nil
}

// buildNestedExpr converts a sub-expression to a filter map using an isolated visitor instance.
// This ensures that nested logical expressions maintain proper scoping.
func (v *Visitor) buildNestedExpr(expr ast.Expr) (map[string]any, error) {
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
		return "", fmt.Errorf("pinecone: failed to extract field key from %T expression", expr)
	}

	return extracted, nil
}

// extractFieldValue extracts a value (literal or list) from an expression.
// The visitor's currentFieldValue state is preserved during extraction.
func (v *Visitor) extractFieldValue(expr ast.Expr) (any, error) {
	savedValue := v.currentFieldValue
	v.currentFieldValue = nil

	err := v.visit(expr)

	extracted := v.currentFieldValue
	v.currentFieldValue = savedValue

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
func (v *Visitor) buildIndexedFieldKey(expr *ast.IndexExpr) (string, error) {
	var parts []string

	current := expr
	for {
		if err := v.visitLiteral(current.Index); err != nil {
			return "", err
		}

		idxValue := v.currentFieldValue
		switch v := idxValue.(type) {
		case string:
			parts = append([]string{v}, parts...)
		case float64:
			parts = append([]string{strconv.Itoa(int(v))}, parts...)
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
