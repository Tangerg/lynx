// Package filter is the metadata-filter mini-language used by
// [github.com/Tangerg/lynx/core/vectorstore]. It exposes three ways to
// produce an [ast.Expr]:
//
//  1. Generic factories ([EQ], [NE], [LT], …, [In], [Like], [And],
//     [Or], [Not]) — type-checked at compile time, ideal for
//     hand-written expressions.
//  2. Fluent [ExprBuilder] — accumulates an AND-by-default chain with
//     deferred error handling, ideal for dynamically-assembled
//     expressions where some predicates may fail validation.
//  3. [Parse] / [ParseAndAnalyze] — parse a textual expression
//     (filter.ebnf grammar) into an AST. Use for user-supplied
//     filters or persisted query strings.
//
// Sub-packages do the heavy lifting:
//
//   - [token]    — token kinds and lexer-emitted [token.Token].
//   - [lexer]    — string → token stream.
//   - [parser]   — token stream → [ast.Expr].
//   - [ast]      — node types and the [ast.Visitor] interface.
//   - [visitors] — semantic analyzer, SQL-like rendering, etc.
//
// Quick start:
//
//	// Programmatic
//	expr := filter.And(
//	    filter.EQ("category", "tech"),
//	    filter.GE("year", 2020),
//	)
//
//	// Fluent
//	expr, err := filter.NewExprBuilder().
//	    EQ("category", "tech").
//	    GE("year", 2020).
//	    Build()
//
//	// Textual
//	expr, err := filter.ParseAndAnalyze(`category == "tech" AND year >= 2020`)
//
// All three forms produce equivalent ASTs. Always run [Analyze] (or
// [ParseAndAnalyze]) before passing to a vector store — it catches
// type mismatches and bad operator/operand pairings the parser alone
// can't.
package filter
