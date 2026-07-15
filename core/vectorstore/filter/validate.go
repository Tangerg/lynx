package filter

import (
	"fmt"
	"math/big"
	"unicode"
)

// analyzer owns semantic validation for the filter tree. It stays private so
// Validate remains the only validation policy exposed by the package.
type analyzer struct {
	active map[Expr]struct{}
}

func (a *analyzer) analyze(expr Predicate) error {
	if isNilExpr(expr) {
		return fmt.Errorf("filter: expression is nil")
	}
	a.active = make(map[Expr]struct{})
	return a.visit(expr)
}

func (a *analyzer) visit(expr Expr) error {
	if isNilExpr(expr) {
		return fmt.Errorf("filter: expression is nil")
	}
	if _, exists := a.active[expr]; exists {
		return fmt.Errorf("filter: expression cycle involving %T", expr)
	}
	a.active[expr] = struct{}{}
	defer delete(a.active, expr)

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
		return fmt.Errorf("filter: identifier is nil")
	}
	if !validIdentifier(ident.Value) {
		return fmt.Errorf("filter: invalid identifier %q at %s", ident.Value, ident.Start())
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
		return fmt.Errorf("filter: literal is nil")
	}

	switch literal.Kind {
	case LiteralString:
		return nil
	case LiteralNull:
		if literal.Value != "null" {
			return fmt.Errorf("filter: invalid NULL literal %q at %s", literal.Value, literal.Start())
		}
		return nil
	case LiteralNumber:
		canonical, err := canonicalNumber(literal.Value)
		if err != nil || canonical != literal.Value {
			return fmt.Errorf("filter: invalid number literal %q at %s", literal.Value, literal.Start())
		}
		return nil
	case LiteralBool:
		if literal.Value != "true" && literal.Value != "false" {
			return fmt.Errorf("filter: invalid boolean literal %q at %s", literal.Value, literal.Start())
		}
		return nil
	default:
		return fmt.Errorf("filter: invalid literal kind %q at %s", literal.Kind, literal.Start())
	}
}

func (a *analyzer) visitList(list *ListLiteral) error {
	if list == nil {
		return fmt.Errorf("filter: list literal is nil")
	}
	if len(list.Values) == 0 {
		return fmt.Errorf("filter: list literal cannot be empty at %s", list.Start())
	}

	first := list.Values[0]
	if first == nil {
		return fmt.Errorf("filter: list element 0 is nil at %s", list.Start())
	}
	if first.IsNull() {
		return fmt.Errorf("filter: list elements cannot be NULL at %s", first.Start())
	}
	if err := a.visitLiteral(first); err != nil {
		return err
	}

	for index, value := range list.Values[1:] {
		if value == nil {
			return fmt.Errorf("filter: list element %d is nil at %s", index+1, list.Start())
		}
		if value.Kind != first.Kind {
			return fmt.Errorf(
				"filter: list element %d has kind %s, expected %s at %s",
				index+1, value.Kind, first.Kind, value.Start(),
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
		return fmt.Errorf("filter: unary expression is nil")
	}
	if !unary.Op.IsUnaryOperator() {
		return fmt.Errorf("filter: invalid unary operator %q at %s", unary.Op, unary.Start())
	}
	if isNilExpr(unary.Right) {
		return fmt.Errorf("filter: NOT operand is nil at %s", unary.Start())
	}
	if !isPredicate(unary.Right) {
		return fmt.Errorf("filter: NOT requires a predicate, got %T at %s", unary.Right, unary.Start())
	}
	return a.visit(unary.Right)
}

func (a *analyzer) visitBinary(binary *BinaryExpr) error {
	if binary == nil {
		return fmt.Errorf("filter: binary expression is nil")
	}
	if !binary.Op.IsBinaryOperator() {
		return fmt.Errorf("filter: invalid binary operator %q at %s", binary.Op, binary.Start())
	}
	if isNilExpr(binary.Left) {
		return fmt.Errorf("filter: %s left operand is nil at %s", binary.Op.Name(), binary.Start())
	}
	if isNilExpr(binary.Right) {
		return fmt.Errorf("filter: %s right operand is nil at %s", binary.Op.Name(), binary.Start())
	}

	switch {
	case binary.Op.IsLogicalOperator():
		return a.visitLogical(binary)
	case binary.Op.IsEqualityOperator():
		return a.visitComparison(binary, false)
	case binary.Op.IsOrderingOperator():
		return a.visitComparison(binary, true)
	case binary.Op == OpIn:
		return a.visitMembership(binary)
	case binary.Op == OpLike:
		return a.visitLike(binary)
	case binary.Op == OpIs:
		return a.visitNullTest(binary)
	default:
		return fmt.Errorf("filter: unsupported binary operator %q at %s", binary.Op, binary.Start())
	}
}

func (a *analyzer) visitLogical(binary *BinaryExpr) error {
	if !isPredicate(binary.Left) {
		return fmt.Errorf("filter: %s left operand must be a predicate, got %T at %s", binary.Op.Name(), binary.Left, binary.Start())
	}
	if !isPredicate(binary.Right) {
		return fmt.Errorf("filter: %s right operand must be a predicate, got %T at %s", binary.Op.Name(), binary.Right, binary.Start())
	}
	if err := a.visit(binary.Left); err != nil {
		return err
	}
	return a.visit(binary.Right)
}

func (a *analyzer) visitComparison(binary *BinaryExpr, numeric bool) error {
	if err := a.visitSelector(binary.Left); err != nil {
		return fmt.Errorf("filter: %s left operand: %w", binary.Op.Name(), err)
	}
	literal, ok := binary.Right.(*Literal)
	if !ok || literal == nil {
		return fmt.Errorf("filter: %s right operand must be a literal, got %T at %s", binary.Op.Name(), binary.Right, binary.Start())
	}
	if literal.IsNull() {
		return fmt.Errorf("filter: %s cannot compare NULL; use IS NULL at %s", binary.Op.Name(), binary.Start())
	}
	if numeric && !literal.IsNumber() {
		return fmt.Errorf("filter: %s right operand must be numeric, got %s at %s", binary.Op.Name(), literal.Kind, literal.Start())
	}
	return a.visitLiteral(literal)
}

func (a *analyzer) visitMembership(binary *BinaryExpr) error {
	if err := a.visitSelector(binary.Left); err != nil {
		return fmt.Errorf("filter: IN left operand: %w", err)
	}
	list, ok := binary.Right.(*ListLiteral)
	if !ok || list == nil {
		return fmt.Errorf("filter: IN right operand must be a list, got %T at %s", binary.Right, binary.Start())
	}
	return a.visitList(list)
}

func (a *analyzer) visitLike(binary *BinaryExpr) error {
	if err := a.visitSelector(binary.Left); err != nil {
		return fmt.Errorf("filter: LIKE left operand: %w", err)
	}
	literal, ok := binary.Right.(*Literal)
	if !ok || literal == nil || !literal.IsString() {
		return fmt.Errorf("filter: LIKE right operand must be a string literal, got %T at %s", binary.Right, binary.Start())
	}
	return a.visitLiteral(literal)
}

func (a *analyzer) visitNullTest(binary *BinaryExpr) error {
	if err := a.visitSelector(binary.Left); err != nil {
		return fmt.Errorf("filter: IS left operand: %w", err)
	}
	literal, ok := binary.Right.(*Literal)
	if !ok || literal == nil || !literal.IsNull() {
		return fmt.Errorf("filter: IS right operand must be NULL, got %T at %s", binary.Right, binary.Start())
	}
	return a.visitLiteral(literal)
}

func (a *analyzer) visitSelector(expr Expr) error {
	if isNilExpr(expr) {
		return fmt.Errorf("selector is nil")
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
		return fmt.Errorf("filter: index expression is nil")
	}
	if err := a.visitSelector(index.Left); err != nil {
		return fmt.Errorf("filter: index base: %w", err)
	}
	if index.Index == nil {
		return fmt.Errorf("filter: index is nil at %s", index.Start())
	}
	if !index.Index.IsString() && !index.Index.IsNumber() {
		return fmt.Errorf("filter: index must be a string or number, got %s at %s", index.Index.Kind, index.Index.Start())
	}
	if err := a.visitLiteral(index.Index); err != nil {
		return err
	}
	if index.Index.IsNumber() && !index.Index.isIntegerIndex() {
		return fmt.Errorf("filter: numeric index must be a non-negative integer, got %q at %s", index.Index.Value, index.Index.Start())
	}
	return nil
}

func (l *Literal) isIntegerIndex() bool {
	if !l.IsNumber() {
		return false
	}
	number, ok := new(big.Rat).SetString(l.Value)
	return ok && number.Sign() >= 0 && number.IsInt() && number.Num().IsInt64()
}

func isPredicate(expr Expr) bool {
	_, ok := expr.(Predicate)
	return ok && !isNilExpr(expr)
}
