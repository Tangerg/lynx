package filter

// optimizer owns boolean-algebra normalization for trees already accepted by
// Validate. It is private: Parse exposes the normalized result, while public
// visitors receive programmatically built predicates unchanged.
type optimizer struct{}

func optimize(predicate Predicate) Predicate {
	return (optimizer{}).rewrite(predicate)
}

func (o optimizer) rewrite(predicate Predicate) Predicate {
	switch node := predicate.(type) {
	case *UnaryExpr:
		return o.rewriteUnary(node)
	case *BinaryExpr:
		return o.rewriteBinary(node)
	default:
		panic("filter: optimizer received an unvalidated predicate")
	}
}

func (o optimizer) rewriteUnary(unary *UnaryExpr) Predicate {
	right := o.rewrite(unary.right)
	if inner, ok := right.(*UnaryExpr); ok && unary.operator == OpNot && inner.operator == OpNot {
		return inner.right
	}
	if right == unary.right {
		return unary
	}
	return &UnaryExpr{
		operator: unary.operator, right: right,
		start: unary.start, end: unary.end,
	}
}

func (o optimizer) rewriteBinary(binary *BinaryExpr) Predicate {
	if !binary.operator.IsLogicalOperator() {
		return binary
	}

	left := o.rewrite(binary.left.(Predicate))
	right := o.rewrite(binary.right.(Predicate))

	terms := o.appendLogicalTerms(nil, binary.operator, left)
	terms = o.appendLogicalTerms(terms, binary.operator, right)
	terms, deduplicated := o.uniquePredicates(terms)
	terms, absorbed := o.removeAbsorbed(terms, binary.operator)
	if deduplicated || absorbed {
		return o.joinLogical(binary.operator, terms)
	}
	if len(terms) == 2 {
		if factored, ok := o.factorCommon(binary.operator, terms[0], terms[1]); ok {
			return o.rewrite(factored)
		}
	}

	if left == binary.left && right == binary.right {
		return binary
	}
	return &BinaryExpr{
		left: left, operator: binary.operator, right: right,
		start: binary.start, end: binary.end,
	}
}

func (o optimizer) appendLogicalTerms(terms []Predicate, operator Operator, predicate Predicate) []Predicate {
	binary, ok := predicate.(*BinaryExpr)
	if !ok || binary.operator != operator {
		return append(terms, predicate)
	}
	left, leftOK := binary.left.(Predicate)
	right, rightOK := binary.right.(Predicate)
	if !leftOK || !rightOK {
		return append(terms, predicate)
	}
	terms = o.appendLogicalTerms(terms, operator, left)
	return o.appendLogicalTerms(terms, operator, right)
}

func (o optimizer) uniquePredicates(predicates []Predicate) ([]Predicate, bool) {
	unique := make([]Predicate, 0, len(predicates))
	changed := false
	for _, candidate := range predicates {
		if o.containsPredicate(unique, candidate) {
			changed = true
			continue
		}
		unique = append(unique, candidate)
	}
	return unique, changed
}

func (optimizer) containsPredicate(predicates []Predicate, candidate Predicate) bool {
	for _, predicate := range predicates {
		if predicate.Equal(candidate) {
			return true
		}
	}
	return false
}

func (o optimizer) removeAbsorbed(predicates []Predicate, operator Operator) ([]Predicate, bool) {
	kept := make([]Predicate, 0, len(predicates))
	changed := false
	dual := operator.dual()
	for i, candidate := range predicates {
		absorbed := false
		for j, predicate := range predicates {
			if i != j && o.containsLogical(candidate, dual, predicate) {
				absorbed = true
				changed = true
				break
			}
		}
		if !absorbed {
			kept = append(kept, candidate)
		}
	}
	return kept, changed
}

func (o optimizer) containsLogical(candidate Predicate, operator Operator, target Predicate) bool {
	binary, ok := candidate.(*BinaryExpr)
	if !ok || binary.operator != operator {
		return false
	}
	if candidate.Equal(target) {
		return true
	}
	left, leftOK := binary.left.(Predicate)
	right, rightOK := binary.right.(Predicate)
	return leftOK && (left.Equal(target) || o.containsLogical(left, operator, target)) ||
		rightOK && (right.Equal(target) || o.containsLogical(right, operator, target))
}

func (o optimizer) factorCommon(operator Operator, left, right Predicate) (Predicate, bool) {
	dual := operator.dual()
	leftBinary, leftOK := left.(*BinaryExpr)
	rightBinary, rightOK := right.(*BinaryExpr)
	if !leftOK || !rightOK || leftBinary.operator != dual || rightBinary.operator != dual {
		return nil, false
	}

	leftTerms := o.appendLogicalTerms(nil, dual, left)
	rightTerms := o.appendLogicalTerms(nil, dual, right)
	common, leftOnly, rightOnly := o.partitionCommon(leftTerms, rightTerms)
	if len(common) == 0 {
		return nil, false
	}
	if len(leftOnly) == 0 {
		return left, true
	}
	if len(rightOnly) == 0 {
		return right, true
	}

	remainder := o.joinLogical(operator, []Predicate{
		o.joinLogical(dual, leftOnly),
		o.joinLogical(dual, rightOnly),
	})
	return o.joinLogical(dual, append(common, remainder)), true
}

func (optimizer) partitionCommon(left, right []Predicate) (common, leftOnly, rightOnly []Predicate) {
	matched := make([]bool, len(right))
	for _, candidate := range left {
		match := -1
		for i, predicate := range right {
			if !matched[i] && candidate.Equal(predicate) {
				match = i
				break
			}
		}
		if match < 0 {
			leftOnly = append(leftOnly, candidate)
			continue
		}
		matched[match] = true
		common = append(common, candidate)
	}
	for i, predicate := range right {
		if !matched[i] {
			rightOnly = append(rightOnly, predicate)
		}
	}
	return common, leftOnly, rightOnly
}

func (optimizer) joinLogical(operator Operator, predicates []Predicate) Predicate {
	if len(predicates) == 0 {
		panic("filter: cannot join an empty predicate set")
	}
	result := predicates[0]
	for _, right := range predicates[1:] {
		result = &BinaryExpr{
			left:     result,
			operator: operator,
			right:    right,
			start:    result.Start(),
			end:      right.End(),
		}
	}
	return result
}
