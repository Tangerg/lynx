package lexer

import (
	"fmt"
	"unicode"
)

func (l *Lexer) peekNextChar() (rune, error) {
	next, _, err := l.reader.ReadRune()
	if err != nil {
		return 0, err
	}
	if err = l.reader.UnreadRune(); err != nil {
		return 0, err
	}
	return next, nil
}

func (l *Lexer) consumeChar() error {
	char, _, err := l.reader.ReadRune()
	if err != nil {
		return err
	}

	l.currentChar = char

	if char == '\n' {
		l.cursor.Line++
		l.cursor.ResetColumn()
	} else {
		l.cursor.Column++
	}
	return nil
}

func (l *Lexer) bufferCurrentChar() {
	l.valueBuffer.WriteRune(l.currentChar)
}

func (l *Lexer) consumeExpected(expected rune) {
	if err := l.consumeChar(); err != nil {
		panic(fmt.Errorf("lexer.consumeExpected: read %q: %w", expected, err))
	}
	if l.currentChar != expected {
		panic(fmt.Errorf("lexer.consumeExpected: want %q, got %q", expected, l.currentChar))
	}
}

func (l *Lexer) skipWhitespace() error {
	for {
		if err := l.consumeChar(); err != nil {
			return err
		}
		if !unicode.IsSpace(l.currentChar) {
			return nil
		}
	}
}
