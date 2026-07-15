package filter_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/core/vectorstore/filter"
)

func TestParse_ProducesCanonicalPredicates(t *testing.T) {
	tests := []struct {
		name  string
		input string
		check func(*testing.T, filter.Expr)
	}{
		{
			name:  "single element IN is a list",
			input: `status in ('active')`,
			check: func(t *testing.T, expr filter.Expr) {
				binary := expr.(*filter.BinaryExpr)
				list, ok := binary.Right.(*filter.ListLiteral)
				if !ok || len(list.Values) != 1 || list.Values[0].Value != "active" {
					t.Fatalf("right = %#v, want one-element ListLiteral", binary.Right)
				}
			},
		},
		{
			name:  "grouping remains a predicate",
			input: `(a == 1)`,
			check: func(t *testing.T, expr filter.Expr) {
				if _, ok := expr.(*filter.BinaryExpr); !ok {
					t.Fatalf("expr = %T, want *filter.BinaryExpr", expr)
				}
			},
		},
		{
			name:  "scientific notation is canonicalized",
			input: `score == 1e3`,
			check: func(t *testing.T, expr filter.Expr) {
				binary := expr.(*filter.BinaryExpr)
				literal := binary.Right.(*filter.Literal)
				if literal.Value != "1000" {
					t.Fatalf("literal.Value = %q, want 1000", literal.Value)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr, err := filter.Parse(tt.input)
			if err != nil {
				t.Fatal(err)
			}
			tt.check(t, expr)
		})
	}
}

func TestParse_RejectsNonPredicateRoots(t *testing.T) {
	for _, input := range []string{`field`, `42`, `'value'`, `(42)`} {
		t.Run(input, func(t *testing.T) {
			_, err := filter.Parse(input)
			if err == nil {
				t.Fatal("expected syntax error")
			}
			var syntaxError *filter.SyntaxError
			if !errors.As(err, &syntaxError) {
				t.Fatalf("error = %T, want *filter.SyntaxError", err)
			}
		})
	}
}

func TestParse_PreservesExplicitLogicalStructure(t *testing.T) {
	expr, err := filter.Parse(`not (not (year >= 2020))`)
	if err != nil {
		t.Fatal(err)
	}
	outer, ok := expr.(*filter.UnaryExpr)
	if !ok {
		t.Fatalf("parsed = %T, want outer *filter.UnaryExpr", expr)
	}
	inner, ok := outer.Right.(*filter.UnaryExpr)
	if !ok {
		t.Fatalf("outer.Right = %T, want inner *filter.UnaryExpr", outer.Right)
	}
	if _, ok := inner.Right.(*filter.BinaryExpr); !ok {
		t.Fatalf("inner.Right = %T, want *filter.BinaryExpr", inner.Right)
	}
}

func TestParse_ValidExpression(t *testing.T) {
	expr, err := filter.Parse(`category == 'tech'`)
	if err != nil {
		t.Fatal(err)
	}
	if expr == nil {
		t.Fatal("nil AST")
	}
}

func TestParse_InvalidExpression(t *testing.T) {
	_, err := filter.Parse(`category ==`)
	if err == nil {
		t.Fatal("incomplete expression must error")
	}
	var syntaxError *filter.SyntaxError
	if !errors.As(err, &syntaxError) {
		t.Fatalf("error = %T, want *filter.SyntaxError", err)
	}
	if syntaxError.Token != "end of input" || syntaxError.Position.Column == 0 {
		t.Fatalf("syntax error = %#v", syntaxError)
	}
}

func TestParse_RejectsOrderingAgainstString(t *testing.T) {
	if _, err := filter.Parse(`year >= 'two-thousand'`); err == nil {
		t.Fatal("ordering against a string must fail validation")
	}
}

func TestParse_HappyPath(t *testing.T) {
	expr, err := filter.Parse(`category == 'tech' AND year >= 2020`)
	if err != nil {
		t.Fatal(err)
	}
	if expr == nil {
		t.Fatal("nil AST")
	}
}

func TestParse_ParseError(t *testing.T) {
	_, err := filter.Parse(`broken (`)
	if err == nil {
		t.Fatal("syntactically invalid input must error")
	}
}

func TestParse_LogicalOperators(t *testing.T) {
	cases := []string{
		`a == 1 AND b == 2`,
		`a == 1 OR b == 2`,
		`NOT a == 1`,
		`(a == 1 OR b == 2) AND c == 3`,
	}
	for _, c := range cases {
		if _, err := filter.Parse(c); err != nil {
			t.Fatalf("Parse(%q) errored: %v", c, err)
		}
	}
}

func TestParse_ErrorMessageNonempty(t *testing.T) {
	_, err := filter.Parse(`@@@`)
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.TrimSpace(err.Error()) == "" {
		t.Fatal("error message should be non-empty")
	}
}

func TestParse_IsNull(t *testing.T) {
	for _, src := range []string{
		`owner is null`,
		`owner is not null`,
		`metadata['k'] is null`,
		`a is null and b == 1`,
	} {
		if _, err := filter.Parse(src); err != nil {
			t.Fatalf("Parse(%q): unexpected error %v", src, err)
		}
	}
}

func TestParse_IsNullRejectsNonNull(t *testing.T) {
	// IS must be followed by NULL (optionally NOT NULL); other right
	// sides are rejected at parse time.
	for _, src := range []string{
		`owner is 5`,
		`owner is 'x'`,
		`owner is`,
	} {
		if _, err := filter.Parse(src); err == nil {
			t.Fatalf("Parse(%q): expected error, got nil", src)
		}
	}
}

func TestParse_NotIn(t *testing.T) {
	// `NOT IN` reuses the NOT + IN tokens: it parses to a NOT wrapping an
	// IN, not a dedicated node.
	expr, err := filter.Parse(`tags not in ('a', 'b', 'c')`)
	if err != nil {
		t.Fatal(err)
	}
	unary, ok := expr.(*filter.UnaryExpr)
	if !ok {
		t.Fatalf("NOT IN should be a UnaryExpr(NOT, ...), got %T", expr)
	}
	if _, ok := unary.Right.(*filter.BinaryExpr); !ok {
		t.Fatalf("NOT IN inner should be a BinaryExpr(IN), got %T", unary.Right)
	}
}

func TestParse_NotInRejectsNonIn(t *testing.T) {
	// Infix NOT only accepts IN after it.
	for _, src := range []string{`a not == 1`, `a not 5`} {
		if _, err := filter.Parse(src); err == nil {
			t.Fatalf("Parse(%q): expected error, got nil", src)
		}
	}
}
