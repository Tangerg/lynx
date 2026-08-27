package filter

import (
	"errors"
	"fmt"
	"unicode"

	"github.com/samber/lo"
)

// analyzer owns semantic validation for the immutable filter tree.
type analyzer struct{}

func (a *analyzer) analyze(expr Predicate) error {
	if lo.IsNil(expr) {
		return errors.New("filter: expression is nil")
	}
	return a.visit(expr)
}

func (a *analyzer) visit(expr Expr) error {
	if lo.IsNil(expr) {
		return errors.New("filter: expression is nil")
	}
	switch node := expr.(type) {
	case *Ident:
		return a.visitIdent(node)
	case *Literal:
		return a.visitLiteral(node)
	case *ListLiteral:
		return a.visitList(node)
	case *UnaryExpr:
		return a.visitUnary(node)
	case *BinaryExpr:
		return a.visitBinary(node)
	case *IndexExpr:
		return a.visitIndex(node)
	default:
		return fmt.Errorf("filter: unsupported expression %T at %s", expr, expr.Start())
	}
}

func (a *analyzer) visitIdent(ident *Ident) error {
	if ident == nil {
		return errors.New("filter: identifier is nil")
	}
	if !validIdentifier(ident.name) {
		return fmt.Errorf("filter: invalid identifier %q at %s", ident.name, ident.Start())
	}
	return nil
}

func validIdentifier(value string) bool {
	if keywordKind(value) != tokenIdent {
		return false
	}
	first := true
	for _, r := range value {
		if first {
			if !unicode.IsLetter(r) {
				return false
			}
			first = false
			continue
		}
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
			return false
		}
	}
	return !first
}

func (a *analyzer) visitLiteral(literal *Literal) error {
	if literal == nil {
		return errors.New("filter: literal is nil")
	}

	switch literal.kind {
	case LiteralString:
		return nil
	case LiteralNull:
		if literal.text != string(LiteralNull) {
			return fmt.Errorf("filter: invalid NULL literal %q at %s", literal.text, literal.Start())
		}
		return nil
	case LiteralNumber:
		canonical, err := canonicalNumber(literal.text)
		if err != nil || canonical != literal.text {
			return fmt.Errorf("filter: invalid number literal %q at %s", literal.text, literal.Start())
		}
		return nil
	case LiteralBool:
		if literal.text != "true" && literal.text != "false" {
			return fmt.Errorf("filter: invalid boolean literal %q at %s", literal.text, literal.Start())
		}
		return nil
	default:
		return fmt.Errorf("filter: invalid literal kind %q at %s", literal.kind, literal.Start())
	}
}

func (a *analyzer) visitList(list *ListLiteral) error {
	if list == nil {
		return errors.New("filter: list literal is nil")
	}
	if len(list.values) == 0 {
		return fmt.Errorf("filter: list literal cannot be empty at %s", list.Start())
	}

	first := list.values[0]
	if first == nil {
		return fmt.Errorf("filter: list element 0 is nil at %s", list.Start())
	}
	if first.IsNull() {
		return fmt.Errorf("filter: list elements cannot be NULL at %s", first.Start())
	}
	if err := a.visitLiteral(first); err != nil {
		return err
	}

	for index, value := range list.values[1:] {
		if value == nil {
			return fmt.Errorf("filter: list element %d is nil at %s", index+1, list.Start())
		}
		if value.kind != first.kind {
			return fmt.Errorf(
				"filter: list element %d has kind %s, expected %s at %s",
				index+1, value.kind, first.kind, value.Start(),
			)
		}
		if err := a.visitLiteral(value); err != nil {
			return err
		}
	}
	return nil
}

func (a *analyzer) visitUnary(unary *UnaryExpr) error {
	if unary == nil {
		return errors.New("filter: unary expression is nil")
	}
	if !unary.operator.IsUnaryOperator() {
		return fmt.Errorf("filter: invalid unary operator %q at %s", unary.operator, unary.Start())
	}
	if lo.IsNil(unary.right) {
		return fmt.Errorf("filter: NOT operand is nil at %s", unary.Start())
	}
	if !a.isPredicate(unary.right) {
		return fmt.Errorf("filter: NOT requires a predicate, got %T at %s", unary.right, unary.Start())
	}
	return a.visit(unary.right)
}

func (a *analyzer) visitBinary(binary *BinaryExpr) error {
	if binary == nil {
		return errors.New("filter: binary expression is nil")
	}
	if !binary.operator.IsBinaryOperator() {
		return fmt.Errorf("filter: invalid binary operator %q at %s", binary.operator, binary.Start())
	}
	if lo.IsNil(binary.left) {
		return fmt.Errorf("filter: %s left operand is nil at %s", binary.operator.Name(), binary.Start())
	}
	if lo.IsNil(binary.right) {
		return fmt.Errorf("filter: %s right operand is nil at %s", binary.operator.Name(), binary.Start())
	}

	switch {
	case binary.operator.IsLogicalOperator():
		return a.visitLogical(binary)
	case binary.operator.IsEqualityOperator():
		return a.visitComparison(binary, false)
	case binary.operator.IsOrderingOperator():
		return a.visitComparison(binary, true)
	case binary.operator == OpIn:
		return a.visitMembership(binary)
	case binary.operator == OpHas:
		return a.visitCollectionMembership(binary)
	case binary.operator == OpLike:
		return a.visitLike(binary)
	case binary.operator == OpIs:
		return a.visitNullTest(binary)
	default:
		return fmt.Errorf("filter: unsupported binary operator %q at %s", binary.operator, binary.Start())
	}
}

func (a *analyzer) visitCollectionMembership(binary *BinaryExpr) error {
	if err := a.visitSelector(binary.left); err != nil {
		return fmt.Errorf("filter: HAS left operand: %w", err)
	}
	literal, ok := binary.right.(*Literal)
	if !ok || literal == nil {
		return fmt.Errorf("filter: HAS right operand must be a literal, got %T at %s", binary.right, binary.Start())
	}
	if literal.IsNull() {
		return fmt.Errorf("filter: HAS cannot test NULL at %s", binary.Start())
	}
	return a.visitLiteral(literal)
}

func (a *analyzer) visitLogical(binary *BinaryExpr) error {
	if !a.isPredicate(binary.left) {
		return fmt.Errorf("filter: %s left operand must be a predicate, got %T at %s", binary.operator.Name(), binary.left, binary.Start())
	}
	if !a.isPredicate(binary.right) {
		return fmt.Errorf("filter: %s right operand must be a predicate, got %T at %s", binary.operator.Name(), binary.right, binary.Start())
	}
	if err := a.visit(binary.left); err != nil {
		return err
	}
	return a.visit(binary.right)
}

func (a *analyzer) visitComparison(binary *BinaryExpr, numeric bool) error {
	if err := a.visitSelector(binary.left); err != nil {
		return fmt.Errorf("filter: %s left operand: %w", binary.operator.Name(), err)
	}
	literal, ok := binary.right.(*Literal)
	if !ok || literal == nil {
		return fmt.Errorf("filter: %s right operand must be a literal, got %T at %s", binary.operator.Name(), binary.right, binary.Start())
	}
	if literal.IsNull() {
		return fmt.Errorf("filter: %s cannot compare NULL; use IS NULL at %s", binary.operator.Name(), binary.Start())
	}
	if numeric && !literal.IsNumber() {
		return fmt.Errorf("filter: %s right operand must be numeric, got %s at %s", binary.operator.Name(), literal.kind, literal.Start())
	}
	return a.visitLiteral(literal)
}

func (a *analyzer) visitMembership(binary *BinaryExpr) error {
	if err := a.visitSelector(binary.left); err != nil {
		return fmt.Errorf("filter: IN left operand: %w", err)
	}
	list, ok := binary.right.(*ListLiteral)
	if !ok || list == nil {
		return fmt.Errorf("filter: IN right operand must be a list, got %T at %s", binary.right, binary.Start())
	}
	return a.visitList(list)
}

func (a *analyzer) visitLike(binary *BinaryExpr) error {
	if err := a.visitSelector(binary.left); err != nil {
		return fmt.Errorf("filter: LIKE left operand: %w", err)
	}
	literal, ok := binary.right.(*Literal)
	if !ok || literal == nil || !literal.IsString() {
		return fmt.Errorf("filter: LIKE right operand must be a string literal, got %T at %s", binary.right, binary.Start())
	}
	return a.visitLiteral(literal)
}

func (a *analyzer) visitNullTest(binary *BinaryExpr) error {
	if err := a.visitSelector(binary.left); err != nil {
		return fmt.Errorf("filter: IS left operand: %w", err)
	}
	literal, ok := binary.right.(*Literal)
	if !ok || literal == nil || !literal.IsNull() {
		return fmt.Errorf("filter: IS right operand must be NULL, got %T at %s", binary.right, binary.Start())
	}
	return a.visitLiteral(literal)
}

func (a *analyzer) visitSelector(expr Expr) error {
	if lo.IsNil(expr) {
		return errors.New("selector is nil")
	}
	switch expr.(type) {
	case *Ident, *IndexExpr:
		return a.visit(expr)
	default:
		return fmt.Errorf("expected identifier or index, got %T at %s", expr, expr.Start())
	}
}

func (a *analyzer) visitIndex(index *IndexExpr) error {
	if index == nil {
		return errors.New("filter: index expression is nil")
	}
	if err := a.visitSelector(index.left); err != nil {
		return fmt.Errorf("filter: index base: %w", err)
	}
	if index.index == nil {
		return fmt.Errorf("filter: index is nil at %s", index.Start())
	}
	if !index.index.IsString() && !index.index.IsNumber() {
		return fmt.Errorf("filter: index must be a string or number, got %s at %s", index.index.Kind(), index.index.Start())
	}
	if err := a.visitLiteral(index.index); err != nil {
		return err
	}
	if index.index.IsNumber() && !index.index.isIntegerIndex() {
		return fmt.Errorf("filter: numeric index must be a non-negative integer, got %q at %s", index.index.Text(), index.index.Start())
	}
	return nil
}

func (*analyzer) isPredicate(expr Expr) bool {
	_, ok := expr.(Predicate)
	return ok && !lo.IsNil(expr)
}
