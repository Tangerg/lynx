package filter

import (
	"fmt"
)

// IdentifierValue is the input constraint for [NewIdent]: a raw string is
// turned into a fresh [Ident]; an existing [*Ident] passes
// through unchanged.
type IdentifierValue interface {
	string | *Ident
}

func newIdent(value any) (*Ident, error) {
	switch typed := value.(type) {
	case string:
		return &Ident{name: typed}, nil
	case *Ident:
		return typed, nil
	default:
		return nil, fmt.Errorf("filter: create identifier: expected string or *filter.Ident, got %T (%v)",
			value, value)
	}
}

// NewIdent panics for the same reason as [NewLiteral]: the constraint admits
// only a string or an existing identifier.
func NewIdent[T IdentifierValue](value T) *Ident {
	ident, err := newIdent(value)
	if err != nil {
		// Unreachable while the generic constraint is honored.
		panic(fmt.Errorf("filter: create identifier: %w", err))
	}
	return ident
}

func identOrIndex(l any) (Selector, error) {
	if ix, ok := l.(*IndexExpr); ok {
		return ix, nil
	}
	return newIdent(l)
}

func leftOperand[L IdentifierValue | *IndexExpr](l L) Selector {
	expr, _ := identOrIndex(l)
	return expr
}
