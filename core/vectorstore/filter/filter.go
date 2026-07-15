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
	if err := Validate(expr); err != nil {
		return nil, err
	}
	return expr, nil
}

// Validate checks a programmatically constructed expression for invalid
// operators, operands, identifiers, and heterogeneous/empty lists. [Parse]
// validates parsed input automatically.
func Validate(expr Predicate) error {
	return validateRoot(expr)
}
