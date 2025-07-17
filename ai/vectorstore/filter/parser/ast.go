package parser

import (
	"strconv"
	"strings"
)

type Node interface {
	String() string
}

type BinaryOpNode struct {
	left  Node
	op    TokenKind
	right Node
}

func (n *BinaryOpNode) Left() Node    { return n.left }
func (n *BinaryOpNode) Op() TokenKind { return n.op }
func (n *BinaryOpNode) Right() Node   { return n.right }
func (n *BinaryOpNode) String() string {
	return n.left.String() + " " + n.op.String() + " " + n.right.String()
}

type UnaryOpNode struct {
	op      TokenKind
	operand Node
}

func (n *UnaryOpNode) Op() TokenKind { return n.op }
func (n *UnaryOpNode) Operand() Node { return n.operand }
func (n *UnaryOpNode) String() string {
	return n.op.String() + " " + n.operand.String()
}

type ComparisonNode struct {
	left  Node
	op    TokenKind
	right Node
}

func (n *ComparisonNode) Left() Node    { return n.left }
func (n *ComparisonNode) Op() TokenKind { return n.op }
func (n *ComparisonNode) Right() Node   { return n.right }
func (n *ComparisonNode) String() string {
	return n.left.String() + " " + n.op.String() + " " + n.right.String()
}

type LikeNode struct {
	field   Node
	pattern Node
}

func (n *LikeNode) Field() Node   { return n.field }
func (n *LikeNode) Pattern() Node { return n.pattern }
func (n *LikeNode) String() string {
	return n.field.String() + " LIKE " + n.pattern.String()
}

type InNode struct {
	field  Node
	values []Node
}

func (n *InNode) Field() Node    { return n.field }
func (n *InNode) Values() []Node { return n.values }
func (n *InNode) String() string {
	valueStrings := make([]string, 0, len(n.values))
	for _, value := range n.values {
		valueStrings = append(valueStrings, value.String())
	}
	return n.field.String() + " IN (" + strings.Join(valueStrings, ", ") + ")"
}

type IdentifierNode struct {
	value string
}

func (n *IdentifierNode) Value() string { return n.value }
func (n *IdentifierNode) String() string {
	return n.value
}

type StringNode struct {
	value string
}

func (n *StringNode) Value() string { return n.value }
func (n *StringNode) String() string {
	return n.value
}

type NumberNode struct {
	value float64
}

func (n *NumberNode) Value() float64 { return n.value }
func (n *NumberNode) String() string {
	return strconv.FormatFloat(n.value, 'g', -1, 64)
}

type BooleanNode struct {
	value bool
}

func (n *BooleanNode) Value() bool { return n.value }
func (n *BooleanNode) String() string {
	return strconv.FormatBool(n.value)
}
