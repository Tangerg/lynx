package token

import (
	"errors"
	"fmt"
	"strconv"
)

// Token represents a lexical token found during parsing, containing its type,
// source position range, and literal value. This is the fundamental unit returned
// by the lexer and consumed by the parser.
type Token struct {
	Kind    Kind     // The type/category of this token
	Start   Position // Source location where this token begins
	End     Position // Source location where this token ends
	Literal string   // The actual text content of the token
}

// String returns a formatted string representation of the token for debugging.
// The output includes the token kind, position range, and literal content.
func (t *Token) String() string {
	return fmt.Sprintf(
		`
Token {
  kind: %s, 
  start: %s, 
  end: %s,
  literal: %s
}`,
		t.Kind.Name(), t.Start.String(), t.End.String(), t.Literal,
	)
}

// Of creates a token with the specified kind, literal, and position range.
func Of(kind Kind, literal string, start, end Position) Token {
	return Token{
		Kind:    kind,
		Literal: literal,
		Start:   start,
		End:     end,
	}
}

// OfKind creates a token using the kind's default literal value.
// The literal is automatically derived from the kind.
func OfKind(kind Kind, start, end Position) Token {
	return Of(kind, kind.Literal(), start, end)
}

// OfEOF creates an EOF (End of File) token.
// The start position is set to NoPosition since EOF has no source content,
// while the end position marks where input terminates.
func OfEOF(pos Position) Token {
	return OfKind(EOF, NoPosition, pos)
}

// OfError creates an error token with the given error message as literal.
// Error tokens allow continued parsing and collection of multiple errors.
// The end position is set to NoPosition since errors are point events.
// If err is nil, uses a default error message.
func OfError(err error, pos Position) Token {
	if err == nil {
		err = errors.New("unexpected error")
	}
	return Of(ERROR, err.Error(), pos, NoPosition)
}

// OfIllegal creates an error token for illegal characters encountered during lexing.
// The error message includes both the character and its location for precise reporting.
func OfIllegal(char rune, pos Position) Token {
	err := fmt.Errorf("illegal character '%c' at %s", char, pos.String())
	return OfError(err, pos)
}

// OfIdent creates an identifier token from a user-defined string and position range.
// This function is specifically for user-defined identifiers (field names, variable names, etc.)
// and always creates an IDENT token. For keywords and operators, use OfKind instead.
func OfIdent(ident string, start, end Position) Token {
	return Of(IDENT, ident, start, end)
}

// OfLiteral creates a token with literal validation and normalization.
// Only supports STRING, NUMBER, TRUE, and FALSE kinds.
// For NUMBER tokens, validates the numeric literal and normalizes the format.
// For TRUE or FALSE tokens, uses the keyword literal from metadata.
// Returns an error token if the kind is unsupported or validation fails.
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
		return OfError(errors.New("unsupported kind: "+kind.Name()), start)
	}
}

// OfNumericLiteral creates a NUMBER token with validation and normalization.
// It parses the literal string as a floating-point number to validate syntax,
// then normalizes the representation using Go's standard formatting.
// This ensures consistent number representation regardless of input format:
//   - "123.000" becomes "123"
//   - "1.23e+02" becomes "123"
//   - "0.000123" becomes "0.000123"
//
// Returns an error token if the literal cannot be parsed as a valid number.
func OfNumericLiteral(literal string, start, end Position) Token {
	number, err := strconv.ParseFloat(literal, 64)
	if err != nil {
		return OfError(err, start)
	}
	// Format using 'g' format: chooses %e or %f automatically for readability
	// -1 precision means use the smallest number of digits necessary
	return Of(NUMBER, strconv.FormatFloat(number, 'g', -1, 64), start, end)
}
