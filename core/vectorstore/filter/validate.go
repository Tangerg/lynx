package filter

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"
)

func validateRoot(expr Predicate) error {
	if isNilExpr(expr) {
		return fmt.Errorf("filter: expression is nil")
	}
	return validateExpr(expr)
}

func validateExpr(expr Expr) error {
	if isNilExpr(expr) {
		return fmt.Errorf("filter: expression is nil")
	}

	switch node := expr.(type) {
	case *Ident:
		return validateIdent(node)
	case *Literal:
		return validateLiteral(node)
	case *ListLiteral:
		return validateList(node)
	case *UnaryExpr:
		return validateUnary(node)
	case *BinaryExpr:
		return validateBinary(node)
	case *IndexExpr:
		return validateIndex(node)
	default:
		return fmt.Errorf("filter: unsupported expression %T at %s", expr, expr.Start())
	}
}

func validateIdent(ident *Ident) error {
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

func validateLiteral(literal *Literal) error {
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

func validateList(list *ListLiteral) error {
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
	if err := validateLiteral(first); err != nil {
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
		if err := validateLiteral(value); err != nil {
			return err
		}
	}
	return nil
}

func validateUnary(unary *UnaryExpr) error {
	if unary == nil {
		return fmt.Errorf("filter: unary expression is nil")
	}
	if unary.Op != OpNot {
		return fmt.Errorf("filter: invalid unary operator %q at %s", unary.Op, unary.Start())
	}
	if isNilExpr(unary.Right) {
		return fmt.Errorf("filter: NOT operand is nil at %s", unary.Start())
	}
	if !isPredicate(unary.Right) {
		return fmt.Errorf("filter: NOT requires a predicate, got %T at %s", unary.Right, unary.Start())
	}
	return validateExpr(unary.Right)
}

func validateBinary(binary *BinaryExpr) error {
	if binary == nil {
		return fmt.Errorf("filter: binary expression is nil")
	}
	if isNilExpr(binary.Left) {
		return fmt.Errorf("filter: %s left operand is nil at %s", binary.Op.Name(), binary.Start())
	}
	if isNilExpr(binary.Right) {
		return fmt.Errorf("filter: %s right operand is nil at %s", binary.Op.Name(), binary.Start())
	}

	switch {
	case binary.Op.IsLogicalOperator():
		return validateLogical(binary)
	case binary.Op.IsEqualityOperator():
		return validateComparison(binary, false)
	case binary.Op.IsOrderingOperator():
		return validateComparison(binary, true)
	case binary.Op == OpIn:
		return validateMembership(binary)
	case binary.Op == OpLike:
		return validateLike(binary)
	case binary.Op == OpIs:
		return validateNullTest(binary)
	default:
		return fmt.Errorf("filter: invalid binary operator %q at %s", binary.Op, binary.Start())
	}
}

func validateLogical(binary *BinaryExpr) error {
	if !isPredicate(binary.Left) {
		return fmt.Errorf("filter: %s left operand must be a predicate, got %T at %s", binary.Op.Name(), binary.Left, binary.Start())
	}
	if !isPredicate(binary.Right) {
		return fmt.Errorf("filter: %s right operand must be a predicate, got %T at %s", binary.Op.Name(), binary.Right, binary.Start())
	}
	if err := validateExpr(binary.Left); err != nil {
		return err
	}
	return validateExpr(binary.Right)
}

func validateComparison(binary *BinaryExpr, numeric bool) error {
	if err := validateSelector(binary.Left); err != nil {
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
	return validateLiteral(literal)
}

func validateMembership(binary *BinaryExpr) error {
	if err := validateSelector(binary.Left); err != nil {
		return fmt.Errorf("filter: IN left operand: %w", err)
	}
	list, ok := binary.Right.(*ListLiteral)
	if !ok || list == nil {
		return fmt.Errorf("filter: IN right operand must be a list, got %T at %s", binary.Right, binary.Start())
	}
	return validateList(list)
}

func validateLike(binary *BinaryExpr) error {
	if err := validateSelector(binary.Left); err != nil {
		return fmt.Errorf("filter: LIKE left operand: %w", err)
	}
	literal, ok := binary.Right.(*Literal)
	if !ok || literal == nil || !literal.IsString() {
		return fmt.Errorf("filter: LIKE right operand must be a string literal, got %T at %s", binary.Right, binary.Start())
	}
	return validateLiteral(literal)
}

func validateNullTest(binary *BinaryExpr) error {
	if err := validateSelector(binary.Left); err != nil {
		return fmt.Errorf("filter: IS left operand: %w", err)
	}
	literal, ok := binary.Right.(*Literal)
	if !ok || literal == nil || !literal.IsNull() {
		return fmt.Errorf("filter: IS right operand must be NULL, got %T at %s", binary.Right, binary.Start())
	}
	return validateLiteral(literal)
}

func validateSelector(expr Expr) error {
	if isNilExpr(expr) {
		return fmt.Errorf("selector is nil")
	}
	switch selector := expr.(type) {
	case *Ident:
		return validateIdent(selector)
	case *IndexExpr:
		return validateIndex(selector)
	default:
		return fmt.Errorf("expected identifier or index, got %T at %s", expr, expr.Start())
	}
}

func validateIndex(index *IndexExpr) error {
	if index == nil {
		return fmt.Errorf("filter: index expression is nil")
	}
	if err := validateSelector(index.Left); err != nil {
		return fmt.Errorf("filter: index base: %w", err)
	}
	if index.Index == nil {
		return fmt.Errorf("filter: index is nil at %s", index.Start())
	}
	if !index.Index.IsString() && !index.Index.IsNumber() {
		return fmt.Errorf("filter: index must be a string or number, got %s at %s", index.Index.Kind, index.Index.Start())
	}
	if err := validateLiteral(index.Index); err != nil {
		return err
	}
	if index.Index.IsNumber() {
		if _, err := numericIndexValue(index.Index.Value); err != nil {
			return fmt.Errorf("filter: numeric index must be a non-negative integer, got %q at %s", index.Index.Value, index.Index.Start())
		}
	}
	return nil
}

func numericIndexValue(value string) (uint64, error) {
	if !strings.ContainsAny(value, ".eE") {
		if strings.HasPrefix(value, "-") {
			number, err := strconv.ParseInt(value, 10, 64)
			if err != nil || number < 0 {
				return 0, fmt.Errorf("invalid index")
			}
			return uint64(number), nil
		}
		return strconv.ParseUint(value, 10, 63)
	}

	number, err := strconv.ParseFloat(value, 64)
	limit := math.Ldexp(1, 63)
	if err != nil || math.IsNaN(number) || math.IsInf(number, 0) ||
		number < 0 || number >= limit || math.Trunc(number) != number {
		return 0, fmt.Errorf("invalid index")
	}
	return uint64(number), nil
}

func isPredicate(expr Expr) bool {
	_, ok := expr.(Predicate)
	return ok && !isNilExpr(expr)
}
