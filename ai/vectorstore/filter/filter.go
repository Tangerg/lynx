package filter

type Operand interface {
}

type Key struct {
	Operand
	key string
}

type Value struct {
	Operand
	value any
}

type ExpressionType string

const (
	AND ExpressionType = "AND"
	OR  ExpressionType = "OR"
	EQ  ExpressionType = "EQ"
	NEQ ExpressionType = "NEQ"
	GT  ExpressionType = "GT"
	GTE ExpressionType = "GTE"
	LT  ExpressionType = "LT"
	LTE ExpressionType = "LTE"
	IN  ExpressionType = "IN"
	NIN ExpressionType = "NIN"
	NOT ExpressionType = "NOT"
)

type Expression struct {
	Operand
	_type ExpressionType
	left  Operand
	right Operand
}

type Group struct {
	Operand
	expression *Expression
}
