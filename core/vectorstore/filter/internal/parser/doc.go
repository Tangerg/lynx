// Package parser builds an [ast.Expr] from filter-language source
// text. It uses a Pratt parser (operator-precedence climbing) on top
// of [lexer.Lexer]. The single entry point [Parse] handles the
// common case; [NewParser] is for callers that want to share a lexer
// or inspect parser state.
package parser
