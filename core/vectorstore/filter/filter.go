package filter

import (
	"fmt"

	"github.com/Tangerg/lynx/core/vectorstore/filter/internal/parser"
	"github.com/Tangerg/lynx/core/vectorstore/filter/internal/visitors"
)

func Parse(input string) (Expr, error) {
	parsed, err := parser.Parse(input)
	if err != nil {
		return nil, err
	}
	expr, err := fromInternal(parsed)
	if err != nil {
		return nil, err
	}
	if err := Validate(expr); err != nil {
		return nil, err
	}
	return simplify(expr), nil
}

// Validate checks a programmatically constructed expression for invalid
// operators, operands, identifiers, and heterogeneous/empty lists. [Parse]
// validates parsed input automatically.
func Validate(expr Expr) error {
	internal, err := toInternal(expr)
	if err != nil {
		return fmt.Errorf("filter.Validate: %w", err)
	}
	return visitors.NewAnalyzer().Visit(internal)
}

func simplify(expr Expr) Expr {
	internal, err := toInternal(expr)
	if err != nil {
		return expr
	}
	optimized := visitors.NewOptimizer().Optimize(internal)
	converted, err := fromInternal(optimized)
	if err != nil {
		return expr
	}
	return converted
}
