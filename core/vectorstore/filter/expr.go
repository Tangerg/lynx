package filter

import (
	"fmt"
	"reflect"
	"strconv"
)

// Position identifies a source line and column. Parsed expressions carry
// positions; programmatically constructed expressions use the zero value.
type Position struct {
	Line   int
	Column int
}

func (p Position) String() string {
	return strconv.Itoa(p.Line) + ":" + strconv.Itoa(p.Column)
}

// Expr is the stable root of the public filter expression tree. The sealed
// method keeps traversal exhaustive while the concrete node types below remain
// inspectable by provider adapters.
type Expr interface {
	Start() Position
	End() Position
	Equal(Expr) bool
	expr()
}

// AtomicExpr is a leaf expression.
type AtomicExpr interface {
	Expr
	atomicExpr()
}

// ComputedExpr evaluates to a boolean or indexed value.
type ComputedExpr interface {
	Expr
	computedExpr()
}

func equalExpr(left, right Expr) bool {
	leftNil := left == nil || reflect.ValueOf(left).IsNil()
	rightNil := right == nil || reflect.ValueOf(right).IsNil()
	if leftNil || rightNil {
		return leftNil && rightNil
	}
	return left.Equal(right)
}

// Ident names a metadata field.
type Ident struct {
	Value string
	start Position
	end   Position
}

func (*Ident) expr()       {}
func (*Ident) atomicExpr() {}
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
	return ok && i != nil && o != nil && i.Value == o.Value
}

// LiteralKind is the semantic type of a literal, independent of lexer tokens.
type LiteralKind string

const (
	LiteralString LiteralKind = "string"
	LiteralNumber LiteralKind = "number"
	LiteralBool   LiteralKind = "bool"
	LiteralNull   LiteralKind = "null"
)

// Literal is a scalar constant. Value is the canonical textual form; typed
// accessors reject mismatched kinds.
type Literal struct {
	Kind  LiteralKind
	Value string
	start Position
	end   Position
}

func (*Literal) expr()       {}
func (*Literal) atomicExpr() {}
func (l *Literal) Start() Position {
	if l == nil {
		return Position{}
	}
	return l.start
}
func (l *Literal) End() Position {
	if l == nil {
		return Position{}
	}
	return l.end
}
func (l *Literal) Equal(other Expr) bool {
	o, ok := other.(*Literal)
	return ok && l != nil && o != nil && l.Kind == o.Kind && l.Value == o.Value
}
func (l *Literal) IsString() bool { return l != nil && l.Kind == LiteralString }
func (l *Literal) IsNumber() bool { return l != nil && l.Kind == LiteralNumber }
func (l *Literal) IsBool() bool   { return l != nil && l.Kind == LiteralBool }
func (l *Literal) IsNull() bool   { return l != nil && l.Kind == LiteralNull }
func (l *Literal) AsString() (string, error) {
	if !l.IsString() {
		return "", fmt.Errorf("filter.Literal.AsString: expected string, got %s", l.Kind)
	}
	return l.Value, nil
}
func (l *Literal) AsNumber() (float64, error) {
	if !l.IsNumber() {
		return 0, fmt.Errorf("filter.Literal.AsNumber: expected number, got %s", l.Kind)
	}
	n, err := strconv.ParseFloat(l.Value, 64)
	if err != nil {
		return 0, fmt.Errorf("filter.Literal.AsNumber: parse %q: %w", l.Value, err)
	}
	return n, nil
}
func (l *Literal) AsBool() (bool, error) {
	if !l.IsBool() {
		return false, fmt.Errorf("filter.Literal.AsBool: expected bool, got %s", l.Kind)
	}
	b, err := strconv.ParseBool(l.Value)
	if err != nil {
		return false, fmt.Errorf("filter.Literal.AsBool: parse %q: %w", l.Value, err)
	}
	return b, nil
}
func (l *Literal) IsSameKind(other *Literal) bool {
	return l != nil && other != nil && l.Kind == other.Kind
}

// ListLiteral is a homogeneous list used by IN expressions.
type ListLiteral struct {
	Values []*Literal
	start  Position
	end    Position
}

func (*ListLiteral) expr()       {}
func (*ListLiteral) atomicExpr() {}
func (l *ListLiteral) Start() Position {
	if l == nil {
		return Position{}
	}
	return l.start
}
func (l *ListLiteral) End() Position {
	if l == nil {
		return Position{}
	}
	return l.end
}
func (l *ListLiteral) Equal(other Expr) bool {
	o, ok := other.(*ListLiteral)
	if !ok || l == nil || o == nil || len(l.Values) != len(o.Values) {
		return false
	}
	for i := range l.Values {
		if !l.Values[i].Equal(o.Values[i]) {
			return false
		}
	}
	return true
}

// UnaryExpr applies an operator to one computed expression.
type UnaryExpr struct {
	Op    Operator
	Right ComputedExpr
	start Position
	end   Position
}

func (*UnaryExpr) expr()         {}
func (*UnaryExpr) computedExpr() {}
func (u *UnaryExpr) Start() Position {
	if u == nil {
		return Position{}
	}
	return u.start
}
func (u *UnaryExpr) End() Position {
	if u == nil {
		return Position{}
	}
	if u.end != (Position{}) {
		return u.end
	}
	if u.Right == nil {
		return Position{}
	}
	return u.Right.End()
}
func (u *UnaryExpr) Equal(other Expr) bool {
	o, ok := other.(*UnaryExpr)
	return ok && u != nil && o != nil && u.Op == o.Op && equalExpr(u.Right, o.Right)
}
func (u *UnaryExpr) Precedence() int { return u.Op.Precedence() }

// BinaryExpr combines two expressions with a comparison, logical, matching,
// or null-test operator.
type BinaryExpr struct {
	Left  Expr
	Op    Operator
	Right Expr
	start Position
	end   Position
}

func (*BinaryExpr) expr()         {}
func (*BinaryExpr) computedExpr() {}
func (b *BinaryExpr) Start() Position {
	if b == nil {
		return Position{}
	}
	if b.start != (Position{}) {
		return b.start
	}
	if b.Left == nil {
		return Position{}
	}
	return b.Left.Start()
}
func (b *BinaryExpr) End() Position {
	if b == nil {
		return Position{}
	}
	if b.end != (Position{}) {
		return b.end
	}
	if b.Right == nil {
		return Position{}
	}
	return b.Right.End()
}
func (b *BinaryExpr) Equal(other Expr) bool {
	o, ok := other.(*BinaryExpr)
	return ok && b != nil && o != nil && b.Op == o.Op && equalExpr(b.Left, o.Left) && equalExpr(b.Right, o.Right)
}
func (b *BinaryExpr) Precedence() int { return b.Op.Precedence() }

// IndexExpr selects an array element or map key from another field/index.
type IndexExpr struct {
	Left  Expr
	Index *Literal
	start Position
	end   Position
}

func (*IndexExpr) expr()         {}
func (*IndexExpr) computedExpr() {}
func (i *IndexExpr) Start() Position {
	if i == nil {
		return Position{}
	}
	if i.start != (Position{}) {
		return i.start
	}
	if i.Left == nil {
		return Position{}
	}
	return i.Left.Start()
}
func (i *IndexExpr) End() Position {
	if i == nil {
		return Position{}
	}
	return i.end
}
func (i *IndexExpr) Equal(other Expr) bool {
	o, ok := other.(*IndexExpr)
	return ok && i != nil && o != nil && equalExpr(i.Left, o.Left) && equalExpr(i.Index, o.Index)
}
