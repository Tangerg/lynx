package lexer

import (
	"errors"
	"io"
	"unicode"

	"github.com/Tangerg/lynx/core/vectorstore/filter/token"
)

func (l *Lexer) scanOptionalSecondChar(secondChar rune, single, paired token.Kind) token.Token {
	next, err := l.peekNextChar()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return l.emitKind(single)
		}
		return l.emitError(err)
	}

	if next != secondChar {
		return l.emitKind(single)
	}

	l.consumeExpected(next)
	return l.emitKind(paired)
}

func (l *Lexer) scanRequiredSecondChar(secondChar rune, kind token.Kind) token.Token {
	if err := l.consumeChar(); err != nil {
		return l.emitError(err)
	}
	if l.currentChar != secondChar {
		return l.emitIllegal()
	}
	return l.emitKind(kind)
}

func (l *Lexer) dispatchToken() token.Token {
	switch l.currentChar {
	case '=':
		return l.scanRequiredSecondChar('=', token.EQ)
	case '!':
		return l.scanRequiredSecondChar('=', token.NE)
	case '<':
		return l.scanOptionalSecondChar('=', token.LT, token.LE)
	case '>':
		return l.scanOptionalSecondChar('=', token.GT, token.GE)
	case '\'':
		return l.scanString()
	case '-':
		return l.scanNegativeNumber()
	case '(':
		return l.emitKind(token.LPAREN)
	case ')':
		return l.emitKind(token.RPAREN)
	case '[':
		return l.emitKind(token.LBRACK)
	case ']':
		return l.emitKind(token.RBRACK)
	case ',':
		return l.emitKind(token.COMMA)
	}

	if unicode.IsDigit(l.currentChar) {
		return l.scanNumber()
	}
	if unicode.IsLetter(l.currentChar) {
		return l.scanIdentifier()
	}

	return l.emitIllegal()
}
