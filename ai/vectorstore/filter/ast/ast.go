package ast

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Tangerg/lynx/ai/vectorstore/filter/token"
)

type Expr interface {
	exprNode()
	String() string
}

type UnaryExpr struct {
	Op    token.Token
	Right Expr
}

func (u *UnaryExpr) exprNode() {}

func (u *UnaryExpr) String() string {
	return fmt.Sprintf("(%s %s)", u.Op.Kind.Literal(), u.Right.String())
}

type BinaryExpr struct {
	Left  Expr
	Op    token.Token
	Right Expr
}

func (b *BinaryExpr) exprNode() {}

func (b *BinaryExpr) String() string {
	return fmt.Sprintf("(%s %s %s)", b.Left.String(), b.Op.Kind.Literal(), b.Right.String())
}

type ParenExpr struct {
	Inner Expr
}

func (p *ParenExpr) exprNode() {}

func (p *ParenExpr) String() string {
	return "(" + p.Inner.String() + ")"
}

type Ident struct {
	Value string
}

func (i *Ident) exprNode()      {}
func (i *Ident) String() string { return i.Value }

type Literal interface {
	Expr
	literalNode()
}

type StringLiteral struct {
	Value string
}

func (l *StringLiteral) exprNode()    {}
func (l *StringLiteral) literalNode() {}

func (l *StringLiteral) String() string {
	return "'" + l.Value + "'"
}

type NumberLiteral struct {
	Value float64
}

func (l *NumberLiteral) exprNode()    {}
func (l *NumberLiteral) literalNode() {}

func (l *NumberLiteral) String() string { return strconv.FormatFloat(l.Value, 'g', -1, 64) }

type BoolLiteral struct {
	Value bool
}

func (l *BoolLiteral) exprNode()    {}
func (l *BoolLiteral) literalNode() {}

func (l *BoolLiteral) String() string { return strconv.FormatBool(l.Value) }

type ListLiteral struct {
	Values []Literal
}

func (l *ListLiteral) exprNode()    {}
func (l *ListLiteral) literalNode() {}

func (l *ListLiteral) String() string {
	if len(l.Values) == 0 {
		return "()"
	}

	sb := strings.Builder{}
	sb.WriteString("(")
	for i, elem := range l.Values {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(elem.String())
	}
	sb.WriteString(")")
	return sb.String()
}
