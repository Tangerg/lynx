// Package token provides lexical analysis utilities for parsing query expressions.
// This file defines the Token structure and factory functions for creating various token types.
package token

import (
	"errors"
	"fmt"
)

// Token represents a lexical token found during parsing, containing its type,
// source position range, and literal value. This is the fundamental unit returned
// by the lexer and consumed by the parser. The token tracks both start and end
// positions to provide precise location information for error reporting and
// source code analysis.
type Token struct {
	Kind    Kind     // The type/category of this token
	Start   Position // Source location where this token begins
	End     Position // Source location where this token ends
	Literal string   // The actual text content of the token
}

// String returns a formatted string representation of the token for debugging.
// This provides comprehensive information about the token including its kind,
// start and end positions in source, and literal content. The position range
// helps identify the exact span of source code that this token represents.
func (tok *Token) String() string {
	return fmt.Sprintf(
		`
Token {
  kind: %s, 
  start: %s, 
  end: %s,
  literal: %s
}`,
		tok.Kind.Name(), tok.Start.String(), tok.End.String(), tok.Literal,
	)
}

// NewToken creates a new token with the given kind and position range.
// The literal value is automatically set to the kind's default literal representation.
// This is typically used for tokens with fixed literals like operators and keywords
// where the literal content is predetermined by the token type.
//
// Parameters:
//   - kind: The token type/category
//   - start: Position where the token begins in the source
//   - end: Position where the token ends in the source
//
// Returns:
//   - Token: A new token with the specified kind, position range, and default literal
func NewToken(kind Kind, start, end Position) Token {
	return NewLiteralToken(kind, kind.Literal(), start, end)
}

// NewLiteralToken creates a new token with explicit kind, literal content, and position range.
// This provides full control over all token properties and is used when the literal
// differs from the kind's default representation (e.g., identifiers, numbers, strings).
// The position range accurately reflects the token's span in the source code.
//
// Parameters:
//   - kind: The token type/category
//   - literal: The actual text content of the token as it appears in source
//   - start: Position where the token begins in the source
//   - end: Position where the token ends in the source
//
// Returns:
//   - Token: A new token with all specified properties
//
// Usage examples:
//   - NewLiteralToken(IDENT, "username", pos1, pos2) for identifier tokens
//   - NewLiteralToken(NUMBER, "123.45", pos1, pos2) for numeric literals
//   - NewLiteralToken(STRING, "hello world", pos1, pos2) for string literals
func NewLiteralToken(kind Kind, literal string, start, end Position) Token {
	return Token{
		Kind:    kind,
		Literal: literal,
		Start:   start,
		End:     end,
	}
}

// NewEOFToken creates a token representing the end of input.
// EOF tokens typically have no literal content and mark the termination
// of the token stream during parsing. The start position is set to NoPosition
// since EOF doesn't correspond to actual source content, while the end position
// indicates where the input terminates.
//
// Parameters:
//   - pos: The position where end of input was encountered
//
// Returns:
//   - Token: An EOF token with empty literal, NoPosition start, and actual end position
//
// Position semantics:
//   - Start: NoPosition (EOF has no source content to start from)
//   - End: Actual position where input terminates
//   - This design distinguishes EOF from regular tokens that span source ranges
//
// The EOF token serves as a sentinel value that signals parsers to complete
// their processing and return final results.
func NewEOFToken(pos Position) Token {
	return Token{
		Kind:    EOF,
		Start:   NoPosition,
		End:     pos,
		Literal: "",
	}
}

// NewErrorToken creates an error token containing the error message as its literal.
// Error tokens are used to represent lexical errors encountered during tokenization,
// allowing the parser to continue and potentially report multiple errors rather than
// stopping at the first issue. The start position indicates where the error occurred,
// while the end position is set to NoPosition since errors don't span source ranges.
//
// Parameters:
//   - err: The error that occurred during tokenization (nil will be replaced with default)
//   - pos: The position where the error was encountered
//
// Returns:
//   - Token: An ERROR token containing the error message as its literal
//
// Position semantics:
//   - Start: Actual position where the error occurred
//   - End: NoPosition (errors are point events, not ranges)
//   - This design helps distinguish error locations from successful token spans
//
// Error recovery strategy:
//   - If err is nil, a default "unexpected error" message is used
//   - The error message becomes the token's literal for detailed reporting
//   - Position information helps locate the source of the problem
//   - Parser can continue processing to find additional errors
func NewErrorToken(err error, pos Position) Token {
	if err == nil {
		err = errors.New("unexpected error")
	}
	return Token{
		Kind:    ERROR,
		Start:   pos,
		End:     NoPosition,
		Literal: err.Error(),
	}
}

// NewIllegalToken creates an error token for illegal characters encountered during lexing.
// This specialized error token provides precise reporting for characters that don't
// belong in the language grammar. The position identifies exactly where the illegal
// character was found, and the error message includes both the character and location.
//
// Parameters:
//   - char: The illegal character that was encountered
//   - pos: The position where the illegal character was found
//
// Returns:
//   - Token: An ERROR token with a descriptive message about the illegal character
//
// Error message format:
//
//	The generated error message follows the pattern "illegal character 'X' at line:column(L:C)"
//	which provides both the problematic character and its exact location for easy debugging.
//
// Usage context:
//
//	This is typically called when the lexer encounters a character that cannot be
//	part of any valid token in the language (e.g., '@' or '#' in a language that
//	doesn't support these characters).
func NewIllegalToken(char rune, pos Position) Token {
	// Create descriptive error message including the character and its position
	err := fmt.Errorf("illegal character '%c' at %s", char, pos.String())

	return NewErrorToken(err, pos)
}
