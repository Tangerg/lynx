package ast

import (
	"github.com/Tangerg/lynx/ai/vectorstore/filter/token"
)

type Expr interface {
	expr()
	Start() token.Position
	End() token.Position
	String() string
}

func newKindToken(kind token.Kind) token.Token {
	return token.OfKind(kind, token.NoPosition, token.NoPosition)
}
