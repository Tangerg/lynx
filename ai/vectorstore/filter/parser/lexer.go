package parser

import (
	"errors"
	"io"
	"strings"
	"unicode"
)

type Lexer struct {
	input         string
	position      Position
	currentChar   rune
	reader        *strings.Reader
	stringBuilder strings.Builder
}

func NewLexer(input string) (*Lexer, error) {
	if len(input) == 0 {
		return nil, errors.New("input is empty")
	}

	return &Lexer{
		input:  input,
		reader: strings.NewReader(input),
		position: Position{
			line:   1,
			column: 1,
		},
	}, nil
}

func (l *Lexer) readRune() error {
	character, _, err := l.reader.ReadRune()
	if err != nil {
		return err
	}

	l.currentChar = character

	if character == '\n' {
		l.position.line++
		l.position.column = 1
	} else {
		l.position.column++
	}

	return nil
}

func (l *Lexer) peekRune() (rune, error) {
	nextChar, _, err := l.reader.ReadRune()
	if err != nil {
		return 0, err
	}

	if err = l.reader.UnreadRune(); err != nil {
		return 0, err
	}

	return nextChar, nil
}

func (l *Lexer) skipSpace() error {
	for {
		if err := l.readRune(); err != nil {
			return err
		}

		if !unicode.IsSpace(l.currentChar) {
			return nil
		}
	}
}

var escapeChars = map[rune]rune{
	'n':  '\n',
	't':  '\t',
	'r':  '\r',
	'\\': '\\',
	'\'': '\'',
}

func (l *Lexer) readString() Token {
	defer l.stringBuilder.Reset()

	for {
		if err := l.readRune(); err != nil {
			return NewErrorToken(err, l.position)
		}

		if l.currentChar == '\'' {
			break
		}

		if l.currentChar != '\\' {
			l.stringBuilder.WriteRune(l.currentChar)
			continue
		}

		if err := l.readRune(); err != nil {
			return NewErrorToken(err, l.position)
		}

		escapeChar, ok := escapeChars[l.currentChar]
		if ok {
			l.stringBuilder.WriteRune(escapeChar)
		} else {
			l.stringBuilder.WriteRune(l.currentChar)
		}
	}

	return NewToken(STRING, l.stringBuilder.String(), l.position)
}

func (l *Lexer) readNumber() Token {
	defer l.stringBuilder.Reset()

	for {
		l.stringBuilder.WriteRune(l.currentChar)

		nextChar, err := l.peekRune()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return NewErrorToken(err, l.position)
		}

		if !unicode.IsDigit(nextChar) {
			break
		}

		if err = l.readRune(); err != nil {
			return NewErrorToken(err, l.position)
		}
	}

	nextChar, err := l.peekRune()
	if err != nil && !errors.Is(err, io.EOF) {
		return NewErrorToken(err, l.position)
	}

	var isFloat bool
	if err == nil && nextChar == '.' {
		isFloat = true

		if err = l.readRune(); err != nil {
			return NewErrorToken(err, l.position)
		}

		for {
			l.stringBuilder.WriteRune(l.currentChar)

			nextChar, err = l.peekRune()
			if err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				return NewErrorToken(err, l.position)
			}

			if !unicode.IsDigit(nextChar) {
				break
			}

			if err = l.readRune(); err != nil {
				return NewErrorToken(err, l.position)
			}
		}
	}

	numberValue := l.stringBuilder.String()
	if isFloat {
		return NewToken(DECIMAL, numberValue, l.position)
	}
	return NewToken(INTEGER, numberValue, l.position)
}

func (l *Lexer) readIdentifier() Token {
	defer l.stringBuilder.Reset()

	for {
		l.stringBuilder.WriteRune(l.currentChar)

		nextChar, err := l.peekRune()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return NewErrorToken(err, l.position)
		}

		if !unicode.IsLetter(nextChar) &&
			!unicode.IsDigit(nextChar) &&
			nextChar != '_' {
			break
		}

		if err = l.readRune(); err != nil {
			return NewErrorToken(err, l.position)
		}
	}

	identifierValue := l.stringBuilder.String()
	tokenType := LookupTokenType(identifierValue)

	return NewToken(tokenType, identifierValue, l.position)
}

func (l *Lexer) readTwoCharOperator(firstChar, expectedSecondChar rune,
	singleCharType, doubleCharType TokenType) Token {

	nextChar, err := l.peekRune()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return NewToken(singleCharType, string(firstChar), l.position)
		}
		return NewErrorToken(err, l.position)
	}

	if nextChar != expectedSecondChar {
		return NewToken(singleCharType, string(firstChar), l.position)
	}

	if err = l.readRune(); err != nil {
		return NewErrorToken(err, l.position)
	}

	operatorValue := string(firstChar) + string(expectedSecondChar)
	return NewToken(doubleCharType, operatorValue, l.position)
}

func (l *Lexer) NextToken() Token {
	err := l.skipSpace()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return NewEOFToken(l.position)
		}
		return NewErrorToken(err, l.position)
	}

	switch l.currentChar {
	case '=':
		return NewToken(EQ, "=", l.position)
	case '!':
		return l.readTwoCharOperator('!', '=', ILLEGAL, NE)
	case '>':
		return l.readTwoCharOperator('>', '=', GT, GE)
	case '<':
		return l.readTwoCharOperator('<', '=', LT, LE)
	case ',':
		return l.NextToken()
	case '\'':
		return l.readString()
	case '(':
		return NewToken(LPAREN, "(", l.position)
	case ')':
		return NewToken(RPAREN, ")", l.position)
	case '[':
		return NewToken(LSQUARE, "[", l.position)
	case ']':
		return NewToken(RSQUARE, "]", l.position)
	case ';':
		return l.NextToken()
	}

	if unicode.IsDigit(l.currentChar) {
		return l.readNumber()
	}

	if unicode.IsLetter(l.currentChar) || l.currentChar == '_' {
		return l.readIdentifier()
	}

	return NewIllegalToken(l.currentChar, l.position)
}

func (l *Lexer) Tokenize() []Token {
	var tokens []Token

	for {
		token := l.NextToken()
		tokens = append(tokens, token)

		if token.tokenType == EOF {
			break
		}
	}

	return tokens
}
