package filter

import "fmt"

// optimizer owns the package's boolean-algebra normalization. It is private:
// callers observe the canonical tree returned by Parse, while public visitors
// continue to receive programmatically built predicates unchanged.
type optimizer struct {
	result Predicate
}

func optimize(predicate Predicate) (Predicate, error) {
	return (&optimizer{}).optimize(predicate)
}

func (o *optimizer) optimize(predicate Predicate) (Predicate, error) {
	if o == nil {
		return nil, fmt.Errorf("filter: optimizer is nil")
	}
	o.result = nil

	result, err := o.rewrite(predicate)
	if err != nil {
		return nil, err
	}
	o.result = result
	return o.result, nil
}

func (o *optimizer) rewrite(predicate Predicate) (Predicate, error) {
	if isNilExpr(predicate) {
		return nil, fmt.Errorf("filter: optimizer predicate is nil")
	}

	switch node := predicate.(type) {
	case *UnaryExpr:
		return o.rewriteUnary(node)
	case *BinaryExpr:
		return o.rewriteBinary(node)
	default:
		return nil, fmt.Errorf("filter: optimizer does not support predicate %T", predicate)
	}
}

func (o *optimizer) rewriteUnary(unary *UnaryExpr) (Predicate, error) {
	right, err := o.rewrite(unary.Right)
	if err != nil {
		return nil, err
	}

	if inner, ok := right.(*UnaryExpr); ok && unary.Op == OpNot && inner.Op == OpNot {
		return inner.Right, nil
	}
	if right == unary.Right {
		return unary, nil
	}
	return &UnaryExpr{
		Op: unary.Op, Right: right,
		start: unary.start, end: unary.end,
	}, nil
}

func (o *optimizer) rewriteBinary(binary *BinaryExpr) (Predicate, error) {
	if !binary.Op.IsLogicalOperator() {
		return binary, nil
	}

	left, ok := binary.Left.(Predicate)
	if !ok || isNilExpr(left) {
		return nil, fmt.Errorf("filter: optimizer %s left operand is %T", binary.Op.Name(), binary.Left)
	}
	right, ok := binary.Right.(Predicate)
	if !ok || isNilExpr(right) {
		return nil, fmt.Errorf("filter: optimizer %s right operand is %T", binary.Op.Name(), binary.Right)
	}

	left, err := o.rewrite(left)
	if err != nil {
		return nil, err
	}
	right, err = o.rewrite(right)
	if err != nil {
		return nil, err
	}

	if left.Equal(right) {
		return left, nil
	}
	dual := OpOr
	if binary.Op == OpOr {
		dual = OpAnd
	}
	if absorbs(left, right, dual) {
		return left, nil
	}
	if absorbs(right, left, dual) {
		return right, nil
	}
	if left == binary.Left && right == binary.Right {
		return binary, nil
	}
	return &BinaryExpr{
		Left: left, Op: binary.Op, Right: right,
		start: binary.start, end: binary.end,
	}, nil
}

func absorbs(predicate, candidate Predicate, operator Operator) bool {
	binary, ok := candidate.(*BinaryExpr)
	if !ok || binary.Op != operator {
		return false
	}
	return predicate.Equal(binary.Left) || predicate.Equal(binary.Right)
}
