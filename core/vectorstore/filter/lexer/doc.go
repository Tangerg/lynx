// Package lexer turns filter-language source text into a stream of
// [token.Token]s. Whitespace is skipped, single-quoted strings honour
// `\n` `\t` `\r` `\'` `\\` escapes, numbers can be negative and
// fractional, and unrecognised characters surface as ILLEGAL tokens
// (with line/column position) so the parser can produce useful error
// messages.
package lexer
