package visitors_test

import (
	"testing"

	"github.com/Tangerg/lynx/core/vectorstore/filter/parser"
	"github.com/Tangerg/lynx/core/vectorstore/filter/visitors"
)

// render parses src, optimizes it, and renders the result back to
// SQL-like text so tests can assert on the simplified shape.
func optimizeRender(t *testing.T, src string) string {
	t.Helper()
	expr, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse(%q): %v", src, err)
	}
	optimized := visitors.NewOptimizer().Optimize(expr)
	v := visitors.NewSQLLikeVisitor()
	v.Visit(optimized)
	if err := v.Error(); err != nil {
		t.Fatalf("render(%q): %v", src, err)
	}
	return v.SQL()
}

func TestOptimizer_Rewrites(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string // expected SQL-like rendering after optimization
	}{
		{"double_not", `not (not (a == 1))`, `a == 1`},
		{"triple_not", `not (not (not (a == 1)))`, `not (a == 1)`},
		{"quad_not", `not (not (not (not (a == 1))))`, `a == 1`},
		{"idempotent_and", `a == 1 and a == 1`, `a == 1`},
		{"idempotent_or", `a == 1 or a == 1`, `a == 1`},
		{"absorption_and", `a == 1 and (a == 1 or b == 2)`, `a == 1`},
		{"absorption_or", `a == 1 or (a == 1 and b == 2)`, `a == 1`},
		{"absorption_reversed", `(a == 1 or b == 2) and a == 1`, `a == 1`},
		{"nested_double_not_in_and", `(not (not (a == 1))) and b == 2`, `a == 1 and b == 2`},
		// No-ops: distinct operands must be preserved verbatim.
		{"nop_and", `a == 1 and b == 2`, `a == 1 and b == 2`},
		{"nop_single_not", `not (a == 1)`, `not (a == 1)`},
		{"nop_comparison", `year >= 2020`, `year >= 2020`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := optimizeRender(t, tc.src)
			if got != tc.want {
				t.Fatalf("optimize(%q) rendered %q, want %q", tc.src, got, tc.want)
			}
		})
	}
}

func TestOptimizer_NilSafe(t *testing.T) {
	if got := visitors.NewOptimizer().Optimize(nil); got != nil {
		t.Fatalf("Optimize(nil) = %v, want nil", got)
	}
}
