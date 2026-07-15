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
	right := o.rewrite(unary.Right)
	if inner, ok := right.(*UnaryExpr); ok && unary.Op == OpNot && inner.Op == OpNot {
		return inner.Right
	}
	if right == unary.Right {
		return unary
	}
	return &UnaryExpr{
		Op: unary.Op, Right: right,
		start: unary.start, end: unary.end,
	}
}

func (o optimizer) rewriteBinary(binary *BinaryExpr) Predicate {
	if !binary.Op.IsLogicalOperator() {
		return binary
	}

	left := o.rewrite(binary.Left.(Predicate))
	right := o.rewrite(binary.Right.(Predicate))

	terms := appendLogicalTerms(nil, binary.Op, left)
	terms = appendLogicalTerms(terms, binary.Op, right)
	terms, deduplicated := uniquePredicates(terms)
	terms, absorbed := removeAbsorbed(terms, binary.Op)
	if deduplicated || absorbed {
		return joinLogical(binary.Op, terms)
	}
	if len(terms) == 2 {
		if factored, ok := factorCommon(binary.Op, terms[0], terms[1]); ok {
			return o.rewrite(factored)
		}
	}

	if left == binary.Left && right == binary.Right {
		return binary
	}
	return &BinaryExpr{
		Left: left, Op: binary.Op, Right: right,
		start: binary.start, end: binary.end,
	}
}

func appendLogicalTerms(terms []Predicate, operator Operator, predicate Predicate) []Predicate {
	binary, ok := predicate.(*BinaryExpr)
	if !ok || binary.Op != operator {
		return append(terms, predicate)
	}
	left, leftOK := binary.Left.(Predicate)
	right, rightOK := binary.Right.(Predicate)
	if !leftOK || !rightOK {
		return append(terms, predicate)
	}
	terms = appendLogicalTerms(terms, operator, left)
	return appendLogicalTerms(terms, operator, right)
}

func uniquePredicates(predicates []Predicate) ([]Predicate, bool) {
	unique := make([]Predicate, 0, len(predicates))
	changed := false
	for _, candidate := range predicates {
		if containsPredicate(unique, candidate) {
			changed = true
			continue
		}
		unique = append(unique, candidate)
	}
	return unique, changed
}

func containsPredicate(predicates []Predicate, candidate Predicate) bool {
	for _, predicate := range predicates {
		if predicate.Equal(candidate) {
			return true
		}
	}
	return false
}

func removeAbsorbed(predicates []Predicate, operator Operator) ([]Predicate, bool) {
	kept := make([]Predicate, 0, len(predicates))
	changed := false
	dual := operator.dual()
	for i, candidate := range predicates {
		absorbed := false
		for j, predicate := range predicates {
			if i != j && containsLogical(candidate, dual, predicate) {
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

func containsLogical(candidate Predicate, operator Operator, target Predicate) bool {
	binary, ok := candidate.(*BinaryExpr)
	if !ok || binary.Op != operator {
		return false
	}
	if candidate.Equal(target) {
		return true
	}
	left, leftOK := binary.Left.(Predicate)
	right, rightOK := binary.Right.(Predicate)
	return leftOK && (left.Equal(target) || containsLogical(left, operator, target)) ||
		rightOK && (right.Equal(target) || containsLogical(right, operator, target))
}

func factorCommon(operator Operator, left, right Predicate) (Predicate, bool) {
	dual := operator.dual()
	leftBinary, leftOK := left.(*BinaryExpr)
	rightBinary, rightOK := right.(*BinaryExpr)
	if !leftOK || !rightOK || leftBinary.Op != dual || rightBinary.Op != dual {
		return nil, false
	}

	leftTerms := appendLogicalTerms(nil, dual, left)
	rightTerms := appendLogicalTerms(nil, dual, right)
	common, leftOnly, rightOnly := partitionCommon(leftTerms, rightTerms)
	if len(common) == 0 {
		return nil, false
	}
	if len(leftOnly) == 0 {
		return left, true
	}
	if len(rightOnly) == 0 {
		return right, true
	}

	remainder := joinLogical(operator, []Predicate{
		joinLogical(dual, leftOnly),
		joinLogical(dual, rightOnly),
	})
	return joinLogical(dual, append(common, remainder)), true
}

func partitionCommon(left, right []Predicate) (common, leftOnly, rightOnly []Predicate) {
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

func joinLogical(operator Operator, predicates []Predicate) Predicate {
	if len(predicates) == 0 {
		panic("filter: cannot join an empty predicate set")
	}
	result := predicates[0]
	for _, right := range predicates[1:] {
		result = &BinaryExpr{
			Left:  result,
			Op:    operator,
			Right: right,
			start: result.Start(),
			end:   right.End(),
		}
	}
	return result
}
