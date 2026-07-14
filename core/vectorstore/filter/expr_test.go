package filter_test

import (
	"testing"

	"github.com/Tangerg/lynx/core/vectorstore/filter"
)

func TestConstructorsBuildStableSemanticNodes(t *testing.T) {
	expr := filter.And(
		filter.EQ("category", "tech"),
		filter.In("year", []int{2024, 2025}),
	)
	if err := filter.Validate(expr); err != nil {
		t.Fatal(err)
	}
	if expr.Op != filter.OpAnd {
		t.Fatalf("operator = %q, want and", expr.Op)
	}
	left, ok := expr.Left.(*filter.BinaryExpr)
	if !ok || left.Op != filter.OpEqual {
		t.Fatalf("left = %#v, want equality", expr.Left)
	}
	literal, ok := left.Right.(*filter.Literal)
	if !ok || literal.Kind != filter.LiteralString || literal.Value != "tech" {
		t.Fatalf("literal = %#v", left.Right)
	}
}

func TestParseReturnsValidatedOptimizedPublicTree(t *testing.T) {
	expr, err := filter.Parse(`not (not (year >= 2020))`)
	if err != nil {
		t.Fatal(err)
	}
	binary, ok := expr.(*filter.BinaryExpr)
	if !ok || binary.Op != filter.OpGreaterEqual {
		t.Fatalf("parsed = %T %#v, want optimized greater-equal", expr, expr)
	}
	if binary.Start().Line != 1 || binary.Start().Column == 0 {
		t.Fatalf("parsed position = %v, want source position", binary.Start())
	}
}

func TestParseRejectsSemanticError(t *testing.T) {
	if _, err := filter.Parse(`name like 42`); err == nil {
		t.Fatal("LIKE with a numeric pattern must fail during Parse")
	}
}

func TestExpressionEqualityIgnoresSourcePosition(t *testing.T) {
	parsed, err := filter.Parse(`category == 'tech'`)
	if err != nil {
		t.Fatal(err)
	}
	built := filter.EQ("category", "tech")
	if !parsed.Equal(built) || !built.Equal(parsed) {
		t.Fatal("equivalent parsed and constructed expressions must be equal")
	}
}

func TestValidateRejectsMalformedPublicTreeWithoutPanicking(t *testing.T) {
	tests := map[string]filter.Expr{
		"nil":                 nil,
		"typed nil":           (*filter.BinaryExpr)(nil),
		"missing left":        &filter.BinaryExpr{Op: filter.OpEqual, Right: filter.NewLiteral("value")},
		"missing right":       &filter.BinaryExpr{Left: filter.NewIdent("field"), Op: filter.OpEqual},
		"missing unary right": &filter.UnaryExpr{Op: filter.OpNot},
		"nil list element":    &filter.BinaryExpr{Left: filter.NewIdent("field"), Op: filter.OpIn, Right: &filter.ListLiteral{Values: []*filter.Literal{nil}}},
		"missing index":       &filter.BinaryExpr{Left: &filter.IndexExpr{Left: filter.NewIdent("field")}, Op: filter.OpEqual, Right: filter.NewLiteral("value")},
	}

	for name, expr := range tests {
		t.Run(name, func(t *testing.T) {
			if err := filter.Validate(expr); err == nil {
				t.Fatal("Validate returned nil error")
			}
		})
	}
}

func TestMalformedExpressionEqualityIsNilSafe(t *testing.T) {
	left := &filter.BinaryExpr{Op: filter.OpEqual}
	right := &filter.BinaryExpr{Op: filter.OpEqual}
	if !left.Equal(right) {
		t.Fatal("equivalent incomplete expressions should compare equal")
	}
}
