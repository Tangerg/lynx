package ast

import (
	"github.com/Tangerg/lynx/ai/vectorstore/filter/token"
)

type Ident struct {
	Token token.Token
	Value string
}

func (i *Ident) expr() {}

func (i *Ident) Start() token.Position {
	return i.Token.Start
}

func (i *Ident) End() token.Position {
	return i.Token.End
}

func (i *Ident) String() string { return i.Value }

type identAble interface {
	string |
		*Ident
}

func isIdentAble(v any) bool {
	switch v.(type) {
	case string:
		return true
	case *Ident:
		return true
	default:
		return false
	}
}

func NewIdent[T identAble](value T) *Ident {
	switch typedValue := any(value).(type) {
	case string:
		return &Ident{
			Token: token.OfIdent(typedValue, token.NoPosition, token.NoPosition),
			Value: typedValue,
		}
	case *Ident:
		return typedValue
	default:
		return nil //It will never case here, just to compile pass
	}
}
