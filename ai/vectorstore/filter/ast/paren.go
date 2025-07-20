package ast

import (
	"github.com/Tangerg/lynx/ai/vectorstore/filter/token"
)

type ParenExpr struct {
	Lparen token.Token
	Rparen token.Token
	Inner  Expr
}

func (p *ParenExpr) expr() {}

func (p *ParenExpr) Start() token.Position {
	return p.Lparen.Start
}

func (p *ParenExpr) End() token.Position {
	return p.Rparen.End
}

func (p *ParenExpr) String() string {
	return "(" + p.Inner.String() + ")"
}

func Paren[T *BinaryExpr | *UnaryExpr | *ParenExpr](inner T) *ParenExpr {
	parenExpr := &ParenExpr{
		Lparen: newKindToken(token.LPAREN),
		Rparen: newKindToken(token.RPAREN),
	}

	switch typedInner := any(inner).(type) {
	case *BinaryExpr:
		parenExpr.Inner = typedInner
	case *UnaryExpr:
		parenExpr.Inner = typedInner
	case *ParenExpr:
		parenExpr.Inner = typedInner
	}

	return parenExpr
}
