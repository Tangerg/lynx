package filter_test

import (
	"math"
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

func TestParseReturnsValidatedPublicTree(t *testing.T) {
	expr, err := filter.Parse(`not (not (year >= 2020))`)
	if err != nil {
		t.Fatal(err)
	}
	outer, ok := expr.(*filter.UnaryExpr)
	if !ok || outer.Op != filter.OpNot {
		t.Fatalf("parsed = %T %#v, want outer NOT", expr, expr)
	}
	if outer.Start().Line != 1 || outer.Start().Column == 0 {
		t.Fatalf("parsed position = %v, want source position", outer.Start())
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
	tests := map[string]filter.Predicate{
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

func TestValidateRejectsInvalidSemanticShapes(t *testing.T) {
	tests := map[string]filter.Predicate{
		"numeric identifier": filter.EQ(filter.NewIdent("123"), 1),
		"keyword identifier": filter.EQ(filter.NewIdent("and"), 1),
		"scalar IN": &filter.BinaryExpr{
			Left: filter.NewIdent("field"), Op: filter.OpIn, Right: filter.NewLiteral(1),
		},
		"fractional index": filter.EQ(filter.Index("field", 1.5), "value"),
		"negative index":   filter.EQ(filter.Index("field", -1), "value"),
		"non-finite number": &filter.BinaryExpr{
			Left: filter.NewIdent("field"), Op: filter.OpEqual,
			Right: &filter.Literal{Kind: filter.LiteralNumber, Value: "NaN"},
		},
		"non-canonical number": &filter.BinaryExpr{
			Left: filter.NewIdent("field"), Op: filter.OpEqual,
			Right: &filter.Literal{Kind: filter.LiteralNumber, Value: "01"},
		},
		"malformed null": &filter.BinaryExpr{
			Left: filter.NewIdent("field"), Op: filter.OpIs,
			Right: &filter.Literal{Kind: filter.LiteralNull, Value: "NULL"},
		},
		"oversized index": filter.EQ(filter.Index("field", uint64(math.MaxUint64)), "value"),
	}

	for name, expr := range tests {
		t.Run(name, func(t *testing.T) {
			if err := filter.Validate(expr); err == nil {
				t.Fatal("Validate returned nil error")
			}
		})
	}
}

func TestNumericLiteralPreservesIntegerPrecision(t *testing.T) {
	const integer = uint64(math.MaxUint64)
	literal := filter.NewLiteral(integer)
	if literal.Value != "18446744073709551615" {
		t.Fatalf("Value = %q, want exact uint64 text", literal.Value)
	}
}

func TestNewLiteralDefersNonFiniteValidation(t *testing.T) {
	expr := filter.EQ("score", math.NaN())
	if err := filter.Validate(expr); err == nil {
		t.Fatal("Validate accepted a non-finite literal")
	}
}

func TestNewLiteralCanonicalizesNegativeZero(t *testing.T) {
	literal := filter.NewLiteral(math.Copysign(0, -1))
	if literal.Value != "0" {
		t.Fatalf("Value = %q, want 0", literal.Value)
	}
}

func TestMalformedExpressionEqualityIsNilSafe(t *testing.T) {
	left := &filter.BinaryExpr{Op: filter.OpEqual}
	right := &filter.BinaryExpr{Op: filter.OpEqual}
	if !left.Equal(right) {
		t.Fatal("equivalent incomplete expressions should compare equal")
	}
	leftList := &filter.ListLiteral{Values: []*filter.Literal{nil}}
	rightList := &filter.ListLiteral{Values: []*filter.Literal{nil}}
	if !leftList.Equal(rightList) {
		t.Fatal("equivalent incomplete lists should compare equal")
	}
}
