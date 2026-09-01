package filter

// Parse turns the portable filter expression into a [Predicate]. The text form
// exists so filters can arrive from configuration or a request, while every
// backend still compiles the same validated AST rather than interpreting a
// vendor dialect.
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
