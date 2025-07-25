package visitors

import (
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/ai/vectorstore/filter/ast"
	"github.com/Tangerg/lynx/ai/vectorstore/filter/token"
)

// Analyzer performs semantic analysis on AST expressions to validate syntax correctness.
// It implements the visitor pattern to traverse and analyze different expression types.
type Analyzer struct {
	err error
}

// Error returns the last error encountered during analysis.
func (a *Analyzer) Error() error {
	return a.err
}

// Visit implements the Visitor interface by analyzing the given expression.
// Returns nil to stop traversal after analysis is complete.
func (a *Analyzer) Visit(expr ast.Expr) ast.Visitor {
	a.err = a.analyze(expr)
	return nil
}

// analyze dispatches analysis to specific methods based on expression type.
// Returns error if expression is nil or unsupported.
func (a *Analyzer) analyze(expr ast.Expr) error {
	if expr == nil {
		return errors.New("expression cannot be nil")
	}

	switch typedExpr := expr.(type) {
	case *ast.Ident:
		return a.analyzeIdent(typedExpr)
	case *ast.Literal:
		return a.analyzeLiteral(typedExpr)
	case *ast.ListLiteral:
		return a.analyzeListLiteral(typedExpr)
	case *ast.UnaryExpr:
		return a.analyzeUnaryExpr(typedExpr)
	case *ast.BinaryExpr:
		return a.analyzeBinaryExpr(typedExpr)
	case *ast.IndexExpr:
		return a.analyzeIndexExpr(typedExpr)
	default:
		return fmt.Errorf("unsupported expression type: %T", typedExpr)
	}
}

// analyzeIdent validates identifier tokens and ensures they are not reserved keywords.
func (a *Analyzer) analyzeIdent(expr *ast.Ident) error {
	if !expr.Token.Kind.Is(token.IDENT) {
		return fmt.Errorf("identifier token must be IDENT type, but got %s", expr.Token.Kind.String())
	}

	if token.IsKeyword(expr.Value) {
		return fmt.Errorf("identifier '%s' cannot be a reserved keyword", expr.Value)
	}

	return nil
}

// analyzeLiteral validates literal expressions including strings, numbers, and booleans.
// Ensures numeric and boolean literals can be properly parsed.
func (a *Analyzer) analyzeLiteral(expr *ast.Literal) error {
	if expr.IsString() {
		return nil
	}

	if expr.IsNumber() {
		if _, err := expr.AsNumber(); err != nil {
			return fmt.Errorf("invalid number literal: %w", err)
		}
		return nil
	}

	if expr.IsBool() {
		if _, err := expr.AsBool(); err != nil {
			return fmt.Errorf("invalid bool literal: %w", err)
		}
		return nil
	}

	return fmt.Errorf("unsupported literal type: %s", expr.Token.Kind.String())
}

// analyzeListLiteral validates list literals ensuring non-empty lists with uniform element types.
// Recursively analyzes each list element for correctness.
func (a *Analyzer) analyzeListLiteral(expr *ast.ListLiteral) error {
	if len(expr.Values) == 0 {
		return errors.New("list literal cannot be empty")
	}

	firstLiteral := expr.Values[0]
	firstType := firstLiteral.Token.Kind.Name()

	for i, literal := range expr.Values {
		if !firstLiteral.IsSameKind(literal) {
			currentType := literal.Token.Kind.Name()
			return fmt.Errorf("list element at index %d has type %s, but expected %s (all elements must have the same type)",
				i, currentType, firstType)
		}

		err := a.analyze(literal)
		if err != nil {
			return fmt.Errorf("error in list element at index %d: %w", i, err)
		}
	}

	return nil
}

// analyzeUnaryExpr validates unary expressions by analyzing the operator type and the right operand.
func (a *Analyzer) analyzeUnaryExpr(expr *ast.UnaryExpr) error {
	if !expr.Op.Kind.IsUnaryOperator() {
		return fmt.Errorf("unsupported unary operator: %s", expr.Op.Kind.String())
	}
	err := a.analyze(expr.Right)
	if err != nil {
		return fmt.Errorf("error in unary expression operand: %w", err)
	}
	return nil
}

// analyzeBinaryExpr validates binary expressions based on operator type.
// Routes analysis to specific methods for logical, equality, ordering, IN, and LIKE operators.
func (a *Analyzer) analyzeBinaryExpr(expr *ast.BinaryExpr) error {
	opName := expr.Op.Kind.String()

	// Logical operators (AND, OR)
	if expr.Op.Kind.IsLogicalOperator() {
		return a.analyzeLogicalOperation(expr, opName)
	}

	// For other operators, left operand must be an identifier
	if _, ok := expr.Left.(*ast.Ident); !ok {
		return fmt.Errorf("%s operator requires an identifier on the left side, but got %T", opName, expr.Left)
	}

	// Equality operators (EQ, NE)
	if expr.Op.Kind.IsEqualityOperator() {
		return a.analyzeEqualityOperation(expr, opName)
	}

	// Comparison operators (LT, LE, GT, GE)
	if expr.Op.Kind.IsOrderingOperator() {
		return a.analyzeOrderingOperation(expr, opName)
	}

	// IN operator
	if expr.Op.Kind.Is(token.IN) {
		return a.analyzeInOperation(expr)
	}

	// LIKE operator
	if expr.Op.Kind.Is(token.LIKE) {
		return a.analyzeLikeOperation(expr)
	}

	return fmt.Errorf("unsupported binary operator: %s", expr.Op.Kind.String())
}

// analyzeLogicalOperation validates logical operators (AND, OR) requiring computed expressions on both sides.
// Recursively analyzes both operands for semantic correctness.
func (a *Analyzer) analyzeLogicalOperation(expr *ast.BinaryExpr, opName string) error {
	_, leftOk := expr.Left.(ast.ComputedExpr)
	_, rightOk := expr.Right.(ast.ComputedExpr)

	if !leftOk {
		return fmt.Errorf("%s operator requires a computed expression on the left side, but got %T", opName, expr.Left)
	}
	if !rightOk {
		return fmt.Errorf("%s operator requires a computed expression on the right side, but got %T", opName, expr.Right)
	}

	// Recursively analyze left and right operands
	err := a.analyze(expr.Left)
	if err != nil {
		return fmt.Errorf("error in %s operator left operand: %w", opName, err)
	}

	err = a.analyze(expr.Right)
	if err != nil {
		return fmt.Errorf("error in %s operator right operand: %w", opName, err)
	}

	return nil
}

// analyzeEqualityOperation validates equality operators (==, !=) with literal values on the right side.
func (a *Analyzer) analyzeEqualityOperation(expr *ast.BinaryExpr, opName string) error {
	if _, ok := expr.Right.(*ast.Literal); !ok {
		return fmt.Errorf("%s operator requires a literal value on the right side, but got %T", opName, expr.Right)
	}

	// Analyze left and right operands
	err := a.analyze(expr.Left)
	if err != nil {
		return fmt.Errorf("error in %s operator left operand: %w", opName, err)
	}

	err = a.analyze(expr.Right)
	if err != nil {
		return fmt.Errorf("error in %s operator right operand: %w", opName, err)
	}

	return nil
}

// analyzeOrderingOperation validates ordering operators (<, <=, >, >=) requiring numeric literals on the right side.
func (a *Analyzer) analyzeOrderingOperation(expr *ast.BinaryExpr, opName string) error {
	literal, ok := expr.Right.(*ast.Literal)
	if !ok {
		return fmt.Errorf("%s operator requires a literal value on the right side, but got %T", opName, expr.Right)
	}

	if !literal.IsNumber() {
		return fmt.Errorf("%s operator requires a numeric literal on the right side, but got %s", opName, literal.Token.Kind.String())
	}

	// Analyze left and right operands
	err := a.analyze(expr.Left)
	if err != nil {
		return fmt.Errorf("error in %s operator left operand: %w", opName, err)
	}

	err = a.analyze(expr.Right)
	if err != nil {
		return fmt.Errorf("error in %s operator right operand: %w", opName, err)
	}

	return nil
}

// analyzeInOperation validates IN operators requiring list literals on the right side.
func (a *Analyzer) analyzeInOperation(expr *ast.BinaryExpr) error {
	if _, ok := expr.Right.(*ast.ListLiteral); !ok {
		return fmt.Errorf("IN operator requires a list literal on the right side, but got %T", expr.Right)
	}

	// Analyze left and right operands
	err := a.analyze(expr.Left)
	if err != nil {
		return fmt.Errorf("error in IN operator left operand: %w", err)
	}

	err = a.analyze(expr.Right)
	if err != nil {
		return fmt.Errorf("error in IN operator right operand: %w", err)
	}

	return nil
}

// analyzeLikeOperation validates LIKE operators requiring string literals on the right side.
func (a *Analyzer) analyzeLikeOperation(expr *ast.BinaryExpr) error {
	literal, ok := expr.Right.(*ast.Literal)
	if !ok {
		return fmt.Errorf("LIKE operator requires a literal value on the right side, but got %T", expr.Right)
	}

	if !literal.IsString() {
		return fmt.Errorf("LIKE operator requires a string literal on the right side, but got %s", literal.Token.Kind.String())
	}

	// Analyze left and right operands
	err := a.analyze(expr.Left)
	if err != nil {
		return fmt.Errorf("error in LIKE operator left operand: %w", err)
	}

	err = a.analyze(expr.Right)
	if err != nil {
		return fmt.Errorf("error in LIKE operator right operand: %w", err)
	}

	return nil
}

// analyzeIndexExpr validates index expressions with identifiers or nested indices on the left.
// Ensures index values are numeric or string literals.
func (a *Analyzer) analyzeIndexExpr(expr *ast.IndexExpr) error {
	// Analyze left side expression
	switch typedLeft := expr.Left.(type) {
	case *ast.Ident:
		err := a.analyze(typedLeft)
		if err != nil {
			return fmt.Errorf("error in index expression identifier: %w", err)
		}
	case *ast.IndexExpr:
		err := a.analyze(typedLeft)
		if err != nil {
			return fmt.Errorf("error in nested index expression: %w", err)
		}
	default:
		return fmt.Errorf("index expression requires an identifier or another index expression on the left side, but got %T", typedLeft)
	}

	// Check index type
	if expr.Index.IsNumber() || expr.Index.IsString() {
		err := a.analyze(expr.Index)
		if err != nil {
			return fmt.Errorf("error in index expression index: %w", err)
		}
		return nil
	}

	return fmt.Errorf("index must be a number or string literal, but got %s", expr.Index.Token.Kind.String())
}
