package filter

import (
	"github.com/Tangerg/lynx/ai/vectorstore/filter/ast"
	"github.com/Tangerg/lynx/ai/vectorstore/filter/parser"
	"github.com/Tangerg/lynx/ai/vectorstore/filter/visitors"
)

// Parse parses a filter expression string into an AST.
// This is the main entry point for parsing filter expressions.
// Returns the root expression node or a parsing error.
func Parse(input string) (ast.Expr, error) {
	return parser.Parse(input)
}

// Analyze performs semantic analysis on a parsed AST expression.
// Validates semantic correctness such as type compatibility and operator usage.
// Returns an error if semantic validation fails.
func Analyze(expr ast.Expr) error {
	analyzer := visitors.NewAnalyzer()
	analyzer.Visit(expr)
	return analyzer.Error()
}

// ParseAndAnalyze combines parsing and semantic analysis in one step.
// Parses the input string and validates the resulting AST semantically.
// Returns the expression and any parsing or analysis errors.
func ParseAndAnalyze(input string) (ast.Expr, error) {
	expr, err := Parse(input)
	if err != nil {
		return nil, err
	}
	return expr, Analyze(expr)
}
