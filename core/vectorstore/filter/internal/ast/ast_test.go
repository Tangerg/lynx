package ast_test

import (
	"testing"

	"github.com/Tangerg/lynx/core/vectorstore/filter/internal/ast"
	"github.com/Tangerg/lynx/core/vectorstore/filter/internal/token"
)

func numberLit(v string) *ast.Literal {
	return &ast.Literal{
		Token: token.OfLiteral(token.NUMBER, v, token.NoPosition, token.NoPosition),
		Value: v,
	}
}

func stringLit(v string) *ast.Literal {
	return &ast.Literal{
		Token: token.OfLiteral(token.STRING, v, token.NoPosition, token.NoPosition),
		Value: v,
	}
}

func boolLit(v bool) *ast.Literal {
	kind := token.FALSE
	literal := "false"
	if v {
		kind = token.TRUE
		literal = "true"
	}
	return &ast.Literal{
		Token: token.OfKind(kind, token.NoPosition, token.NoPosition),
		Value: literal,
	}
}

func TestLiteral_AsString(t *testing.T) {
	lit := stringLit("hello")
	got, err := lit.AsString()
	if err != nil || got != "hello" {
		t.Fatalf("AsString = %q, %v", got, err)
	}

	if _, err := numberLit("1").AsString(); err == nil {
		t.Fatal("number AsString must error")
	}
}

func TestLiteral_AsNumber(t *testing.T) {
	got, err := numberLit("42.5").AsNumber()
	if err != nil || got != 42.5 {
		t.Fatalf("AsNumber = %v, %v", got, err)
	}

	if _, err := stringLit("x").AsNumber(); err == nil {
		t.Fatal("string AsNumber must error")
	}
}

func TestLiteral_AsBool(t *testing.T) {
	if v, err := boolLit(true).AsBool(); err != nil || !v {
		t.Fatalf("true AsBool = %v, %v", v, err)
	}
	if v, err := boolLit(false).AsBool(); err != nil || v {
		t.Fatalf("false AsBool = %v, %v", v, err)
	}
	if _, err := stringLit("yes").AsBool(); err == nil {
		t.Fatal("string AsBool must error")
	}
}

func TestLiteral_IsSameKind(t *testing.T) {
	a := numberLit("1")
	b := numberLit("2")
	c := stringLit("x")

	if !a.IsSameKind(b) {
		t.Error("two numbers must be same kind")
	}
	if a.IsSameKind(c) {
		t.Error("number and string must differ")
	}
	if a.IsSameKind(nil) {
		t.Error("nil arg must be false")
	}
}

func TestUnaryExpr_Precedence(t *testing.T) {
	notTok := token.OfKind(token.NOT, token.NoPosition, token.NoPosition)
	inner := &ast.BinaryExpr{
		Left:  &ast.Ident{Token: token.OfIdent("a", token.NoPosition, token.NoPosition), Value: "a"},
		Op:    token.OfKind(token.EQ, token.NoPosition, token.NoPosition),
		Right: numberLit("1"),
	}
	u := &ast.UnaryExpr{Op: notTok, Right: inner}

	if u.Precedence() != token.NOT.Precedence() {
		t.Error("UnaryExpr.Precedence must mirror operator")
	}
}

func TestBinaryExpr_Precedence(t *testing.T) {
	andTok := token.OfKind(token.AND, token.NoPosition, token.NoPosition)
	b := &ast.BinaryExpr{Left: numberLit("1"), Op: andTok, Right: numberLit("2")}

	if b.Precedence() != token.AND.Precedence() {
		t.Error("BinaryExpr.Precedence must mirror operator")
	}
}

func TestExpr_Equal(t *testing.T) {
	ident := func(v string) *ast.Ident {
		return &ast.Ident{Token: token.OfIdent(v, token.NoPosition, token.NoPosition), Value: v}
	}
	eq := token.OfKind(token.EQ, token.NoPosition, token.NoPosition)
	not := token.OfKind(token.NOT, token.NoPosition, token.NoPosition)

	left := &ast.BinaryExpr{Left: ident("a"), Op: eq, Right: numberLit("1")}
	same := &ast.BinaryExpr{Left: ident("a"), Op: eq, Right: numberLit("1")}
	different := &ast.BinaryExpr{Left: ident("a"), Op: eq, Right: numberLit("2")}
	if !left.Equal(same) {
		t.Fatal("identical binary expressions must be equal")
	}
	if left.Equal(different) {
		t.Fatal("different binary expressions must not be equal")
	}

	list := &ast.ListLiteral{
		Lparen: token.OfKind(token.LPAREN, token.NoPosition, token.NoPosition),
		Rparen: token.OfKind(token.RPAREN, token.NoPosition, token.NoPosition),
		Values: []*ast.Literal{stringLit("a"), stringLit("b")},
	}
	sameList := &ast.ListLiteral{
		Lparen: token.OfKind(token.LPAREN, token.NoPosition, token.NoPosition),
		Rparen: token.OfKind(token.RPAREN, token.NoPosition, token.NoPosition),
		Values: []*ast.Literal{stringLit("a"), stringLit("b")},
	}
	if !list.Equal(sameList) {
		t.Fatal("identical list literals must be equal")
	}

	unary := &ast.UnaryExpr{Op: not, Right: left}
	if !unary.Equal(&ast.UnaryExpr{Op: not, Right: same}) {
		t.Fatal("identical unary expressions must be equal")
	}
}

func TestListLiteral_PositionRange(t *testing.T) {
	lp := token.Position{Line: 1, Column: 5}
	rp := token.Position{Line: 1, Column: 10}
	list := &ast.ListLiteral{
		Lparen: token.OfKind(token.LPAREN, lp, lp),
		Rparen: token.OfKind(token.RPAREN, rp, rp),
		Values: []*ast.Literal{numberLit("1"), numberLit("2")},
	}

	if got := list.Start(); got != lp {
		t.Errorf("Start = %v, want %v", got, lp)
	}
	if got := list.End(); got != rp {
		t.Errorf("End = %v, want %v", got, rp)
	}
}
