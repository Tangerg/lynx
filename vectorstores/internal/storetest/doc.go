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
//	        return v.Visit(expr)
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
