// Package token defines the lexical token shape and constructors used
// by the filter language's lexer and parser. The token kinds enum
// lives in [kind.go]; positions live in [position.go]; this file
// gathers the [Token] struct and its factory helpers.
package token

import (
	"errors"
	"fmt"
	"strconv"
)

// Token is one lexical unit produced by the lexer: kind, source span,
// and the original lexeme. The parser consumes a stream of these.
type Token struct {
	// Kind classifies the token (IDENT, NUMBER, STRING, AND, ...).
	Kind Kind

	// Start is the source position of the first byte.
	Start Position

	// End is the source position one past the last byte.
	End Position

	// Literal is the lexeme as written in the source (or, for
	// error/EOF tokens, a human-readable note).
	Literal string
}

// String renders the token as `KIND(literal)@start-end` — single-line
// debug summary suitable for logs and %v output.
func (t *Token) String() string {
	return fmt.Sprintf("%s(%q)@%s-%s", t.Kind.Name(), t.Literal, t.Start, t.End)
}

// Of returns a token with explicit kind, literal, and span.
func Of(kind Kind, literal string, start, end Position) Token {
	return Token{Kind: kind, Literal: literal, Start: start, End: end}
}

// OfKind returns a token whose literal is derived from the kind — used
// for keywords and fixed-text operators where the lexeme is known
// statically.
func OfKind(kind Kind, start, end Position) Token {
	return Of(kind, kind.Literal(), start, end)
}

// OfEOF returns an EOF sentinel positioned at pos. EOF has no source
// content, so Start is [NoPosition].
func OfEOF(pos Position) Token {
	return OfKind(EOF, NoPosition, pos)
}

// OfError returns an ERROR token carrying err's message as the
// literal. err==nil falls back to a generic message. End is
// [NoPosition] — errors are point events.
func OfError(err error, pos Position) Token {
	if err == nil {
		err = errors.New("unexpected error")
	}
	return Of(ERROR, err.Error(), pos, NoPosition)
}

// OfIllegal returns an ERROR token for an illegal character. The
// embedded message names the character and its location.
func OfIllegal(char rune, pos Position) Token {
	return OfError(fmt.Errorf("illegal character '%c' at %s", char, pos.String()), pos)
}

// OfIdent returns an IDENT token. For keywords and operators use
// [OfKind] instead — IDENT is reserved for user-supplied names.
func OfIdent(ident string, start, end Position) Token {
	return Of(IDENT, ident, start, end)
}

// OfLiteral returns a token for a literal of one of the four allowed
// kinds: STRING, NUMBER, TRUE, FALSE. NUMBER literals are validated
// and normalized via [OfNumericLiteral]. Unsupported kinds yield an
// ERROR token.
func OfLiteral(kind Kind, literal string, start, end Position) Token {
	switch kind {
	case NUMBER:
		return OfNumericLiteral(literal, start, end)
	case STRING:
		return Of(STRING, literal, start, end)
	case TRUE:
		return OfKind(TRUE, start, end)
	case FALSE:
		return OfKind(FALSE, start, end)
	default:
		return OfError(errors.New("token.OfLiteral: unsupported kind: "+kind.Name()), start)
	}
}

// OfNumericLiteral builds a NUMBER token, validating that literal
// parses as float64 and normalizing the printed form via 'g'-format.
//
// Normalization examples:
//
//	"123.000"   → "123"
//	"1.23e+02"  → "123"
//	"0.000123"  → "0.000123"
//
// Returns an ERROR token when literal does not parse.
func OfNumericLiteral(literal string, start, end Position) Token {
	number, err := strconv.ParseFloat(literal, 64)
	if err != nil {
		return OfError(err, start)
	}
	return Of(NUMBER, strconv.FormatFloat(number, 'g', -1, 64), start, end)
}
