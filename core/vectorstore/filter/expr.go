package filter

import (
	"fmt"
	"math"
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

// Selector identifies a metadata value. Ident selects a top-level field;
// IndexExpr selects a nested field or array element.
type Selector interface {
	Expr
	selector()
}

// Predicate is a boolean expression accepted by vector-store filters.
type Predicate interface {
	Expr
	predicate()
}

func equalExpr(left, right Expr) bool {
	leftNil := isNilExpr(left)
	rightNil := isNilExpr(right)
	if leftNil || rightNil {
		return leftNil && rightNil
	}
	return left.Equal(right)
}

func isNilExpr(expr Expr) bool {
	if expr == nil {
		return true
	}
	switch node := expr.(type) {
	case *Ident:
		return node == nil
	case *Literal:
		return node == nil
	case *ListLiteral:
		return node == nil
	case *UnaryExpr:
		return node == nil
	case *BinaryExpr:
		return node == nil
	case *IndexExpr:
		return node == nil
	default:
		return false
	}
}

// Ident names a metadata field.
type Ident struct {
	Value string
	start Position
	end   Position
}

func (*Ident) expr()     {}
func (*Ident) selector() {}
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

func (*Literal) expr() {}
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
	if l == nil {
		return "", fmt.Errorf("filter.Literal.AsString: literal is nil")
	}
	if !l.IsString() {
		return "", fmt.Errorf("filter.Literal.AsString: expected string, got %s", l.Kind)
	}
	return l.Value, nil
}
func (l *Literal) AsNumber() (float64, error) {
	if l == nil {
		return 0, fmt.Errorf("filter.Literal.AsNumber: literal is nil")
	}
	if !l.IsNumber() {
		return 0, fmt.Errorf("filter.Literal.AsNumber: expected number, got %s", l.Kind)
	}
	n, err := strconv.ParseFloat(l.Value, 64)
	if err != nil {
		return 0, fmt.Errorf("filter.Literal.AsNumber: parse %q: %w", l.Value, err)
	}
	if math.IsNaN(n) || math.IsInf(n, 0) {
		return 0, fmt.Errorf("filter.Literal.AsNumber: %q is not finite", l.Value)
	}
	return n, nil
}
func (l *Literal) AsBool() (bool, error) {
	if l == nil {
		return false, fmt.Errorf("filter.Literal.AsBool: literal is nil")
	}
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

func (*ListLiteral) expr() {}
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
		if !equalExpr(l.Values[i], o.Values[i]) {
			return false
		}
	}
	return true
}

// UnaryExpr negates one predicate.
type UnaryExpr struct {
	Op    Operator
	Right Predicate
	start Position
	end   Position
}

func (*UnaryExpr) expr()      {}
func (*UnaryExpr) predicate() {}
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
	return u.end
}
func (u *UnaryExpr) Equal(other Expr) bool {
	o, ok := other.(*UnaryExpr)
	return ok && u != nil && o != nil && u.Op == o.Op && equalExpr(u.Right, o.Right)
}

// BinaryExpr combines two expressions with a comparison, logical, matching,
// or null-test operator.
type BinaryExpr struct {
	Left  Expr
	Op    Operator
	Right Expr
	start Position
	end   Position
}

func (*BinaryExpr) expr()      {}
func (*BinaryExpr) predicate() {}
func (b *BinaryExpr) Start() Position {
	if b == nil {
		return Position{}
	}
	return b.start
}
func (b *BinaryExpr) End() Position {
	if b == nil {
		return Position{}
	}
	return b.end
}
func (b *BinaryExpr) Equal(other Expr) bool {
	o, ok := other.(*BinaryExpr)
	return ok && b != nil && o != nil && b.Op == o.Op && equalExpr(b.Left, o.Left) && equalExpr(b.Right, o.Right)
}

// IndexExpr selects an array element or map key from another field/index.
type IndexExpr struct {
	Left  Selector
	Index *Literal
	start Position
	end   Position
}

func (*IndexExpr) expr()     {}
func (*IndexExpr) selector() {}
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
	return ok && i != nil && o != nil && equalExpr(i.Left, o.Left) && equalExpr(i.Index, o.Index)
}
