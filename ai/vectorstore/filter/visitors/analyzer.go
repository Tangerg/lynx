package visitors

import (
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/ai/vectorstore/filter/ast"
	"github.com/Tangerg/lynx/ai/vectorstore/filter/token"
)

// Analyzer performs semantic analysis on AST expressions using the visitor pattern.
// It validates syntax correctness and maintains error state throughout analysis.
type Analyzer struct {
	err error
}

func NewAnalyzer() *Analyzer {
	return &Analyzer{}
}

// Error returns the last error encountered during analysis.
func (a *Analyzer) Error() error {
	return a.err
}

// Visit implements the Visitor interface and analyzes the given expression.
// Returns nil to stop traversal after analysis completion.
func (a *Analyzer) Visit(expr ast.Expr) ast.Visitor {
	a.err = a.analyze(expr)
	return nil
}

// analyze dispatches analysis to specific methods based on expression type.
func (a *Analyzer) analyze(expr ast.Expr) error {
	if expr == nil {
		return errors.New("expression cannot be nil")
	}

	switch e := expr.(type) {
	case *ast.Ident:
		return a.analyzeIdent(e)
	case *ast.Literal:
		return a.analyzeLiteral(e)
	case *ast.ListLiteral:
		return a.analyzeListLiteral(e)
	case *ast.UnaryExpr:
		return a.analyzeUnaryExpr(e)
	case *ast.BinaryExpr:
		return a.analyzeBinaryExpr(e)
	case *ast.IndexExpr:
		return a.analyzeIndexExpr(e)
	default:
		return fmt.Errorf("unsupported expression type: %T at %s", e, expr.Start().String())
	}
}

// analyzeIdent validates identifier tokens and ensures they are not reserved keywords.
func (a *Analyzer) analyzeIdent(ident *ast.Ident) error {
	if !ident.Token.Kind.Is(token.IDENT) {
		return fmt.Errorf("expected identifier token, got: %s(%s) at %s",
			ident.Token.Literal, ident.Token.Kind.Name(), ident.Start().String())
	}

	if !token.IsIdentifier(ident.Value) {
		return fmt.Errorf("'%s(%s)' cannot be used as identifier at %s",
			ident.Token.Literal, ident.Token.Kind.Name(), ident.Start().String())
	}

	return nil
}

// analyzeLiteral validates literal expressions including strings, numbers, and booleans.
// Ensures numeric and boolean literals can be properly parsed.
func (a *Analyzer) analyzeLiteral(lit *ast.Literal) error {
	pos := lit.Start().String()

	if lit.IsString() {
		return nil
	}

	if lit.IsNumber() {
		if _, err := lit.AsNumber(); err != nil {
			return fmt.Errorf("invalid number literal at %s", pos)
		}
		return nil
	}

	if lit.IsBool() {
		if _, err := lit.AsBool(); err != nil {
			return fmt.Errorf("invalid boolean literal at %s", pos)
		}
		return nil
	}

	return fmt.Errorf("unsupported literal type: %s(%s) at %s",
		lit.Token.Literal, lit.Token.Kind.Name(), pos)
}

// analyzeListLiteral validates list literals ensuring non-empty lists with uniform element types.
// Each list element is recursively analyzed for correctness.
func (a *Analyzer) analyzeListLiteral(list *ast.ListLiteral) error {
	pos := list.Start().String()

	if len(list.Values) == 0 {
		return fmt.Errorf("list literal cannot be empty at %s", pos)
	}

	firstElement := list.Values[0]
	expectedType := firstElement.Token.Kind.Name()

	for i, element := range list.Values {
		if !firstElement.IsSameKind(element) {
			actualType := element.Token.Kind.Name()
			return fmt.Errorf("list element at index %d has type '%s', expected '%s' (all elements must have same type) at %s",
				i, actualType, expectedType, element.Start().String())
		}

		if err := a.analyze(element); err != nil {
			return err
		}
	}

	return nil
}

// analyzeUnaryExpr validates unary expressions by checking operator type and operand.
func (a *Analyzer) analyzeUnaryExpr(unary *ast.UnaryExpr) error {
	pos := unary.Start().String()

	if !unary.Op.Kind.IsUnaryOperator() {
		return fmt.Errorf("unsupported unary operator: %s(%s) at %s",
			unary.Op.Literal, unary.Op.Kind.Name(), pos)
	}

	return a.analyze(unary.Right)
}

// analyzeBinaryExpr validates binary expressions based on operator type.
// Routes analysis to specific methods for different operator categories.
func (a *Analyzer) analyzeBinaryExpr(binary *ast.BinaryExpr) error {
	pos := binary.Start().String()

	// Handle logical operators (AND, OR)
	if binary.Op.Kind.IsLogicalOperator() {
		return a.analyzeLogicalOperation(binary)
	}

	// For non-logical operators, left operand must be identifier or index expression
	switch binary.Left.(type) {
	case *ast.Ident, *ast.IndexExpr:
		// Valid left operand types
	default:
		return fmt.Errorf("operator '%s(%s)' requires identifier or index expression on left side, got: %T at %s",
			binary.Op.Literal, binary.Op.Kind.Name(), binary.Left, pos)
	}

	// Handle equality operators (EQ, NE)
	if binary.Op.Kind.IsEqualityOperator() {
		return a.analyzeEqualityOperation(binary)
	}

	// Handle comparison operators (LT, LE, GT, GE)
	if binary.Op.Kind.IsOrderingOperator() {
		return a.analyzeOrderingOperation(binary)
	}

	// Handle IN operator
	if binary.Op.Kind.Is(token.IN) {
		return a.analyzeInOperation(binary)
	}

	// Handle LIKE operator
	if binary.Op.Kind.Is(token.LIKE) {
		return a.analyzeLikeOperation(binary)
	}

	return fmt.Errorf("unsupported binary operator: %s(%s) at %s",
		binary.Op.Literal, binary.Op.Kind.Name(), pos)
}

// analyzeLogicalOperation validates logical operators (AND, OR) requiring computed expressions.
// Both operands are recursively analyzed for semantic correctness.
func (a *Analyzer) analyzeLogicalOperation(binary *ast.BinaryExpr) error {
	pos := binary.Start().String()
	opName := binary.Op.Literal

	// Validate operand types
	if _, ok := binary.Left.(ast.ComputedExpr); !ok {
		return fmt.Errorf("operator '%s(%s)' requires computed expression on left side, got: %T at %s",
			opName, binary.Op.Kind.Name(), binary.Left, pos)
	}
	if _, ok := binary.Right.(ast.ComputedExpr); !ok {
		return fmt.Errorf("operator '%s(%s)' requires computed expression on right side, got: %T at %s",
			opName, binary.Op.Kind.Name(), binary.Right, pos)
	}

	// Analyze left operand
	if err := a.analyze(binary.Left); err != nil {
		return err
	}

	// Analyze right operand
	return a.analyze(binary.Right)
}

// analyzeEqualityOperation validates equality operators (==, !=) with literal values.
func (a *Analyzer) analyzeEqualityOperation(binary *ast.BinaryExpr) error {
	pos := binary.Start().String()
	opName := binary.Op.Literal

	// Right operand must be a literal
	if _, ok := binary.Right.(*ast.Literal); !ok {
		return fmt.Errorf("operator '%s(%s)' requires literal value on right side, got: %T at %s",
			opName, binary.Op.Kind.Name(), binary.Right, pos)
	}

	// Analyze left operand
	if err := a.analyze(binary.Left); err != nil {
		return err
	}

	// Analyze right operand
	return a.analyze(binary.Right)
}

// analyzeOrderingOperation validates ordering operators (<, <=, >, >=) requiring numeric literals.
func (a *Analyzer) analyzeOrderingOperation(binary *ast.BinaryExpr) error {
	pos := binary.Start().String()
	opName := binary.Op.Literal

	// Right operand must be a numeric literal
	literal, ok := binary.Right.(*ast.Literal)
	if !ok {
		return fmt.Errorf("operator '%s(%s)' requires literal value on right side, got: %T at %s",
			opName, binary.Op.Kind.Name(), binary.Right, pos)
	}
	if !literal.IsNumber() {
		return fmt.Errorf("operator '%s(%s)' requires numeric literal on right side, got: %s(%s) at %s",
			opName, binary.Op.Kind.Name(), literal.Token.Literal, literal.Token.Kind.Name(), pos)
	}

	// Analyze left operand
	if err := a.analyze(binary.Left); err != nil {
		return err
	}

	// Analyze right operand
	return a.analyze(binary.Right)
}

// analyzeInOperation validates IN operators requiring list literals on the right side.
func (a *Analyzer) analyzeInOperation(binary *ast.BinaryExpr) error {
	pos := binary.Start().String()

	// Right operand must be a list literal
	if _, ok := binary.Right.(*ast.ListLiteral); !ok {
		return fmt.Errorf("operator 'IN(%s)' requires list literal on right side, got: %T at %s",
			binary.Op.Kind.Name(), binary.Right, pos)
	}

	// Analyze left operand
	if err := a.analyze(binary.Left); err != nil {
		return err
	}

	// Analyze right operand
	return a.analyze(binary.Right)
}

// analyzeLikeOperation validates LIKE operators requiring string literals on the right side.
func (a *Analyzer) analyzeLikeOperation(binary *ast.BinaryExpr) error {
	pos := binary.Start().String()

	// Right operand must be a string literal
	literal, ok := binary.Right.(*ast.Literal)
	if !ok {
		return fmt.Errorf("operator 'LIKE(%s)' requires literal value on right side, got: %T at %s",
			binary.Op.Kind.Name(), binary.Right, pos)
	}
	if !literal.IsString() {
		return fmt.Errorf("operator 'LIKE(%s)' requires string literal on right side, got: %s(%s) at %s",
			binary.Op.Kind.Name(), literal.Token.Literal, literal.Token.Kind.Name(), pos)
	}

	// Analyze left operand
	if err := a.analyze(binary.Left); err != nil {
		return err
	}

	// Analyze right operand
	return a.analyze(binary.Right)
}

// analyzeIndexExpr validates index expressions with identifiers or nested indices.
// Index values must be numeric or string literals.
func (a *Analyzer) analyzeIndexExpr(index *ast.IndexExpr) error {
	pos := index.Start().String()

	// Validate left side expression
	switch left := index.Left.(type) {
	case *ast.Ident:
		if err := a.analyze(left); err != nil {
			return err
		}
	case *ast.IndexExpr:
		if err := a.analyze(left); err != nil {
			return err
		}
	default:
		return fmt.Errorf("index expression requires identifier or index expression on left side, got: %T at %s",
			left, pos)
	}

	// Validate index type and value
	if !index.Index.IsNumber() && !index.Index.IsString() {
		return fmt.Errorf("index must be number or string literal, got: %s(%s) at %s",
			index.Index.Token.Literal, index.Index.Token.Kind.Name(), pos)
	}

	return a.analyze(index.Index)
}
