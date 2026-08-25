package filter

func Parse(input string) (Predicate, error) {
	p, err := newParser(input)
	if err != nil {
		return nil, err
	}
	expr, err := p.parse()
	if err != nil {
		return nil, err
	}
	if err := expr.Validate(); err != nil {
		return nil, err
	}
	return optimize(expr), nil
}

func validatePredicate(expr Predicate) error { return (&analyzer{}).analyze(expr) }
