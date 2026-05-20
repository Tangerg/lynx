// Package storetest is the shared test-fixture layer for vector-store
// vendors. The first export is [VisitorConformance], a portable suite
// every vendor's visitor opts into to gain free coverage on the AST
// shapes the filter mini-language supports.
//
// Each vendor wires the suite up in a single test file:
//
//	func TestVisitor_Conformance(t *testing.T) {
//	    storetest.VisitorConformance(t, func(src string) error {
//	        expr, err := filter.ParseAndAnalyze(src)
//	        if err != nil {
//	            return err
//	        }
//	        v := redis.NewVisitor(myFieldSchema)
//	        v.Visit(expr)
//	        return v.Error()
//	    })
//	}
//
// Output equivalence (the actual emitted SQL / filter struct) is NOT
// covered by the suite — backends emit heterogeneous output types and
// the vendor's own tests still own that responsibility. The suite
// only guarantees "every valid AST shape visits without error; every
// well-known invalid AST shape produces an error".
//
// # Field identifiers
//
// Every success case uses a disjoint field name per filter-value type
// so schema-required backends (redis, elasticsearch, opensearch, …)
// can declare each identifier with one fixed type:
//
//	author           — string-comparable
//	year             — number-comparable
//	published        — bool-comparable
//	n, a, b, c, d    — number-comparable (used in ordering / AND / OR)
//	tags             — string-list (IN)
//	years            — number-list (IN)
//	flags            — bool-list (IN)
//	title            — string-pattern (LIKE)
//	metadata['author'], metadata['a']['b'] — keyed access
//
// # Capability gaps
//
// A backend that genuinely doesn't support a shape (redis can't IN on
// numeric fields, for example) can opt out via [Options.Skip]. Each
// entry documents a real vendor capability gap; use sparingly.
package storetest

import (
	"slices"
	"strings"
	"testing"
)

// BuildFn parses a filter expression source and feeds it through the
// vendor's visitor. It returns nil on success, an error on failure.
// Implementations are responsible for assembling the AST (typically
// via [filter.ParseAndAnalyze]) and driving the vendor visitor.
type BuildFn func(src string) error

// Options tunes the conformance suite for vendors with genuine
// capability gaps.
type Options struct {
	// Skip is the list of case names the vendor cannot support. Each
	// matching case is recorded as [testing.T.Skip] rather than run.
	// Use sparingly — every entry documents a real divergence from
	// the common filter language.
	Skip []string
}

// VisitorConformance runs the standard expression-coverage suite
// against a vendor's visitor.
//
// The case lists below are the union of what every backend's filter
// language must accept (success cases) and the known-rejected shapes
// every backend must error on (failure cases). Adding a new shape
// here exercises it across ALL vendors that opt into the suite — the
// single best lever for "no more silent visitor regressions on the
// 27th provider".
func VisitorConformance(t *testing.T, build BuildFn, opts ...Options) {
	t.Helper()

	var opt Options
	if len(opts) > 0 {
		opt = opts[0]
	}

	success := []struct {
		name string
		src  string
	}{
		{"equality_string", `author == 'Alice'`},
		{"equality_number", `year == 2020`},
		{"equality_bool", `published == true`},
		{"inequality", `author != 'Alice'`},
		{"lt", `n < 10`},
		{"lte", `n <= 10`},
		{"gt", `n > 10`},
		{"gte", `n >= 10`},
		{"and", `a == 1 and b == 2`},
		{"or", `a == 1 or b == 2`},
		{"not", `not (a == 1)`},
		{"in_strings", `tags in ('a', 'b', 'c')`},
		{"in_numbers", `years in (2020, 2021, 2022)`},
		{"in_bools", `flags in (true, false)`},
		{"like", `title like '%foo%'`},
		{"indexed_key", `metadata['author'] == 'Alice'`},
		{"nested_index", `metadata['a']['b'] == 'x'`},
		{"nested_logical", `(a == 1 and b == 2) or (c == 3 and not (d == 4))`},
	}
	for _, tc := range success {
		t.Run("Success_"+tc.name, func(t *testing.T) {
			if slices.Contains(opt.Skip, tc.name) {
				t.Skip("vendor opted out of this conformance case")
			}
			if err := build(tc.src); err != nil {
				t.Fatalf("expected success on %q, got error: %v", tc.src, err)
			}
		})
	}

	failure := []struct {
		name string
		src  string
		// hint is an optional substring expected in the error message.
		// Empty hint means "any error is acceptable" — useful when
		// vendors wrap with their own prefixes and we don't want to
		// over-couple to wording.
		hint string
	}{
		// Parser unwraps single-element parens to a bare literal, so
		// `a in (1)` reaches the visitor as `a IN <literal>` — exercises
		// the "right side is not a list" rejection branch.
		{"in_scalar", `a in (1)`, "IN"},
		// LIKE with a non-string right side hits every backend's pattern
		// validation.
		{"like_number", `title like 42`, ""},
	}
	for _, tc := range failure {
		t.Run("Failure_"+tc.name, func(t *testing.T) {
			if slices.Contains(opt.Skip, tc.name) {
				t.Skip("vendor opted out of this conformance case")
			}
			err := build(tc.src)
			if err == nil {
				t.Fatalf("expected error on %q, got nil", tc.src)
			}
			if tc.hint != "" && !strings.Contains(err.Error(), tc.hint) {
				// Hint mismatch is informational only — vendors that
				// wrap errors with their own prefixes still pass the
				// suite as long as they error at all.
				t.Logf("err = %v (hint %q not in error — fine if vendor wraps)", err, tc.hint)
			}
		})
	}
}
