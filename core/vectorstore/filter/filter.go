// Package filter implements the metadata-filter mini-language used by
// [github.com/Tangerg/lynx/core/vectorstore]. The language is a small
// boolean DSL (see filter.ebnf / filter.g4 for the grammar); the
// public surface is the three top-level helpers in this file plus the
// programmatic [ExprBuilder] and the [ast] / [token] / [lexer] / [parser] /
// [visitors] subpackages.
package filter

import (
	"github.com/Tangerg/lynx/core/vectorstore/filter/ast"
	"github.com/Tangerg/lynx/core/vectorstore/filter/parser"
	"github.com/Tangerg/lynx/core/vectorstore/filter/visitors"
)

// Parse turns a textual filter expression into an [ast.Expr]. The
// returned tree is syntactically valid but not yet semantically
// checked — call [Analyze] (or [ParseAndAnalyze]) before passing it to
// a vector store.
//
// Example:
//
//	expr, err := filter.Parse(`category = "tech" AND year >= 2020`)
func Parse(input string) (ast.Expr, error) {
	return parser.Parse(input)
}

// Analyze performs semantic checks on a parsed expression — type
// compatibility, valid operator/operand pairings, etc. Returns the
// first violation found.
func Analyze(expr ast.Expr) error {
	analyzer := visitors.NewAnalyzer()
	analyzer.Visit(expr)
	return analyzer.Error()
}

// ParseAndAnalyze chains [Parse], [Analyze], and [Optimize]: it parses
// the input, validates it (errors carry the user's original shape), then
// returns the simplified, semantically-equivalent AST ready for a
// backend visitor. Returns the first error from the parse or analyze
// stage.
//
// Optimization is folded in here because this is the canonical
// "give me a ready-to-use filter" entry point; callers that need the
// unoptimized tree can call [Parse] + [Analyze] directly.
//
// Example:
//
//	expr, err := filter.ParseAndAnalyze(`tags IN ["go", "rag"]`)
func ParseAndAnalyze(input string) (ast.Expr, error) {
	expr, err := Parse(input)
	if err != nil {
		return nil, err
	}
	if err := Analyze(expr); err != nil {
		return nil, err
	}
	return Optimize(expr), nil
}

// Optimize returns a simplified, semantically-equivalent form of expr,
// folding dead logic (multiple NOTs, idempotent and absorption laws)
// before a backend visitor translates it. It is optional and pure: a
// valid analyzed tree stays valid and the matching record set is
// unchanged. See [visitors.Optimizer] for the exact rewrites.
//
// Example:
//
//	expr, _ := filter.ParseAndAnalyze(`not (not (year >= 2020))`)
//	expr = filter.Optimize(expr) // → year >= 2020
func Optimize(expr ast.Expr) ast.Expr {
	return visitors.NewOptimizer().Optimize(expr)
}
