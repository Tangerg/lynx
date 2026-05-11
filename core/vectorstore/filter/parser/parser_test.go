package parser_test

import (
	"errors"
	"testing"

	"github.com/Tangerg/lynx/core/vectorstore/filter/ast"
	"github.com/Tangerg/lynx/core/vectorstore/filter/parser"
)

func TestParse_Equality(t *testing.T) {
	expr, err := parser.Parse(`name == 'john'`)
	if err != nil {
		t.Fatal(err)
	}
	binary, ok := expr.(*ast.BinaryExpr)
	if !ok {
		t.Fatalf("type = %T, want *BinaryExpr", expr)
	}
	if _, ok := binary.Left.(*ast.Ident); !ok {
		t.Fatalf("left = %T, want *Ident", binary.Left)
	}
	if _, ok := binary.Right.(*ast.Literal); !ok {
		t.Fatalf("right = %T, want *Literal", binary.Right)
	}
}

func TestParse_LogicalAndPrecedence(t *testing.T) {
	// `a == 1 OR b == 2 AND c == 3` should bind as
	// `a==1 OR (b==2 AND c==3)` since AND > OR.
	expr, err := parser.Parse(`a == 1 OR b == 2 AND c == 3`)
	if err != nil {
		t.Fatal(err)
	}
	root := expr.(*ast.BinaryExpr)
	if root.Op.Literal != "or" {
		t.Fatalf("root op = %q, want or", root.Op.Literal)
	}
	right := root.Right.(*ast.BinaryExpr)
	if right.Op.Literal != "and" {
		t.Fatalf("right.op = %q, want and", right.Op.Literal)
	}
}

func TestParse_ParenGrouping(t *testing.T) {
	expr, err := parser.Parse(`(a == 1 OR b == 2) AND c == 3`)
	if err != nil {
		t.Fatal(err)
	}
	root := expr.(*ast.BinaryExpr)
	if root.Op.Literal != "and" {
		t.Fatalf("root op = %q, want and", root.Op.Literal)
	}
	left := root.Left.(*ast.BinaryExpr)
	if left.Op.Literal != "or" {
		t.Fatalf("left op = %q, want or", left.Op.Literal)
	}
}

func TestParse_NotUnary(t *testing.T) {
	expr, err := parser.Parse(`NOT (a == 1)`)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := expr.(*ast.UnaryExpr); !ok {
		t.Fatalf("type = %T, want *UnaryExpr", expr)
	}
}

func TestParse_InWithList(t *testing.T) {
	expr, err := parser.Parse(`status IN ('active', 'pending')`)
	if err != nil {
		t.Fatal(err)
	}
	binary := expr.(*ast.BinaryExpr)
	list, ok := binary.Right.(*ast.ListLiteral)
	if !ok {
		t.Fatalf("right = %T, want *ListLiteral", binary.Right)
	}
	if len(list.Values) != 2 {
		t.Fatalf("values len = %d, want 2", len(list.Values))
	}
}

func TestParse_LikeExpression(t *testing.T) {
	expr, err := parser.Parse(`name LIKE 'jo%'`)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := expr.(*ast.BinaryExpr); !ok {
		t.Fatalf("type = %T", expr)
	}
}

func TestParse_IndexExpression(t *testing.T) {
	expr, err := parser.Parse(`tags[0] == 'urgent'`)
	if err != nil {
		t.Fatal(err)
	}
	binary := expr.(*ast.BinaryExpr)
	if _, ok := binary.Left.(*ast.IndexExpr); !ok {
		t.Fatalf("left = %T, want *IndexExpr", binary.Left)
	}
}

func TestParse_NestedIndex(t *testing.T) {
	expr, err := parser.Parse(`matrix[1][2] == 0`)
	if err != nil {
		t.Fatal(err)
	}
	binary := expr.(*ast.BinaryExpr)
	outer := binary.Left.(*ast.IndexExpr)
	if _, ok := outer.Left.(*ast.IndexExpr); !ok {
		t.Fatalf("inner = %T, want *IndexExpr", outer.Left)
	}
}

func TestParse_BooleanIndexRejected(t *testing.T) {
	_, err := parser.Parse(`obj[true] == 1`)
	if err == nil {
		t.Fatal("boolean index must error")
	}
	if _, ok := errors.AsType[*parser.ParseError](err); !ok {
		t.Fatalf("err type = %T, want *ParseError (wrapped)", err)
	}
}

func TestParse_TrailingCommaRejected(t *testing.T) {
	if _, err := parser.Parse(`x IN (1, 2,)`); err == nil {
		t.Fatal("trailing comma must error")
	}
}

func TestParse_TypeMismatchInList(t *testing.T) {
	if _, err := parser.Parse(`x IN (1, 'two')`); err == nil {
		t.Fatal("mixed-type list must error")
	}
}

func TestParse_EmptyParensRejected(t *testing.T) {
	if _, err := parser.Parse(`a == ()`); err == nil {
		t.Fatal("empty () must error")
	}
}

func TestParse_IncompleteExpression(t *testing.T) {
	if _, err := parser.Parse(`a ==`); err == nil {
		t.Fatal("incomplete expression must error")
	}
}

func TestParse_TrailingTokenRejected(t *testing.T) {
	if _, err := parser.Parse(`a == 1 b`); err == nil {
		t.Fatal("trailing token must error")
	}
}

func TestParse_UnexpectedStartToken(t *testing.T) {
	if _, err := parser.Parse(`== 1`); err == nil {
		t.Fatal("operator at start must error")
	}
}

func TestParse_LexicalErrorPropagates(t *testing.T) {
	// '@' is not part of the language — lexer emits ERROR; parser must
	// surface it as ParseError.
	_, err := parser.Parse(`a @ 1`)
	if err == nil {
		t.Fatal("illegal character must error")
	}
}

func TestParseError_FormatIncludesPosition(t *testing.T) {
	_, err := parser.Parse(`a ==`)
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() == "" {
		t.Fatal("error message must not be empty")
	}
}

func TestNewParser_EmptyInput(t *testing.T) {
	if _, err := parser.NewParser(""); err == nil {
		t.Fatal("empty input must error")
	}
}
