package visitors

import (
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/core/vectorstore/filter/ast"
	"github.com/Tangerg/lynx/core/vectorstore/filter/token"
)

// Analyzer is the semantic-validation visitor — checks that operator
// operands have compatible types, identifiers are not keywords, list
// literals are non-empty and homogeneous, etc. Run it after parsing
// and before any backend translates the filter to a query.
//
// Example:
//
//	expr, _ := filter.Parse(`category == 'tech' AND year >= 2020`)
//	if err := visitors.NewAnalyzer().Visit(expr); err != nil {
//	    return err
//	}
type Analyzer struct{}

func NewAnalyzer() *Analyzer { return &Analyzer{} }

func (a *Analyzer) Visit(expr ast.Expr) error {
	return a.visit(expr)
}

func (a *Analyzer) visit(expr ast.Expr) error {
	if expr == nil {
		return errors.New("visitors.Analyzer: expression is nil")
	}

	switch node := expr.(type) {
	case *ast.Ident:
		return a.visitIdent(node)
	case *ast.Literal:
		return a.visitLiteral(node)
	case *ast.ListLiteral:
		return a.visitListLiteral(node)
	case *ast.UnaryExpr:
		return a.visitUnaryExpr(node)
	case *ast.BinaryExpr:
		return a.visitBinaryExpr(node)
	case *ast.IndexExpr:
		return a.visitIndexExpr(node)
	default:
		return fmt.Errorf("visitors.Analyzer: unsupported expression type %T at %s",
			node, expr.Start().String())
	}
}

func (a *Analyzer) visitIdent(ident *ast.Ident) error {
	if !ident.Token.Kind.Is(token.IDENT) {
		return fmt.Errorf("visitors.Analyzer: expected IDENT token, got %s(%s) at %s",
			ident.Token.Literal, ident.Token.Kind.Name(), ident.Start().String())
	}
	if !token.IsIdentifier(ident.Value) {
		return fmt.Errorf("visitors.Analyzer: %q cannot be used as identifier at %s",
			ident.Token.Literal, ident.Start().String())
	}
	return nil
}

func (a *Analyzer) visitLiteral(lit *ast.Literal) error {
	pos := lit.Start().String()

	switch {
	case lit.IsString():
		return nil
	case lit.IsNumber():
		if _, err := lit.AsNumber(); err != nil {
			return fmt.Errorf("visitors.Analyzer: invalid number literal at %s", pos)
		}
		return nil
	case lit.IsBool():
		if _, err := lit.AsBool(); err != nil {
			return fmt.Errorf("visitors.Analyzer: invalid boolean literal at %s", pos)
		}
		return nil
	}
	return fmt.Errorf("visitors.Analyzer: unsupported literal %s(%s) at %s",
		lit.Token.Literal, lit.Token.Kind.Name(), pos)
}

func (a *Analyzer) visitListLiteral(list *ast.ListLiteral) error {
	pos := list.Start().String()

	if len(list.Values) == 0 {
		return fmt.Errorf("visitors.Analyzer: list literal cannot be empty at %s", pos)
	}

	first := list.Values[0]
	expected := first.Token.Kind.Name()

	for i, element := range list.Values {
		if !first.IsSameKind(element) {
			return fmt.Errorf("visitors.Analyzer: list element %d has type %s, expected %s (lists must be homogeneous) at %s",
				i, element.Token.Kind.Name(), expected, element.Start().String())
		}
		if err := a.visit(element); err != nil {
			return err
		}
	}
	return nil
}

func (a *Analyzer) visitUnaryExpr(unary *ast.UnaryExpr) error {
	if !unary.Op.Kind.IsUnaryOperator() {
		return fmt.Errorf("visitors.Analyzer: unsupported unary operator %s(%s) at %s",
			unary.Op.Literal, unary.Op.Kind.Name(), unary.Start().String())
	}
	return a.visit(unary.Right)
}

func (a *Analyzer) visitBinaryExpr(binary *ast.BinaryExpr) error {
	if binary.Op.Kind.IsLogicalOperator() {
		return a.visitLogicalOperation(binary)
	}

	// Non-logical operators require an identifier or index expression
	// on the left.
	switch binary.Left.(type) {
	case *ast.Ident, *ast.IndexExpr:
	default:
		return fmt.Errorf("visitors.Analyzer: operator %s(%s) requires identifier or index expression on the left, got %T at %s",
			binary.Op.Literal, binary.Op.Kind.Name(), binary.Left, binary.Start().String())
	}

	switch {
	case binary.Op.Kind.IsEqualityOperator():
		return a.visitEqualityOperation(binary)
	case binary.Op.Kind.IsOrderingOperator():
		return a.visitOrderingOperation(binary)
	case binary.Op.Kind.Is(token.IN):
		return a.visitInOperation(binary)
	case binary.Op.Kind.Is(token.LIKE):
		return a.visitLikeOperation(binary)
	case binary.Op.Kind.IsNullOperator():
		return a.visitNullTest(binary)
	}

	return fmt.Errorf("visitors.Analyzer: unsupported binary operator %s(%s) at %s",
		binary.Op.Literal, binary.Op.Kind.Name(), binary.Start().String())
}

func (a *Analyzer) visitNullTest(binary *ast.BinaryExpr) error {
	lit, ok := binary.Right.(*ast.Literal)
	if !ok || !lit.IsNull() {
		return fmt.Errorf("visitors.Analyzer: %s expects NULL on the right, got %T at %s",
			binary.Op.Literal, binary.Right, binary.Start().String())
	}
	return nil
}

func (a *Analyzer) visitLogicalOperation(binary *ast.BinaryExpr) error {
	pos := binary.Start().String()
	op := binary.Op.Literal

	if _, ok := binary.Left.(ast.ComputedExpr); !ok {
		return fmt.Errorf("visitors.Analyzer: %s(%s) requires computed expression on the left, got %T at %s",
			op, binary.Op.Kind.Name(), binary.Left, pos)
	}
	if _, ok := binary.Right.(ast.ComputedExpr); !ok {
		return fmt.Errorf("visitors.Analyzer: %s(%s) requires computed expression on the right, got %T at %s",
			op, binary.Op.Kind.Name(), binary.Right, pos)
	}

	if err := a.visit(binary.Left); err != nil {
		return err
	}
	return a.visit(binary.Right)
}

func (a *Analyzer) visitEqualityOperation(binary *ast.BinaryExpr) error {
	if _, ok := binary.Right.(*ast.Literal); !ok {
		return fmt.Errorf("visitors.Analyzer: %s(%s) requires literal on the right, got %T at %s",
			binary.Op.Literal, binary.Op.Kind.Name(), binary.Right, binary.Start().String())
	}

	if err := a.visit(binary.Left); err != nil {
		return err
	}
	return a.visit(binary.Right)
}

func (a *Analyzer) visitOrderingOperation(binary *ast.BinaryExpr) error {
	pos := binary.Start().String()

	literal, ok := binary.Right.(*ast.Literal)
	if !ok {
		return fmt.Errorf("visitors.Analyzer: %s(%s) requires literal on the right, got %T at %s",
			binary.Op.Literal, binary.Op.Kind.Name(), binary.Right, pos)
	}
	if !literal.IsNumber() {
		return fmt.Errorf("visitors.Analyzer: %s(%s) requires numeric literal on the right, got %s(%s) at %s",
			binary.Op.Literal, binary.Op.Kind.Name(),
			literal.Token.Literal, literal.Token.Kind.Name(), pos)
	}

	if err := a.visit(binary.Left); err != nil {
		return err
	}
	return a.visit(binary.Right)
}

func (a *Analyzer) visitInOperation(binary *ast.BinaryExpr) error {
	pos := binary.Start().String()

	if _, ok := binary.Right.(*ast.ListLiteral); !ok {
		if _, ok := binary.Right.(*ast.Literal); !ok {
			return fmt.Errorf("visitors.Analyzer: IN(%s) requires list literal on the right, got %T at %s",
				binary.Op.Kind.Name(), binary.Right, pos)
		}
	}

	if err := a.visit(binary.Left); err != nil {
		return err
	}
	return a.visit(binary.Right)
}

func (a *Analyzer) visitLikeOperation(binary *ast.BinaryExpr) error {
	pos := binary.Start().String()

	literal, ok := binary.Right.(*ast.Literal)
	if !ok {
		return fmt.Errorf("visitors.Analyzer: LIKE(%s) requires literal on the right, got %T at %s",
			binary.Op.Kind.Name(), binary.Right, pos)
	}
	if !literal.IsString() {
		return fmt.Errorf("visitors.Analyzer: LIKE(%s) requires string literal on the right, got %s(%s) at %s",
			binary.Op.Kind.Name(),
			literal.Token.Literal, literal.Token.Kind.Name(), pos)
	}

	if err := a.visit(binary.Left); err != nil {
		return err
	}
	return a.visit(binary.Right)
}

func (a *Analyzer) visitIndexExpr(index *ast.IndexExpr) error {
	pos := index.Start().String()

	switch left := index.Left.(type) {
	case *ast.Ident, *ast.IndexExpr:
		if err := a.visit(left); err != nil {
			return err
		}
	default:
		return fmt.Errorf("visitors.Analyzer: index expression requires identifier or index on the left, got %T at %s",
			left, pos)
	}

	if !index.Index.IsNumber() && !index.Index.IsString() {
		return fmt.Errorf("visitors.Analyzer: index must be number or string literal, got %s(%s) at %s",
			index.Index.Token.Literal, index.Index.Token.Kind.Name(), pos)
	}
	return a.visit(index.Index)
}
