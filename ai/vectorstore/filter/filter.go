package filter

import "github.com/spf13/cast"

type Expression interface {
	Expression() string
}

type Field struct {
	field string
}

func (f *Field) Expression() string {
	return f.field
}

type Value struct {
	value any
}

func (v *Value) Expression() string {
	return cast.ToString(v.value)
}

type Operator string

func (o Operator) Expression() string {
	return string(o)
}

const (
	AND  Operator = "AND"
	OR   Operator = "OR"
	NOT  Operator = "NOT"
	EQ   Operator = "="
	NEQ  Operator = "!="
	GT   Operator = ">"
	GTE  Operator = ">="
	LT   Operator = "<"
	LTE  Operator = "<="
	IN   Operator = "IN"
	NIN  Operator = "NOT IN"
	LIKE Operator = "LIKE"
)

type Condition struct {
	operator Operator
	left     Expression
	right    Expression
}

func (c *Condition) Expression() string {
	return c.left.Expression() + " " + c.operator.Expression() + " " + c.right.Expression()
}

type Group struct {
	inner Expression
}

func (g *Group) Expression() string {
	return "(" + g.inner.Expression() + ")"
}
