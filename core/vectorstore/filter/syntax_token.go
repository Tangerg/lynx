package filter

import "github.com/Tangerg/lynx/core/vectorstore/filter/token"

func newKindToken(kind token.Kind) token.Token {
	return token.OfKind(kind, token.NoPosition, token.NoPosition)
}
