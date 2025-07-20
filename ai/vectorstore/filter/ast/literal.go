package ast

import (
	"fmt"
	"strconv"

	"github.com/spf13/cast"

	"github.com/Tangerg/lynx/ai/vectorstore/filter/token"
)

type Literal struct {
	Token token.Token
	Value string
}

func (l *Literal) expr() {}

func (l *Literal) Start() token.Position {
	return l.Token.Start
}

func (l *Literal) End() token.Position {
	return l.Token.End
}

func (l *Literal) String() string {
	// for string, must add "'"
	if l.IsString() {
		return "'" + l.Value + "'"
	}
	// for number/true/false
	return l.Value
}

func (l *Literal) IsString() bool {
	return l.Token.Kind.Is(token.STRING)
}

func (l *Literal) AsString() (string, error) {
	if !l.IsString() {
		return "", fmt.Errorf("expecting a STRING literal, but got %s", l.Token.Kind.Name())
	}
	return l.Value, nil
}

func (l *Literal) IsNumber() bool {
	return l.Token.Kind.Is(token.NUMBER)
}

func (l *Literal) AsNumber() (float64, error) {
	if !l.IsNumber() {
		return 0, fmt.Errorf("expecting a NUMBER literal, but got %s", l.Token.Kind.Name())
	}
	return strconv.ParseFloat(l.Value, 64)
}

func (l *Literal) IsBool() bool {
	return l.Token.Kind.Is(token.TRUE) || l.Token.Kind.Is(token.FALSE)
}

func (l *Literal) AsBool() (bool, error) {
	switch {
	case l.Token.Kind.Is(token.TRUE):
		return true, nil
	case l.Token.Kind.Is(token.FALSE):
		return false, nil
	default:
		return false, fmt.Errorf("expecting a TRUE or FALSE literal, but got %s", l.Token.Kind.Name())
	}
}

type numberAble interface {
	int | int8 | int16 | int32 | int64 |
		uint | uint8 | uint16 | uint32 | uint64 |
		float32 | float64
}

func isNumberAble(v any) bool {
	switch v.(type) {
	case int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64:
		return true
	default:
		return false
	}
}

// literalAble
// string 'tom' 'tom@gmail.com'
// number 1,2,3 /-1,-2,-3 / 1.23 / -1.23
// bool true/false
// *NumberLiteral
// *StringLiteral
// *BoolLiteral
type literalAble interface {
	numberAble |
		string |
		bool |
		*Literal
}

func isLiteralAble(v any) bool {
	if isNumberAble(v) {
		return true
	}
	switch v.(type) {
	case string:
		return true
	case bool:
		return true
	case *Literal:
		return true
	default:
		return false
	}
}

func NewLiteral[T literalAble](value T) *Literal {
	switch typedValue := any(value).(type) {
	case int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64:
		number := cast.ToString(typedValue)
		return &Literal{
			Token: token.OfLiteral(token.NUMBER, number, token.NoPosition, token.NoPosition),
			Value: number,
		}
	case string:
		return &Literal{
			Token: token.OfLiteral(token.STRING, typedValue, token.NoPosition, token.NoPosition),
			Value: typedValue,
		}
	case bool:
		var kind = token.FALSE
		if typedValue {
			kind = token.TRUE
		}
		return &Literal{
			Token: newKindToken(kind),
			Value: kind.Literal(),
		}
	case *Literal:
		return typedValue
	default:
		return nil //It will never case here, just to compile pass
	}
}

func NewLiterals[T literalAble](values []T) []*Literal {
	var literals []*Literal
	for _, value := range values {
		literals = append(literals, NewLiteral(value))
	}
	return literals
}
