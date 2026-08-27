package filter

import "errors"

// IndexExpr selects an array element or map key from another field/index.
type IndexExpr struct {
	left  Selector
	index *Literal
	start Position
	end   Position
}

func (*IndexExpr) expr()     {}
func (*IndexExpr) selector() {}

func (i *IndexExpr) Left() Selector {
	if i == nil {
		return nil
	}
	return i.left
}

func (i *IndexExpr) Index() *Literal {
	if i == nil {
		return nil
	}
	return i.index
}

func (i *IndexExpr) Path() ([]string, error) {
	if i == nil {
		return nil, errors.New("filter: read index path: index expression is nil")
	}
	path, err := i.left.Path()
	if err != nil {
		return nil, err
	}
	key, err := i.index.Key()
	if err != nil {
		return nil, err
	}
	return append(path, key), nil
}

func (i *IndexExpr) Start() Position {
	if i == nil {
		return Position{}
	}
	return i.start
}

func (i *IndexExpr) End() Position {
	if i == nil {
		return Position{}
	}
	return i.end
}

func (i *IndexExpr) Equal(other Expr) bool {
	o, ok := other.(*IndexExpr)
	return ok && i != nil && o != nil && equalExpr(i.left, o.left) && equalExpr(i.index, o.index)
}
