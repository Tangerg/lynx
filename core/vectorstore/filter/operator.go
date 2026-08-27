package filter

import "fmt"

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
	OpHas          Operator = "has"
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
	case OpHas:
		return "HAS"
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
func (o Operator) IsLogicalOperator() bool { return o == OpAnd || o == OpOr }
func (o Operator) IsMembershipOperator() bool {
	return o == OpIn || o == OpHas
}
func (o Operator) IsMatchingOperator() bool { return o.IsMembershipOperator() || o == OpLike }
func (o Operator) IsNullOperator() bool     { return o == OpIs }
func (o Operator) IsBinaryOperator() bool {
	return o.IsComparisonOperator() || o.IsLogicalOperator() || o.IsMatchingOperator() || o.IsNullOperator()
}
func (o Operator) IsUnaryOperator() bool { return o == OpNot }

// LogicalString returns the canonical uppercase form of a logical operator.
func (o Operator) LogicalString() (string, error) {
	switch o {
	case OpAnd:
		return "AND", nil
	case OpOr:
		return "OR", nil
	default:
		return "", fmt.Errorf("filter: format logical operator: expected logical operator, got %s", o.Name())
	}
}

// Inverse returns the exact inverse comparison operator.
func (o Operator) Inverse() (Operator, error) {
	switch o {
	case OpEqual:
		return OpNotEqual, nil
	case OpNotEqual:
		return OpEqual, nil
	case OpLess:
		return OpGreaterEqual, nil
	case OpLessEqual:
		return OpGreater, nil
	case OpGreater:
		return OpLessEqual, nil
	case OpGreaterEqual:
		return OpLess, nil
	default:
		return "", fmt.Errorf("filter: invert operator: %s has no direct inverse", o.Name())
	}
}

func (o Operator) dual() Operator {
	if o == OpAnd {
		return OpOr
	}
	if o == OpOr {
		return OpAnd
	}
	return ""
}
