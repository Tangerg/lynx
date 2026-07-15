package filter_test

import (
	"math"
	"strings"
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

func TestParseReturnsValidatedCanonicalTree(t *testing.T) {
	expr, err := filter.Parse(`not (not (year >= 2020))`)
	if err != nil {
		t.Fatal(err)
	}
	binary, ok := expr.(*filter.BinaryExpr)
	if !ok || binary.Op != filter.OpGreaterEqual {
		t.Fatalf("parsed = %T %#v, want optimized comparison", expr, expr)
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

func TestProgrammaticCompositePositionsRemainZero(t *testing.T) {
	parsed, err := filter.Parse(`category == 'tech'`)
	if err != nil {
		t.Fatal(err)
	}
	programmatic := filter.Not(parsed)
	if programmatic.Start() != (filter.Position{}) || programmatic.End() != (filter.Position{}) {
		t.Fatalf("programmatic position = %s..%s, want zero positions", programmatic.Start(), programmatic.End())
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

func TestValidateRejectsExpressionCycles(t *testing.T) {
	unary := &filter.UnaryExpr{Op: filter.OpNot}
	unary.Right = unary

	binary := &filter.BinaryExpr{Op: filter.OpAnd, Right: filter.EQ("ready", true)}
	binary.Left = binary

	index := &filter.IndexExpr{Index: filter.NewLiteral("key")}
	index.Left = index

	for name, predicate := range map[string]filter.Predicate{
		"unary":  unary,
		"binary": binary,
		"index":  filter.EQ(index, "value"),
	} {
		t.Run(name, func(t *testing.T) {
			err := filter.Validate(predicate)
			if err == nil || !strings.Contains(err.Error(), "cycle") {
				t.Fatalf("Validate() error = %v, want cycle error", err)
			}
		})
	}
}

func TestValidateReportsInvalidOperatorBeforeOperands(t *testing.T) {
	err := filter.Validate(&filter.BinaryExpr{Op: filter.Operator("xor")})
	if err == nil || !strings.Contains(err.Error(), "invalid binary operator") {
		t.Fatalf("Validate() error = %v, want invalid operator", err)
	}

	err = filter.Validate(&filter.UnaryExpr{Op: filter.OpAnd})
	if err == nil || !strings.Contains(err.Error(), "invalid unary operator") {
		t.Fatalf("Validate() error = %v, want invalid operator", err)
	}
}

func TestValidateNumericIndexBoundaries(t *testing.T) {
	if err := filter.Validate(filter.EQ(filter.Index("items", int64(math.MaxInt64)), "value")); err != nil {
		t.Fatalf("max int64 index: %v", err)
	}
	if err := filter.Validate(filter.EQ(filter.Index("items", uint64(math.MaxInt64)+1), "value")); err == nil {
		t.Fatal("Validate accepted an index above max int64")
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
