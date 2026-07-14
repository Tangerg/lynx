package token

import (
	"errors"
	"fmt"
	"strconv"
)

// Token is one lexical unit produced by the lexer: kind, source span,
// and the original lexeme. The parser consumes a stream of these.
type Token struct {
	Kind    Kind
	Start   Position
	End     Position
	Literal string
}

func (t *Token) String() string {
	return fmt.Sprintf("%s(%q)@%s-%s", t.Kind.Name(), t.Literal, t.Start, t.End)
}

func Of(kind Kind, literal string, start, end Position) Token {
	return Token{Kind: kind, Literal: literal, Start: start, End: end}
}

func OfKind(kind Kind, start, end Position) Token {
	return Of(kind, kind.Literal(), start, end)
}

func OfEOF(pos Position) Token {
	return OfKind(EOF, NoPosition, pos)
}

func OfError(err error, pos Position) Token {
	if err == nil {
		err = errors.New("token.OfError: nil error supplied")
	}
	return Of(ERROR, err.Error(), pos, NoPosition)
}

func OfIllegal(char rune, pos Position) Token {
	return OfError(fmt.Errorf("token.OfIllegal: illegal character %q at %s", char, pos.String()), pos)
}

func OfIdent(ident string, start, end Position) Token {
	return Of(IDENT, ident, start, end)
}

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
		return OfError(fmt.Errorf("token.OfLiteral: unsupported kind %s", kind.Name()), start)
	}
}

func OfNumericLiteral(literal string, start, end Position) Token {
	number, err := strconv.ParseFloat(literal, 64)
	if err != nil {
		return OfError(err, start)
	}
	return Of(NUMBER, strconv.FormatFloat(number, 'g', -1, 64), start, end)
}
