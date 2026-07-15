package filter

// Operator is a semantic filter operation. It is independent from lexer token
// kinds and is safe for provider adapters to switch on.
type Operator string

const (
	OpEqual        Operator = "=="
	OpNotEqual     Operator = "!="
	OpLess         Operator = "<"
	OpLessEqual    Operator = "<="
	OpGreater      Operator = ">"
	OpGreaterEqual Operator = ">="
	OpAnd          Operator = "and"
	OpOr           Operator = "or"
	OpNot          Operator = "not"
	OpIn           Operator = "in"
	OpLike         Operator = "like"
	OpIs           Operator = "is"
)

func (o Operator) String() string { return string(o) }
func (o Operator) Name() string {
	switch o {
	case OpEqual:
		return "EQ"
	case OpNotEqual:
		return "NE"
	case OpLess:
		return "LT"
	case OpLessEqual:
		return "LE"
	case OpGreater:
		return "GT"
	case OpGreaterEqual:
		return "GE"
	case OpAnd:
		return "AND"
	case OpOr:
		return "OR"
	case OpNot:
		return "NOT"
	case OpIn:
		return "IN"
	case OpLike:
		return "LIKE"
	case OpIs:
		return "IS"
	default:
		return "INVALID"
	}
}
func (o Operator) Is(other Operator) bool   { return o == other }
func (o Operator) IsEqualityOperator() bool { return o == OpEqual || o == OpNotEqual }
func (o Operator) IsOrderingOperator() bool {
	return o == OpLess || o == OpLessEqual || o == OpGreater || o == OpGreaterEqual
}
func (o Operator) IsComparisonOperator() bool {
	return o.IsEqualityOperator() || o.IsOrderingOperator()
}
func (o Operator) IsLogicalOperator() bool  { return o == OpAnd || o == OpOr }
func (o Operator) IsMatchingOperator() bool { return o == OpIn || o == OpLike }
func (o Operator) IsNullOperator() bool     { return o == OpIs }
func (o Operator) IsBinaryOperator() bool {
	return o.IsComparisonOperator() || o.IsLogicalOperator() || o.IsMatchingOperator() || o.IsNullOperator()
}
func (o Operator) IsUnaryOperator() bool { return o == OpNot }
