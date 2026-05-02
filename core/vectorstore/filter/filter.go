// Package filter implements the metadata-filter mini-language used by
// [github.com/Tangerg/lynx/core/vectorstore]. The language is a small
// boolean DSL (see filter.ebnf / filter.g4 for the grammar); the
// public surface is the three top-level helpers in this file plus the
// programmatic [Builder] and the [ast] / [token] / [lexer] / [parser] /
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

// ParseAndAnalyze chains [Parse] and [Analyze]. Returns the AST plus
// the first error from either stage.
//
// Example:
//
//	expr, err := filter.ParseAndAnalyze(`tags IN ["go", "rag"]`)
func ParseAndAnalyze(input string) (ast.Expr, error) {
	expr, err := Parse(input)
	if err != nil {
		return nil, err
	}
	return expr, Analyze(expr)
}
