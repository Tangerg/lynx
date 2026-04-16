package milvus

import (
	"fmt"
	"strings"

	"github.com/Tangerg/lynx/core/vectorstore/filter/ast"
	"github.com/Tangerg/lynx/core/vectorstore/filter/token"
)

// ToFilter converts an AST filter expression into a Milvus filter expression string.
//
// The generated string uses Milvus expression syntax and can be passed directly
// to search or delete options via WithFilter.
//
// Supported operations:
//   - Logical:          AND, OR, NOT
//   - Equality:         ==, !=
//   - Ordering:         <, <=, >, >=
//   - Membership:       IN
//   - Pattern matching: LIKE
//
// Field access:
//   - Simple field:      age → age
//   - JSON field key:    metadata["key"] → metadata["key"]
//   - Nested JSON key:   metadata["user"]["name"] → metadata["user"]["name"]
//
// Value encoding:
//   - Strings are wrapped in double quotes with internal double quotes escaped.
//   - Integers are formatted without a decimal point.
//   - Floats use %g notation.
//   - Booleans use Milvus syntax: True / False.
func ToFilter(expr ast.Expr) (string, error) {
	return exprToMilvus(expr)
}

func exprToMilvus(expr ast.Expr) (string, error) {
	if expr == nil {
		return "", fmt.Errorf("milvus: nil expression")
	}

	switch node := expr.(type) {
	case *ast.BinaryExpr:
		return binaryExprToMilvus(node)
	case *ast.UnaryExpr:
		return unaryExprToMilvus(node)
	case *ast.Ident:
		return node.Value, nil
	case *ast.IndexExpr:
		return indexExprToMilvus(node)
	case *ast.Literal:
		return literalToMilvus(node)
	case *ast.ListLiteral:
		return listLiteralToMilvus(node)
	default:
		return "", fmt.Errorf("milvus: unsupported expression type %T", node)
	}
}

func binaryExprToMilvus(expr *ast.BinaryExpr) (string, error) {
	switch expr.Op.Kind {
	case token.AND:
		return logicalToMilvus(expr, "and")
	case token.OR:
		return logicalToMilvus(expr, "or")
	case token.EQ:
		return comparisonToMilvus(expr, "==")
	case token.NE:
		return comparisonToMilvus(expr, "!=")
	case token.LT:
		return comparisonToMilvus(expr, "<")
	case token.LE:
		return comparisonToMilvus(expr, "<=")
	case token.GT:
		return comparisonToMilvus(expr, ">")
	case token.GE:
		return comparisonToMilvus(expr, ">=")
	case token.IN:
		return inToMilvus(expr)
	case token.LIKE:
		return likeToMilvus(expr)
	default:
		return "", fmt.Errorf("milvus: unsupported binary operator '%s' at %s",
			expr.Op.Literal, expr.Start().String())
	}
}

func logicalToMilvus(expr *ast.BinaryExpr, op string) (string, error) {
	left, err := exprToMilvus(expr.Left)
	if err != nil {
		return "", err
	}

	right, err := exprToMilvus(expr.Right)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("(%s) %s (%s)", left, op, right), nil
}

func comparisonToMilvus(expr *ast.BinaryExpr, op string) (string, error) {
	left, err := exprToMilvus(expr.Left)
	if err != nil {
		return "", err
	}

	right, err := exprToMilvus(expr.Right)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s %s %s", left, op, right), nil
}

func inToMilvus(expr *ast.BinaryExpr) (string, error) {
	left, err := exprToMilvus(expr.Left)
	if err != nil {
		return "", err
	}

	listLit, ok := expr.Right.(*ast.ListLiteral)
	if !ok {
		return "", fmt.Errorf("milvus: 'IN' operator requires a list on the right side at %s, got %T",
			expr.Start().String(), expr.Right)
	}
	if len(listLit.Values) == 0 {
		return "", fmt.Errorf("milvus: 'IN' operator requires a non-empty list at %s",
			expr.Start().String())
	}

	right, err := listLiteralToMilvus(listLit)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s in %s", left, right), nil
}

func likeToMilvus(expr *ast.BinaryExpr) (string, error) {
	left, err := exprToMilvus(expr.Left)
	if err != nil {
		return "", err
	}

	lit, ok := expr.Right.(*ast.Literal)
	if !ok {
		return "", fmt.Errorf("milvus: 'LIKE' operator requires a string literal on the right side at %s, got %T",
			expr.Start().String(), expr.Right)
	}
	if !lit.IsString() {
		return "", fmt.Errorf("milvus: 'LIKE' operator requires a string pattern at %s, got %s",
			expr.Start().String(), lit.Token.Kind.Name())
	}

	right, err := literalToMilvus(lit)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s like %s", left, right), nil
}

func unaryExprToMilvus(expr *ast.UnaryExpr) (string, error) {
	if expr.Op.Kind != token.NOT {
		return "", fmt.Errorf("milvus: unsupported unary operator '%s' at %s",
			expr.Op.Literal, expr.Start().String())
	}

	operand, err := exprToMilvus(expr.Right)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("not (%s)", operand), nil
}

func indexExprToMilvus(expr *ast.IndexExpr) (string, error) {
	left, err := exprToMilvus(expr.Left)
	if err != nil {
		return "", err
	}

	lit := expr.Index

	if lit.IsString() {
		key, err := lit.AsString()
		if err != nil {
			return "", fmt.Errorf("milvus: failed to extract index key at %s: %w", expr.Start().String(), err)
		}
		return fmt.Sprintf(`%s["%s"]`, left, key), nil
	}

	if lit.IsNumber() {
		num, err := lit.AsNumber()
		if err != nil {
			return "", fmt.Errorf("milvus: failed to extract index key at %s: %w", expr.Start().String(), err)
		}
		return fmt.Sprintf("%s[%d]", left, int(num)), nil
	}

	return "", fmt.Errorf("milvus: unsupported index type in expression at %s", expr.Start().String())
}

func literalToMilvus(lit *ast.Literal) (string, error) {
	if lit.IsString() {
		s, err := lit.AsString()
		if err != nil {
			return "", fmt.Errorf("milvus: failed to convert string literal at %s: %w", lit.Start().String(), err)
		}
		escaped := strings.ReplaceAll(s, `"`, `\"`)
		return fmt.Sprintf(`"%s"`, escaped), nil
	}

	if lit.IsNumber() {
		n, err := lit.AsNumber()
		if err != nil {
			return "", fmt.Errorf("milvus: failed to convert number literal at %s: %w", lit.Start().String(), err)
		}
		if n == float64(int64(n)) {
			return fmt.Sprintf("%d", int64(n)), nil
		}
		return fmt.Sprintf("%g", n), nil
	}

	if lit.IsBool() {
		b, err := lit.AsBool()
		if err != nil {
			return "", fmt.Errorf("milvus: failed to convert bool literal at %s: %w", lit.Start().String(), err)
		}
		if b {
			return "True", nil
		}
		return "False", nil
	}

	return "", fmt.Errorf("milvus: unsupported literal type '%s' at %s",
		lit.Token.Kind.Name(), lit.Start().String())
}

func listLiteralToMilvus(list *ast.ListLiteral) (string, error) {
	parts := make([]string, 0, len(list.Values))

	for i, lit := range list.Values {
		s, err := literalToMilvus(lit)
		if err != nil {
			return "", fmt.Errorf("milvus: failed to convert list element at index %d: %w", i, err)
		}
		parts = append(parts, s)
	}

	return "[" + strings.Join(parts, ", ") + "]", nil
}
