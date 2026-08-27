package filter

import "errors"

// Ident names a metadata field.
type Ident struct {
	name  string
	start Position
	end   Position
}

func (*Ident) expr()     {}
func (*Ident) selector() {}

func (i *Ident) Name() string {
	if i == nil {
		return ""
	}
	return i.name
}

func (i *Ident) Path() ([]string, error) {
	if i == nil {
		return nil, errors.New("filter: read identifier path: identifier is nil")
	}
	return []string{i.name}, nil
}

func (i *Ident) Start() Position {
	if i == nil {
		return Position{}
	}
	return i.start
}

func (i *Ident) End() Position {
	if i == nil {
		return Position{}
	}
	return i.end
}

func (i *Ident) Equal(other Expr) bool {
	o, ok := other.(*Ident)
	return ok && i != nil && o != nil && i.name == o.name
}
