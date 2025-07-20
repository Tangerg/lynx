package ast

import (
	"fmt"

	"github.com/Tangerg/lynx/ai/vectorstore/filter/token"
)

type UnaryExpr struct {
	Op    token.Token
	Right Expr
}

func (u *UnaryExpr) expr() {}

func (u *UnaryExpr) Start() token.Position {
	return u.Op.Start
}

func (u *UnaryExpr) End() token.Position {
	return u.Right.End()
}

func (u *UnaryExpr) String() string {
	return fmt.Sprintf("(%s %s)", u.Op.Kind.Literal(), u.Right.String())
}

func NOT[T *BinaryExpr | *UnaryExpr | *ParenExpr](r T) *UnaryExpr {
	unaryExpr := &UnaryExpr{
		Op: newKindToken(token.NOT),
	}

	switch typedR := any(r).(type) {
	case *BinaryExpr:
		unaryExpr.Right = typedR
	case *UnaryExpr:
		unaryExpr.Right = typedR
	case *ParenExpr:
		unaryExpr.Right = typedR
	}

	return unaryExpr
}
