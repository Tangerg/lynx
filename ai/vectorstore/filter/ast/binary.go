package ast

import (
	"fmt"

	"github.com/Tangerg/lynx/ai/vectorstore/filter/token"
)

type BinaryExpr struct {
	Left  Expr
	Op    token.Token
	Right Expr
}

func (b *BinaryExpr) expr() {}

func (b *BinaryExpr) Start() token.Position {
	return b.Left.Start()
}

func (b *BinaryExpr) End() token.Position {
	return b.Right.End()
}

func (b *BinaryExpr) String() string {
	return fmt.Sprintf("(%s %s %s)", b.Left.String(), b.Op.Kind.Literal(), b.Right.String())
}

func ComparisonBinary[L identAble, R literalAble](l L, r R, op token.Kind) *BinaryExpr {
	if !op.IsComparisonOperator() {
		panic("invalid comparison operator: " + op.Name())
	}

	return &BinaryExpr{
		Left:  NewIdent(l),
		Op:    newKindToken(op),
		Right: NewLiteral(r),
	}
}

// EQ
// id == 1
// age == 18
// name == 'tome'
// email == 'tom@gmail.com'
// active == true/false
func EQ[L identAble, R literalAble](l L, r R) *BinaryExpr {
	return ComparisonBinary(l, r, token.EQ)
}

// NE
// id != 1
// age != 18
// name != 'tome'
// email != 'tom@gmail.com'
// active != true/false
func NE[L identAble, R literalAble](l L, r R) *BinaryExpr {
	return ComparisonBinary(l, r, token.NE)
}

func LT[L identAble, R literalAble](l L, r R) *BinaryExpr {
	return ComparisonBinary(l, r, token.LT)
}

func LE[L identAble, R literalAble](l L, r R) *BinaryExpr {
	return ComparisonBinary(l, r, token.LE)
}

func GT[L identAble, R literalAble](l L, r R) *BinaryExpr {
	return ComparisonBinary(l, r, token.GT)
}

func GE[L identAble, R literalAble](l L, r R) *BinaryExpr {
	return ComparisonBinary(l, r, token.GE)
}

func LogicalBinary[T *BinaryExpr | *UnaryExpr | *ParenExpr](l T, r T, op token.Kind) *BinaryExpr {
	if !op.IsLogicalOperator() {
		panic("invalid logical operator: " + op.Name())
	}

	binaryExpr := &BinaryExpr{
		Op: newKindToken(op),
	}

	switch typedL := any(l).(type) {
	case *BinaryExpr:
		binaryExpr.Left = typedL
	case *UnaryExpr:
		binaryExpr.Left = typedL
	case *ParenExpr:
		binaryExpr.Left = typedL
	}

	switch typedR := any(r).(type) {
	case *BinaryExpr:
		binaryExpr.Right = typedR
	case *UnaryExpr:
		binaryExpr.Right = typedR
	case *ParenExpr:
		binaryExpr.Right = typedR
	}

	return binaryExpr
}

func AND[T *BinaryExpr | *UnaryExpr | *ParenExpr](l T, r T) *BinaryExpr {
	return LogicalBinary(l, r, token.AND)
}

func OR[T *BinaryExpr | *UnaryExpr | *ParenExpr](l T, r T) *BinaryExpr {
	return LogicalBinary(l, r, token.OR)
}

func IN[K identAble, V listLiteralAble](l K, r V) *BinaryExpr {
	return &BinaryExpr{
		Left:  NewIdent(l),
		Op:    newKindToken(token.IN),
		Right: NewListLiteral(r),
	}
}

func LIKE[K identAble, V literalAble](l K, r V) *BinaryExpr {
	literal := NewLiteral(r)
	_, err := literal.AsString()
	if err != nil {
		panic(err)
	}
	return &BinaryExpr{
		Left:  NewIdent(l),
		Op:    newKindToken(token.LIKE),
		Right: literal,
	}
}
