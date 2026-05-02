package ast_test

import (
	"testing"

	"github.com/Tangerg/lynx/core/vectorstore/filter/ast"
	"github.com/Tangerg/lynx/core/vectorstore/filter/token"
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

func TestUnaryExpr_PositionAndPrecedence(t *testing.T) {
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
	// EQ (4) binds tighter than NOT (3), so the right side does NOT
	// bind looser — no parens needed.
	if u.IsRightLower() {
		t.Errorf("EQ binds tighter than NOT — IsRightLower must be false")
	}
}

func TestUnaryExpr_RightLowerWhenLooserOp(t *testing.T) {
	notTok := token.OfKind(token.NOT, token.NoPosition, token.NoPosition)
	// NOT applied to (a OR b) — OR(1) < NOT(3), so right IS lower.
	inner := &ast.BinaryExpr{
		Left:  &ast.Ident{Token: token.OfIdent("a", token.NoPosition, token.NoPosition), Value: "a"},
		Op:    token.OfKind(token.OR, token.NoPosition, token.NoPosition),
		Right: &ast.Ident{Token: token.OfIdent("b", token.NoPosition, token.NoPosition), Value: "b"},
	}
	u := &ast.UnaryExpr{Op: notTok, Right: inner}
	if !u.IsRightLower() {
		t.Error("OR binds looser than NOT — IsRightLower must be true")
	}
}

func TestBinaryExpr_Precedence(t *testing.T) {
	left := &ast.BinaryExpr{
		Left:  &ast.Ident{Token: token.OfIdent("a", token.NoPosition, token.NoPosition), Value: "a"},
		Op:    token.OfKind(token.OR, token.NoPosition, token.NoPosition),
		Right: numberLit("1"),
	}
	outer := &ast.BinaryExpr{
		Left:  left,
		Op:    token.OfKind(token.AND, token.NoPosition, token.NoPosition),
		Right: numberLit("2"),
	}

	if !outer.IsLeftLower() {
		t.Error("OR binds looser than AND — IsLeftLower must be true")
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
