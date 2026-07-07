package lexer

import (
	"errors"
	"io"
	"unicode"

	"github.com/Tangerg/lynx/core/vectorstore/filter/token"
)

func (l *Lexer) resolveEscape(char rune) rune {
	switch char {
	case 'n':
		return '\n'
	case 't':
		return '\t'
	case 'r':
		return '\r'
	case '\'':
		return '\''
	case '\\':
		return '\\'
	default:
		return char
	}
}

func (l *Lexer) scanString() token.Token {
	defer l.valueBuffer.Reset()

	for {
		if err := l.consumeChar(); err != nil {
			return l.emitError(err)
		}

		if l.currentChar == '\'' {
			break
		}

		if l.currentChar == '\\' {
			if err := l.consumeChar(); err != nil {
				return l.emitError(err)
			}
			l.valueBuffer.WriteRune(l.resolveEscape(l.currentChar))
			continue
		}

		l.bufferCurrentChar()
	}

	return l.emitLiteral(token.STRING, l.valueBuffer.String())
}

func (l *Lexer) collectDigits() error {
	for {
		next, err := l.peekNextChar()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		if !unicode.IsDigit(next) {
			return nil
		}

		l.consumeExpected(next)
		l.bufferCurrentChar()
	}
}

func (l *Lexer) scanNumber() token.Token {
	defer l.valueBuffer.Reset()

	l.bufferCurrentChar()

	if err := l.collectDigits(); err != nil {
		return l.emitError(err)
	}

	next, err := l.peekNextChar()
	if err != nil && !errors.Is(err, io.EOF) {
		return l.emitError(err)
	}

	if err == nil && next == '.' {
		l.consumeExpected(next)
		l.bufferCurrentChar()

		if err = l.consumeChar(); err != nil {
			return l.emitError(err)
		}
		if !unicode.IsDigit(l.currentChar) {
			return l.emitIllegal()
		}
		l.bufferCurrentChar()

		if err = l.collectDigits(); err != nil {
			return l.emitError(err)
		}
	}

	return l.emitLiteral(token.NUMBER, l.valueBuffer.String())
}

func (l *Lexer) scanNegativeNumber() token.Token {
	if err := l.consumeChar(); err != nil {
		return l.emitError(err)
	}
	if !unicode.IsDigit(l.currentChar) {
		return l.emitIllegal()
	}

	number := l.scanNumber()
	if !number.Kind.Is(token.NUMBER) {
		return number
	}
	return l.emitLiteral(token.NUMBER, "-"+number.Literal)
}

func (l *Lexer) scanIdentifier() token.Token {
	defer l.valueBuffer.Reset()

	for {
		l.bufferCurrentChar()

		next, err := l.peekNextChar()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return l.emitError(err)
		}
		if !token.IsLiteralChar(next) {
			break
		}

		l.consumeExpected(next)
	}

	value := l.valueBuffer.String()
	kind := token.KindOf(value)

	if kind.IsKeyword() {
		return l.emitKind(kind)
	}
	return l.emitIdent(value)
}
