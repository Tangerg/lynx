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
		return &Ident{Value: typed}, nil
	case *Ident:
		return typed, nil
	default:
		return nil, fmt.Errorf("filter.newIdent: expected string or *filter.Ident, got %T (%v)",
			value, value)
	}
}

// NewIdent builds an [*Ident] from either a string name or an
// existing identifier node. Position is always zero — these are
// hand-built nodes, not parsed ones.
func NewIdent[T IdentifierValue](value T) *Ident {
	ident, err := newIdent(value)
	if err != nil {
		// Unreachable while the generic constraint is honored.
		panic(fmt.Errorf("filter.NewIdent: %w", err))
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
