package ast

import (
	"strings"

	"github.com/Tangerg/lynx/ai/vectorstore/filter/token"
)

type ListLiteral struct {
	Lparen token.Token
	Rparen token.Token
	Values []*Literal
}

func (l *ListLiteral) expr() {}

func (l *ListLiteral) Start() token.Position {
	return l.Lparen.Start
}

func (l *ListLiteral) End() token.Position {
	return l.Rparen.End
}

func (l *ListLiteral) String() string {
	if len(l.Values) == 0 {
		return "()"
	}

	sb := strings.Builder{}
	sb.WriteString("(")
	for i, elem := range l.Values {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(elem.String())
	}
	sb.WriteString(")")
	return sb.String()
}

type listLiteralAble interface {
	[]int | []int8 | []int16 | []int32 | []int64 |
		[]uint | []uint8 | []uint16 | []uint32 | []uint64 |
		[]float32 | []float64
	[]string |
		[]bool |
		[]*Literal
	*ListLiteral
}

func NewListLiteral[T listLiteralAble](value T) *ListLiteral {
	listLiteral, ok := any(value).(*ListLiteral)
	if ok {
		return listLiteral
	}

	listLiteral = &ListLiteral{
		Lparen: newKindToken(token.LPAREN),
		Rparen: newKindToken(token.RPAREN),
	}

	switch typedValue := any(value).(type) {
	case []*Literal:
		listLiteral.Values = typedValue
	case []int:
		listLiteral.Values = NewLiterals(typedValue)
	case []int8:
		listLiteral.Values = NewLiterals(typedValue)
	case []int16:
		listLiteral.Values = NewLiterals(typedValue)
	case []int32:
		listLiteral.Values = NewLiterals(typedValue)
	case []int64:
		listLiteral.Values = NewLiterals(typedValue)
	case []uint:
		listLiteral.Values = NewLiterals(typedValue)
	case []uint8:
		listLiteral.Values = NewLiterals(typedValue)
	case []uint16:
		listLiteral.Values = NewLiterals(typedValue)
	case []uint32:
		listLiteral.Values = NewLiterals(typedValue)
	case []uint64:
		listLiteral.Values = NewLiterals(typedValue)
	case []float32:
		listLiteral.Values = NewLiterals(typedValue)
	case []float64:
		listLiteral.Values = NewLiterals(typedValue)
	case []string:
		listLiteral.Values = NewLiterals(typedValue)
	case []bool:
		listLiteral.Values = NewLiterals(typedValue)
	}

	return listLiteral
}
