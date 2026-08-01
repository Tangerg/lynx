package pinecone

import (
	"errors"
	"fmt"
	"strings"

	"google.golang.org/protobuf/types/known/structpb"

	"github.com/Tangerg/lynx/core/vectorstore/filter"
	"github.com/Tangerg/lynx/vectorstores/internal/filtercompile"
)

// Visitor transforms AST filter expressions into Pinecone metadata filter conditions.
// It traverses semantic filter expressions and converts them to the provider query shape
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
//   - IN produces {"field": {"$in": [...]}}
//   - HAS uses scalar equality on list-valued string metadata
//   - NOT is lowered into inverse comparison operators and De Morgan rewrites
//   - LIKE is not supported by Pinecone metadata filters
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

// Visit translates one semantic filter expression.
// It walks the whole tree rooted at expr and returns the first error
// encountered, or nil when the entire expression was accepted.
func (v *Visitor) Visit(expr filter.Predicate) error {
	v.err = nil
	v.result = nil
	v.currentFieldKey = ""
	v.currentFieldValue = nil
	v.err = v.visit(expr)
	return v.err
}

// visit dispatches conversion to specialized methods based on expression type.
func (v *Visitor) visit(expr filter.Expr) error {
	if expr == nil {
		return errors.New("pinecone: cannot process nil expression")
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
		return fmt.Errorf("pinecone: unsupported expression type %T", node)
	}
}

// visitBinaryExpr routes binary expressions to the appropriate
// handler via [filtercompile.DispatchBinary]. visitComparisonExpr
// internally splits equality vs ordering since pinecone emits
// different filter shapes for the two families.
func (v *Visitor) visitBinaryExpr(expr *filter.BinaryExpr) error {
	return filtercompile.DispatchBinary(expr, filtercompile.BinaryHandlers{
		Logical:    v.visitLogicalExpr,
		Comparison: v.visitComparisonExpr,
		In:         v.visitInExpr,
		Has:        v.visitHasExpr,
		Like:       v.visitLikeExpr,
	})
}

// visitHasExpr uses Pinecone's documented scalar equality behavior for
// list-valued string metadata.
func (v *Visitor) visitHasExpr(expr *filter.BinaryExpr) error {
	fieldKey, err := v.extractFieldKey(expr.Left)
	if err != nil {
		return fmt.Errorf("pinecone: extract collection field at %s: %w", expr.Start().String(), err)
	}
	fieldValue, err := v.extractFieldValue(expr.Right)
	if err != nil {
		return fmt.Errorf("pinecone: extract collection member at %s: %w", expr.Start().String(), err)
	}
	if _, ok := fieldValue.(string); !ok {
		return fmt.Errorf("pinecone: HAS requires a string member because Pinecone collection metadata is string-only at %s", expr.Start().String())
	}
	v.result = map[string]any{fieldKey: map[string]any{"$eq": fieldValue}}
	return nil
}

// visitComparisonExpr routes to equality or ordering based on the
// operator family. Pinecone uses distinct filter shapes for each.
func (v *Visitor) visitComparisonExpr(expr *filter.BinaryExpr) error {
	if expr.Op.IsEqualityOperator() {
		return v.visitEqualityExpr(expr)
	}
	return v.visitOrderingExpr(expr)
}

// visitLikeExpr — Pinecone metadata filters do not support LIKE.
func (v *Visitor) visitLikeExpr(expr *filter.BinaryExpr) error {
	return fmt.Errorf("pinecone: LIKE operator is not supported in Pinecone metadata filters at %s",
		expr.Start().String())
}

// visitUnaryExpr handles unary expressions.
// Only the NOT operator is supported.
func (v *Visitor) visitUnaryExpr(expr *filter.UnaryExpr) error {
	return filtercompile.DispatchUnary(expr, v.visitNotExpr)
}

// visitIdent extracts and stores the identifier name as the current field key.
func (v *Visitor) visitIdent(ident *filter.Ident) error {
	v.currentFieldKey = ident.Value
	return nil
}

// visitLiteral converts an AST literal to its Go value and stores it as the current field value.
func (v *Visitor) visitLiteral(lit *filter.Literal) error {
	value, err := v.literalToValue(lit)
	if err != nil {
		return err
	}
	v.currentFieldValue = value
	return nil
}

// visitListLiteral converts a list of literals into a Go slice and stores it.
func (v *Visitor) visitListLiteral(list *filter.ListLiteral) error {
	values := make([]any, 0, len(list.Values))

	for i, lit := range list.Values {
		value, err := v.literalToValue(lit)
		if err != nil {
			return fmt.Errorf("pinecone: convert list element at index %d: %w", i, err)
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
func (v *Visitor) visitIndexExpr(expr *filter.IndexExpr) error {
	fieldKey, err := v.buildIndexedFieldKey(expr)
	if err != nil {
		return fmt.Errorf("pinecone: build field path at %s: %w",
			expr.Start().String(), err)
	}
	v.currentFieldKey = fieldKey
	return nil
}

// visitLogicalExpr handles logical operators (AND, OR).
// Produces {"$and": [left, right]} or {"$or": [left, right]}.
func (v *Visitor) visitLogicalExpr(expr *filter.BinaryExpr) error {
	left, err := v.buildNestedExpr(expr.Left)
	if err != nil {
		return fmt.Errorf("pinecone: process left operand of '%s' at %s: %w",
			expr.Op.String(), expr.Start().String(), err)
	}

	right, err := v.buildNestedExpr(expr.Right)
	if err != nil {
		return fmt.Errorf("pinecone: process right operand of '%s' at %s: %w",
			expr.Op.String(), expr.Start().String(), err)
	}

	switch expr.Op {
	case filter.OpAnd:
		v.result = map[string]any{"$and": []any{left, right}}
	case filter.OpOr:
		v.result = map[string]any{"$or": []any{left, right}}
	default:
		return fmt.Errorf("pinecone: unexpected logical operator '%s' at %s",
			expr.Op.String(), expr.Start().String())
	}

	return nil
}

// visitNotExpr lowers NOT into Pinecone's supported inverse operators and De
// Morgan rewrites. Pinecone supports only $and and $or as logical operators;
// emitting MongoDB's $nor would produce an invalid provider request.
func (v *Visitor) visitNotExpr(expr *filter.UnaryExpr) error {
	condition, err := v.buildNegatedExpr(expr.Right)
	if err != nil {
		return fmt.Errorf("pinecone: process NOT operand at %s: %w",
			expr.Start().String(), err)
	}
	v.result = condition
	return nil
}

func (v *Visitor) buildNegatedExpr(expr filter.Expr) (map[string]any, error) {
	switch node := expr.(type) {
	case *filter.UnaryExpr:
		if !node.Op.Is(filter.OpNot) {
			return nil, fmt.Errorf("cannot negate unary operator %s", node.Op.Name())
		}
		return v.buildNestedExpr(node.Right)
	case *filter.BinaryExpr:
		switch {
		case node.Op.IsLogicalOperator():
			left, err := v.buildNegatedExpr(node.Left)
			if err != nil {
				return nil, err
			}
			right, err := v.buildNegatedExpr(node.Right)
			if err != nil {
				return nil, err
			}
			op := "$or"
			if node.Op.Is(filter.OpOr) {
				op = "$and"
			}
			return map[string]any{op: []any{left, right}}, nil
		case node.Op.IsComparisonOperator():
			inverted := *node
			var err error
			inverted.Op, err = inverseComparisonOperator(node.Op)
			if err != nil {
				return nil, err
			}
			return v.buildNestedExpr(&inverted)
		case node.Op.Is(filter.OpIn):
			return v.buildListMembershipExpr(node, "$nin")
		case node.Op.Is(filter.OpHas):
			return v.buildNegatedCollectionMembershipExpr(node)
		default:
			return nil, fmt.Errorf("cannot negate operator %s", node.Op.Name())
		}
	default:
		return nil, fmt.Errorf("cannot negate expression %T", expr)
	}
}

func inverseComparisonOperator(operator filter.Operator) (filter.Operator, error) {
	switch operator {
	case filter.OpEqual:
		return filter.OpNotEqual, nil
	case filter.OpNotEqual:
		return filter.OpEqual, nil
	case filter.OpLess:
		return filter.OpGreaterEqual, nil
	case filter.OpLessEqual:
		return filter.OpGreater, nil
	case filter.OpGreater:
		return filter.OpLessEqual, nil
	case filter.OpGreaterEqual:
		return filter.OpLess, nil
	default:
		return "", fmt.Errorf("cannot invert comparison operator %s", operator.Name())
	}
}

func (v *Visitor) buildNegatedCollectionMembershipExpr(expr *filter.BinaryExpr) (map[string]any, error) {
	fieldKey, err := v.extractFieldKey(expr.Left)
	if err != nil {
		return nil, err
	}
	fieldValue, err := v.extractFieldValue(expr.Right)
	if err != nil {
		return nil, err
	}
	if _, ok := fieldValue.(string); !ok {
		return nil, fmt.Errorf("HAS requires a string member at %s", expr.Start().String())
	}
	return map[string]any{fieldKey: map[string]any{"$ne": fieldValue}}, nil
}

// visitEqualityExpr handles equality operators (==, !=).
// Examples:
//   - status == "active"  → {"status": {"$eq": "active"}}
//   - age != 18           → {"age": {"$ne": 18}}
func (v *Visitor) visitEqualityExpr(expr *filter.BinaryExpr) error {
	fieldKey, err := v.extractFieldKey(expr.Left)
	if err != nil {
		return fmt.Errorf("pinecone: extract field key from '%s' at %s: %w",
			expr.Op.String(), expr.Start().String(), err)
	}

	fieldValue, err := v.extractFieldValue(expr.Right)
	if err != nil {
		return fmt.Errorf("pinecone: extract value from '%s' at %s: %w",
			expr.Op.String(), expr.Start().String(), err)
	}

	switch expr.Op {
	case filter.OpEqual:
		v.result = map[string]any{fieldKey: map[string]any{"$eq": fieldValue}}
	case filter.OpNotEqual:
		v.result = map[string]any{fieldKey: map[string]any{"$ne": fieldValue}}
	default:
		return fmt.Errorf("pinecone: unexpected equality operator '%s' at %s",
			expr.Op.String(), expr.Start().String())
	}

	return nil
}

// visitOrderingExpr handles ordering operators (<, <=, >, >=).
// Examples:
//   - age > 18     → {"age": {"$gt": 18}}
//   - price <= 99  → {"price": {"$lte": 99}}
func (v *Visitor) visitOrderingExpr(expr *filter.BinaryExpr) error {
	fieldKey, err := v.extractFieldKey(expr.Left)
	if err != nil {
		return fmt.Errorf("pinecone: extract field key from '%s' at %s: %w",
			expr.Op.String(), expr.Start().String(), err)
	}

	fieldValue, err := v.extractFieldValue(expr.Right)
	if err != nil {
		return fmt.Errorf("pinecone: extract value from '%s' at %s: %w",
			expr.Op.String(), expr.Start().String(), err)
	}

	switch expr.Op {
	case filter.OpLess:
		v.result = map[string]any{fieldKey: map[string]any{"$lt": fieldValue}}
	case filter.OpLessEqual:
		v.result = map[string]any{fieldKey: map[string]any{"$lte": fieldValue}}
	case filter.OpGreater:
		v.result = map[string]any{fieldKey: map[string]any{"$gt": fieldValue}}
	case filter.OpGreaterEqual:
		v.result = map[string]any{fieldKey: map[string]any{"$gte": fieldValue}}
	default:
		return fmt.Errorf("pinecone: unexpected ordering operator '%s' at %s",
			expr.Op.String(), expr.Start().String())
	}

	return nil
}

// visitInExpr handles the IN operator for membership testing.
// The right operand must be a non-empty list literal.
// Example: status IN ["active", "pending"] → {"status": {"$in": ["active", "pending"]}}
func (v *Visitor) visitInExpr(expr *filter.BinaryExpr) error {
	result, err := v.buildListMembershipExpr(expr, "$in")
	if err != nil {
		return err
	}
	v.result = result
	return nil
}

func (v *Visitor) buildListMembershipExpr(expr *filter.BinaryExpr, operator string) (map[string]any, error) {
	fieldKey, err := v.extractFieldKey(expr.Left)
	if err != nil {
		return nil, fmt.Errorf("pinecone: extract field key from '%s' at %s: %w",
			expr.Op.Name(),
			expr.Start().String(), err)
	}

	listLit, err := filtercompile.RequireListLiteral(expr)
	if err != nil {
		return nil, fmt.Errorf("pinecone: %w", err)
	}

	if err = v.visitListLiteral(listLit); err != nil {
		return nil, err
	}

	return map[string]any{fieldKey: map[string]any{operator: v.currentFieldValue}}, nil
}

// buildNestedExpr converts a sub-expression to a filter map using an isolated visitor instance.
// This ensures that nested logical expressions maintain proper scoping.
func (v *Visitor) buildNestedExpr(expr filter.Expr) (map[string]any, error) {
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
func (v *Visitor) extractFieldKey(expr filter.Expr) (string, error) {
	savedKey := v.currentFieldKey
	v.currentFieldKey = ""

	err := v.visit(expr)

	extracted := v.currentFieldKey
	v.currentFieldKey = savedKey

	if err != nil {
		return "", err
	}
	if extracted == "" {
		return "", fmt.Errorf("pinecone: extract field key from %T expression", expr)
	}

	return extracted, nil
}

// extractFieldValue extracts a value (literal or list) from an expression.
// The visitor's currentFieldValue state is preserved during extraction.
func (v *Visitor) extractFieldValue(expr filter.Expr) (any, error) {
	savedValue := v.currentFieldValue
	v.currentFieldValue = nil

	err := v.visit(expr)

	extracted := v.currentFieldValue
	v.currentFieldValue = savedValue

	if err != nil {
		return nil, err
	}
	if extracted == nil {
		return nil, fmt.Errorf("pinecone: extract value from %T expression", expr)
	}

	return extracted, nil
}

// buildIndexedFieldKey constructs a dot-separated field path from an index expression.
// Transformation examples:
//   - user["name"]                → "user.name"
//   - metadata["tags"][0]         → "metadata.tags.0"
//   - config["db"]["host"]        → "config.db.host"
func (v *Visitor) buildIndexedFieldKey(expr *filter.IndexExpr) (string, error) {
	var parts []string

	current := expr
	for {
		key, err := filtercompile.LiteralAsKey(current.Index)
		if err != nil {
			return "", fmt.Errorf("pinecone: %w", err)
		}
		parts = append([]string{key}, parts...)

		switch left := current.Left.(type) {
		case *filter.IndexExpr:
			current = left
		case *filter.Ident:
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
func (v *Visitor) literalToValue(lit *filter.Literal) (any, error) {
	if lit.IsString() {
		return lit.AsString()
	}
	if lit.IsNumber() {
		return filtercompile.NumberToFloat64(lit)
	}
	if lit.IsBool() {
		return lit.AsBool()
	}
	return nil, fmt.Errorf("pinecone: unsupported literal type '%s'", lit.Kind)
}

// ToFilter converts an AST filter expression into a Pinecone MetadataFilter (*structpb.Struct).
//
// This is the main entry point for converting filter expressions written in
// the Lynx filter DSL into Pinecone's native metadata filter format.
//
// Supported operations:
//   - Logical:    AND, OR, NOT (lowered through inverse operators)
//   - Equality:   ==, !=
//   - Ordering:   <, <=, >, >=
//   - Membership: IN, HAS
//
// Note: The LIKE operator is not supported by Pinecone metadata filters.
//
// Conversion semantics:
//   - AND: {"$and": [left, right]}
//   - OR:  {"$or":  [left, right]}
//   - NOT: lowered with inverse comparisons, $nin, and De Morgan rewrites
//   - ==:  {"field": {"$eq": value}}
//   - !=:  {"field": {"$ne": value}}
//   - <:   {"field": {"$lt": value}}
//   - <=:  {"field": {"$lte": value}}
//   - >:   {"field": {"$gt": value}}
//   - >=:  {"field": {"$gte": value}}
//   - IN:  {"field": {"$in": [values...]}}
//   - HAS: {"field": {"$eq": value}} for list-valued string metadata
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
func ToFilter(expr filter.Predicate) (*structpb.Struct, error) {
	conv := NewVisitor()
	if err := conv.Visit(expr); err != nil {
		return nil, err
	}
	return conv.Filter()
}
